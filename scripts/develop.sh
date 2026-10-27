#!/usr/bin/env bash

# Usage: ./develop.sh [flags...] [-- extra server args...]
#
# This is a thin wrapper that delegates to the Go development orchestrator
# at scripts/develop. See that package for the full implementation.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# Prototype branch only: the coder-ephemeral workspace passes no server flags,
# so turn on MCP Apps here and let coderd reach the demo MCP server on loopback.
export CODER_EXPERIMENTS=${CODER_EXPERIMENTS:-mcp-apps}
export CODER_MCP_ALLOWED_PRIVATE_CIDRS=${CODER_MCP_ALLOWED_PRIVATE_CIDRS:-127.0.0.1/32}

make -j MAKE_TIMED=1 build/.bin/develop
# Prototype branch only: run the demo MCP server next to coderd. It shares
# this session, so it stops when develop.sh stops. When the port is taken
# (another copy runs), it exits and develop.sh carries on.
go build -o build/.bin/mcpappdemo ./scripts/mcpappdemo
build/.bin/mcpappdemo --addr 127.0.0.1:8811 &
exec build/.bin/develop "$@"
