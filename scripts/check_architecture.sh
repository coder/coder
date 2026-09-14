#!/usr/bin/env bash
# Umbrella architecture-boundary check.
#
# Delegates to existing import-boundary scripts. New architecture rules can be
# added here as needed.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "--- check architecture (import boundaries)"

"$SCRIPT_DIR/check_enterprise_imports.sh"
"$SCRIPT_DIR/check_codersdk_imports.sh"

echo "OK: architecture checks passed."
