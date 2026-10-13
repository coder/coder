"""Verifier for csv-summary. Runs the agent's script on the shipped CSV and on
a second hidden CSV so a hard-coded answer does not pass."""

import json
import os
import subprocess
import sys
import tempfile

HIDDEN = """date,region,product,units,unit_price
2026-02-01,south,widget,3,1.10
2026-02-01,south,gadget,2,5.00
2026-02-02,north,widget,0,1.10
2026-02-03,east,gizmo,1,0.333
"""


def run(csv_path: str) -> dict:
    proc = subprocess.run(
        [sys.executable, "summarize.py", csv_path],
        capture_output=True,
        text=True,
        timeout=30,
        cwd="/app",
    )
    if proc.returncode != 0:
        print("non-zero exit:", proc.stderr)
        sys.exit(1)
    try:
        out = json.loads(proc.stdout)
    except json.JSONDecodeError:
        print("stdout is not a single JSON object:", repr(proc.stdout[:300]))
        sys.exit(1)
    if not isinstance(out, dict):
        print("expected JSON object")
        sys.exit(1)
    return out


def check(actual: dict, expected: dict) -> None:
    if list(actual.keys()) != sorted(expected.keys()):
        print("keys mismatch:", list(actual.keys()), "vs", sorted(expected.keys()))
        sys.exit(1)
    for k, v in expected.items():
        if abs(float(actual[k]) - v) > 0.005:
            print(f"value mismatch for {k}: {actual[k]} vs {v}")
            sys.exit(1)


if not os.path.exists("/app/summarize.py"):
    print("summarize.py missing")
    sys.exit(1)

check(run("sales.csv"), {"east": 87.47, "north": 0.0, "west": 59.49})

with tempfile.NamedTemporaryFile("w", suffix=".csv", delete=False) as f:
    f.write(HIDDEN)
check(run(f.name), {"east": 0.33, "north": 0.0, "south": 13.30})
print("ok")
