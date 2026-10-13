"""Compare two Harbor job directories (baseline vs candidate) and print a
Markdown summary with a verdict.

Usage:
    python -m harbor_coder.compare --baseline jobs/main --candidate jobs/branch \
        [--labels main branch] [--fail-on-regression 0.1]

Rewards are read from each trial's ``result.json`` (``verifier_result.rewards``).
A trial that errored or has no reward counts as 0 so infrastructure failures
are visible in the rate instead of silently dropped. Exit status is 0 unless
``--fail-on-regression`` is set and the candidate's mean reward is lower than
the baseline's by more than that amount.
"""

from __future__ import annotations

import argparse
import json
import statistics
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any


def _load_trials(job_dir: Path) -> list[dict[str, Any]]:
    trials = []
    for result in sorted(job_dir.glob("*/result.json")):
        try:
            trials.append(json.loads(result.read_text()))
        except json.JSONDecodeError:
            continue
    return trials


def _reward(trial: dict[str, Any]) -> float:
    rewards = ((trial.get("verifier_result") or {}).get("rewards")) or {}
    if not rewards:
        return 0.0
    if "reward" in rewards:
        return float(rewards["reward"])
    return float(statistics.fmean(float(v) for v in rewards.values()))


def _summarize(trials: list[dict[str, Any]]) -> dict[str, Any]:
    by_task: dict[str, list[float]] = defaultdict(list)
    cost = 0.0
    tokens_in = tokens_out = 0
    errors = 0
    durations: list[float] = []
    for t in trials:
        by_task[t.get("task_name") or "?"].append(_reward(t))
        ar = t.get("agent_result") or {}
        cost += ar.get("cost_usd") or 0.0
        tokens_in += ar.get("n_input_tokens") or 0
        tokens_out += ar.get("n_output_tokens") or 0
        if t.get("exception_info"):
            errors += 1
        ex = t.get("agent_execution") or {}
        if ex.get("started_at") and ex.get("finished_at"):
            from datetime import datetime

            s = datetime.fromisoformat(ex["started_at"])
            f = datetime.fromisoformat(ex["finished_at"])
            durations.append((f - s).total_seconds())
    all_rewards = [r for rs in by_task.values() for r in rs]
    return {
        "n": len(trials),
        "errors": errors,
        "mean": statistics.fmean(all_rewards) if all_rewards else 0.0,
        "by_task": {k: statistics.fmean(v) for k, v in by_task.items()},
        "cost": cost,
        "tokens_in": tokens_in,
        "tokens_out": tokens_out,
        "median_sec": statistics.median(durations) if durations else None,
    }


def _fmt_rate(x: float) -> str:
    return f"{x:.2f}"


def _verdict(delta: float, n_tasks: int, n_trials: int) -> str:
    if n_trials == 0:
        return "no data"
    if abs(delta) < 1e-9:
        return "no difference"
    direction = "better" if delta > 0 else "worse"
    # With tiny suites a single flipped trial moves the mean a lot; say so.
    caveat = (
        " (small sample, treat as a smoke signal)"
        if n_tasks < 5 or n_trials < 10
        else ""
    )
    return f"candidate is {direction} by {delta:+.2f}{caveat}"


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--baseline", required=True, type=Path)
    p.add_argument("--candidate", required=True, type=Path)
    p.add_argument("--labels", nargs=2, default=["main", "branch"])
    p.add_argument("--fail-on-regression", type=float, default=None)
    p.add_argument("--json-out", type=Path, default=None)
    args = p.parse_args()

    base = _summarize(_load_trials(args.baseline))
    cand = _summarize(_load_trials(args.candidate))
    lb, lc = args.labels
    delta = cand["mean"] - base["mean"]
    tasks = sorted(set(base["by_task"]) | set(cand["by_task"]))
    verdict = _verdict(delta, len(tasks), min(base["n"], cand["n"]))

    lines = [
        "## Coder Agents eval: branch vs main",
        "",
        f"**Verdict:** {verdict}",
        "",
        f"| Metric | {lb} | {lc} | delta |",
        "| --- | ---: | ---: | ---: |",
        f"| Mean reward | {_fmt_rate(base['mean'])} | {_fmt_rate(cand['mean'])} | {delta:+.2f} |",
        f"| Trials (errors) | {base['n']} ({base['errors']}) | {cand['n']} ({cand['errors']}) | |",
        f"| Total cost | ${base['cost']:.4f} | ${cand['cost']:.4f} | ${cand['cost'] - base['cost']:+.4f} |",
        f"| Input tokens | {base['tokens_in']} | {cand['tokens_in']} | {cand['tokens_in'] - base['tokens_in']:+d} |",
        f"| Output tokens | {base['tokens_out']} | {cand['tokens_out']} | {cand['tokens_out'] - base['tokens_out']:+d} |",
    ]
    if base["median_sec"] is not None and cand["median_sec"] is not None:
        lines.append(
            f"| Median agent time | {base['median_sec']:.0f}s | {cand['median_sec']:.0f}s | "
            f"{cand['median_sec'] - base['median_sec']:+.0f}s |"
        )
    lines += [
        "",
        "### Per task",
        "",
        f"| Task | {lb} | {lc} | delta |",
        "| --- | ---: | ---: | ---: |",
    ]
    for task in tasks:
        b = base["by_task"].get(task)
        c = cand["by_task"].get(task)
        d = (c - b) if (b is not None and c is not None) else None
        lines.append(
            f"| {task} | {_fmt_rate(b) if b is not None else '-'} | "
            f"{_fmt_rate(c) if c is not None else '-'} | {f'{d:+.2f}' if d is not None else '-'} |"
        )
    report = "\n".join(lines)
    print(report)

    if args.json_out:
        args.json_out.write_text(
            json.dumps(
                {
                    "baseline": base,
                    "candidate": cand,
                    "delta": delta,
                    "verdict": verdict,
                },
                indent=2,
            )
        )

    if args.fail_on_regression is not None and delta < -args.fail_on_regression:
        print(
            f"::error::regression: {delta:+.2f} exceeds threshold {args.fail_on_regression}",
            file=sys.stderr,
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
