---
name: perf-report
description: >-
  Generate an Envoy-Gateway-vs-competitor (nginx/kong) performance report from a
  finished load test. Use after the `/run envoy-performance-test` pipeline
  completes, or when the user asks for a "performance report", "perf report",
  "envoy vs nginx report", "envoy vs kong report", or to summarize load-test /
  k6 results into an HTML report and PR comment.
---

# Envoy Gateway performance report

Turns a finished performance test (the `envoy-gateway-app` `tests/performance`
suite) into a ready-to-read HTML report plus a PR summary comment, by querying
the installation's Mimir with the same PromQL as the Grafana dashboards — no
manual dashboard reading.

The scripts live next to this file. `$DIR` below = the directory containing this
`SKILL.md` (when installed as a plugin skill, that is the plugin's copy).

## Pick the execution mode first

This skill runs in one of two environments. Detect which and follow the matching
path in step 2. `fetch_metrics.py` **auto-detects** the mode (it uses the
in-cluster Mimir service when `KUBERNETES_SERVICE_HOST` is set, i.e. inside a
pod, and localhost otherwise), so in practice you rarely pass `--mode`
explicitly — but be deliberate about which environment you are in.

- **Local (interactive)** — a human runs `/perf-report` in Claude Code on their
  machine. Mimir is reached via a `kubectl port-forward`.
- **In-cluster (Tekton)** — the pipeline spawns a headless agent in a pod on the
  management cluster. Mimir is reached directly at `mimir-gateway.mimir.svc`,
  with no port-forward. Inputs (cluster_id, PR number) arrive as arguments in
  the invoking prompt, not by asking.

## Inputs to collect

- **cluster_id** — the workload cluster the test ran on (`cluster_id` /
  `$workload_cluster` label).
- **installation / MC** — the management cluster whose Mimir holds the metrics.
  (Local mode: the kubeconfig context. In-cluster: already the pod's cluster.)
- **PR number** — where `/run envoy-performance-test` was triggered, for posting.
- optional: `testid` (default `e2e-load-test-<cluster_id>`), `competitor`
  (`auto`|`nginx`|`kong`, default auto-detected), `lookback` hours (default 6).

In **local** mode, ask the user for anything missing (never guess the cluster).
In **in-cluster** mode, take them from the invocation prompt/args and fail
loudly if absent — do not prompt an unattended pipeline.

The competitor (nginx vs kong) and both scenario time windows are **always
auto-detected** from the k6 series — never ask for them.

## Mimir credentials (both modes)

The Mimir gateway typically enforces basic auth — the same credentials the perf
suite mirrors from `kube-system/alloy-metrics` for remote-write. Read them from
the MC and pass them to the fetch script via env (works in both modes):

```bash
export MIMIR_USERNAME=$(kubectl -n kube-system get secret alloy-metrics -o jsonpath='{.data.metrics-username}' | base64 -d)
export MIMIR_PASSWORD=$(kubectl -n kube-system get secret alloy-metrics -o jsonpath='{.data.metrics-password}' | base64 -d)
```

If a query returns 401/403, this step was missed. If the gateway is open for
in-cluster traffic, the header is simply ignored — harmless to set anyway.

## Steps

Work in a scratch dir.

1. **Credentials** — export `MIMIR_USERNAME`/`MIMIR_PASSWORD` as above.

2. **Reach Mimir** — depending on mode:

   **Local:**

   ```bash
   kubectl config use-context <installation-context>
   kubectl -n mimir get svc mimir-gateway            # confirm it exists + the port
   kubectl -n mimir port-forward svc/mimir-gateway 8080:80   # run in background
   ```

   Give the port-forward a second to establish. If the cluster isn't reachable,
   stop and tell the user what's needed (VPN, teleport login, kubeconfig).
   Prefer the kubernetes MCP tools if they're wired up for this installation.

   **In-cluster (Tekton):** nothing to do — the service URL is reachable
   directly. Skip the port-forward.

3. **Fetch the metrics** (mode auto-detected; add `--mode` only to force it):

   ```bash
   # Local
   python3 "$DIR/fetch_metrics.py" --cluster-id <cluster_id> \
     --lookback 6 --output results.json
   # In-cluster (equivalent to --mode in-cluster --mimir-url http://mimir-gateway.mimir.svc/)
   python3 "$DIR/fetch_metrics.py" --cluster-id <cluster_id> \
     --mode in-cluster --output results.json
   ```

   Requires `python3` + PyYAML (`pip install pyyaml` if missing). It prints the
   chosen `mode`/`url`, the detected competitor, and both windows to stderr —
   sanity-check that the windows look like real ~20-minute runs, not stray
   seconds. If it reports no `envoy_simulation` data, widen `--lookback` or
   re-check cluster_id/testid; the k6 metrics must still be within Mimir
   retention.

4. **Render the report + PR summary:**

   ```bash
   python3 "$DIR/render_report.py" --input results.json --output report.html
   python3 "$DIR/render_report.py" --input results.json --format markdown --output summary.md
   ```

5. **Publish (if a PR number was given):**
   - `gh` can't attach a file to a comment, so create a gist and link it:

     ```bash
     gh gist create report.html --public=false --desc "Envoy perf report <cluster_id>"
     ```

     Then re-render the markdown with the gist URL:
     `render_report.py ... --format markdown --report-url <gist-url> --output summary.md`.
   - Post the comment: `gh pr comment <pr> --body-file summary.md`.
   - In-cluster, the pipeline may prefer uploading `report.html` as a pipeline
     artifact instead of a gist — either is fine; still post `summary.md`.
   - Always also report the local `report.html` path.

6. **Clean up** the port-forward (local mode only).

## Adding narrative

The scripts produce the numbers, tables, charts, and a deterministic per-metric
verdict. After generating them, read `results.json` and add 2–4 sentences of
interpretation — where Envoy's latency advantage is largest, whether the
CPU/memory trade-off is expected (Envoy typically uses more memory), and any SLO
breaches (p95<500ms / p99<1000ms / error<0.1% — baked into the k6 scenario).
Cite only values present in `results.json`; never invent numbers. In unattended
in-cluster runs keep this brief and prepend it to the PR comment.

## Notes / limits

- The two scenarios run in **separate time windows** (Envoy first, competitor
  after a wait). Charts overlay them on a common elapsed-time axis; they are not
  concurrent traffic.
- nginx exposes no upstream-latency histogram, so that comparison is Envoy-only
  for nginx runs (kong has it). Expected, not a bug.
- Latencies are normalized to milliseconds (Envoy/kong histograms are already ms;
  nginx seconds ×1000), matching the dashboards.
- `queries.yaml` is the source of truth for what's measured; keep it in sync with
  the Grafana dashboards (see README.md).
