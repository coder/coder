#!/usr/bin/env bash
# Writes 1 or 0 to /logs/verifier/reward.txt based on the workspace end state.
mkdir -p /logs/verifier
if [ -f /app/hello.txt ] && [ "$(cat /app/hello.txt)" = "Hello, world!" ]; then
  echo 1 > /logs/verifier/reward.txt
else
  echo "hello.txt missing or wrong:"; cat /app/hello.txt 2>/dev/null || true
  echo 0 > /logs/verifier/reward.txt
fi
