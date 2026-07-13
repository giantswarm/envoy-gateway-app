# perf-report skill

Automates the "read the Grafana dashboards → write an HTML comparison report"
step that follows a `/run envoy-performance-test` load test. Given a finished
run, it queries the installation's Mimir with the **same PromQL the dashboards
use**, then produces a self-contained HTML report and a PR summary comment.

```text
SKILL.md          orchestration Claude follows for /perf-report
queries.yaml      PromQL manifest — source of truth, mirrored from the 3 dashboards
fetch_metrics.py  Mimir → results.json (stdlib HTTP; needs PyYAML)
render_report.py  results.json → report.html and/or a markdown PR summary
```

## Two run modes

Same scripts, two environments. `fetch_metrics.py` auto-selects based on whether
it is running inside a pod (`KUBERNETES_SERVICE_HOST`); override with `--mode` or
an explicit `--mimir-url`.

| Mode | When | Mimir URL | Reach |
| --- | --- | --- | --- |
| `local` | a human runs `/perf-report` in Claude Code | `http://localhost:8080` | `kubectl -n mimir port-forward svc/mimir-gateway 8080:80` |
| `in-cluster` | Tekton pipeline pod on the MC | `http://mimir-gateway.mimir.svc/` | direct, no port-forward |

Both modes may need the Mimir gateway's **basic-auth credentials** — the same
`kube-system/alloy-metrics` creds the perf suite mirrors for remote-write. Pass
them via `--username/--password` or `MIMIR_USERNAME`/`MIMIR_PASSWORD`:

```bash
export MIMIR_USERNAME=$(kubectl -n kube-system get secret alloy-metrics -o jsonpath='{.data.metrics-username}' | base64 -d)
export MIMIR_PASSWORD=$(kubectl -n kube-system get secret alloy-metrics -o jsonpath='{.data.metrics-password}' | base64 -d)
```

### Local (Phase 1 — manual)

Prereqs: `python3`, `pip install pyyaml`, `kubectl` context for the installation, `gh`.

```bash
kubectl config use-context <installation>
kubectl -n mimir port-forward svc/mimir-gateway 8080:80 &      # background
python3 fetch_metrics.py --cluster-id <wc> --output results.json   # mode auto=local
python3 render_report.py --input results.json --output report.html
python3 render_report.py --input results.json --format markdown --output summary.md
```

Or just run `/perf-report` in Claude Code and answer the prompts.

### In-cluster (Phase 2 — Tekton)

```bash
python3 fetch_metrics.py --cluster-id <wc> --mode in-cluster --output results.json
# ... render + gh pr comment, as above
```

### Inputs

- `--cluster-id` (required): the `cluster_id` / `$workload_cluster` label.
- `--testid`: defaults to `e2e-load-test-<cluster-id>`.
- `--mode auto|local|in-cluster`: selects the default Mimir URL (default auto).
- `--mimir-url`: explicit base URL; overrides `--mode`.
- `--username`/`--password` (or `MIMIR_USERNAME`/`MIMIR_PASSWORD`): gateway basic auth.
- `--competitor auto|nginx|kong`: auto-detected from the k6 scenarios by default.
- `--lookback <hours>`: how far back to search for the run (default 6).
- `--tenant` (default `giantswarm`): `X-Scope-OrgID` header.

## Data flow it mirrors

`/run envoy-performance-test` → Tekton runs the Ginkgo suite in `tests/performance`
→ deploys Envoy Gateway + nginx **or** kong + the microservices demo → creates a
k6 `TestRun`. k6 runs two staggered scenarios (`envoy_simulation`, then
`nginx_simulation`/`kong_simulation`) and remote-writes to
`mimir-gateway.mimir.svc/api/v1/push` tagged `testid=e2e-load-test-<cluster>`.
Envoy/nginx/kong + cAdvisor metrics land in the same Mimir. The scripts query
`…/prometheus/api/v1/query_range` with header `X-Scope-OrgID: giantswarm`.

## Keeping queries in sync with the dashboards

`queries.yaml` is copied from three dashboards in the `giantswarm/dashboards`
repo (paths in the file header). When those dashboards change, diff their `expr`
fields against `queries.yaml` and reconcile — keep expressions byte-identical so
the report and Grafana never disagree. Quick extraction of a dashboard's exprs:

```bash
python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));
def w(ps):
 for p in ps:
  if p.get("panels"): w(p["panels"])
  for t in p.get("targets",[]) or []:
   if t.get("expr"): print(p.get("title"),"::"," ".join(t["expr"].split()))
w(d["panels"])' <dashboard.json>
```

## Phase 2 — automate at pipeline end

`/run` is a central Giant Swarm **Tekton** mechanism, not wired in this repo's
`.github`. To make the report generate automatically when the TestRun finishes,
add a step to the performance pipeline (in the Tekton pipeline definition, not
here) that, after the suite passes:

1. runs in a pod with in-cluster access to `mimir-gateway.mimir.svc` (no
   port-forward needed — use `--mimir-url http://mimir-gateway.mimir.svc/`),
2. runs `fetch_metrics.py` + `render_report.py` with the run's `cluster_id`,
3. posts `summary.md` to the PR (the pipeline already knows the PR) and uploads
   `report.html` as a pipeline artifact / gist.

The scripts are dependency-light (Python 3 + PyYAML, stdlib HTTP) specifically so
the same code runs unchanged in that pod. The cluster_id and PR number are known
to the pipeline; pass them as args instead of prompting.
```
