#!/usr/bin/env bash

# Usage: ./scripts/develop-local-cluster.sh <command> [flags...]
#
# This is a thin wrapper for the local kind development orchestrator.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

make -j MAKE_TIMED=1 build/.bin/develop-local-cluster
exec build/.bin/develop-local-cluster "$@"
