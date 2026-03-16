#!/usr/bin/env bash

# Usage: ./develop.sh [--agpl] [--port <port>] [-- extra server args...]
#
# This is a thin wrapper that delegates to the Go development orchestrator
# at scripts/develop. See that package for the full implementation.
#
# If the --agpl parameter is specified, builds only the AGPL-licensed code (no
# Coder enterprise features). The --port parameter changes the API port. The
# frontend dev server still listens on port 8080.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

GOBIN="$(pwd)/build/.bin" go install ./scripts/develop
exec ./build/.bin/develop "$@"
