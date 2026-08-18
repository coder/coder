#!/usr/bin/env bash
# Stops and removes the coder/sandbox microVM for this controller lifecycle.
set -euo pipefail

: "${CODER_SANDBOX_ID:?CODER_SANDBOX_ID is required}"
SANDBOX_NAME="coder-${CODER_SANDBOX_ID}"
SANDBOX_BIN="${CODER_SANDBOX_BIN:-$(command -v coder-sandbox || true)}"

if [ -z "${SANDBOX_BIN}" ]; then
	echo "[sandbox-destroy] coder-sandbox not found, nothing to stop"
	exit 0
fi

"${SANDBOX_BIN}" down "${SANDBOX_NAME}"
