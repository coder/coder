#!/usr/bin/env bash

# This script regenerates the Terraform version and SHA256 checksums embedded in
# install.sh from provisioner/terraform/terraform_SHA256SUMS, a verbatim copy of
# HashiCorp's GPG-signed terraform_<version>_SHA256SUMS. install.sh is served on
# its own, so it can't read the checksum file at install time.
# scripts/Dockerfile.base reads the checksum file directly.
#
# It also fails if TerraformVersion in provisioner/terraform/install.go doesn't
# match the checksum file.
#
# Usage: ./scripts/gen_install_sh_terraform.sh

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

sums=provisioner/terraform/terraform_SHA256SUMS
install_sh=install.sh
install_go=provisioner/terraform/install.go

line_re='^[0-9a-f]{64}  terraform_([0-9]+\.[0-9]+\.[0-9]+)_[a-z0-9]+_[a-z0-9]+\.zip$'
if bad="$(grep -nvE "$line_re" "$sums")"; then
	error "$sums has lines that aren't in SHA256SUMS format:"$'\n'"$bad"
fi
versions="$(sed -E "s/${line_re}/\1/" "$sums" | sort -u)"
if [[ -z "$versions" ]] || [[ "$versions" == *$'\n'* ]]; then
	error "$sums must list exactly one Terraform version, found: ${versions//$'\n'/ }"
fi
version="$versions"

go_version="$(sed -nE 's/^[[:space:]]*TerraformVersion[[:space:]]*=[[:space:]]*version\.Must\(version\.NewVersion\("([^"]+)"\)\).*/\1/p' "$install_go")"
if [[ "$go_version" != "$version" ]]; then
	error "TerraformVersion in $install_go is \"$go_version\", but $sums is for $version"
fi

# install.sh only installs Terraform on the platforms it supports.
entries="$(grep -E '_(darwin|freebsd|linux)_[a-z0-9]+\.zip$' "$sums")" ||
	error "$sums has no darwin, freebsd or linux entries"

block="terraform_sha256sums() {"$'\n'$'\t'"cat <<-EOF"$'\n'
while IFS= read -r entry; do
	block+=$'\t\t'"$entry"$'\n'
done <<<"$entries"
block+=$'\t'"EOF"$'\n'"}"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

BLOCK="$block" VERSION="$version" awk '
	/^# BEGIN GENERATED: terraform_sha256sums/ {
		print
		print ENVIRON["BLOCK"]
		skip = 1
		found++
		next
	}
	/^# END GENERATED: terraform_sha256sums/ { skip = 0 }
	skip { next }
	/^\tTERRAFORM_VERSION="[^"]*"$/ {
		print "\tTERRAFORM_VERSION=\"" ENVIRON["VERSION"] "\""
		set_version++
		next
	}
	{ print }
	END { exit !(found == 1 && set_version == 1 && !skip) }
' "$install_sh" >"$tmp" ||
	error "$install_sh must have one TERRAFORM_VERSION line and one GENERATED: terraform_sha256sums block"

# Write through the existing file to keep its mode, and only when it changed so
# make doesn't see a new mtime.
if ! cmp -s "$tmp" "$install_sh"; then
	cat "$tmp" >"$install_sh"
	log "Updated $install_sh for Terraform $version"
fi
