#!/usr/bin/env bash
# Tears down the coder/sandbox microVM created by
# sandbox-create-microvm.sh.
#
# The platform has already revoked the child agent token and the scoped AI
# session token by the time this runs, so a surviving guest process cannot
# authenticate. Removing the microVM still matters: coder/sandbox does not
# reconcile orphaned VMs after a daemon restart, so a leaked VM would keep
# running with a stale egress ruleset.
#
# CODER_AI_SESSION_TOKEN is deliberately absent here: it may have been
# rotated, and teardown must not depend on a credential.
set -euo pipefail

SANDBOX_NAME="coder-${CODER_SANDBOX_ID}"

log() { echo "[sandbox-destroy-microvm] $*"; }

SANDBOX_BIN="${CODER_SANDBOX_BIN:-$(command -v coder-sandbox || true)}"
if [ -z "${SANDBOX_BIN}" ]; then
	log "coder-sandbox binary not found, nothing to clean up"
	exit 0
fi

"${SANDBOX_BIN}" down "${SANDBOX_NAME}" >/dev/null 2>&1 || log "sandbox ${SANDBOX_NAME} already gone"

log "removed ${SANDBOX_NAME}"
