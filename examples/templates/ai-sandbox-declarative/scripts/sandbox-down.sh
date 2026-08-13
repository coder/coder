#!/usr/bin/env bash
# Reclaims the coder/sandbox microVM on workspace stop.
#
# Credentials are already inert by the time this runs: the server revokes
# this build's keys on the stop transition, and the bound agent's token dies
# with its build. This script only reclaims the guest, so a failure here
# leaks a microVM but never a credential.
set -euo pipefail

log() { echo "[sandbox-down] $*"; }

SANDBOX_NAME="${CODER_AI_SANDBOX_NAME:?sandbox name is required}"

SANDBOX_BIN="${CODER_SANDBOX_BIN:-$(command -v coder-sandbox || true)}"
if [ -z "${SANDBOX_BIN}" ]; then
	log "coder-sandbox not found, nothing to reclaim"
	exit 0
fi

log "tearing down ${SANDBOX_NAME}"
"${SANDBOX_BIN}" down "${SANDBOX_NAME}" >/dev/null 2>&1 ||
	log "sandbox ${SANDBOX_NAME} already gone"

log "done"
