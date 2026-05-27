#!/usr/bin/env bash

# This script checks the mise values used by CI and dogfood images:
# - mise.toml min_version is the source of truth for the mise version.
# - .github/actions/setup-mise/checksums.toml stores pinned binary checksums.
# - .github/actions/setup-mise/action.yml
# - flake.nix
# - scripts/dogfood/mise-oci-wrapper.sh
# - dogfood/coder/ubuntu-*/Dockerfile.base

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

check_not_empty() {
	local label="$1"
	local value="$2"

	log "INFO : ${label}: ${value}"
	if [[ -z "${value}" ]]; then
		error "Missing mise value for ${label}"
	fi
}

check_equal() {
	local label="$1"
	local actual="$2"
	local expected="$3"

	check_not_empty "${label}" "${actual}"
	if [[ "${actual}" != "${expected}" ]]; then
		error "Mise mismatch for ${label}: expected ${expected}, got ${actual}"
	fi
}

mise_version="$(sed -n 's/^min_version = "\([^"]*\)"/\1/p' mise.toml)"
check_not_empty "mise.toml min_version" "${mise_version}"

action_version="$(
	awk '
		$1 == "mise-version:" { in_input = 1; next }
		in_input && /^  [A-Za-z0-9_-]+:/ { exit }
		in_input && $1 == "default:" {
			gsub(/"/, "", $2)
			print $2
			exit
		}
	' .github/actions/setup-mise/action.yml
)"
check_equal ".github/actions/setup-mise/action.yml" "${action_version}" "${mise_version}"

checksum_version="$(
	awk -v version="${mise_version}" '
		$0 == "[\"" version "\"]" {
			print version
			exit
		}
	' .github/actions/setup-mise/checksums.toml
)"
check_equal ".github/actions/setup-mise/checksums.toml" "${checksum_version}" "${mise_version}"

linux_x64_checksum="$(./scripts/mise_checksum.sh .github/actions/setup-mise/checksums.toml "${mise_version}" linux-x64)"
check_not_empty ".github/actions/setup-mise/checksums.toml linux-x64" "${linux_x64_checksum}"

flake_version="$(
	awk '
		/^[[:space:]]*mise = / { in_mise = 1; next }
		in_mise && /^[[:space:]]*version = / {
			gsub(/[";]/, "", $3)
			print $3
			exit
		}
		in_mise && /^[[:space:]]*};/ { exit }
	' flake.nix
)"
check_equal "flake.nix" "${flake_version}" "${mise_version}"

wrapper_version="$(sed -n 's/^MISE_VERSION="v\([^"]*\)"/\1/p' scripts/dogfood/mise-oci-wrapper.sh)"
check_equal "scripts/dogfood/mise-oci-wrapper.sh" "${wrapper_version}" "${mise_version}"
wrapper_checksum="$(sed -n 's/^MISE_SHA256="\([a-f0-9]*\)"/\1/p' scripts/dogfood/mise-oci-wrapper.sh)"
check_equal "scripts/dogfood/mise-oci-wrapper.sh sha256" "${wrapper_checksum}" "${linux_x64_checksum}"

for dockerfile in dogfood/coder/ubuntu-*/Dockerfile.base; do
	dockerfile_version="$(sed -n 's/.*MISE_VERSION=v\([0-9.]*\).*/\1/p' "${dockerfile}" | head -n 1)"
	check_equal "${dockerfile}" "${dockerfile_version}" "${mise_version}"

	dockerfile_checksum="$(sed -n 's/.*MISE_SHA256=\([a-f0-9]*\).*/\1/p' "${dockerfile}" | head -n 1)"
	check_equal "${dockerfile} sha256" "${dockerfile_checksum}" "${linux_x64_checksum}"
done

log "Mise version check passed, all versions are ${mise_version}"
