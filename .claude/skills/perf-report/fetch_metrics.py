#!/usr/bin/env python3
"""Fetch Envoy-vs-competitor performance metrics from Mimir into results.json.

Reads the PromQL manifest (queries.yaml), auto-detects the competitor
(nginx/kong) and both k6 scenario windows from the k6 series, runs every query
over the matching window, and writes a results.json consumed by render_report.py.

HTTP uses only the standard library. The single third-party dependency is
PyYAML (to parse queries.yaml):  pip install pyyaml

Two modes, auto-selected (override with --mode / --mimir-url):
  * local     — developer machine, via `kubectl -n mimir port-forward
                svc/mimir-gateway 8080:80` -> http://localhost:8080
  * in-cluster — Tekton pipeline pod on the MC -> http://mimir-gateway.mimir.svc/
                (no port-forward; picked automatically when KUBERNETES_SERVICE_HOST
                is set)

    # local
    python3 fetch_metrics.py --cluster-id my-wc --output results.json
    # in-cluster (Tekton) — creds via env
    MIMIR_USERNAME=... MIMIR_PASSWORD=... \
      python3 fetch_metrics.py --cluster-id my-wc --output results.json

The k6 testid defaults to  e2e-load-test-<cluster-id>  (see the perf suite).
The Mimir gateway may require basic auth (same creds the perf suite mirrors from
kube-system/alloy-metrics) — pass --username/--password or MIMIR_USERNAME/MIMIR_PASSWORD.
"""
import argparse
import base64
import json
import os
import sys
import urllib.parse
import urllib.request
from datetime import datetime, timezone

try:
    import yaml
except ImportError:
    sys.exit("PyYAML is required: pip install pyyaml")

ENVOY_SCENARIO = "envoy_simulation"
COMPETITOR_SCENARIOS = {"nginx_simulation": "nginx", "kong_simulation": "kong"}

# In-cluster Mimir gateway (Tekton pipeline mode). Reachable without a
# port-forward from any pod running in the management cluster.
IN_CLUSTER_MIMIR_URL = "http://mimir-gateway.mimir.svc/"
# Local mode: what `kubectl -n mimir port-forward svc/mimir-gateway 8080:80`
# exposes on the developer's machine.
LOCAL_MIMIR_URL = "http://localhost:8080"


def running_in_cluster():
    """True when executing inside a Kubernetes pod (e.g. the Tekton runner)."""
    return bool(os.environ.get("KUBERNETES_SERVICE_HOST"))


def build_headers(tenant, username, password):
    """Mimir query headers: tenant scope + optional basic auth.

    The gateway enforces the same basic auth the perf suite uses for
    remote-write (mirrored from kube-system/alloy-metrics). Credentials come
    from --username/--password or the MIMIR_USERNAME/MIMIR_PASSWORD env vars.
    """
    headers = {"X-Scope-OrgID": tenant}
    username = username or os.environ.get("MIMIR_USERNAME")
    password = password or os.environ.get("MIMIR_PASSWORD")
    if username and password:
        token = base64.b64encode(f"{username}:{password}".encode()).decode()
        headers["Authorization"] = f"Basic {token}"
    return headers


# --------------------------------------------------------------------------- #
# Mimir HTTP
# --------------------------------------------------------------------------- #
def _get(url, headers, timeout):
    req = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode())


def query_range(base, headers, expr, start, end, step, timeout=60):
    """Run a range query; return list of {'labels':{}, 'points':[(ts,val)]}."""
    qs = urllib.parse.urlencode(
        {"query": expr, "start": f"{start:.3f}", "end": f"{end:.3f}", "step": f"{step}s"}
    )
    url = f"{base.rstrip('/')}/prometheus/api/v1/query_range?{qs}"
    data = _get(url, headers, timeout)
    if data.get("status") != "success":
        raise RuntimeError(f"query failed: {data.get('error', data)}")
    out = []
    for series in data["data"]["result"]:
        points = []
        for ts, val in series.get("values", []):
            try:
                f = float(val)
            except (TypeError, ValueError):
                continue
            if f != f:  # NaN
                continue
            points.append((float(ts), f))
        out.append({"labels": series.get("metric", {}), "points": points})
    return out


# --------------------------------------------------------------------------- #
# Aggregation
# --------------------------------------------------------------------------- #
def series_from_points(points, window_start):
    """Turn raw (ts,val) points into a chartable series + summary stats.

    x is seconds elapsed since the window start, so envoy and competitor series
    (which ran in different absolute windows) overlay on a common elapsed axis.
    """
    t = [round(ts - window_start, 1) for ts, _ in points]
    v = [val for _, val in points]
    stats = {"mean": None, "max": None, "min": None, "last": None}
    if v:
        stats = {
            "mean": sum(v) / len(v),
            "max": max(v),
            "min": min(v),
            "last": v[-1],
        }
    return {"t": t, "v": v, "stats": stats}


def merge_points(result):
    """Flatten all series returned for one expr into a single point list.

    Nearly every expr in the manifest aggregates to a single series; if more
    than one comes back we sum by timestamp so the report still gets one line.
    """
    if not result:
        return []
    if len(result) == 1:
        return result[0]["points"]
    acc = {}
    for s in result:
        for ts, val in s["points"]:
            acc[ts] = acc.get(ts, 0.0) + val
    return sorted(acc.items())


# --------------------------------------------------------------------------- #
# Window / competitor detection
# --------------------------------------------------------------------------- #
def detect_scenarios(base, headers, testid, now, lookback_s, step=30):
    """Find each k6 scenario's [start,end] window from k6_http_reqs_total."""
    expr = f'sum by (scenario) (k6_http_reqs_total{{testid="{testid}"}})'
    res = query_range(base, headers, expr, now - lookback_s, now, step)
    windows = {}
    for s in res:
        scen = s["labels"].get("scenario")
        pts = s["points"]
        if not scen or not pts:
            continue
        windows[scen] = (pts[0][0], pts[-1][0])
    return windows


# --------------------------------------------------------------------------- #
# Main
# --------------------------------------------------------------------------- #
def run_side(base, headers, spec, cluster_id, testid, rate, window, side_key):
    """Run one metric's queries for one side (envoy or competitor)."""
    start, end = window
    step = max(15, int((end - start) / 200))

    def fmt(expr):
        return (
            expr.replace("{cluster_id}", cluster_id)
            .replace("{testid}", testid)
            .replace("{rate}", rate)
        )

    if spec.get("kind") == "percentile":
        block = spec.get(side_key)
        if not block:
            return None  # e.g. nginx has no upstream-latency histogram
        out = {}
        for p, expr in block.items():
            pts = merge_points(query_range(base, headers, fmt(expr), start, end, step))
            out[p] = series_from_points(pts, start)
        return {"kind": "percentile", "percentiles": out}
    else:
        expr = spec.get(side_key)
        if not expr:
            return None
        pts = merge_points(query_range(base, headers, fmt(expr), start, end, step))
        return {"kind": "single", "series": series_from_points(pts, start)}


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--cluster-id", required=True, help="workload cluster_id")
    ap.add_argument("--testid", help="k6 testid (default: e2e-load-test-<cluster-id>)")
    ap.add_argument("--mode", choices=["auto", "local", "in-cluster"], default="auto",
                    help="local = port-forward on localhost; in-cluster = mimir-gateway.mimir.svc "
                         "(Tekton). 'auto' picks in-cluster when running inside a pod.")
    ap.add_argument("--mimir-url", default=None,
                    help="explicit Mimir gateway base URL; overrides --mode")
    ap.add_argument("--tenant", default="giantswarm", help="X-Scope-OrgID header")
    ap.add_argument("--username", default=None,
                    help="Mimir basic-auth user (or env MIMIR_USERNAME); "
                         "mirrors kube-system/alloy-metrics metrics-username")
    ap.add_argument("--password", default=None,
                    help="Mimir basic-auth password (or env MIMIR_PASSWORD); "
                         "mirrors kube-system/alloy-metrics metrics-password")
    ap.add_argument("--competitor", choices=["auto", "nginx", "kong"], default="auto")
    ap.add_argument("--lookback", type=float, default=6.0, help="hours to search for the run")
    ap.add_argument("--now", type=float, default=None,
                    help="reference unix time (default: real now)")
    ap.add_argument("--queries", default=None, help="path to queries.yaml")
    ap.add_argument("--output", default="results.json")
    args = ap.parse_args()

    import os
    qpath = args.queries or os.path.join(os.path.dirname(os.path.abspath(__file__)), "queries.yaml")
    with open(qpath) as f:
        manifest = yaml.safe_load(f)
    rate = manifest.get("rate_interval", "5m")

    testid = args.testid or f"e2e-load-test-{args.cluster_id}"

    # Resolve the Mimir URL: explicit --mimir-url wins; else derive from --mode
    # (auto = in-cluster when running inside a pod, local otherwise).
    if args.mimir_url:
        base, mode = args.mimir_url, "explicit"
    else:
        mode = args.mode
        if mode == "auto":
            mode = "in-cluster" if running_in_cluster() else "local"
        base = IN_CLUSTER_MIMIR_URL if mode == "in-cluster" else LOCAL_MIMIR_URL

    headers = build_headers(args.tenant, args.username, args.password)
    auth = "with basic auth" if "Authorization" in headers else "no auth"
    print(f"Mimir mode={mode} url={base} ({auth})", file=sys.stderr)

    now = args.now if args.now is not None else datetime.now(timezone.utc).timestamp()
    lookback_s = args.lookback * 3600

    print(f"Detecting k6 scenarios for testid={testid} ...", file=sys.stderr)
    windows = detect_scenarios(base, headers, testid, now, lookback_s)
    if ENVOY_SCENARIO not in windows:
        sys.exit(f"No '{ENVOY_SCENARIO}' data found for testid={testid} in the last "
                 f"{args.lookback}h. Check --cluster-id/--testid/--lookback and connectivity.")

    # Resolve competitor
    comp_scen = None
    for scen in windows:
        if scen in COMPETITOR_SCENARIOS:
            comp_scen = scen
            break
    if args.competitor != "auto":
        want = f"{args.competitor}_simulation"
        comp_scen = want if want in windows else comp_scen
    if not comp_scen:
        sys.exit(f"No competitor scenario (nginx/kong) found. Scenarios seen: {list(windows)}")
    competitor = COMPETITOR_SCENARIOS[comp_scen]

    envoy_win = windows[ENVOY_SCENARIO]
    comp_win = windows[comp_scen]
    print(f"  envoy window : {_fmt_win(envoy_win)}", file=sys.stderr)
    print(f"  {competitor} window : {_fmt_win(comp_win)}", file=sys.stderr)

    results = {
        "meta": {
            "cluster_id": args.cluster_id,
            "testid": testid,
            "competitor": competitor,
            "mode": mode,
            "rate_interval": rate,
            "envoy_window": {"start": envoy_win[0], "end": envoy_win[1],
                             "duration_s": round(envoy_win[1] - envoy_win[0])},
            "competitor_window": {"start": comp_win[0], "end": comp_win[1],
                                  "duration_s": round(comp_win[1] - comp_win[0])},
        },
        "k6": {"envoy": {}, "competitor": {}},
        "comparison": {},
    }

    # k6 client-side metrics per scenario
    for key, spec in manifest.get("k6", {}).items():
        for side, scen, win in (("envoy", ENVOY_SCENARIO, envoy_win),
                                ("competitor", comp_scen, comp_win)):
            expr = (spec["expr"].replace("{testid}", testid)
                    .replace("{scenario}", scen).replace("{rate}", rate))
            pts = merge_points(query_range(base, headers, expr, win[0], win[1],
                                           max(15, int((win[1] - win[0]) / 200))))
            s = series_from_points(pts, win[0])
            results["k6"][side][key] = {"title": spec["title"], "unit": spec["unit"], **s}

    # Server-side comparison metrics
    for key, spec in manifest.get("comparison", {}).items():
        entry = {
            "title": spec["title"], "unit": spec["unit"],
            "kind": spec.get("kind", "single"),
            "lower_is_better": spec.get("lower_is_better", True),
        }
        entry["envoy"] = run_side(base, headers, spec, args.cluster_id, testid, rate,
                                  envoy_win, "envoy")
        entry["competitor"] = run_side(base, headers, spec, args.cluster_id, testid, rate,
                                       comp_win, competitor)
        results["comparison"][key] = entry
        print(f"  fetched {key}", file=sys.stderr)

    with open(args.output, "w") as f:
        json.dump(results, f, indent=2)
    print(f"Wrote {args.output} (competitor={competitor})", file=sys.stderr)


def _fmt_win(w):
    return (f"{datetime.fromtimestamp(w[0], timezone.utc):%H:%M:%S}"
            f"–{datetime.fromtimestamp(w[1], timezone.utc):%H:%M:%S} UTC "
            f"({round(w[1]-w[0])}s)")


if __name__ == "__main__":
    main()
