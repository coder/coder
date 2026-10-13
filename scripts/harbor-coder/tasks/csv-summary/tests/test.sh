#!/usr/bin/env bash
mkdir -p /logs/verifier
cd /app || { echo 0 > /logs/verifier/reward.txt; exit 0; }
if python /tests/check.py; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
