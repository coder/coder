#!/usr/bin/env bash
# Runs the chat actor dogfood harness end to end: dev server, MCP server,
# botemu setup, botemu tokens, botemu run. See README.md.
#
# Usage: ./scripts/chatactor-dogfood/run.sh [botemu run flags]
#   -only S3,S7      run only the listed scenarios
#   -s9              also run the optional S9 scenario
#   -new-chat=false  reuse the chat_id (and S9B ids) stored in state.json
#
# Environment:
#   OPENAI_API_KEY        required; never printed
#   OPENAI_BASE_URL       OpenAI-compatible base URL; defaults to the Coder AI Gateway
#   CODER_DEV_SKIP_START  set to 1 when a dev server is already running on 127.0.0.1:3000
#   CODER_DEV_READY_TIMEOUT  seconds to wait for the dev server (default 900)
set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/../lib.sh"
cdroot

if [[ ${1:-} == "-h" || ${1:-} == "--help" ]]; then
	sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
	exit 0
fi

HARNESS_DIR="scripts/chatactor-dogfood"
LOG_DIR="${HARNESS_DIR}/logs"
BIN_DIR="build/chatactor-dogfood"
ACCESS_URL="http://127.0.0.1:3000"
MCP_HEALTH_URL="http://127.0.0.1:3999/healthz"
READY_TIMEOUT="${CODER_DEV_READY_TIMEOUT:-900}"
export OPENAI_BASE_URL="${OPENAI_BASE_URL:-https://dogfood.cdr.dev/api/v2/ai-gateway/openai/v1}"

dependencies docker jq curl go
requiredenvs OPENAI_API_KEY

mkdir -p "${LOG_DIR}" "${BIN_DIR}"

DEVELOP_PID=""
MCP_PID=""
cleanup() {
	local rc=$?
	if [[ -n ${MCP_PID} ]] && kill -0 "${MCP_PID}" 2>/dev/null; then
		log "Stopping MCP server (pid ${MCP_PID})"
		kill -TERM "${MCP_PID}" 2>/dev/null || true
		wait "${MCP_PID}" 2>/dev/null || true
	fi
	if [[ -n ${DEVELOP_PID} ]] && kill -0 "${DEVELOP_PID}" 2>/dev/null; then
		log "Stopping dev server (pid ${DEVELOP_PID})"
		kill -INT "${DEVELOP_PID}" 2>/dev/null || true
		wait "${DEVELOP_PID}" 2>/dev/null || true
	fi
	exit "${rc}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

healthy() {
	curl -fsS -o /dev/null --max-time 2 "$1"
}

if [[ ${CODER_DEV_SKIP_START:-0} == 1 ]]; then
	healthy "${ACCESS_URL}/healthz" || error "CODER_DEV_SKIP_START=1 but ${ACCESS_URL}/healthz is not reachable"
	log "Using the dev server already running at ${ACCESS_URL}"
	if [[ ! -f "${LOG_DIR}/develop.log" ]]; then
		log "WARNING: ${LOG_DIR}/develop.log is missing; S5 and S8 server log checks will fail"
	fi
else
	log "Starting dev server; output in ${LOG_DIR}/develop.log"
	CODER_DEV_STARTER_TEMPLATE=docker ./scripts/develop.sh -- \
		--experiments=oauth2 \
		--mcp-allowed-private-cidrs=127.0.0.0/8 \
		--verbose >"${LOG_DIR}/develop.log" 2>&1 &
	DEVELOP_PID=$!

	deadline=$((SECONDS + READY_TIMEOUT))
	until grep -q "Coder is now running in development mode" "${LOG_DIR}/develop.log" 2>/dev/null &&
		healthy "${ACCESS_URL}/healthz"; do
		if ! kill -0 "${DEVELOP_PID}" 2>/dev/null; then
			tail -n 40 "${LOG_DIR}/develop.log" >&2
			error "dev server exited before it became ready; see ${LOG_DIR}/develop.log"
		fi
		if ((SECONDS >= deadline)); then
			error "dev server not ready after ${READY_TIMEOUT}s; see ${LOG_DIR}/develop.log"
		fi
		sleep 3
	done
	log "Dev server is ready"
fi

log "Building botemu"
go build -o "${BIN_DIR}/botemu" "./${HARNESS_DIR}/botemu"

log "botemu setup"
"${BIN_DIR}/botemu" setup 2>&1 | tee "${LOG_DIR}/setup.log"

log "Starting MCP server; output in ${LOG_DIR}/mcpserver.log"
"${HARNESS_DIR}/run_mcpserver.sh" >"${LOG_DIR}/mcpserver.log" 2>&1 &
MCP_PID=$!
for _ in $(seq 1 60); do
	if healthy "${MCP_HEALTH_URL}"; then
		break
	fi
	if ! kill -0 "${MCP_PID}" 2>/dev/null; then
		cat "${LOG_DIR}/mcpserver.log" >&2
		error "MCP server exited; see ${LOG_DIR}/mcpserver.log"
	fi
	sleep 1
done
healthy "${MCP_HEALTH_URL}" || error "MCP server not reachable at ${MCP_HEALTH_URL}"

log "botemu tokens"
"${BIN_DIR}/botemu" tokens 2>&1 | tee "${LOG_DIR}/tokens.log"

log "botemu run $*"
"${BIN_DIR}/botemu" run "$@" 2>&1 | tee "${LOG_DIR}/run.log"

log "Results written to ${HARNESS_DIR}/results.md"
