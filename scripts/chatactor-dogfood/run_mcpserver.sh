#!/usr/bin/env bash
# Builds and starts the dogfood MCP server with per-user token labels read
# from state.json. Token values are passed through the environment only;
# nothing is echoed. Extra arguments are passed to the server.
set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/../lib.sh"
cdroot

dependencies jq

HARNESS_DIR="scripts/chatactor-dogfood"
BIN_DIR="build/chatactor-dogfood"
mkdir -p "${HARNESS_DIR}/logs" "${BIN_DIR}"

TOKEN_LABELS="$(jq -r '[.users | to_entries[] | select(.value.mcp_token != null and .value.mcp_token != "") | "\(.value.mcp_token)=\(.key)"] | join(",")' "${HARNESS_DIR}/state.json")"
export TOKEN_LABELS

go build -o "${BIN_DIR}/mcpserver" "./${HARNESS_DIR}/mcpserver"
exec "${BIN_DIR}/mcpserver" -addr 127.0.0.1:3999 -log "${HARNESS_DIR}/logs/whoami_calls.jsonl" "$@"
