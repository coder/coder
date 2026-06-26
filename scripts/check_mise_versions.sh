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

check_sha256_format() {
	local label="$1"
	local value="$2"

	if [[ -z "${value}" ]]; then
		error "Missing mise value for ${label}"
	fi
	if [[ ! "${value}" =~ ^[a-f0-9]{64}$ ]]; then
		error "Expected 64-character lowercase SHA256 for ${label}: ${value}"
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

declare -A setup_mise_checksums=()
for target in linux-x64 linux-arm64 macos-x64 macos-arm64 windows-x64; do
	checksum="$(./scripts/mise_checksum.sh .github/actions/setup-mise/checksums.toml "${mise_version}" "${target}")"
	check_not_empty ".github/actions/setup-mise/checksums.toml ${target}" "${checksum}"
	check_sha256_format ".github/actions/setup-mise/checksums.toml ${target}" "${checksum}"
	setup_mise_checksums["${target}"]="${checksum}"
done
linux_x64_checksum="${setup_mise_checksums["linux-x64"]}"

sri_sha256_to_hex() {
	local label="$1"
	local sri="$2"

	if [[ "${sri}" != sha256-* ]]; then
		error "Expected SRI SHA256 hash for ${label}: ${sri}"
	fi

	printf '%s' "${sri#sha256-}" | openssl base64 -A -d | od -An -tx1 -v | tr -d ' \n'
}

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

declare -A flake_targets=(
	["x86_64-linux"]="linux-x64"
	["aarch64-linux"]="linux-arm64"
	["x86_64-darwin"]="macos-x64"
	["aarch64-darwin"]="macos-arm64"
)
for system in "${!flake_targets[@]}"; do
	target="${flake_targets[${system}]}"
	expected_checksum="${setup_mise_checksums[${target}]}"

	flake_hash="$(
		awk -v nix_system="${system}" '
			/^[[:space:]]*hash = \{/ { in_hash = 1; next }
			in_hash && $1 == nix_system {
				gsub(/[";]/, "", $3)
				print $3
				exit
			}
			in_hash && /^[[:space:]]*};/ { exit }
		' flake.nix
	)"
	check_not_empty "flake.nix ${system} hash" "${flake_hash}"

	actual_checksum="$(sri_sha256_to_hex "flake.nix ${system}" "${flake_hash}")"
	check_equal "flake.nix ${system} sha256" "${actual_checksum}" "${expected_checksum}"
done

wrapper_version="$(sed -n 's/^MISE_VERSION="v\([^"]*\)"/\1/p' scripts/dogfood/mise-oci-wrapper.sh)"
check_equal "scripts/dogfood/mise-oci-wrapper.sh" "${wrapper_version}" "${mise_version}"
wrapper_checksum="$(sed -n 's/^MISE_SHA256="\([a-f0-9]*\)"/\1/p' scripts/dogfood/mise-oci-wrapper.sh)"
check_equal "scripts/dogfood/mise-oci-wrapper.sh sha256" "${wrapper_checksum}" "${linux_x64_checksum}"
check_sha256_format "scripts/dogfood/mise-oci-wrapper.sh sha256" "${wrapper_checksum}"

for dockerfile in dogfood/coder/ubuntu-*/Dockerfile.base; do
	dockerfile_version="$(sed -n 's/.*MISE_VERSION=v\([0-9.]*\).*/\1/p' "${dockerfile}" | head -n 1)"
	check_equal "${dockerfile}" "${dockerfile_version}" "${mise_version}"

	dockerfile_checksum="$(sed -n 's/.*MISE_SHA256=\([a-f0-9]*\).*/\1/p' "${dockerfile}" | head -n 1)"
	check_equal "${dockerfile} sha256" "${dockerfile_checksum}" "${linux_x64_checksum}"
	check_sha256_format "${dockerfile} sha256" "${dockerfile_checksum}"
done

log "Mise version check passed, all versions are ${mise_version}"
