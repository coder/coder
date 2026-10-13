#!/usr/bin/env bash
cd /app && cat > summarize.py <<'PY'
import csv, json, sys
from collections import defaultdict

totals = defaultdict(float)
with open(sys.argv[1], newline="") as f:
    for row in csv.DictReader(f):
        totals[row["region"]] += int(row["units"]) * float(row["unit_price"])
print(json.dumps({k: round(v, 2) for k, v in sorted(totals.items())}))
PY
