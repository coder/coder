#!/usr/bin/env bash

set -euo pipefail

# Thin wrapper that launches the interactive release wizard.
# Usage: ./scripts/release.sh [flags]
#
# Flags are passed directly to the Go program.
# Run ./scripts/release.sh --help for details.

cd "$(dirname "${BASH_SOURCE[0]}")/.."
exec go run ./scripts/releaser "$@"
