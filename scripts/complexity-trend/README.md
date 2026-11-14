# Go complexity trend

Measures how the complexity of the Go code in `coder/coder` changed over a
window of time, one merged pull request at a time, and finds the hotspots
where high complexity meets high churn.

Experimental. Nothing in the build, CI, or the main Go module depends on it.

## Run it

Prerequisites: Go, `git`, an authenticated `gh` CLI, Python 3, and matplotlib
for the report step. Run from a clone of `coder/coder` whose `origin` points
at GitHub.

```sh
cd scripts/complexity-trend

# 1. List PRs merged into main (writes data/prs.json and prs.md).
python3 complexity_trend.py fetch --since 2025-11-14 --until 2026-11-15

# 2. Measure every PR (writes data/functions.csv, data/files.csv, snapshots).
python3 complexity_trend.py measure

# 3. Build charts, heat maps, rankings, summary.md, and report.html.
python3 -m venv .venv && .venv/bin/pip install matplotlib
.venv/bin/python complexity_trend.py report
```

`fetch` takes about 5 minutes, `measure` about 90 seconds on 13 cores, and
`report` about 30 seconds. `--until` is exclusive and defaults to today;
`--since` defaults to 365 days before `--until`.

## How it works

- `fetch` queries `gh pr list --base main --state merged` in 14-day windows
  to stay under the search API's 1,000 result cap. It then walks the
  first-parent history of `origin/main` and adds stacked PRs: they merge into
  another PR's branch, so the search misses them, but their squashed commit
  still lands on `main` with `(#N)` in the subject.
- `measure` builds `measure/` (a separate Go module, so the main module gains
  no dependencies). For each PR it diffs the merge commit against its first
  parent with `git diff -M -U0 -- '*.go'`, parses both versions of each
  changed file, and scores every function declaration with the `gocyclo` and
  `gocognit` libraries. Closures count toward the function that contains
  them, which matches the command-line tools. It also snapshots every Go
  function at the parent of the first PR and at the last PR, so the report
  can check that the per-PR deltas add up.
- `report` excludes generated files (`// Code generated ... DO NOT EDIT.`),
  pairs functions that moved between files in one PR, and builds the outputs
  below.

### Function groups

| Status in `functions.csv` | Meaning |
|---|---|
| `new` | The function did not exist before the PR. |
| `removed` | The PR deleted the function. |
| `changed` | The function existed before and its complexity went up or down. |
| `touched` | The PR changed lines in the function, but neither score changed. Kept for churn. |

Functions are keyed by file path and `Receiver.Name`, with pointer and type
parameters stripped. Renamed files are followed through `git diff -M`. A
function that disappears from one file and appears in another within one PR
is reported as a move (`moved_from`/`moved_to` in the report, counted as a
change to an existing function). Methods pair anywhere; plain functions pair
only within one directory.

### Hotspot score

`score = cognitive complexity now x sqrt(PRs that touched it x lines changed)`

The square root is the geometric mean of how often and how much the code
changed, so complexity and churn carry equal weight. Hotspots cover
production code only (no `_test.go`, no generated files) that still exists at
the end of the window. Churn history follows file renames and function moves.

## Outputs

| Path | Contents |
|---|---|
| `prs.md` | Every PR in the window: number, title, author, merge date, merge commit. |
| `data/prs.json` | The same list, machine-readable. |
| `data/functions.csv` | One row per function a PR touched: before and after scores, lines, status. |
| `data/files.csv` | One row per Go file a PR touched: totals before and after, lines added and removed. |
| `data/snapshot_start.csv`, `data/snapshot_end.csv` | Every Go function at the start and end of the window. |
| `data/pr_summary.csv` | Per-PR complexity flows (new functions, existing up, existing down, deleted). |
| `data/hotspots_files.csv`, `data/hotspots_functions.csv` | Full hotspot rankings. |
| `data/summary.json`, `summary.md` | Headline numbers and top tables. |
| `img/*.png`, `report.html` | Charts and heat maps. |

## Limitations

- Only `FuncDecl` bodies are scored. Package-level `var x = func() {...}`
  literals are not.
- A few commits on `main` have no PR and are not measured. The report shows
  the gap between the per-PR sum and the full-tree snapshots.
- Move pairing is a heuristic. Two unrelated functions with the same name in
  one directory can pair, and a moved function that changed its name counts
  as removed plus new.
- The hotspot ranking follows renames inside the window only.
