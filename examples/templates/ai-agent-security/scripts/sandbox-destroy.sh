#!/usr/bin/env bash
# Tears down the Docker sandbox created by sandbox-create.sh.
#
# The platform has already revoked the child agent token and the scoped AI
# session token by the time this runs, so a surviving process cannot
# authenticate. Removing the container and network is still required to
# release local resources.
#
# CODER_AI_SESSION_TOKEN is deliberately absent here: it may have been
# rotated, and teardown must not depend on a credential.
set -euo pipefail

SANDBOX_NAME="sb-${CODER_SANDBOX_ID}"
NETWORK_NAME="sbnet-${CODER_SANDBOX_ID}"

log() { echo "[sandbox-destroy] $*"; }

if ! command -v docker >/dev/null 2>&1; then
	log "docker CLI not found, nothing to clean up"
	exit 0
fi

docker rm -f "${SANDBOX_NAME}" >/dev/null 2>&1 || log "container ${SANDBOX_NAME} already gone"
docker network rm "${NETWORK_NAME}" >/dev/null 2>&1 || true

log "removed ${SANDBOX_NAME}"
