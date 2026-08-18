#!/usr/bin/env bash
# Boots a coder/sandbox microVM and starts the server-created child agent.
set -euo pipefail

: "${CODER_SANDBOX_ID:?CODER_SANDBOX_ID is required}"
: "${CODER_AI_AGENT_URL:?CODER_AI_AGENT_URL is required}"
: "${CODER_AI_AGENT_TOKEN:?CODER_AI_AGENT_TOKEN is required}"
: "${CODER_AI_SANDBOX_POLICY_FILE:?CODER_AI_SANDBOX_POLICY_FILE is required}"
: "${CODER_AI_SANDBOX_POLICY_RELOAD_SCRIPT:?CODER_AI_SANDBOX_POLICY_RELOAD_SCRIPT is required}"

SANDBOX_NAME="coder-${CODER_SANDBOX_ID}"
DESCRIPTOR_PATH="${HOME}/.config/coder-sandbox/coder-ai/${SANDBOX_NAME}.yaml"

log() { echo "[sandbox-create] $*"; }

shell_quote() {
	printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

SANDBOX_BIN="${CODER_SANDBOX_BIN:-$(command -v coder-sandbox || true)}"
if [ -z "${SANDBOX_BIN}" ]; then
	log "coder-sandbox not found on PATH; bake it into the workspace image"
	exit 1
fi

if [ ! -e /dev/kvm ]; then
	log "/dev/kvm is not present"
	exit 1
fi
if [ ! -r /dev/kvm ] || [ ! -w /dev/kvm ]; then
	log "/dev/kvm is not readable and writable; verify the kvm_gid parameter"
	exit 1
fi

# The daemon does not resume VMs after it restarts. Always discard stale state
# before booting the server-reconciled sandbox with its current child token.
log "removing stale ${SANDBOX_NAME} state, if present"
"${SANDBOX_BIN}" down "${SANDBOX_NAME}" >/dev/null 2>&1 || true

# The controller has already invoked this hook for the initial policy. Render it
# again after stale-state cleanup so this script is also safe to run manually.
"${CODER_AI_SANDBOX_POLICY_RELOAD_SCRIPT}"

log "booting ${SANDBOX_NAME}"
"${SANDBOX_BIN}" up "${SANDBOX_NAME}" -f "${DESCRIPTOR_PATH}"

agent_command="CODER_AGENT_URL=$(shell_quote "${CODER_AI_AGENT_URL}") CODER_AGENT_TOKEN=$(shell_quote "${CODER_AI_AGENT_TOKEN}")"
if [ -n "${CODER_AI_SESSION_TOKEN:-}" ]; then
	agent_command="${agent_command} CODER_SESSION_TOKEN=$(shell_quote "${CODER_AI_SESSION_TOKEN}")"
fi
agent_command="${agent_command} exec /opt/coder agent"
agent_command_quoted="$(shell_quote "${agent_command}")"

# coder-sandbox ssh is a one-off exec. setsid and shell redirection detach the
# long-running child agent from that SSH session.
log "starting the child agent inside ${SANDBOX_NAME}"
"${SANDBOX_BIN}" ssh "${SANDBOX_NAME}" -- \
	"setsid sh -c ${agent_command_quoted} </dev/null >/tmp/coder-agent.log 2>&1 &"

log "started ${SANDBOX_NAME}"
