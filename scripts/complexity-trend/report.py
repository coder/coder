"""Build charts, heat maps, hotspot rankings, and a report from the raw data.

Run through ``complexity_trend.py report``. Requires matplotlib.
"""

import csv
import datetime as dt
import html
import json
import math
import os
from collections import defaultdict

import matplotlib

matplotlib.use("Agg")
import matplotlib.dates as mdates  # noqa: E402
import matplotlib.pyplot as plt  # noqa: E402
from matplotlib.colors import LogNorm  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.join(HERE, "data")
IMG = os.path.join(HERE, "img")
METRICS = {"cognitive": "Cognitive complexity (gocognit)", "cyclo": "Cyclomatic complexity (gocyclo)"}
TOP_N = 30

FLOW_STYLE = [
    ("new_funcs", "Added by new functions", "#2ca02c"),
    ("existing_up", "Added to existing functions", "#ff7f0e"),
    ("existing_down", "Removed from existing functions", "#1f77b4"),
    ("removed_funcs", "Removed with deleted functions", "#9467bd"),
]


def read_csv(name):
    with open(os.path.join(DATA, name)) as f:
        return list(csv.DictReader(f))


def parse_time(s):
    return dt.datetime.strptime(s, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=dt.timezone.utc)


def is_prod(path):
    return not path.endswith("_test.go")


def churn_index(touches, lines):
    """Geometric mean of how often and how much the code changed."""
    return math.sqrt(touches * lines)


def load():
    with open(os.path.join(DATA, "prs.json")) as f:
        prs = json.load(f)["prs"]
    for p in prs:
        p["time"] = parse_time(p["merged_at"])
    prs.sort(key=lambda p: p["time"])
    funcs = defaultdict(list)
    for r in read_csv("functions.csv"):
        if r["generated"] == "1":
            continue
        for k in ("cyclo_before", "cyclo_after", "cognitive_before", "cognitive_after",
                  "loc_before", "loc_after", "lines_added", "lines_removed"):
            r[k] = int(r[k])
        funcs[int(r["pr"])].append(r)
    files = defaultdict(list)
    for r in read_csv("files.csv"):
        if r["generated"] == "1":
            continue
        for k in ("lines_added", "lines_removed", "cyclo_before", "cyclo_after",
                  "cognitive_before", "cognitive_after"):
            r[k] = int(r[k])
        files[int(r["pr"])].append(r)
    return prs, funcs, files


def pair_moves(rows):
    """Treat a function removed from one file and added to another in the
    same PR as a move, so relocations do not count as new complexity.

    Methods pair by receiver and name anywhere; plain functions pair only
    within one directory, because names like New repeat across packages.
    """
    removed = defaultdict(list)
    for r in rows:
        if r["status"] == "removed":
            removed[r["func"]].append(r)
    for r in rows:
        if r["status"] != "new":
            continue
        for cand in removed.get(r["func"], []):
            if cand["status"] != "removed":
                continue
            if "." in r["func"] or os.path.dirname(cand["file"]) == os.path.dirname(r["file"]):
                cand["status"] = "moved_from"
                r["status"] = "moved_to"
                r["moved_from"] = cand
                break


def flows(rows, metric):
    out = {k: 0 for k, _, _ in FLOW_STYLE}
    b, a = f"{metric}_before", f"{metric}_after"
    for r in rows:
        st = r["status"]
        if st == "new":
            out["new_funcs"] += r[a]
        elif st == "removed":
            out["removed_funcs"] -= r[b]
        elif st in ("changed", "touched"):
            d = r[a] - r[b]
            out["existing_up" if d > 0 else "existing_down"] += d
        elif st == "moved_to":
            d = r[a] - r["moved_from"][b]
            out["existing_up" if d > 0 else "existing_down"] += d
    return out


def snapshot_totals(name):
    tot = defaultdict(int)
    for r in read_csv(name):
        if r["generated"] == "1":
            continue
        kind = "test" if r["test"] == "1" else "prod"
        tot[("cognitive", kind)] += int(r["cognitive"])
        tot[("cyclo", kind)] += int(r["cyclo"])
    return tot


# ------------------------------------------------------------ trend charts


def plot_running_total(prs, per_pr, start, end):
    fig, axes = plt.subplots(2, 1, figsize=(12, 7), sharex=True)
    for ax, metric in zip(axes, METRICS):
        series = {}
        for kind in ("prod", "test"):
            total = start[(metric, kind)]
            ys = [total]
            for p in prs:
                total += per_pr[p["number"]][metric][kind]
                ys.append(total)
            series[kind] = ys
        xs = [prs[0]["time"] - dt.timedelta(hours=1)] + [p["time"] for p in prs]
        ax.stackplot(xs, series["prod"], series["test"], labels=["Production code", "Test code"],
                     colors=["#4c72b0", "#c4cbe0"], step="post")
        measured = end[(metric, "prod")] + end[(metric, "test")]
        ax.scatter([prs[-1]["time"]], [measured], color="black", zorder=5, s=18,
                   label="Full-tree measurement at end")
        ax.set_ylabel("Total")
        ax.set_title(METRICS[metric])
        ax.grid(alpha=0.3)
        ax.legend(loc="upper left", fontsize=8)
    axes[-1].xaxis.set_major_formatter(mdates.DateFormatter("%b %Y"))
    fig.suptitle("Running total of Go complexity on main (generated code excluded)")
    fig.tight_layout()
    fig.savefig(os.path.join(IMG, "running_total.png"), dpi=110)
    plt.close(fig)


def plot_flows(prs, per_pr, metric):
    fig, axes = plt.subplots(2, 1, figsize=(12, 8), sharex=True,
                             gridspec_kw={"height_ratios": [1, 1]})
    xs = [p["time"] for p in prs]
    ax = axes[0]
    pos = [0.0] * len(prs)
    neg = [0.0] * len(prs)
    for key, label, color in FLOW_STYLE:
        vals = [per_pr[p["number"]]["flows"][metric][key] for p in prs]
        base = pos if key in ("new_funcs", "existing_up") else neg
        ax.bar(xs, vals, bottom=base, width=0.12, color=color, label=label, linewidth=0)
        for i, v in enumerate(vals):
            base[i] += v
    ax.axhline(0, color="black", linewidth=0.5)
    ax.set_yscale("symlog", linthresh=10)
    ax.set_ylabel("Per PR (symlog)")
    ax.set_title("Each merged PR")
    ax.grid(alpha=0.3)

    ax = axes[1]
    weeks = defaultdict(lambda: defaultdict(int))
    for p in prs:
        wk = (p["time"] - dt.timedelta(days=p["time"].weekday())).date()
        for key, v in per_pr[p["number"]]["flows"][metric].items():
            weeks[wk][key] += v
    wks = sorted(weeks)
    wx = [dt.datetime.combine(w, dt.time(), dt.timezone.utc) + dt.timedelta(days=3) for w in wks]
    pos = [0] * len(wks)
    neg = [0] * len(wks)
    for key, label, color in FLOW_STYLE:
        vals = [weeks[w][key] for w in wks]
        base = pos if key in ("new_funcs", "existing_up") else neg
        ax.bar(wx, vals, bottom=list(base), width=5.5, color=color, label=label, linewidth=0)
        for i, v in enumerate(vals):
            base[i] += v
    net = [sum(weeks[w].values()) for w in wks]
    ax.plot(wx, net, color="black", marker=".", linewidth=1, label="Net")
    ax.axhline(0, color="black", linewidth=0.5)
    ax.set_ylabel("Per week")
    ax.set_title("Weekly totals")
    ax.grid(alpha=0.3)
    ax.xaxis.set_major_formatter(mdates.DateFormatter("%b %Y"))
    handles, labels = ax.get_legend_handles_labels()
    fig.legend(handles, labels, loc="lower center", ncol=5, fontsize=9, frameon=False)
    fig.suptitle(f"{METRICS[metric]} added and removed per PR (generated code excluded)")
    fig.tight_layout(rect=(0, 0.04, 1, 1))
    fig.savefig(os.path.join(IMG, f"flows_{metric}.png"), dpi=110)
    plt.close(fig)


def plot_cumulative_flows(prs, per_pr):
    fig, axes = plt.subplots(1, 2, figsize=(13, 4.5))
    xs = [p["time"] for p in prs]
    for ax, metric in zip(axes, METRICS):
        for key, label, color in FLOW_STYLE:
            total, ys = 0, []
            for p in prs:
                total += per_pr[p["number"]]["flows"][metric][key]
                ys.append(total)
            ax.plot(xs, ys, color=color, label=label)
        total, ys = 0, []
        for p in prs:
            total += sum(per_pr[p["number"]]["flows"][metric].values())
            ys.append(total)
        ax.plot(xs, ys, color="black", linestyle="--", label="Net")
        ax.axhline(0, color="black", linewidth=0.5)
        ax.set_title(METRICS[metric])
        ax.set_ylabel("Cumulative since window start")
        ax.grid(alpha=0.3)
        ax.xaxis.set_major_formatter(mdates.DateFormatter("%b %y"))
    axes[0].legend(fontsize=8)
    fig.suptitle("Cumulative complexity flows: new functions versus existing functions")
    fig.tight_layout()
    fig.savefig(os.path.join(IMG, "cumulative_flows.png"), dpi=110)
    plt.close(fig)


# ------------------------------------------------------------ hotspots


def churn_history(prs, funcs, files):
    """Accumulate churn per file and per function, following renames and
    moves so history stays attached to the code's final location."""
    file_stats = defaultdict(lambda: {"touches": 0, "lines": 0, "months": defaultdict(int)})
    func_stats = defaultdict(lambda: {"touches": 0, "lines": 0, "months": defaultdict(int)})
    for p in prs:
        month = p["time"].strftime("%Y-%m")
        for r in files[p["number"]]:
            if r["status"] == "R" and r["old_file"] in file_stats:
                file_stats[r["file"]] = file_stats.pop(r["old_file"])
                for key in [k for k in func_stats if k[0] == r["old_file"]]:
                    func_stats[(r["file"], key[1])] = func_stats.pop(key)
            s = file_stats[r["file"]]
            lines = r["lines_added"] + r["lines_removed"]
            s["touches"] += 1
            s["lines"] += lines
            s["months"][month] += lines
        for r in funcs[p["number"]]:
            key = (r["file"], r["func"])
            if r["status"] == "moved_to":
                old = (r["moved_from"]["file"], r["func"])
                if old in func_stats:
                    func_stats[key] = func_stats.pop(old)
            if r["status"] == "moved_from":
                continue
            s = func_stats[key]
            lines = r["lines_added"] + r["lines_removed"]
            s["touches"] += 1
            s["lines"] += lines
            s["months"][month] += lines
    return file_stats, func_stats


def rank_hotspots(file_stats, func_stats):
    end_files = defaultdict(lambda: {"cognitive": 0, "cyclo": 0, "funcs": 0})
    end_funcs = {}
    for r in read_csv("snapshot_end.csv"):
        if r["generated"] == "1" or not is_prod(r["file"]):
            continue
        f = end_files[r["file"]]
        f["cognitive"] += int(r["cognitive"])
        f["cyclo"] += int(r["cyclo"])
        f["funcs"] += 1
        end_funcs[(r["file"], r["func"])] = {"cognitive": int(r["cognitive"]), "cyclo": int(r["cyclo"]),
                                             "loc": int(r["loc"])}

    def build(stats, current, label):
        out = []
        for key, s in stats.items():
            cur = current.get(key)
            if not cur or s["touches"] == 0 or cur["cognitive"] == 0:
                continue
            churn = churn_index(s["touches"], s["lines"])
            row = {"name": label(key), "touches": s["touches"], "lines_changed": s["lines"],
                   "churn": churn, "score": cur["cognitive"] * churn, "months": s["months"]}
            row.update(cur)
            out.append(row)
        out.sort(key=lambda r: r["score"], reverse=True)
        return out

    files = build(file_stats, end_files, lambda k: k)
    funcs = build(func_stats, end_funcs, lambda k: f"{k[0]}:{k[1]}")
    return files, funcs


def short_name(name, limit=90):
    """Shorten a path-like label in the middle so both ends stay readable."""
    if len(name) <= limit:
        return name
    head = limit // 3
    return name[:head] + "..." + name[-(limit - head - 3):]


def plot_hotspot_scatter(rows, kind, labeled=12):
    fig, (ax, side) = plt.subplots(1, 2, figsize=(15, 7), dpi=110, gridspec_kw={"width_ratios": [3, 1.6]})
    xs = [r["churn"] for r in rows]
    ys = [r["cognitive"] for r in rows]
    cs = [r["score"] for r in rows]
    sc = ax.scatter(xs, ys, c=cs, cmap="inferno_r", norm=LogNorm(vmin=max(1, min(cs)), vmax=max(cs)),
                    s=12, alpha=0.8, linewidths=0)
    top = rows[:labeled]
    ax.scatter([r["churn"] for r in top], [r["cognitive"] for r in top], s=40, facecolors="none",
               edgecolors="black", linewidths=0.8)
    ax.set_xscale("log")
    ax.set_yscale("log")
    ax.set_xlabel("Churn: sqrt(PRs that touched it x lines changed)")
    ax.set_ylabel("Cognitive complexity now")
    ax.grid(alpha=0.3, which="both")
    fig.colorbar(sc, ax=ax, label="Hotspot score = complexity x churn")
    ax.set_title(f"Hotspot map: {kind} (production code, top {labeled} numbered)")
    side.axis("off")
    side.text(0, 1, "\n".join(f"{i:>2}. {short_name(r['name'], 75)}" for i, r in enumerate(top, 1)),
              va="top", ha="left", fontsize=8, family="monospace", transform=side.transAxes)
    fig.tight_layout()

    # Rank numbers stay legible where points crowd; names go in the side
    # panel. Place each number at the first offset that avoids the rings and
    # earlier numbers, in final pixel coordinates.
    pts = [ax.transData.transform((r["churn"], r["cognitive"])) for r in top]
    placed = [(x - 7, y - 7, x + 7, y + 7) for x, y in pts]
    offsets = [(7, 5), (7, -18), (-20, 5), (-20, -18), (7, 20), (-20, 20), (18, -6), (-32, -6),
               (7, -32), (-20, -32), (7, 34), (-20, 34), (32, 10), (-46, 10)]
    for i, (r, (px, py)) in enumerate(zip(top, pts), 1):
        # Bold 8 pt digits at 110 dpi are about 8 px wide and 12 px tall.
        w = 9 * len(str(i))
        for dx, dy in offsets:
            box = (px + dx - 1, py + dy - 2, px + dx + w + 1, py + dy + 13)
            if not any(box[0] < b[2] and b[0] < box[2] and box[1] < b[3] and b[1] < box[3] for b in placed):
                break
        placed.append(box)
        ax.annotate(str(i), (r["churn"], r["cognitive"]), fontsize=8, fontweight="bold",
                    xytext=(dx, dy), textcoords="offset pixels",
                    arrowprops={"arrowstyle": "-", "linewidth": 0.5, "shrinkA": 0, "shrinkB": 4})
    fig.savefig(os.path.join(IMG, f"hotspots_{kind}_scatter.png"))
    plt.close(fig)


def plot_hotspot_heatmap(rows, kind, months):
    top = rows[:TOP_N]
    height = 0.32 * len(top) + 2
    fig = plt.figure(figsize=(14, height))
    gs = fig.add_gridspec(1, 3, width_ratios=[1, 1, len(months)], wspace=0.04, top=1 - 0.6 / height)
    axc, axt, axm = fig.add_subplot(gs[0]), fig.add_subplot(gs[1]), fig.add_subplot(gs[2])
    labels = [short_name(r["name"]) for r in top]

    def column(ax, values, cmap, title):
        norm = LogNorm(vmin=max(1, min(values)), vmax=max(values))
        ax.imshow([[v] for v in values], aspect="auto", cmap=cmap, norm=norm)
        for i, v in enumerate(values):
            ax.text(0, i, str(v), ha="center", va="center", fontsize=7,
                    color="white" if norm(v) > 0.6 else "black")
        ax.set_xticks([])
        ax.set_title(title, fontsize=8)

    column(axc, [r["cognitive"] for r in top], "Reds", "Cognitive\nnow")
    axc.set_yticks(range(len(top)))
    axc.set_yticklabels(labels, fontsize=7)
    column(axt, [r["touches"] for r in top], "Purples", "PRs\ntouching")
    axt.set_yticks([])
    matrix = [[r["months"].get(m, 0) for m in months] for r in top]
    flat = [v for row in matrix for v in row if v > 0]
    im = axm.imshow([[v if v > 0 else float("nan") for v in row] for row in matrix], aspect="auto",
                    cmap="YlOrRd", norm=LogNorm(vmin=1, vmax=max(flat) if flat else 1))
    axm.set_xticks(range(len(months)))
    axm.set_xticklabels(months, rotation=45, ha="right", fontsize=7)
    axm.set_yticks([])
    axm.set_title("Lines changed per month", fontsize=8)
    fig.colorbar(im, ax=axm, fraction=0.03, pad=0.01, label="Lines changed")
    fig.suptitle(f"Top {len(top)} {kind} hotspots by complexity x churn (production code)",
                 y=1 - 0.15 / height)
    fig.savefig(os.path.join(IMG, f"heatmap_{kind}.png"), dpi=110, bbox_inches="tight")
    plt.close(fig)


def directory_growth(prs, files):
    """Net cognitive complexity change per Go package directory, credited to
    each file's final location so package moves do not split the history."""
    per_file = defaultdict(int)
    for p in prs:
        for r in files[p["number"]]:
            if r["status"] == "R" and r["old_file"] in per_file:
                per_file[r["file"]] += per_file.pop(r["old_file"])
            per_file[r["file"]] += r["cognitive_after"] - r["cognitive_before"]
    grow = defaultdict(int)
    for path, delta in per_file.items():
        if is_prod(path):
            grow[os.path.dirname(path) or "(root)"] += delta
    return sorted(grow.items(), key=lambda kv: kv[1], reverse=True)


def plot_directory_growth(growth):
    top = growth[:20]
    fig, ax = plt.subplots(figsize=(10, 6))
    ax.barh([d for d, _ in reversed(top)], [v for _, v in reversed(top)], color="#4c72b0")
    ax.set_xlabel("Net cognitive complexity added over the window")
    ax.set_title("Where production complexity grew (top 20 packages)")
    ax.grid(alpha=0.3, axis="x")
    fig.tight_layout()
    fig.savefig(os.path.join(IMG, "directory_growth.png"), dpi=110)
    plt.close(fig)


# ------------------------------------------------------------ outputs


def write_csv(name, header, rows):
    with open(os.path.join(DATA, name), "w", newline="") as f:
        w = csv.writer(f, lineterminator="\n")
        w.writerow(header)
        w.writerows(rows)


def table_md(rows, kind, n=15):
    head = "| # | " + ("File" if kind == "files" else "Function") + \
        " | Cognitive | Cyclomatic | PRs | Lines changed | Score |\n|---|---|---|---|---|---|---|\n"
    body = "".join(
        f"| {i} | `{r['name']}` | {r['cognitive']} | {r['cyclo']} | {r['touches']} | "
        f"{r['lines_changed']} | {r['score']:,.0f} |\n"
        for i, r in enumerate(rows[:n], 1))
    return head + body


def main():
    os.makedirs(IMG, exist_ok=True)
    prs, funcs, files = load()
    for p in prs:
        pair_moves(funcs[p["number"]])

    per_pr = {}
    pr_rows = []
    for p in prs:
        n = p["number"]
        entry = {"flows": {m: flows(funcs[n], m) for m in METRICS}}
        for m in METRICS:
            entry[m] = {"prod": 0, "test": 0}
            for r in files[n]:
                entry[m]["prod" if is_prod(r["file"]) else "test"] += r[f"{m}_after"] - r[f"{m}_before"]
        counts = defaultdict(int)
        for r in funcs[n]:
            counts[r["status"]] += 1
        entry["counts"] = counts
        per_pr[n] = entry
        pr_rows.append([n, p["merged_at"], p["merge_sha"], p["author"]] +
                       [entry["flows"][m][k] for m in METRICS for k, _, _ in FLOW_STYLE] +
                       [entry[m]["prod"] + entry[m]["test"] for m in METRICS] +
                       [counts["new"], counts["removed"], counts["changed"],
                        counts["moved_to"], counts["touched"]])
    write_csv("pr_summary.csv",
              ["pr", "merged_at", "merge_sha", "author"] +
              [f"{m}_{k}" for m in METRICS for k, _, _ in FLOW_STYLE] +
              [f"{m}_net" for m in METRICS] +
              ["funcs_new", "funcs_removed", "funcs_changed", "funcs_moved", "funcs_touched_same_complexity"],
              pr_rows)

    start, end = snapshot_totals("snapshot_start.csv"), snapshot_totals("snapshot_end.csv")
    plot_running_total(prs, per_pr, start, end)
    for m in METRICS:
        plot_flows(prs, per_pr, m)
    plot_cumulative_flows(prs, per_pr)

    file_stats, func_stats = churn_history(prs, funcs, files)
    hot_files, hot_funcs = rank_hotspots(file_stats, func_stats)
    months = sorted({p["time"].strftime("%Y-%m") for p in prs})
    for kind, rows in (("files", hot_files), ("functions", hot_funcs)):
        plot_hotspot_scatter(rows, kind)
        plot_hotspot_heatmap(rows, kind, months)
        write_csv(f"hotspots_{kind}.csv",
                  ["rank", "name", "cognitive", "cyclo", "touches", "lines_changed", "churn", "score"],
                  [[i, r["name"], r["cognitive"], r["cyclo"], r["touches"], r["lines_changed"],
                    f"{r['churn']:.2f}", f"{r['score']:.0f}"] for i, r in enumerate(rows, 1)])
    growth = directory_growth(prs, files)
    plot_directory_growth(growth)

    stats = summarize(prs, per_pr, start, end, hot_files, hot_funcs, growth)
    with open(os.path.join(DATA, "summary.json"), "w") as f:
        json.dump(stats, f, indent=1)
        f.write("\n")
    write_summary_md(stats, hot_files, hot_funcs, growth)
    write_html(stats, hot_files, hot_funcs)


def summarize(prs, per_pr, start, end, hot_files, hot_funcs, growth):
    s = {"prs": len(prs), "since": prs[0]["merged_at"], "until": prs[-1]["merged_at"]}
    s["prs_touching_go"] = sum(1 for p in prs if any(per_pr[p["number"]]["flows"]["cognitive"].values())
                               or per_pr[p["number"]]["counts"])
    for m in METRICS:
        st = start[(m, "prod")] + start[(m, "test")]
        en = end[(m, "prod")] + end[(m, "test")]
        tracked = sum(per_pr[p["number"]][m]["prod"] + per_pr[p["number"]][m]["test"] for p in prs)
        fl = {k: sum(per_pr[p["number"]]["flows"][m][k] for p in prs) for k, _, _ in FLOW_STYLE}
        nets = sorted(((per_pr[p["number"]][m]["prod"] + per_pr[p["number"]][m]["test"], p["number"])
                       for p in prs), reverse=True)
        s[m] = {
            "start": st, "end": en, "start_prod": start[(m, "prod")], "end_prod": end[(m, "prod")],
            "start_test": start[(m, "test")], "end_test": end[(m, "test")],
            "growth_pct": round(100 * (en - st) / st, 1), "tracked_by_prs": tracked,
            "untracked": en - st - tracked, "flows": fl,
            "top_increase_prs": nets[:10], "top_decrease_prs": nets[-10:][::-1],
            "prs_increasing": sum(1 for v, _ in nets if v > 0),
            "prs_decreasing": sum(1 for v, _ in nets if v < 0),
        }
    counts = defaultdict(int)
    for p in prs:
        for k, v in per_pr[p["number"]]["counts"].items():
            counts[k] += v
    s["function_counts"] = dict(counts)
    s["directory_growth_top"] = growth[:10]
    top_share = sum(r["score"] for r in hot_files[:20]) / max(1, sum(r["score"] for r in hot_files))
    s["top20_file_score_share_pct"] = round(100 * top_share, 1)
    s["hot_files"] = len(hot_files)
    s["hot_funcs"] = len(hot_funcs)
    return s


def write_summary_md(s, hot_files, hot_funcs, growth):
    c = s["cognitive"]
    y = s["cyclo"]
    fl = c["flows"]
    added = fl["new_funcs"] + fl["existing_up"]
    lines = [
        "# Go complexity trend: summary",
        "",
        f"Window: {s['since']} to {s['until']}, {s['prs']} PRs merged into `main`. "
        "Generated Go files are excluded.",
        "",
        "| Metric | Start | End | Change | From PRs | Not attributed to a PR |",
        "|---|---|---|---|---|---|",
        f"| Cognitive | {c['start']:,} | {c['end']:,} | +{c['end'] - c['start']:,} ({c['growth_pct']}%) | "
        f"{c['tracked_by_prs']:,} | {c['untracked']:,} |",
        f"| Cyclomatic | {y['start']:,} | {y['end']:,} | +{y['end'] - y['start']:,} ({y['growth_pct']}%) | "
        f"{y['tracked_by_prs']:,} | {y['untracked']:,} |",
        "",
        "Cognitive complexity flows over the window:",
        "",
        f"- Added by new functions: {fl['new_funcs']:,} ({100 * fl['new_funcs'] / added:.0f}% of all additions)",
        f"- Added to existing functions: {fl['existing_up']:,}",
        f"- Removed from existing functions: {fl['existing_down']:,}",
        f"- Removed with deleted functions: {fl['removed_funcs']:,}",
        f"- PRs that raised complexity: {c['prs_increasing']:,}; lowered it: {c['prs_decreasing']:,}",
        "",
        "Function changes: " + ", ".join(f"{k}: {v:,}" for k, v in sorted(s["function_counts"].items())),
        "",
        "## Top file hotspots",
        "",
        table_md(hot_files, "files"),
        "## Top function hotspots",
        "",
        table_md(hot_funcs, "functions"),
        "## Net cognitive complexity growth by package (production code)",
        "",
        "| Package directory | Net change |",
        "|---|---|",
    ] + [f"| `{d}` | {v:+,} |" for d, v in growth[:15]] + [""]
    with open(os.path.join(HERE, "summary.md"), "w") as f:
        f.write("\n".join(lines))


def write_html(s, hot_files, hot_funcs):
    def table(rows, kind):
        head = "<tr><th>#</th><th>{}</th><th>Cognitive</th><th>Cyclomatic</th><th>PRs</th>" \
               "<th>Lines changed</th><th>Score</th></tr>".format("File" if kind == "files" else "Function")
        body = "".join(
            f"<tr><td>{i}</td><td><code>{html.escape(r['name'])}</code></td><td>{r['cognitive']}</td>"
            f"<td>{r['cyclo']}</td><td>{r['touches']}</td><td>{r['lines_changed']}</td>"
            f"<td>{r['score']:,.0f}</td></tr>" for i, r in enumerate(rows[:50], 1))
        return f"<table>{head}{body}</table>"

    with open(os.path.join(HERE, "summary.md")) as f:
        summary = html.escape(f.read())
    imgs = ["running_total", "flows_cognitive", "flows_cyclo", "cumulative_flows", "directory_growth",
            "hotspots_files_scatter", "heatmap_files", "hotspots_functions_scatter", "heatmap_functions"]
    body = "".join(f'<h2>{name.replace("_", " ")}</h2><img src="img/{name}.png">' for name in imgs)
    page = f"""<!doctype html><html><head><meta charset="utf-8"><title>Go complexity trend</title>
<style>body{{font-family:sans-serif;max-width:1400px;margin:auto}}img{{max-width:100%}}
table{{border-collapse:collapse;font-size:13px}}td,th{{border:1px solid #ccc;padding:2px 6px}}</style>
</head><body><h1>Go complexity trend for coder/coder</h1><pre>{summary}</pre>{body}
<h2>Top 50 file hotspots</h2>{table(hot_files, "files")}
<h2>Top 50 function hotspots</h2>{table(hot_funcs, "functions")}</body></html>"""
    with open(os.path.join(HERE, "report.html"), "w") as f:
        f.write(page)
