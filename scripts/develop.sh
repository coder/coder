#!/usr/bin/env bash

# Usage: ./develop.sh [flags...] [-- extra server args...]
#
# This is a thin wrapper that delegates to the Go development orchestrator
# at scripts/develop. See that package for the full implementation.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

make -j MAKE_TIMED=1 build/.bin/develop

# Prototype only (do not merge): turn on the token-budget experiment for
# develop.sh runs, including the eph-pr preview, unless the caller sets one.
export CODER_EXPERIMENTS="${CODER_EXPERIMENTS:-chat-token-budget}"
exec build/.bin/develop "$@"
