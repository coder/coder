#!/usr/bin/env bash
mkdir -p /logs/verifier
cd /app || { echo 0 > /logs/verifier/reward.txt; exit 0; }
pip install --quiet --no-cache-dir pytest==8.3.3 >/dev/null 2>&1 || true
if python /tests/check.py; then echo 1 > /logs/verifier/reward.txt; else echo 0 > /logs/verifier/reward.txt; fi
