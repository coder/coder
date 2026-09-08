#!/usr/bin/env bash

# This script prints the desktop runtime version: a digest of every input that
# affects the built archive. Published archives live under this version, and
# runtime.lock pins it, so a change to any build input produces a new version
# that must be published and re-pinned before it is embedded.
#
# Usage: ./version.sh

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"

cdroot
cd scripts/desktopruntime

# Keep this list in sync with the paths in .github/workflows/desktop-runtime.yaml
# and the DESKTOP_RUNTIME_SRC_FILES definition in the Makefile.
inputs=(
	Dockerfile
	pam_stub.c
	build.sh
)

# Hash file names and contents together so renames change the version too.
for f in "${inputs[@]}"; do
	if [[ ! -f "$f" ]]; then
		error "desktop runtime input '$f' does not exist"
	fi
	printf '%s\0' "$f"
	cat "$f"
	printf '\0'
done | sha256sum | cut -c1-16
