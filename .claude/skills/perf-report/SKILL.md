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
suite) into a self-contained HTML report — shipped to the PR as a `.tar.gz` the
reader downloads and extracts locally — plus a PR summary comment, by querying
the installation's Mimir with the same PromQL as the Grafana dashboards — no
manual dashboard reading.

The scripts live next to this file. In this repo that is
`.claude/skills/perf-report/`; when installed as a plugin skill it is the
plugin's copy. The commands below use the in-repo path — substitute the plugin
path if that is where the skill is installed.

> **Never put a shell variable assignment in a Bash command.** Write
> `python3 .claude/skills/perf-report/fetch_metrics.py …`, not
> `DIR=... && python3 "$DIR/fetch_metrics.py" …`. When this skill runs in CI it
> is under an allowlist (`--permission-mode dontAsk`), and a permission rule
> **cannot match past a variable assignment** — the whole call gets denied even
> though `python3 …` on its own is allowed. This silently cost a full pipeline
> run. Spell every path out literally. (Referencing an already-exported variable
> mid-command is fine; it is the leading `VAR=…` that breaks matching.)

## Pick the execution mode first

This skill runs in one of two environments. Detect which and follow the matching
path in step 2.

- **Local (interactive)** — a human runs `/perf-report` in Claude Code on their
  machine. Mimir is reached via a `kubectl port-forward` you open yourself.
- **Pipeline (Tekton)** — the `generate-perf-report` finally-task of
  `envoy-performance-test` spawns a headless agent. The PR number and a
  `mimir-url` arrive as arguments in the invoking prompt, not by asking.

> **If the prompt gives you a `mimir-url`, always pass it through as
> `--mimir-url` and do not open a port-forward of your own.** The pipeline pod
> runs on the CI installation, not the test MC, so `mimir-gateway.mimir.svc` is
> *not* resolvable there — but `fetch_metrics.py` auto-detects "in-cluster" from
> `KUBERNETES_SERVICE_HOST` and would silently pick exactly that unreachable
> URL. The task has already opened the tunnel and exported
> `MIMIR_USERNAME`/`MIMIR_PASSWORD`; skip steps 1 and 2 entirely.

Outside that case `fetch_metrics.py` auto-detects the mode (in-cluster Mimir
service when `KUBERNETES_SERVICE_HOST` is set, localhost otherwise), so you
rarely pass `--mode` explicitly.

## Inputs to collect

- **installation / MC** — the management cluster whose Mimir holds the metrics.
  (Local mode: the kubeconfig context. In-cluster: already the pod's cluster.)
- **PR number** — where `/run envoy-performance-test` was triggered, for posting.
- optional: `cluster_id` (see below), `testid` (default
  `e2e-load-test-<cluster_id>`), `competitor` (`auto`|`nginx`|`kong`, default
  auto-detected), `lookback` hours (default 6).

**Don't go looking for the workload cluster.** The suite's AfterSuite deletes it
long before the report is generated, so there is nothing left to list on the MC.
It is recovered from Mimir instead: the suites tag every k6 series with
`testid=e2e-load-test-<cluster_id>`, and `fetch_metrics.py` defaults to
`--cluster-id auto`, which reads that label back. Neither mode needs the cluster
as an input — pass `--cluster-id` only when the user names one explicitly, or to
disambiguate (below).

In **local** mode, ask the user for anything still missing. In **in-cluster**
mode, take it from the invocation prompt/args and fail loudly if absent — do not
prompt an unattended pipeline.

The competitor (nginx vs kong), both scenario time windows, and the cluster are
**always auto-detected** from the k6 series — never ask for them.

Discovery picks the most recent run inside the lookback and prints which one it
chose plus any earlier runs it ignored. Read that line back to the user — it is
the only confirmation that the report covers the run they meant. Runs that
*overlap in time* (several suites in one pipeline matrix, or two PRs at once) are
refused rather than guessed: use `--list-runs` to get them as JSON and generate
one report per `cluster_id`.

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

   **Pipeline (Tekton):** nothing to do — the task already opened the tunnel and
   told you the `mimir-url`. Do not start your own port-forward.

3. **Fetch the metrics** (mode auto-detected; add `--mode` only to force it):

   ```bash
   # Local — cluster discovered from the k6 testid labels
   python3 .claude/skills/perf-report/fetch_metrics.py --lookback 6 --output results.json
   # Pipeline — pass the mimir-url the task gave you, spelled out literally
   python3 .claude/skills/perf-report/fetch_metrics.py --mimir-url http://localhost:8080 --output results.json
   # Pin a specific run instead of discovering it
   python3 .claude/skills/perf-report/fetch_metrics.py --cluster-id <cluster_id> --output results.json
   # Several runs at once: list them, then loop
   python3 .claude/skills/perf-report/fetch_metrics.py --list-runs
   ```

   Requires `python3` + PyYAML (`pip install pyyaml` if missing). It prints the
   chosen `mode`/`url`, the discovered `cluster_id`, the detected competitor, and
   both windows to stderr — sanity-check that the cluster is the one you expect
   and that the windows look like real ~20-minute runs, not stray seconds. If it
   finds no run at all, widen `--lookback`; if it reports no `envoy_simulation`
   data for a run it did find, re-check testid; the k6 metrics must still be
   within Mimir retention either way.

4. **Render the report + PR summary:**

   ```bash
   python3 .claude/skills/perf-report/render_report.py --input results.json --output report.html
   python3 .claude/skills/perf-report/render_report.py --input results.json --format markdown --output summary.md
   ```

5. **Package the report as a tarball** — the PR gets the compressed archive, never
   the bare HTML (GitHub won't render it inline anyway):

   ```bash
   tar -czf report.tar.gz report.html
   ```

6. **Publish (if a PR number was given):** comments can't carry attachments and
   gists can't hold binaries, so upload the tarball to the repo's dedicated
   `perf-reports` branch and link its download URL.

   > **Pipeline mode: skip this entire step.** The `generate-perf-report` Tekton
   > task publishes by itself — it creates the branch, uploads the tarball,
   > verifies it is readable, re-renders the markdown with the resulting URL and
   > posts the comment. Write your interpretation to `narrative.md` in the output
   > directory instead and stop; the task prepends it to the comment. Do not run
   > `gh` at all (the CI allowlist denies it), and do not construct a download URL
   > yourself — a URL written before a verified upload produces a comment with a
   > dead link, which is exactly what happened before this was moved out of the
   > agent's hands.

   ```bash
   REPO=giantswarm/envoy-gateway-app
   BRANCH=perf-reports
   DEST="pr-<pr>/<cluster_id>-$(date -u +%Y%m%dT%H%M%SZ)/report.tar.gz"

   # create the branch once, off main, if it does not exist yet
   gh api "repos/$REPO/git/ref/heads/$BRANCH" >/dev/null 2>&1 || \
     gh api --method POST "repos/$REPO/git/refs" \
       -f ref="refs/heads/$BRANCH" \
       -f sha="$(gh api "repos/$REPO/git/ref/heads/main" --jq .object.sha)"

   # upload; body built by python so the base64 blob never goes through argv
   python3 -c 'import base64,json,sys; print(json.dumps({
     "message": sys.argv[2], "branch": sys.argv[3],
     "content": base64.b64encode(open(sys.argv[1],"rb").read()).decode()}))' \
     report.tar.gz "perf report for PR #<pr> (<cluster_id>)" "$BRANCH" \
     | gh api --method PUT "repos/$REPO/contents/$DEST" --input -

   URL="https://github.com/$REPO/raw/$BRANCH/$DEST"
   ```

   The timestamp in `DEST` keeps every run at a fresh path, so the upload is
   always a create and never needs an existing blob `sha`.

   Then re-render the markdown with that URL and post it:

   ```bash
   python3 .claude/skills/perf-report/render_report.py --input results.json --format markdown \
     --report-url "$URL" --output summary.md
   gh pr comment <pr> --body-file summary.md
   ```

   - `render_report.py` already writes the download-and-extract instructions into
     the comment — don't restate them in your narrative.
   - Same flow in-cluster; it only needs `gh` authenticated with a token that can
     push to the repo. If the pipeline also uploads `report.tar.gz` as its own
     artifact, fine — still post `summary.md`.
   - Always also report the local `report.tar.gz` path.

7. **Clean up** the port-forward (local mode only).

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
