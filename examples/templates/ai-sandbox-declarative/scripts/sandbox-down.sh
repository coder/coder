#!/usr/bin/env bash
# Reclaims the sandbox container and its network on workspace stop.
#
# Credentials are already inert by the time this runs: the server revokes
# this build's keys on the stop transition, and the bound agent's token dies
# with its build. A failure here leaks a container, never a credential.
set -euo pipefail

log() { echo "[sandbox-down] $*"; }

SANDBOX_ID="${CODER_SANDBOX_ID:-}"
WORKSPACE_CONTAINER="${CODER_AI_SANDBOX_WORKSPACE:-}"

if [ -z "${SANDBOX_ID}" ]; then
	log "no sandbox ID supplied, nothing to reclaim"
	exit 0
fi
if ! command -v docker >/dev/null 2>&1; then
	log "docker CLI not found, nothing to reclaim"
	exit 0
fi

SANDBOX_NAME="sb-${SANDBOX_ID}"
NETWORK_NAME="sbnet-${SANDBOX_ID}"

docker rm -f "${SANDBOX_NAME}" >/dev/null 2>&1 || log "container already gone"

# Detach the workspace before removing the network, or removal fails while
# an endpoint remains attached.
if [ -n "${WORKSPACE_CONTAINER}" ]; then
	docker network disconnect -f "${NETWORK_NAME}" "${WORKSPACE_CONTAINER}" >/dev/null 2>&1 || true
fi
docker network rm "${NETWORK_NAME}" >/dev/null 2>&1 || log "network already gone"

log "reclaimed ${SANDBOX_NAME}"
