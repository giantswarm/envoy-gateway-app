#!/usr/bin/env python3
"""Render results.json (from fetch_metrics.py) into a self-contained HTML report.

    python3 render_report.py --input results.json --output report.html

No third-party dependencies, no external assets: charts are inline SVG so the
file works as a standalone artifact (e.g. attached to a PR) with no network.
"""
import argparse
import html
import json
from datetime import datetime, timezone

ENVOY = "#2563eb"   # blue
COMPET = "#f97316"  # orange
P_COLORS = {"p50": "#94a3b8", "p90": "#38bdf8", "p99": "#2563eb"}


# --------------------------------------------------------------------------- #
# Value formatting
# --------------------------------------------------------------------------- #
def fmt(unit, x):
    if x is None:
        return "—"
    if unit == "bytes":
        return f"{x / 1024 / 1024:.1f} MiB"
    if unit == "ms":
        return f"{x:.1f} ms"
    if unit == "cores":
        return f"{x:.3f}"
    if unit in ("%",):
        return f"{x:.2f} %"
    if unit == "ratio":
        return f"{x * 100:.2f} %"
    if unit == "req/s":
        return f"{x:.1f} req/s"
    if unit == "count":
        return f"{x:,.0f}"
    return f"{x:.2f}"


def pct_delta(a, b):
    """Signed relative change of a vs b, as a % string."""
    if a is None or b is None or b == 0:
        return "—"
    return f"{(a - b) / abs(b) * 100:+.1f}%"


# --------------------------------------------------------------------------- #
# SVG line chart
# --------------------------------------------------------------------------- #
def svg_chart(series, unit, width=680, height=240):
    """series: list of {name, color, t:[...], v:[...]}. Returns an <svg> string."""
    pad_l, pad_r, pad_t, pad_b = 56, 12, 12, 30
    plotted = [s for s in series if s["t"] and s["v"]]
    if not plotted:
        return '<div class="empty">no data</div>'

    xs = [x for s in plotted for x in s["t"]]
    ys = [y for s in plotted for y in s["v"]]
    xmin, xmax = min(xs), max(xs)
    ymin, ymax = min(ys), max(ys)
    if ymin == ymax:
        ymin, ymax = (0, ymax * 1.5 or 1)
    else:
        ymin = min(ymin, 0) if ymin > 0 else ymin
    xspan = (xmax - xmin) or 1
    yspan = (ymax - ymin) or 1

    def px(x):
        return pad_l + (x - xmin) / xspan * (width - pad_l - pad_r)

    def py(y):
        return height - pad_b - (y - ymin) / yspan * (height - pad_t - pad_b)

    parts = [f'<svg viewBox="0 0 {width} {height}" class="chart" '
             f'preserveAspectRatio="xMidYMid meet" role="img">']

    # y gridlines + labels (4 steps)
    for i in range(5):
        yv = ymin + yspan * i / 4
        y = py(yv)
        parts.append(f'<line x1="{pad_l}" y1="{y:.1f}" x2="{width-pad_r}" y2="{y:.1f}" '
                     f'class="grid"/>')
        parts.append(f'<text x="{pad_l-6}" y="{y+3:.1f}" class="ylab">{_short(yv)}</text>')
    # x labels (start / mid / end elapsed seconds)
    for frac in (0, 0.5, 1):
        xv = xmin + xspan * frac
        x = px(xv)
        parts.append(f'<text x="{x:.1f}" y="{height-8}" class="xlab">{xv:.0f}s</text>')

    for s in plotted:
        pts = " ".join(f"{px(x):.1f},{py(y):.1f}" for x, y in zip(s["t"], s["v"]))
        parts.append(f'<polyline points="{pts}" fill="none" stroke="{s["color"]}" '
                     f'stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>')
        # emphasized endpoint
        ex, ey = px(s["t"][-1]), py(s["v"][-1])
        parts.append(f'<circle cx="{ex:.1f}" cy="{ey:.1f}" r="3.2" fill="{s["color"]}"/>')
    parts.append("</svg>")
    return "".join(parts)


def _short(x):
    ax = abs(x)
    if ax >= 1e9:
        return f"{x/1e9:.1f}G"
    if ax >= 1e6:
        return f"{x/1e6:.1f}M"
    if ax >= 1e3:
        return f"{x/1e3:.1f}k"
    if ax >= 1 or x == 0:
        return f"{x:.0f}"
    return f"{x:.2f}"


def legend(items):
    chips = "".join(
        f'<span class="chip"><i style="background:{c}"></i>{html.escape(n)}</span>'
        for n, c in items)
    return f'<div class="legend">{chips}</div>'


# --------------------------------------------------------------------------- #
# Representative value + verdict per comparison metric
# --------------------------------------------------------------------------- #
def representative(side):
    """Pick the headline scalar for a side of a comparison metric."""
    if side is None:
        return None
    if side["kind"] == "percentile":
        p99 = side["percentiles"].get("p99")
        return p99["stats"]["mean"] if p99 else None
    return side["series"]["stats"]["mean"]


# --------------------------------------------------------------------------- #
# HTML assembly
# --------------------------------------------------------------------------- #
def render(data):
    meta = data["meta"]
    comp = meta["competitor"]
    comp_title = comp.capitalize()

    def side_name(side):
        return "Envoy" if side == "envoy" else comp_title

    rows, sections = [], []
    envoy_wins = comp_wins = 0

    for key, entry in data["comparison"].items():
        e_val = representative(entry["envoy"])
        c_val = representative(entry["competitor"])
        unit = entry["unit"]
        winner = None
        if e_val is not None and c_val is not None:
            lb = entry["lower_is_better"]
            envoy_better = (e_val < c_val) if lb else (e_val > c_val)
            winner = "Envoy" if envoy_better else comp_title
            envoy_wins += int(envoy_better)
            comp_wins += int(not envoy_better)

        label = entry["title"] + (" (p99)" if entry["kind"] == "percentile" else "")
        rows.append(
            f"<tr><td>{html.escape(label)}</td>"
            f"<td class='num'>{fmt(unit, e_val)}</td>"
            f"<td class='num'>{fmt(unit, c_val)}</td>"
            f"<td class='num'>{pct_delta(e_val, c_val)}</td>"
            f"<td class='win {'envoy' if winner=='Envoy' else 'comp'}'>{winner or '—'}</td></tr>"
        )

        # chart + per-metric detail
        if entry["kind"] == "percentile":
            e_series, c_series = [], []
            detail_rows = []
            for p in ("p50", "p90", "p99"):
                ep = entry["envoy"]["percentiles"].get(p) if entry["envoy"] else None
                cp = entry["competitor"]["percentiles"].get(p) if entry["competitor"] else None
                em = ep["stats"]["mean"] if ep else None
                cm = cp["stats"]["mean"] if cp else None
                detail_rows.append(
                    f"<tr><td>{p}</td><td class='num'>{fmt(unit, em)}</td>"
                    f"<td class='num'>{fmt(unit, cm)}</td>"
                    f"<td class='num'>{pct_delta(em, cm)}</td></tr>")
            e99 = entry["envoy"]["percentiles"].get("p99") if entry["envoy"] else None
            c99 = entry["competitor"]["percentiles"].get("p99") if entry["competitor"] else None
            chart_series = []
            if e99:
                chart_series.append({"name": "Envoy p99", "color": ENVOY, **e99})
            if c99:
                chart_series.append({"name": f"{comp_title} p99", "color": COMPET, **c99})
            leg = legend([("Envoy p99", ENVOY), (f"{comp_title} p99", COMPET)])
            detail = (f"<table class='mini'><tr><th></th><th class='num'>Envoy</th>"
                      f"<th class='num'>{comp_title}</th><th class='num'>Δ</th></tr>"
                      f"{''.join(detail_rows)}</table>")
            body = (leg + svg_chart(chart_series, unit) + detail)
        else:
            es = entry["envoy"]["series"] if entry["envoy"] else None
            cs = entry["competitor"]["series"] if entry["competitor"] else None
            chart_series = []
            if es:
                chart_series.append({"name": "Envoy", "color": ENVOY, **es})
            if cs:
                chart_series.append({"name": comp_title, "color": COMPET, **cs})
            leg = legend([("Envoy", ENVOY), (comp_title, COMPET)])
            stat = (f"<table class='mini'><tr><th></th><th class='num'>mean</th><th class='num'>peak</th></tr>"
                    f"<tr><td>Envoy</td><td class='num'>{fmt(unit, es['stats']['mean'] if es else None)}</td>"
                    f"<td class='num'>{fmt(unit, es['stats']['max'] if es else None)}</td></tr>"
                    f"<tr><td>{comp_title}</td><td class='num'>{fmt(unit, cs['stats']['mean'] if cs else None)}</td>"
                    f"<td class='num'>{fmt(unit, cs['stats']['max'] if cs else None)}</td></tr></table>")
            body = leg + svg_chart(chart_series, unit) + stat

        sections.append(
            f"<section class='metric'><h3>{html.escape(entry['title'])} "
            f"<span class='unit'>({html.escape(unit)})</span></h3>{body}</section>")

    # k6 client-side table
    k6_rows = []
    all_keys = list(data["k6"]["envoy"].keys())
    for k in all_keys:
        em = data["k6"]["envoy"][k]
        cm = data["k6"]["competitor"].get(k, {})
        title, unit = em["title"], em["unit"]
        ev = em["stats"]["mean"]
        cv = cm.get("stats", {}).get("mean")
        # counts (iterations) use last/total rather than mean
        if unit == "count":
            ev = em["stats"]["last"]
            cv = cm.get("stats", {}).get("last")
        k6_rows.append(
            f"<tr><td>{html.escape(title)}</td>"
            f"<td class='num'>{fmt(unit, ev)}</td>"
            f"<td class='num'>{fmt(unit, cv)}</td></tr>")

    overall = ("Envoy leads on the majority of server-side metrics."
               if envoy_wins > comp_wins else
               f"{comp_title} leads on the majority of server-side metrics."
               if comp_wins > envoy_wins else "Envoy and the competitor are evenly matched.")

    generated = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    ew = meta["envoy_window"]
    cw = meta["competitor_window"]

    return TEMPLATE.format(
        comp_title=comp_title,
        cluster_id=html.escape(meta["cluster_id"]),
        testid=html.escape(meta["testid"]),
        generated=generated,
        rate=html.escape(meta["rate_interval"]),
        envoy_dur=ew["duration_s"], comp_dur=cw["duration_s"],
        overall=html.escape(overall),
        envoy_wins=envoy_wins, comp_wins=comp_wins,
        summary_rows="".join(rows),
        sections="".join(sections),
        k6_rows="".join(k6_rows),
        css=CSS,
    )


CSS = """
:root{
  --bg:#f7f8fa;--card:#ffffff;--fg:#111927;--muted:#5c6b7f;--line:#e3e8ef;
  --th-bg:#f1f4f8;--e-bg:#dbeafe;--e-fg:#1e40af;--c-bg:#ffedd5;--c-fg:#9a3412;
  --shadow:0 1px 2px rgba(17,25,39,.05),0 1px 3px rgba(17,25,39,.04);
}
@media (prefers-color-scheme:dark){
  :root{
    --bg:#0f1419;--card:#1a212b;--fg:#e6edf3;--muted:#93a1b3;--line:#28323f;
    --th-bg:#212b37;--e-bg:#17314f;--e-fg:#9dc3ff;--c-bg:#43290f;--c-fg:#ffc08a;
    --shadow:0 1px 2px rgba(0,0,0,.4);
  }
}
:root[data-theme="light"]{
  --bg:#f7f8fa;--card:#ffffff;--fg:#111927;--muted:#5c6b7f;--line:#e3e8ef;
  --th-bg:#f1f4f8;--e-bg:#dbeafe;--e-fg:#1e40af;--c-bg:#ffedd5;--c-fg:#9a3412;
  --shadow:0 1px 2px rgba(17,25,39,.05),0 1px 3px rgba(17,25,39,.04);
}
:root[data-theme="dark"]{
  --bg:#0f1419;--card:#1a212b;--fg:#e6edf3;--muted:#93a1b3;--line:#28323f;
  --th-bg:#212b37;--e-bg:#17314f;--e-fg:#9dc3ff;--c-bg:#43290f;--c-fg:#ffc08a;
  --shadow:0 1px 2px rgba(0,0,0,.4);
}
*{box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;
color:var(--fg);background:var(--bg);margin:0;padding:0;line-height:1.5;
-webkit-font-smoothing:antialiased}
.wrap{max-width:980px;margin:0 auto;padding:40px 22px 72px}
h1{font-size:27px;font-weight:700;letter-spacing:-.01em;margin:0 0 3px;text-wrap:balance}
h2{font-size:14px;font-weight:600;text-transform:uppercase;letter-spacing:.06em;
color:var(--muted);margin:40px 0 14px;padding-bottom:8px;border-bottom:1px solid var(--line)}
h3{font-size:15px;font-weight:600;margin:0 0 12px}
.sub{color:var(--muted);font-size:13px}
.meta{display:flex;flex-wrap:wrap;gap:6px 22px;margin:18px 0;font-size:12.5px;color:var(--muted)}
.meta b{color:var(--fg);font-weight:600}
.verdict{background:var(--card);border:1px solid var(--line);border-radius:12px;
padding:18px 20px;margin:22px 0;box-shadow:var(--shadow)}
.verdict .big{font-size:17px;font-weight:650;letter-spacing:-.01em}
.tally{display:inline-block;padding:2px 9px;border-radius:20px;font-size:12px;
font-weight:600;margin-left:8px;font-variant-numeric:tabular-nums}
.tally.e{background:var(--e-bg);color:var(--e-fg)}.tally.c{background:var(--c-bg);color:var(--c-fg)}
table{border-collapse:collapse;width:100%;background:var(--card);font-size:13px;
border:1px solid var(--line);border-radius:12px;overflow:hidden;box-shadow:var(--shadow)}
th,td{padding:9px 13px;text-align:left;border-bottom:1px solid var(--line)}
th{background:var(--th-bg);font-weight:600;font-size:11px;text-transform:uppercase;
letter-spacing:.04em;color:var(--muted)}
tr:last-child td{border-bottom:none}
td.num{text-align:right;font-variant-numeric:tabular-nums}
th.num{text-align:right}
td.win{font-weight:600}td.win.envoy{color:var(--e-fg)}td.win.comp{color:var(--c-fg)}
.grid-metrics{display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-top:14px}
@media(max-width:720px){.grid-metrics{grid-template-columns:1fr}}
.metric{background:var(--card);border:1px solid var(--line);border-radius:12px;
padding:16px;box-shadow:var(--shadow)}
.metric .unit{color:var(--muted);font-weight:400;font-size:12px}
.chart{width:100%;height:auto;display:block;margin:4px 0}
.chart .grid{stroke:var(--line);stroke-width:1}
.chart .ylab{fill:var(--muted);font-size:10px;text-anchor:end;font-variant-numeric:tabular-nums}
.chart .xlab{fill:var(--muted);font-size:10px;text-anchor:middle}
.legend{display:flex;gap:14px;font-size:12px;color:var(--muted);margin-bottom:6px}
.chip{display:flex;align-items:center;gap:5px}
.chip i{width:11px;height:11px;border-radius:3px;display:inline-block}
table.mini{margin-top:12px;font-size:12px;box-shadow:none}
table.mini th,table.mini td{padding:5px 9px}
.empty{color:var(--muted);font-style:italic;padding:30px 0;text-align:center}
footer{margin-top:44px;color:var(--muted);font-size:12px;line-height:1.6}
"""

TEMPLATE = """<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Envoy vs {comp_title} — Performance Report</title>
<style>{css}</style></head>
<body><div class="wrap">
<h1>Envoy Gateway vs {comp_title}</h1>
<div class="sub">Load-test performance comparison</div>
<div class="meta">
  <span>Cluster <b>{cluster_id}</b></span>
  <span>Test ID <b>{testid}</b></span>
  <span>Rate window <b>{rate}</b></span>
  <span>Envoy run <b>{envoy_dur}s</b></span>
  <span>{comp_title} run <b>{comp_dur}s</b></span>
  <span>Generated <b>{generated}</b></span>
</div>

<div class="verdict">
  <div class="big">{overall}</div>
  <div class="sub" style="margin-top:6px">Server-side metric wins:
    <span class="tally e">Envoy {envoy_wins}</span>
    <span class="tally c">{comp_title} {comp_wins}</span>
  </div>
</div>

<h2>Summary</h2>
<table>
<tr><th>Metric</th><th class="num">Envoy</th><th class="num">{comp_title}</th><th class="num">Δ (Envoy vs {comp_title})</th><th>Better</th></tr>
{summary_rows}
</table>
<div class="sub" style="margin-top:8px">Latency rows compare p99 means. Δ is Envoy relative to {comp_title};
for latency/CPU/memory lower is better, for RPS/success-rate higher is better.</div>

<h2>Server-side metrics</h2>
<div class="grid-metrics">{sections}</div>

<h2>k6 client-side (per scenario)</h2>
<table>
<tr><th>Metric</th><th class="num">Envoy scenario</th><th class="num">{comp_title} scenario</th></tr>
{k6_rows}
</table>

<footer>
Charts show values over elapsed test time (x = seconds since each scenario's start;
the two scenarios ran in separate windows and are overlaid for comparison).
Queries mirror the Grafana dashboards k6-tests-results, envoy-vs-nginx-loadtesting and
envoy-vs-kong-loadtesting. Generated by the /perf-report skill.
</footer>
</div></body></html>
"""


def render_markdown(data, report_url=None):
    """A compact Markdown summary suitable for a PR comment."""
    meta = data["meta"]
    comp_title = meta["competitor"].capitalize()
    lines = [f"## 🚦 Envoy Gateway vs {comp_title} — performance report", ""]
    lines.append(f"Cluster `{meta['cluster_id']}` · test `{meta['testid']}` · "
                 f"Envoy {meta['envoy_window']['duration_s']}s / "
                 f"{comp_title} {meta['competitor_window']['duration_s']}s")
    lines.append("")
    lines.append(f"| Metric | Envoy | {comp_title} | Δ | Better |")
    lines.append("|---|--:|--:|--:|:--|")
    e_wins = c_wins = 0
    for entry in data["comparison"].values():
        e = representative(entry["envoy"])
        c = representative(entry["competitor"])
        winner = "—"
        if e is not None and c is not None:
            lb = entry["lower_is_better"]
            envoy_better = (e < c) if lb else (e > c)
            winner = "**Envoy**" if envoy_better else f"**{comp_title}**"
            e_wins += int(envoy_better)
            c_wins += int(not envoy_better)
        label = entry["title"] + (" (p99)" if entry["kind"] == "percentile" else "")
        lines.append(f"| {label} | {fmt(entry['unit'], e)} | {fmt(entry['unit'], c)} "
                     f"| {pct_delta(e, c)} | {winner} |")
    overall = ("Envoy leads on the majority of server-side metrics."
               if e_wins > c_wins else
               f"{comp_title} leads on the majority of server-side metrics."
               if c_wins > e_wins else "Evenly matched.")
    lines += ["", f"**Verdict:** {overall} (server-side wins — Envoy {e_wins} / {comp_title} {c_wins})"]
    if report_url:
        lines += ["", f"📄 [Full report with charts]({report_url})"]
    lines += ["", "<sub>Generated by the /perf-report skill from Mimir "
              "(mirrors the Grafana Envoy-vs-* load-testing dashboards).</sub>"]
    return "\n".join(lines)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--input", default="results.json")
    ap.add_argument("--output", default="report.html")
    ap.add_argument("--format", choices=["html", "markdown"], default="html")
    ap.add_argument("--report-url", default=None,
                    help="link inserted into the markdown summary (e.g. gist URL)")
    args = ap.parse_args()
    with open(args.input) as f:
        data = json.load(f)
    with open(args.output, "w") as f:
        f.write(render_markdown(data, args.report_url) if args.format == "markdown"
                else render(data))
    print(f"Wrote {args.output}")


if __name__ == "__main__":
    main()
