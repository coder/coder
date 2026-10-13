#!/usr/bin/env bash
# Pass only if the original tests are unchanged and all pass.
mkdir -p /logs/verifier
cd /app || { echo 0 > /logs/verifier/reward.txt; exit 0; }

if ! cmp -s test_intervals.py /tests/test_intervals.py; then
  echo "test_intervals.py was modified"
  echo 0 > /logs/verifier/reward.txt
  exit 0
fi

if python -m pytest -q test_intervals.py; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
