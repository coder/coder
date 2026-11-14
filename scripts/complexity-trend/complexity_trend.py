#!/usr/bin/env python3
"""Measure how Go complexity in coder/coder changed, one merged PR at a time.

Subcommands:
  fetch    List PRs merged into main in a date window (gh CLI) and write
           data/prs.json and prs.md.
  measure  Build the Go measurer and record per-function and per-file
           cyclomatic and cognitive complexity before and after each PR.
  report   Aggregate the raw data into charts, heat maps, hotspot tables,
           an HTML report, and a findings summary.
  all      Run fetch, measure, and report.

See README.md for details.
"""

import argparse
import datetime as dt
import json
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.join(HERE, "data")
REPO_SLUG = "coder/coder"


def run(cmd, **kw):
    return subprocess.run(cmd, check=True, text=True, capture_output=True, **kw).stdout


def repo_root():
    return run(["git", "-C", HERE, "rev-parse", "--show-toplevel"]).strip()


# ---------------------------------------------------------------- fetch


def fetch(args):
    end = dt.date.fromisoformat(args.until) if args.until else dt.date.today()
    start = dt.date.fromisoformat(args.since) if args.since else end - dt.timedelta(days=365)
    prs = {}
    # The search API returns at most 1000 results per query, so query in
    # windows that stay well under that limit.
    cur = start
    while cur < end:
        nxt = min(cur + dt.timedelta(days=14), end)
        # merged:A..B is inclusive on both ends; dedupe by PR number.
        q = f"merged:{cur.isoformat()}..{(nxt - dt.timedelta(days=1)).isoformat()}"
        out = run([
            "gh", "pr", "list", "-R", REPO_SLUG, "--base", "main", "--state", "merged",
            "--search", q, "--limit", "1000",
            "--json", "number,title,author,mergedAt,mergeCommit",
        ])
        batch = json.loads(out)
        if len(batch) >= 1000:
            sys.exit(f"window {q} hit the 1000 result limit; shrink the window")
        for p in batch:
            prs[p["number"]] = {
                "number": p["number"],
                "title": p["title"],
                "author": (p.get("author") or {}).get("login", "ghost"),
                "merged_at": p["mergedAt"],
                "merge_sha": (p.get("mergeCommit") or {}).get("oid", ""),
            }
        print(f"{q}: {len(batch)} PRs", file=sys.stderr)
        cur = nxt
    add_stacked_prs(prs, start, end)
    rows = sorted(prs.values(), key=lambda p: (p["merged_at"], p["number"]))
    os.makedirs(DATA, exist_ok=True)
    with open(os.path.join(DATA, "prs.json"), "w") as f:
        json.dump({"since": start.isoformat(), "until": end.isoformat(), "prs": rows}, f, indent=1)
        f.write("\n")
    write_prs_md()


def write_prs_md():
    with open(os.path.join(DATA, "prs.json")) as f:
        doc = json.load(f)
    rows = doc["prs"]
    with open(os.path.join(HERE, "prs.md"), "w") as f:
        f.write(f"# PRs merged into `main` of {REPO_SLUG}\n\n")
        f.write(f"Window: {doc['since']} to {doc['until']} (exclusive). Total: {len(rows)}.\n\n")
        f.write("PRs marked *stacked* merged into another PR's branch; the merge commit is the commit "
                "that brought them onto `main`.\n\n")
        f.write("| PR | Title | Author | Merged (UTC) | Merge commit |\n|---|---|---|---|---|\n")
        for p in rows:
            # The repository forbids em and en dashes in committed files.
            title = re.sub("[\u2013\u2014]", "-", p["title"]).replace("|", "\\|")
            if p.get("stacked_on"):
                title += " *stacked*"
            f.write(f"| #{p['number']} | {title} | @{p['author']} | {p['merged_at']} | `{p['merge_sha'][:12]}` |\n")
    print(f"wrote {len(rows)} PRs", file=sys.stderr)


def add_stacked_prs(prs, start, end):
    """Add PRs that landed on main as part of a stack.

    A stacked PR is merged into its parent branch, so the base:main search
    misses it and its mergeCommit is not on main. Its squashed commit still
    lands on main's first-parent history with "(#N)" in the subject, so match
    those commits back to their PRs and use the commit on main as the merge
    commit.
    """
    root = repo_root()
    run(["git", "-C", root, "fetch", "--quiet", "origin", "main"])
    log = run(["git", "-C", root, "log", "--first-parent", "origin/main", "--format=%H%x00%cI%x00%s",
               f"--since={start.isoformat()}T00:00:00Z", f"--until={end.isoformat()}T00:00:00Z"])
    known = {p["merge_sha"] for p in prs.values()}
    unmatched = 0
    for line in log.splitlines():
        sha, committed, subject = line.split("\x00", 2)
        if sha in known:
            continue
        m = re.search(r"\(#(\d+)\)\s*$", subject)
        if not m or int(m.group(1)) in prs:
            unmatched += 1
            continue
        view = subprocess.run(["gh", "pr", "view", m.group(1), "-R", REPO_SLUG,
                               "--json", "number,title,author,mergedAt,baseRefName,state"],
                              text=True, capture_output=True)
        # The number can point at an issue or a PR that no longer resolves.
        if view.returncode != 0:
            unmatched += 1
            continue
        p = json.loads(view.stdout)
        if p["state"] != "MERGED":
            unmatched += 1
            continue
        prs[p["number"]] = {
            "number": p["number"],
            "title": p["title"],
            "author": (p.get("author") or {}).get("login", "ghost"),
            # The PR merged into its stack parent earlier; this is when the
            # change reached main.
            "merged_at": dt.datetime.fromisoformat(committed).astimezone(dt.timezone.utc)
            .strftime("%Y-%m-%dT%H:%M:%SZ"),
            "merge_sha": sha,
            "stacked_on": p["baseRefName"],
        }
    print(f"first-parent commits on main without a PR: {unmatched}", file=sys.stderr)


# ---------------------------------------------------------------- measure


def load_prs():
    with open(os.path.join(DATA, "prs.json")) as f:
        return json.load(f)["prs"]


def measure(args):
    root = repo_root()
    prs = load_prs()
    run(["git", "-C", root, "fetch", "--quiet", "origin", "main"])
    binary = os.path.join(DATA, ".measure")
    run(["go", "build", "-o", binary, "./measure"], cwd=HERE)
    missing = [p["number"] for p in prs
               if subprocess.run(["git", "-C", root, "cat-file", "-e", p["merge_sha"] + "^{commit}"],
                                 capture_output=True).returncode != 0]
    if missing:
        sys.exit(f"merge commits not available locally for PRs: {missing[:20]}")
    stdin = "".join(f"{p['number']} {p['merge_sha']}\n" for p in prs)
    subprocess.run([binary, "-repo", root, "-out", DATA], input=stdin, text=True, check=True)
    # Snapshots of the whole tree just before the first PR and just after the
    # last one anchor the running total and let the report check that the
    # per-PR deltas add up.
    order = {sha: i for i, sha in enumerate(reversed(
        run(["git", "-C", root, "rev-list", "--first-parent", "origin/main"]).split()))}
    by_position = sorted(prs, key=lambda p: order[p["merge_sha"]])
    first, last = by_position[0]["merge_sha"], by_position[-1]["merge_sha"]
    subprocess.run([binary, "-repo", root, "-out", DATA, "-snapshot", first + "^1",
                    "-snapshot-name", "snapshot_start.csv"], check=True)
    subprocess.run([binary, "-repo", root, "-out", DATA, "-snapshot", last,
                    "-snapshot-name", "snapshot_end.csv"], check=True)
    os.remove(binary)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    f = sub.add_parser("fetch")
    f.add_argument("--since", help="start date YYYY-MM-DD (default: 365 days before --until)")
    f.add_argument("--until", help="end date YYYY-MM-DD, exclusive (default: today)")
    sub.add_parser("measure")
    sub.add_parser("report")
    a = sub.add_parser("all")
    a.add_argument("--since")
    a.add_argument("--until")
    args = ap.parse_args()
    if args.cmd in ("fetch", "all"):
        fetch(args)
    if args.cmd in ("measure", "all"):
        measure(args)
    if args.cmd in ("report", "all"):
        # Imported lazily so fetch and measure need no third-party packages.
        import report
        report.main()


if __name__ == "__main__":
    main()
