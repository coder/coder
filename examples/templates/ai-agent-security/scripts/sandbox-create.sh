#!/usr/bin/env bash
# Creates a Docker sandbox holding an AI-bound child agent.
#
# The platform supplies every value this script needs and owns everything
# security relevant: the child agent row, its binding to an AI identity,
# both tokens, the egress proxy, and the audit stream. This script only
# builds the isolation boundary and starts the child agent inside it.
#
#   CODER_AI_AGENT_URL     coderd URL for the child agent
#   CODER_AI_AGENT_TOKEN   the bound child agent token, minted server side
#   CODER_AI_SESSION_TOKEN scoped AI session token for CLI use inside
#   CODER_EGRESS_PROXY     parent-side proxy as bare host:port
#   CODER_SANDBOX_ID       lifecycle correlation ID
#
# CODER_AI_SANDBOX_EGRESS_ENFORCEMENT declares what this script claims about
# routing. The claim is honored here:
#   forced   an internal Docker network with no route out, so the proxy is
#            the only path that can carry traffic
#   advisory a normal bridge network plus proxy variables, which a process
#            can simply ignore
#   none     no routing claim at all
set -euo pipefail

SANDBOX_NAME="sb-${CODER_SANDBOX_ID}"
NETWORK_NAME="sbnet-${CODER_SANDBOX_ID}"
ENFORCEMENT="${CODER_AI_SANDBOX_EGRESS_ENFORCEMENT:-none}"
IMAGE="${CODER_AI_SANDBOX_IMAGE:-codercom/example-base:ubuntu}"

log() { echo "[sandbox-create] $*"; }

if ! command -v docker >/dev/null 2>&1; then
	log "docker CLI not found in the workspace image"
	exit 1
fi

# The child agent needs the coder binary. Copying the parent's binary keeps
# the sandbox from needing egress before its policy is even in force.
CODER_BIN="$(command -v coder || true)"
if [ -z "${CODER_BIN}" ]; then
	log "coder binary not found on PATH"
	exit 1
fi

# The proxy binds all interfaces on the workspace container, so the sandbox
# reaches it at the Docker bridge gateway rather than at loopback.
PROXY_PORT="${CODER_EGRESS_PROXY##*:}"
GATEWAY_IP="$(ip route show default 2>/dev/null | awk '/default/ {print $3; exit}')"
if [ -z "${GATEWAY_IP}" ]; then
	log "could not determine the bridge gateway address"
	exit 1
fi
SANDBOX_PROXY="${GATEWAY_IP}:${PROXY_PORT}"

# Clean up a container left behind by a previous run of this sandbox.
docker rm -f "${SANDBOX_NAME}" >/dev/null 2>&1 || true
docker network rm "${NETWORK_NAME}" >/dev/null 2>&1 || true

NETWORK_ARGS=()
case "${ENFORCEMENT}" in
forced)
	# An internal network has no route off the host, so nothing inside can
	# reach the internet except through the parent-side proxy. This is what
	# the forced attestation is claiming.
	docker network create --internal "${NETWORK_NAME}" >/dev/null
	NETWORK_ARGS=(--network "${NETWORK_NAME}")
	log "created internal network ${NETWORK_NAME} (forced)"
	;;
advisory | none)
	log "using the default bridge network (${ENFORCEMENT})"
	;;
*)
	log "unknown enforcement ${ENFORCEMENT}"
	exit 1
	;;
esac

# Create, then copy the binary in, then start: the child cannot run before
# it has both the binary and its platform-issued credentials.
docker create \
	--name "${SANDBOX_NAME}" \
	"${NETWORK_ARGS[@]}" \
	--add-host "host.docker.internal:${GATEWAY_IP}" \
	-e CODER_AGENT_URL="${CODER_AI_AGENT_URL}" \
	-e CODER_AGENT_TOKEN="${CODER_AI_AGENT_TOKEN}" \
	-e CODER_SESSION_TOKEN="${CODER_AI_SESSION_TOKEN}" \
	-e HTTP_PROXY="http://${SANDBOX_PROXY}" \
	-e HTTPS_PROXY="http://${SANDBOX_PROXY}" \
	-e http_proxy="http://${SANDBOX_PROXY}" \
	-e https_proxy="http://${SANDBOX_PROXY}" \
	-e NO_PROXY="localhost,127.0.0.1,::1" \
	--entrypoint /bin/sh \
	"${IMAGE}" \
	-c 'exec /opt/coder-agent agent' >/dev/null

docker cp "${CODER_BIN}" "${SANDBOX_NAME}:/opt/coder-agent" >/dev/null
docker start "${SANDBOX_NAME}" >/dev/null

log "started ${SANDBOX_NAME} with egress via ${SANDBOX_PROXY} (${ENFORCEMENT})"
