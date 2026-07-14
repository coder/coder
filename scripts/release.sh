#!/usr/bin/env bash

set -euo pipefail

# Thin wrapper that launches the interactive release wizard.
# Usage: ./scripts/release.sh [flags]
#
# Flags are passed directly to the Go program. The wizard is the legacy
# (v1) release tool; the non-interactive tooling is available directly
# via `go run ./scripts/releaser <rc|branch|release>`.
# Run ./scripts/release.sh --help for details.

cd "$(dirname "${BASH_SOURCE[0]}")/.."
exec go run ./scripts/releaser --legacy "$@"
