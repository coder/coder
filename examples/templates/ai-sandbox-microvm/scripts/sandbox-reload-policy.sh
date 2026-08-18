#!/usr/bin/env bash
# Rebuilds the descriptor watched by coder/sandbox from the exported
# runtime.network document. The daemon polls this descriptor every 500ms and
# applies valid runtime policy changes atomically.
set -euo pipefail

: "${CODER_SANDBOX_ID:?CODER_SANDBOX_ID is required}"
: "${CODER_AI_SANDBOX_POLICY_FILE:?CODER_AI_SANDBOX_POLICY_FILE is required}"

SANDBOX_NAME="coder-${CODER_SANDBOX_ID}"
IMAGE="${CODER_AI_SANDBOX_IMAGE:-ubuntu:24.04}"
MEMORY_MIB="${CODER_AI_SANDBOX_MEMORY_MIB:-1024}"
DESCRIPTOR_DIR="${HOME}/.config/coder-sandbox/coder-ai"
DESCRIPTOR_PATH="${DESCRIPTOR_DIR}/${SANDBOX_NAME}.yaml"

log() { echo "[sandbox-reload-policy] $*"; }

yaml_quote() {
	printf "'%s'" "$(printf '%s' "$1" | sed "s/'/''/g")"
}

case "${MEMORY_MIB}" in
'' | *[!0-9]*)
	log "CODER_AI_SANDBOX_MEMORY_MIB must be a positive integer"
	exit 1
	;;
esac

if [ ! -s "${CODER_AI_SANDBOX_POLICY_FILE}" ]; then
	log "policy file is missing or empty: ${CODER_AI_SANDBOX_POLICY_FILE}"
	exit 1
fi

CODER_BIN="${CODER_BIN:-$(command -v coder || true)}"
if [ -z "${CODER_BIN}" ]; then
	log "static coder binary not found on PATH"
	exit 1
fi
CODER_BIN="$(readlink -f "${CODER_BIN}")"

mkdir -p "${DESCRIPTOR_DIR}"
temp_descriptor="$(mktemp "${DESCRIPTOR_DIR}/.${SANDBOX_NAME}.XXXXXX.tmp")"
trap 'rm -f "${temp_descriptor}"' EXIT

{
	cat <<EOF
version: 2
sandbox:
  image:
    reference: $(yaml_quote "${IMAGE}")
  resources:
    memory: $(yaml_quote "${MEMORY_MIB}MiB")
    tmpSize: 10GiB
  environment: {}
  secrets: []
  mounts:
    - type: bind
      source: $(yaml_quote "${CODER_BIN}")
      target: /opt/coder
      readOnly: true
      noExec: false
      noSuid: true
      noDev: true
  network:
    ingress:
      publish: []
    icmp:
      enabled: false
    dns:
      enabled: false
runtime:
  network:
EOF
	sed 's/^/    /' "${CODER_AI_SANDBOX_POLICY_FILE}"
	cat <<'EOF'
  mcp:
    default: ask
    servers: []
EOF
} >"${temp_descriptor}"

mv "${temp_descriptor}" "${DESCRIPTOR_PATH}"
trap - EXIT
log "rendered ${DESCRIPTOR_PATH}"
