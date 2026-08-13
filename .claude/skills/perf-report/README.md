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

The HTML report is never posted to the PR as-is: it is tarred (`report.tar.gz`),
uploaded to the repo's `perf-reports` branch, and linked from the summary comment,
so the reader downloads the archive and extracts `report.html` on their machine.

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
python3 fetch_metrics.py --output results.json                      # mode auto=local,
                                                                   # cluster auto-discovered
python3 render_report.py --input results.json --output report.html
python3 render_report.py --input results.json --format markdown --output summary.md
tar -czf report.tar.gz report.html                                 # what the PR gets
```

Or just run `/perf-report` in Claude Code and answer the prompts.

### In-cluster (Phase 2 — Tekton)

```bash
python3 fetch_metrics.py --mode in-cluster --output results.json
# ... render + gh pr comment, as above
```

### Inputs

- `--cluster-id` (default `auto`): the `cluster_id` / `$workload_cluster` label.
  `auto` recovers it from the k6 `testid` label (`e2e-load-test-<cluster_id>`),
  which is the only trace left once the suite has deleted the cluster. Runs
  overlapping in time are refused instead of guessed — pass the cluster
  explicitly, or use `--list-runs`.
- `--list-runs`: print the discoverable runs as JSON and exit, for fanning out
  one report per cluster.
- `--testid`: defaults to `e2e-load-test-<cluster-id>`; conversely, given alone it
  is where `--cluster-id auto` reads the cluster from.
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
`.github`. The `envoy-performance-test` pipeline runs this skill from its
`generate-perf-report` finally-task (see `tekton-resources`). That task:

1. port-forwards `svc/mimir-gateway` on the test MC and passes
   `--mimir-url http://localhost:8080`,
2. runs `fetch_metrics.py` + `render_report.py`,
3. tars `report.html` into `report.tar.gz`, uploads it to the `perf-reports`
   branch (or as a pipeline artifact), and posts `summary.md` — linking that
   archive — to the PR (the pipeline already knows the PR).

The scripts are dependency-light (Python 3 + PyYAML, stdlib HTTP) specifically so
the same code runs unchanged in that pod. The PR number is known to the pipeline;
pass it as an arg instead of prompting. The cluster is **not** passed in — the
workload cluster is deleted by the suite's AfterSuite before this step runs, so
`fetch_metrics.py` discovers it from Mimir instead (`--cluster-id auto`). That
also keeps the pipeline free of any Tekton result plumbing between the test task
and the report task, which matters because the test task is matrixed and a
matrixed task's results can only be consumed as an array.

### Why the pipeline port-forwards instead of using Mimir's public host

The report pod runs on the CI installation, not on the test MC, so
`mimir-gateway.mimir.svc` is not resolvable from it. The obvious alternative —
Mimir's public `HTTPRoute` host, `mimir.<mc>.gaws.gigantic.io` — does **not**
work: that route only exposes `/api/v1/push`, `/prometheus/config/v1/rules`,
`/prometheus/api/v1/query` and `/otlp/v1/metrics`. Gateway-API `PathPrefix`
matching is segment-based, so `/prometheus/api/v1/query` does not cover
`/prometheus/api/v1/query_range`, and `query_range` is the only endpoint
`fetch_metrics.py` uses — every request 404s. The sibling
`observability.<mc>.gaws.gigantic.io` route does expose `query_range` but
requires a JWT and rejects the `alloy-metrics` basic-auth credentials with 401.
A port-forward to the in-cluster service bypasses the route and serves the full
Prometheus API, which is why `_get()` also carries a retry loop: that tunnel
stalls periodically and a report makes ~30 sequential range queries.

### Permissions in the pipeline

The task runs `claude --permission-mode dontAsk`, which never prompts and
auto-denies anything not pre-approved. The allowlist it needs is
`.claude/perf-report-ci-settings.json` in this repo, passed explicitly with
`--settings`. It is deliberately **not** named `.claude/settings.json`: that
name is loaded automatically for everyone working in this repo, and these rules
should apply only to the unattended pipeline run. Add a rule there whenever the
skill starts using a new command.

Two things about that allowlist are easy to get wrong:

- **No shell variable assignments.** A permission rule cannot match past a
  variable assignment, so `DIR=... && python3 ...` is denied even though
  `Bash(python3 *)` is allowed — and no extra rule can fix it, because the
  restriction is on matching past the assignment. This is why the steps above
  spell every path out literally. It cost one whole pipeline run.
- **`env` is intentionally not allowlisted.** The agent sometimes tries
  `env | grep -c MIMIR_USERNAME` as a sanity check; that gets denied and it
  retries without it, which is fine. Do not "fix" this by allowing `env` — the
  pod's environment holds `MIMIR_PASSWORD`, `GITHUB_TOKEN` and
  `ANTHROPIC_API_KEY`, and a dump would land in the Tekton log.

If a run misbehaves, the task passes `--output-format stream-json --verbose`, so
the denied command appears in the step log. Plain `-p` output only shows the
model saying it needs permission, without naming the call.
