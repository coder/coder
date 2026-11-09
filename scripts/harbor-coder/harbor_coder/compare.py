"""Compare two Harbor job directories and print a Markdown summary.

    python -m harbor_coder.compare --baseline jobs/baseline --candidate jobs/candidate --labels main my-branch

Infrastructure errors are excluded: trials that raised before the agent
started, and trials whose chat failed for gateway or deployment reasons
(``CoderInfraError``). Other trials with no reward score 0.
"""

from __future__ import annotations

import argparse
import json
import random
import statistics
import sys
from collections import defaultdict
from pathlib import Path

# Must match harbor_coder.agent.CoderInfraError; Harbor records the class name.
_INFRA_EXCEPTION = "CoderInfraError"
_BOOTSTRAP_SAMPLES = 10_000


def _summarize(job_dir: Path) -> dict:
    by_task: dict[str, list[float]] = defaultdict(list)
    cost = 0.0
    infra = queued = 0
    models: set[str] = set()
    versions: set[str] = set()
    # A trial dir without result.json crashed hard enough that Harbor never
    # finalized it; surface it rather than silently shrinking the sample.
    missing = [
        d.name
        for d in sorted(job_dir.iterdir())
        if d.is_dir() and not d.name.startswith(".") and not (d / "result.json").exists()
    ]
    for f in sorted(job_dir.glob("*/result.json")):
        t = json.loads(f.read_text())
        if (v := (t.get("agent_info") or {}).get("version")) and v != "unknown":
            versions.add(v)
        exc = t.get("exception_info") or {}
        if exc and (
            not (t.get("agent_execution") or {}).get("started_at") or exc.get("exception_type") == _INFRA_EXCEPTION
        ):
            infra += 1
            continue
        rewards = (t.get("verifier_result") or {}).get("rewards") or {}
        by_task[t.get("task_name") or "?"].append(float(rewards.get("reward", 0.0)))
        agent = t.get("agent_result") or {}
        cost += agent.get("cost_usd") or 0.0
        meta = agent.get("metadata") or {}
        if m := meta.get("model"):
            models.add(m)
        queued += bool(meta.get("queued_for_capacity"))
    rewards = [r for rs in by_task.values() for r in rs]
    return {
        "by_task": by_task,
        "mean": statistics.fmean(rewards) if rewards else 0.0,
        "n": len(rewards),
        "infra": infra,
        "missing": missing,
        "queued": queued,
        "cost": cost,
        "model": ", ".join(sorted(models)) or "-",
        "version": ", ".join(sorted(versions)) or "-",
    }


def paired_ci(
    base: dict[str, list[float]], cand: dict[str, list[float]], n: int = _BOOTSTRAP_SAMPLES, seed: int = 0
) -> tuple[float, float, float, int] | None:
    """Mean per-task difference and its 95% bootstrap interval over tasks scored in both arms."""
    tasks = sorted(set(base) & set(cand))
    if not tasks:
        return None
    diffs = [statistics.fmean(cand[t]) - statistics.fmean(base[t]) for t in tasks]
    rng = random.Random(seed)
    means = sorted(statistics.fmean(rng.choices(diffs, k=len(diffs))) for _ in range(n))
    return statistics.fmean(diffs), means[int(0.025 * n)], means[int(0.975 * n) - 1], len(tasks)


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
    ci = paired_ci(base["by_task"], cand["by_task"])
    if ci is None:
        verdict = "no data"
    else:
        delta, lo, hi, n_tasks = ci
        interval = f"{delta:+.3f}, 95% CI [{lo:+.3f}, {hi:+.3f}] over {n_tasks} tasks"
        if lo > 0 or hi < 0:
            verdict = f"{lc} is {'better' if delta > 0 else 'worse'} than {lb}: {interval}"
        else:
            verdict = f"no significant difference: {interval}"
    tasks = sorted(set(base["by_task"]) | set(cand["by_task"]))
    lines = [
        f"## Coder Agents eval: {lc} vs {lb}",
        "",
        f"**{verdict}**",
        "",
        f"| | {lb} | {lc} |",
        "| --- | ---: | ---: |",
        f"| Task success | {base['mean']:.3f} | {cand['mean']:.3f} |",
        f"| coderd | {base['version']} | {cand['version']} |",
        f"| Model | {base['model']} | {cand['model']} |",
        f"| Trials scored | {base['n']} | {cand['n']} |",
        f"| Infra errors (excluded) | {base['infra']} | {cand['infra']} |",
        f"| Cost | ${base['cost']:.2f} | ${cand['cost']:.2f} |",
    ]
    for key, label in (("missing", "Trials without result.json"), ("queued", "Trials queued for chat capacity")):
        b, c = (len(s[key]) if isinstance(s[key], list) else s[key] for s in (base, cand))
        if b or c:
            lines.append(f"| {label} | {b} | {c} |")
    lines += ["", f"| Task | {lb} | {lc} |", "| --- | ---: | ---: |"]
    lines += [
        f"| {t} | {_rate(base['by_task'].get(t, []))} | {_rate(cand['by_task'].get(t, []))} |" for t in tasks
    ]
    for label, s in ((lb, base), (lc, cand)):
        if s["missing"]:
            print(f"warning: {label}: no result.json in {', '.join(s['missing'])}", file=sys.stderr)
    print("\n".join(lines))


if __name__ == "__main__":
    main()
