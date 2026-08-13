#!/usr/bin/env bash
# Boots a coder/sandbox microVM holding the AI-bound agent.
#
# Supplied by the platform at exec time (see agent script ExtraEnv):
#   CODER_EGRESS_PROXY   host-side policy proxy, already listening
#   CODER_SANDBOX_ID     correlation ID for audit records
#
# Supplied by the template:
#   CODER_AI_SANDBOX_NAME   stable sandbox handle
#   CODER_AI_SANDBOX_IMAGE  guest image
#   CODER_AI_SANDBOX_ALLOW  admin-declared extra egress hosts
#   CODER_AI_AGENT_URL      coderd URL
#   CODER_AI_AGENT_TOKEN    the BOUND agent's token, from coder_agent.ai.token
#
# EGRESS OWNERSHIP. coder/sandbox confines the guest with its own egress
# lock: the guest opens exactly one TCP path, to the sandbox's host-side
# recording proxy, which applies the allowlist and writes requests.log. The
# guest therefore cannot reach CODER_EGRESS_PROXY, and HTTP_PROXY inside the
# guest is reserved by the sandbox for its own recorder. For this backend:
#
#   * enforcement and per-request recording live in coder/sandbox
#   * the platform owns identity, binding, credential starvation, and the
#     attestation
#   * the platform's own egress event stream stays EMPTY for this sandbox
#
# That is a real gap, not an oversight. See the README.
set -euo pipefail

log() { echo "[sandbox-up] $*"; }

SANDBOX_NAME="${CODER_AI_SANDBOX_NAME:?sandbox name is required}"
IMAGE="${CODER_AI_SANDBOX_IMAGE:-ubuntu:latest}"
EXTRA_ALLOW="${CODER_AI_SANDBOX_ALLOW:-}"
AGENT_URL="${CODER_AI_AGENT_URL:?agent URL is required}"
AGENT_TOKEN="${CODER_AI_AGENT_TOKEN:?bound agent token is required}"

SANDBOX_BIN="${CODER_SANDBOX_BIN:-$(command -v coder-sandbox || true)}"
if [ -z "${SANDBOX_BIN}" ]; then
	log "coder-sandbox not found: set CODER_SANDBOX_BIN or add it to PATH"
	exit 1
fi

if [ ! -e /dev/kvm ]; then
	log "/dev/kvm is absent: the microVM backend requires hardware virtualization"
	exit 1
fi

CODER_BIN="$(command -v coder || true)"
if [ -z "${CODER_BIN}" ]; then
	log "coder binary not found on PATH, cannot start the bound agent"
	exit 1
fi

# The bound agent must reach the control plane, so the coderd host is the
# one non-negotiable allowlist entry. Everything else is admin-declared.
CODERD_HOST="${AGENT_URL#*://}"
CODERD_HOST="${CODERD_HOST%%/*}"
CODERD_HOST="${CODERD_HOST%%:*}"
if [ -z "${CODERD_HOST}" ]; then
	log "could not derive the coderd host from ${AGENT_URL}"
	exit 1
fi

ALLOW="${CODERD_HOST}"
if [ -n "${EXTRA_ALLOW}" ]; then
	ALLOW="${ALLOW},${EXTRA_ALLOW}"
fi

# The platform proxy is running and reachable from the workspace, even though
# this backend's guest cannot use it. Recording it makes the difference
# between the two backends visible in the logs.
log "platform egress proxy: ${CODER_EGRESS_PROXY:-<none>} (unused by this backend)"
log "sandbox correlation id: ${CODER_SANDBOX_ID:-<none>}"
log "booting ${SANDBOX_NAME} (image ${IMAGE}, allow ${ALLOW})"

# The token is passed as an -e value, which makes it visible in the host
# process list. That is a demo simplification; a production script should
# write it to a file mounted read-only into the guest instead.
"${SANDBOX_BIN}" up "${SANDBOX_NAME}" \
	-image "${IMAGE}" \
	-allow "${ALLOW}" \
	-v "${CODER_BIN}:/opt/coder-agent:ro" \
	-e "CODER_AGENT_URL=${AGENT_URL}" \
	-e "CODER_AGENT_TOKEN=${AGENT_TOKEN}"

# Start the bound agent detached. The sandbox SSH exec runs a one-off
# command, so the agent is backgrounded with setsid to survive it.
log "starting the bound agent inside ${SANDBOX_NAME}"
"${SANDBOX_BIN}" ssh "${SANDBOX_NAME}" -- \
	"setsid sh -c 'exec /opt/coder-agent agent' </dev/null >/tmp/coder-agent.log 2>&1 &"

log "started ${SANDBOX_NAME}; egress enforced and recorded by coder/sandbox"
