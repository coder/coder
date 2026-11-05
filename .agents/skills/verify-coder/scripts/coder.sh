#!/usr/bin/env bash
# Runs the coder CLI that develop.sh built, logged in as the admin user of the
# recorded instance. Usage: coder.sh <coder args...>
set -euo pipefail
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
vc_load_run
exec env -u CODER_SESSION_TOKEN -u CODER_URL "$(vc_coder_bin)" --global-config "$VC_CONFIG_DIR" "$@"
