#!/usr/bin/env bash

# This file checks all codersdk imports to be sure it doesn't import any packages
# that are being replaced in go.mod.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

deps=$(./scripts/list_dependencies.sh github.com/coder/coder/v2/codersdk)

set +e
replaces=$(grep "^replace" go.mod | awk '{print $2}')
conflicts=$(echo "$deps" | grep -xF -f <(echo "$replaces"))

if [ -n "${conflicts}" ]; then
	error "$(printf 'codersdk cannot import the following packages being replaced in go.mod:\n%s' "${conflicts}")"
fi
log "codersdk imports OK"
