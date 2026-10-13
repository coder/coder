#!/usr/bin/env bash
mkdir -p /logs/verifier
cd /app || { echo 0 > /logs/verifier/reward.txt; exit 0; }
if ! cmp -s test_logdate.py /tests/test_logdate.py; then
  echo "test_logdate.py was modified"; echo 0 > /logs/verifier/reward.txt; exit 0
fi
if python -m pytest -q test_logdate.py; then echo 1 > /logs/verifier/reward.txt; else echo 0 > /logs/verifier/reward.txt; fi
