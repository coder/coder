"""Compare two Harbor job directories and print a Markdown summary.

    python -m harbor_coder.compare --baseline jobs/baseline --candidate jobs/candidate --labels main my-branch

Trials that raised before the agent started are infrastructure errors and are
excluded; trials with no reward after the agent ran score 0.
"""

from __future__ import annotations

import argparse
import json
import statistics
from collections import defaultdict
from pathlib import Path


def _summarize(job_dir: Path) -> dict:
    by_task: dict[str, list[float]] = defaultdict(list)
    cost = 0.0
    infra = 0
    models: set[str] = set()
    for f in sorted(job_dir.glob("*/result.json")):
        t = json.loads(f.read_text())
        if t.get("exception_info") and not (t.get("agent_execution") or {}).get("started_at"):
            infra += 1
            continue
        rewards = (t.get("verifier_result") or {}).get("rewards") or {}
        by_task[t.get("task_name") or "?"].append(float(rewards.get("reward", 0.0)))
        agent = t.get("agent_result") or {}
        cost += agent.get("cost_usd") or 0.0
        if m := (agent.get("metadata") or {}).get("model"):
            models.add(m)
    rewards = [r for rs in by_task.values() for r in rs]
    return {
        "by_task": by_task,
        "mean": statistics.fmean(rewards) if rewards else 0.0,
        "n": len(rewards),
        "infra": infra,
        "cost": cost,
        "model": ", ".join(sorted(models)) or "-",
    }


def _rate(xs: list[float]) -> str:
    return f"{sum(1 for x in xs if x >= 1.0)}/{len(xs)}" if xs else "-"


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--baseline", required=True, type=Path)
    p.add_argument("--candidate", required=True, type=Path)
    p.add_argument("--labels", nargs=2, default=["main", "branch"])
    args = p.parse_args()
    base, cand = _summarize(args.baseline), _summarize(args.candidate)
    lb, lc = args.labels
    delta = cand["mean"] - base["mean"]
    tasks = sorted(set(base["by_task"]) | set(cand["by_task"]))
    if base["n"] == 0 or cand["n"] == 0:
        verdict = "no data"
    elif abs(delta) < 1e-9:
        verdict = "no difference in task success"
    else:
        verdict = f"{lc} is {'better' if delta > 0 else 'worse'} than {lb} by {delta:+.2f}"
        if len(tasks) < 10 or min(base["n"], cand["n"]) < 20:
            verdict += " (small sample; one flipped trial moves this a lot)"
    lines = [
        f"## Coder Agents eval: {lc} vs {lb}",
        "",
        f"**{verdict}**",
        "",
        f"| | {lb} | {lc} |",
        "| --- | ---: | ---: |",
        f"| Task success | {base['mean']:.2f} | {cand['mean']:.2f} |",
        f"| Model | {base['model']} | {cand['model']} |",
        f"| Trials scored | {base['n']} | {cand['n']} |",
        f"| Infra errors (excluded) | {base['infra']} | {cand['infra']} |",
        f"| Cost | ${base['cost']:.2f} | ${cand['cost']:.2f} |",
        "",
        f"| Task | {lb} | {lc} |",
        "| --- | ---: | ---: |",
    ]
    lines += [
        f"| {t} | {_rate(base['by_task'].get(t, []))} | {_rate(cand['by_task'].get(t, []))} |" for t in tasks
    ]
    print("\n".join(lines))


if __name__ == "__main__":
    main()
