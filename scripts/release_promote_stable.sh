#!/usr/bin/env bash

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# This script is a convenience wrapper around the release promote command.
#
# Sed hack to make help text look like this script.
exec go run "${SCRIPT_DIR}/release" promote "$@"
