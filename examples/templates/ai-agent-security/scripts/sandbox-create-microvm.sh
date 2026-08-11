#!/usr/bin/env bash
# Creates a coder/sandbox microVM holding an AI-bound child agent.
#
# This is the microVM backend of the sandbox script contract. The platform
# supplies the child identity, both tokens, and the audit session; this
# script builds the isolation boundary and starts the child agent inside.
#
#   CODER_AI_AGENT_URL     coderd URL for the child agent
#   CODER_AI_AGENT_TOKEN   the bound child agent token, minted server side
#   CODER_AI_SESSION_TOKEN scoped AI session token for CLI use inside
#   CODER_EGRESS_PROXY     parent-side proxy as bare host:port
#   CODER_SANDBOX_ID       lifecycle correlation ID
#
# EGRESS OWNERSHIP DIFFERS FROM THE DOCKER BACKEND. coder/sandbox confines
# the guest with its own egress lock: the guest can open exactly one TCP
# path, to the sandbox's host-side recording proxy, which then applies its
# own allowlist and writes requests.log. The guest therefore cannot reach
# CODER_EGRESS_PROXY at all, and HTTP_PROXY inside the guest is reserved by
# the sandbox for its own recorder. Chaining the two proxies is not
# possible today, so for this backend:
#
#   * enforcement and per-request recording live in coder/sandbox
#   * the platform still owns identity, binding, credential starvation,
#     the session record, and the attestation
#   * the platform's egress event stream stays EMPTY for this sandbox
#
# That last point is a real gap, not an oversight: see the README.
#
# The allowlist below is deliberately minimal. The child agent must reach
# coderd, and nothing else is permitted unless the template declares it in
# CODER_AI_SANDBOX_ALLOW. Widening it is an admin action, exactly like the
# template egress policy.
set -euo pipefail

SANDBOX_NAME="coder-${CODER_SANDBOX_ID}"
IMAGE="${CODER_AI_SANDBOX_IMAGE:-ubuntu:latest}"
MEMORY_MIB="${CODER_AI_SANDBOX_MEMORY_MIB:-1024}"
EXTRA_ALLOW="${CODER_AI_SANDBOX_ALLOW:-}"

log() { echo "[sandbox-create-microvm] $*"; }

SANDBOX_BIN="${CODER_SANDBOX_BIN:-$(command -v coder-sandbox || true)}"
if [ -z "${SANDBOX_BIN}" ]; then
	log "coder-sandbox binary not found: set CODER_SANDBOX_BIN or add it to PATH"
	exit 1
fi

if [ ! -e /dev/kvm ]; then
	log "/dev/kvm is not present: the microVM backend requires hardware virtualization"
	exit 1
fi

CODER_BIN="$(command -v coder || true)"
if [ -z "${CODER_BIN}" ]; then
	log "coder binary not found on PATH, cannot start the child agent"
	exit 1
fi

# The child agent has to reach the control plane, so the coderd host is the
# one non-negotiable allowlist entry. Everything else is admin-declared.
CODERD_HOST="${CODER_AI_AGENT_URL#*://}"
CODERD_HOST="${CODERD_HOST%%/*}"
CODERD_HOST="${CODERD_HOST%%:*}"
if [ -z "${CODERD_HOST}" ]; then
	log "could not derive the coderd host from CODER_AI_AGENT_URL"
	exit 1
fi

ALLOW="${CODERD_HOST}"
if [ -n "${EXTRA_ALLOW}" ]; then
	ALLOW="${ALLOW},${EXTRA_ALLOW}"
fi

# Reconcile: the controller re-runs this script with the same sandbox name
# after a parent agent restart. Tear down any previous instance so the boot
# below is deterministic. The platform has already rotated the tokens.
if "${SANDBOX_BIN}" ls 2>/dev/null | grep -q "^${SANDBOX_NAME} "; then
	log "removing previous instance of ${SANDBOX_NAME}"
	"${SANDBOX_BIN}" down "${SANDBOX_NAME}" >/dev/null 2>&1 || true
fi

# Boot the microVM. The coder binary is bind mounted read-only rather than
# downloaded, so the guest needs no egress before its policy is in force.
#
# The tokens are passed as -e values, which makes them visible in the host
# process list. That is a demo simplification; a production script should
# write them to a file mounted read-only into the guest instead.
log "booting ${SANDBOX_NAME} (image ${IMAGE}, allow ${ALLOW})"
"${SANDBOX_BIN}" up "${SANDBOX_NAME}" \
	-image "${IMAGE}" \
	-allow "${ALLOW}" \
	-memory "${MEMORY_MIB}" \
	-v "${CODER_BIN}:/opt/coder-agent:ro" \
	-e "CODER_AGENT_URL=${CODER_AI_AGENT_URL}" \
	-e "CODER_AGENT_TOKEN=${CODER_AI_AGENT_TOKEN}" \
	-e "CODER_SESSION_TOKEN=${CODER_AI_SESSION_TOKEN}"

# Start the child agent detached. The sandbox SSH exec runs a one-off
# command, so the agent is backgrounded with setsid to survive it.
log "starting the child agent inside ${SANDBOX_NAME}"
"${SANDBOX_BIN}" ssh "${SANDBOX_NAME}" -- \
	"setsid sh -c 'exec /opt/coder-agent agent' </dev/null >/tmp/coder-agent.log 2>&1 &"

log "started ${SANDBOX_NAME}; egress recorded by coder/sandbox, not the platform proxy"
