#!/usr/bin/env bash
# Builds a container-beside-container sandbox holding the AI-bound agent.
#
# Supplied by the platform at exec time, after the egress proxy is listening:
#   CODER_EGRESS_PROXY   host:port of the workspace-side policy proxy
#   CODER_SANDBOX_ID     correlation ID for audit records
#
# Supplied by the template:
#   CODER_AI_SANDBOX_IMAGE      image for the sandbox container
#   CODER_AI_SANDBOX_WORKSPACE  this workspace's container name
#   CODER_AI_AGENT_URL          coderd URL
#   CODER_AI_AGENT_TOKEN        the BOUND agent's token
#
# TOPOLOGY. The sandbox is a sibling container, not a nested one: the Docker
# CLI runs here but the daemon is the host's. The sandbox joins an
# --internal network, which has no route off the host, and the workspace
# joins that same network under a fixed alias. The result is that the
# workspace-side proxy is the only reachable path out of the sandbox:
#
#   [sandbox] --internal net--> [workspace proxy] --bridge--> internet
#
# That is what makes egress_enforcement = "forced" an honest claim for this
# backend. It is structural: a process in the sandbox that ignores the proxy
# environment variables still cannot route anywhere.
set -euo pipefail

log() { echo "[sandbox-up] $*"; }

SANDBOX_ID="${CODER_SANDBOX_ID:?platform must supply a sandbox ID}"
PROXY="${CODER_EGRESS_PROXY:?platform must supply the egress proxy address}"
IMAGE="${CODER_AI_SANDBOX_IMAGE:-codercom/example-base:ubuntu}"
WORKSPACE_CONTAINER="${CODER_AI_SANDBOX_WORKSPACE:?template must supply the workspace container name}"
AGENT_URL="${CODER_AI_AGENT_URL:?template must supply the coderd URL}"
AGENT_TOKEN="${CODER_AI_AGENT_TOKEN:?template must supply the bound agent token}"

SANDBOX_NAME="sb-${SANDBOX_ID}"
NETWORK_NAME="sbnet-${SANDBOX_ID}"
# A fixed alias means the sandbox's proxy address never depends on how the
# workspace container happens to be named or addressed.
PROXY_ALIAS="coder-egress-proxy"
PROXY_PORT="${PROXY##*:}"

if ! command -v docker >/dev/null 2>&1; then
	log "docker CLI not found in the workspace image"
	exit 1
fi

# The sandbox needs the agent binary. Copying this workspace's binary avoids
# requiring egress before the sandbox's own policy is in force.
CODER_BIN="$(command -v coder || true)"
if [ -z "${CODER_BIN}" ]; then
	log "coder binary not found on PATH"
	exit 1
fi

# Clean up anything a previous build left behind.
docker rm -f "${SANDBOX_NAME}" >/dev/null 2>&1 || true
docker network rm "${NETWORK_NAME}" >/dev/null 2>&1 || true

# --internal is the enforcement: no route off the host for anything on it.
docker network create --internal "${NETWORK_NAME}" >/dev/null
log "created internal network ${NETWORK_NAME}"

# Put the workspace on that network too, so the sandbox can reach the proxy
# that runs here. Without this the sandbox is fully isolated and cannot even
# reach coderd.
docker network connect --alias "${PROXY_ALIAS}" \
	"${NETWORK_NAME}" "${WORKSPACE_CONTAINER}" >/dev/null 2>&1 ||
	log "workspace already attached to ${NETWORK_NAME}"

SANDBOX_PROXY="http://${PROXY_ALIAS}:${PROXY_PORT}"
log "sandbox egress will route through ${SANDBOX_PROXY}"

# Create, copy the binary in, then start: the agent cannot run before it has
# both its binary and its platform-issued credentials.
docker create \
	--name "${SANDBOX_NAME}" \
	--network "${NETWORK_NAME}" \
	--label "coder.sandbox_id=${SANDBOX_ID}" \
	-e CODER_AGENT_URL="${AGENT_URL}" \
	-e CODER_AGENT_TOKEN="${AGENT_TOKEN}" \
	-e HTTP_PROXY="${SANDBOX_PROXY}" \
	-e HTTPS_PROXY="${SANDBOX_PROXY}" \
	-e http_proxy="${SANDBOX_PROXY}" \
	-e https_proxy="${SANDBOX_PROXY}" \
	-e NO_PROXY="localhost,127.0.0.1,::1" \
	--entrypoint /bin/sh \
	"${IMAGE}" \
	-c 'exec /opt/coder-agent agent' >/dev/null

docker cp "${CODER_BIN}" "${SANDBOX_NAME}:/opt/coder-agent" >/dev/null
docker start "${SANDBOX_NAME}" >/dev/null

log "started ${SANDBOX_NAME}: egress forced through the platform proxy"
