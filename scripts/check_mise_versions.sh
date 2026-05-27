#!/usr/bin/env bash

# This script ensures that the pinned mise binary version and Linux checksum
# stay aligned across local development, dogfood images, and CI.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

check_value() {
	local label="$1"
	local actual="$2"
	local expected="$3"

	log "INFO : ${label}: ${actual}"
	if [[ -z "${actual}" ]]; then
		error "Missing mise value for ${label}"
	fi
	if [[ "${actual}" != "${expected}" ]]; then
		error "Mise mismatch for ${label}: expected ${expected}, got ${actual}"
	fi
}

mise_version="$(sed -n 's/^min_version = "\([^"]*\)"/\1/p' mise.toml)"
check_value "mise.toml min_version" "${mise_version}" "${mise_version}"

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
check_value ".github/actions/setup-mise/action.yml" "${action_version}" "${mise_version}"

checksum_version="$(
	awk -v version="${mise_version}" '
		$0 == "[\"" version "\"]" {
			print version
			exit
		}
	' .github/actions/setup-mise/checksums.toml
)"
check_value ".github/actions/setup-mise/checksums.toml" "${checksum_version}" "${mise_version}"

linux_x64_checksum="$(
	awk -F= -v version="${mise_version}" '
		$0 == "[\"" version "\"]" { in_table = 1; next }
		/^\[/ { in_table = 0 }
		in_table {
			key = $1
			gsub(/^[[:space:]]+|[[:space:]]+$/, "", key)
			if (key == "linux-x64") {
				value = $2
				gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
				gsub(/^"|"$/, "", value)
				print value
				exit
			}
		}
	' .github/actions/setup-mise/checksums.toml
)"
check_value ".github/actions/setup-mise/checksums.toml linux-x64" "${linux_x64_checksum}" "${linux_x64_checksum}"

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
check_value "flake.nix" "${flake_version}" "${mise_version}"

wrapper_version="$(sed -n 's/^MISE_VERSION="v\([^"]*\)"/\1/p' scripts/dogfood/mise-oci-wrapper.sh)"
check_value "scripts/dogfood/mise-oci-wrapper.sh" "${wrapper_version}" "${mise_version}"
wrapper_checksum="$(sed -n 's/^MISE_SHA256="\([a-f0-9]*\)"/\1/p' scripts/dogfood/mise-oci-wrapper.sh)"
check_value "scripts/dogfood/mise-oci-wrapper.sh sha256" "${wrapper_checksum}" "${linux_x64_checksum}"

for dockerfile in dogfood/coder/ubuntu-*/Dockerfile.base; do
	dockerfile_version="$(sed -n 's/.*MISE_VERSION=v\([0-9.]*\).*/\1/p' "${dockerfile}" | head -n 1)"
	check_value "${dockerfile}" "${dockerfile_version}" "${mise_version}"

	dockerfile_checksum="$(sed -n 's/.*MISE_SHA256=\([a-f0-9]*\).*/\1/p' "${dockerfile}" | head -n 1)"
	check_value "${dockerfile} sha256" "${dockerfile_checksum}" "${linux_x64_checksum}"
done

log "Mise version check passed, all versions are ${mise_version}"
