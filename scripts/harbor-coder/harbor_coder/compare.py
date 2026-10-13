"""Compare two Harbor job directories (baseline vs candidate) and print a
Markdown summary with a verdict.

Usage:
    python -m harbor_coder.compare --baseline jobs/main --candidate jobs/branch \
        [--labels main branch] [--fail-on-regression 0.1] [--json-out out.json]

Rewards are read from each trial's ``result.json`` (``verifier_result.rewards``).
Trials that failed before the agent started (workspace build, image build,
terraform) are infrastructure errors: they are counted separately and excluded
from the pass rate, since they say nothing about the prompt. Trials that
failed after the agent started (chat error, timeout, verifier failure) score 0.

Beyond pass rate, the report compares behavioral signals the Coder Agents
adapter records in ``agent_result.metadata`` (steps, tool calls, whether tests
were run, whether the agent stopped to ask, cost, time), because a system
prompt change usually moves those before it moves pass/fail.

Exit status is 0 unless ``--fail-on-regression`` is set and the candidate's
mean reward is lower than the baseline's by more than that amount.
"""

from __future__ import annotations

import argparse
import json
import statistics
import sys
from collections import defaultdict
from datetime import datetime
from pathlib import Path
from typing import Any

# Behavioral metrics: (metadata key, label, lower-is-better)
_BEHAVIOR = [
    ("n_turns", "Steps", True),
    ("n_tool_calls", "Tool calls", True),
    ("n_exec", "Commands run", None),
    ("n_edit", "Edits", None),
    ("n_tool_errors", "Tool errors", True),
    ("ran_tests", "Ran tests", False),
    ("asked_question", "Stopped to ask", True),
    ("agent_seconds", "Agent time (s)", True),
]


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


def _duration(trial: dict[str, Any]) -> float | None:
    ex = trial.get("agent_execution") or {}
    if ex.get("started_at") and ex.get("finished_at"):
        s = datetime.fromisoformat(ex["started_at"])
        f = datetime.fromisoformat(ex["finished_at"])
        return (f - s).total_seconds()
    return None


def _is_infra_error(trial: dict[str, Any]) -> bool:
    """True when the trial raised before the agent ever ran."""
    if not trial.get("exception_info"):
        return False
    return not ((trial.get("agent_execution") or {}).get("started_at"))


def _summarize(trials: list[dict[str, Any]]) -> dict[str, Any]:
    by_task: dict[str, list[float]] = defaultdict(list)
    behavior: dict[str, list[float]] = defaultdict(list)
    cost = 0.0
    tokens_in = tokens_out = 0
    infra_errors = agent_errors = 0
    durations: list[float] = []
    for t in trials:
        if _is_infra_error(t):
            infra_errors += 1
            continue
        if t.get("exception_info"):
            agent_errors += 1
        by_task[t.get("task_name") or "?"].append(_reward(t))
        ar = t.get("agent_result") or {}
        cost += ar.get("cost_usd") or 0.0
        tokens_in += ar.get("n_input_tokens") or 0
        tokens_out += ar.get("n_output_tokens") or 0
        d = _duration(t)
        if d is not None:
            durations.append(d)
        meta = ar.get("metadata") or {}
        for key, _, _ in _BEHAVIOR:
            if key in meta and meta[key] is not None:
                behavior[key].append(float(meta[key]))
    all_rewards = [r for rs in by_task.values() for r in rs]
    return {
        "n": len(all_rewards),
        "infra_errors": infra_errors,
        "agent_errors": agent_errors,
        "mean": statistics.fmean(all_rewards) if all_rewards else 0.0,
        "by_task": {k: v for k, v in by_task.items()},
        "cost": cost,
        "tokens_in": tokens_in,
        "tokens_out": tokens_out,
        "median_sec": statistics.median(durations) if durations else None,
        "behavior": {k: statistics.fmean(v) for k, v in behavior.items() if v},
    }


def _rate(xs: list[float]) -> str:
    if not xs:
        return "-"
    return f"{sum(1 for x in xs if x >= 1.0)}/{len(xs)}"


def _mean(xs: list[float]) -> float | None:
    return statistics.fmean(xs) if xs else None


def _verdict(delta: float, n_tasks: int, n_trials: int, flipped: int) -> str:
    if n_trials == 0:
        return "no data"
    if abs(delta) < 1e-9:
        return "no difference in task success"
    direction = "better" if delta > 0 else "worse"
    # Below ~10 tasks or ~20 trials a couple of flipped trials move the mean
    # by whole tenths, so say so rather than imply a real effect.
    small = n_tasks < 10 or n_trials < 20
    caveat = (
        f" ({flipped} task{'s' if flipped != 1 else ''} changed; small sample, "
        "compare against a main-vs-main run before trusting the direction)"
        if small
        else f" ({flipped} tasks changed)"
    )
    return f"candidate is {direction} on task success by {delta:+.2f}{caveat}"


def _fmt(x: float | None, kind: str) -> str:
    if x is None:
        return "-"
    if kind == "bool":
        return f"{x * 100:.0f}%"
    if kind == "sec":
        return f"{x:.0f}"
    return f"{x:.1f}"


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
    flipped = sum(
        1
        for t in tasks
        if _mean(base["by_task"].get(t, [])) != _mean(cand["by_task"].get(t, []))
    )
    verdict = _verdict(delta, len(tasks), min(base["n"], cand["n"]), flipped)

    lines = [
        "## Coder Agents eval: branch vs main",
        "",
        f"**Verdict:** {verdict}",
        "",
        f"| Metric | {lb} | {lc} | delta |",
        "| --- | ---: | ---: | ---: |",
        f"| Task success (mean reward) | {base['mean']:.2f} | {cand['mean']:.2f} | {delta:+.2f} |",
        f"| Trials scored (agent errors) | {base['n']} ({base['agent_errors']}) | {cand['n']} ({cand['agent_errors']}) | |",
        f"| Infra errors (excluded) | {base['infra_errors']} | {cand['infra_errors']} | |",
        f"| Total cost | ${base['cost']:.4f} | ${cand['cost']:.4f} | ${cand['cost'] - base['cost']:+.4f} |",
        f"| Input tokens | {base['tokens_in']} | {cand['tokens_in']} | {cand['tokens_in'] - base['tokens_in']:+d} |",
        f"| Output tokens | {base['tokens_out']} | {cand['tokens_out']} | {cand['tokens_out'] - base['tokens_out']:+d} |",
    ]
    if base["median_sec"] is not None and cand["median_sec"] is not None:
        lines.append(
            f"| Median trial time | {base['median_sec']:.0f}s | {cand['median_sec']:.0f}s | "
            f"{cand['median_sec'] - base['median_sec']:+.0f}s |"
        )

    lines += [
        "",
        "### Per task (passes/attempts)",
        "",
        f"| Task | {lb} | {lc} | delta |",
        "| --- | ---: | ---: | ---: |",
    ]
    for task in tasks:
        b = base["by_task"].get(task, [])
        c = cand["by_task"].get(task, [])
        mb, mc = _mean(b), _mean(c)
        d = f"{mc - mb:+.2f}" if (mb is not None and mc is not None) else "-"
        lines.append(f"| {task} | {_rate(b)} | {_rate(c)} | {d} |")

    if base["behavior"] or cand["behavior"]:
        lines += [
            "",
            "### Behavior (per-trial mean)",
            "",
            f"| Signal | {lb} | {lc} | delta | |",
            "| --- | ---: | ---: | ---: | --- |",
        ]
        for key, label, lower_better in _BEHAVIOR:
            b = base["behavior"].get(key)
            c = cand["behavior"].get(key)
            if b is None and c is None:
                continue
            kind = (
                "bool"
                if key in ("ran_tests", "asked_question")
                else ("sec" if key == "agent_seconds" else "num")
            )
            d = (c - b) if (b is not None and c is not None) else None
            dstr = "-"
            note = ""
            if d is not None:
                dstr = f"{d * 100:+.0f}pt" if kind == "bool" else f"{d:+.1f}"
                # Only label a direction when the rounded delta is visible.
                visible = abs(d) >= (0.005 if kind == "bool" else 0.05)
                if lower_better is not None and visible:
                    improved = (d < 0) if lower_better else (d > 0)
                    note = "better" if improved else "worse"
            lines.append(
                f"| {label} | {_fmt(b, kind)} | {_fmt(c, kind)} | {dstr} | {note} |"
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
