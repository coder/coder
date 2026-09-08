#!/usr/bin/env bash

# This script downloads the pinned desktop runtime archive for an architecture
# and verifies it against the sha256 recorded in runtime.lock. It is what the
# Makefile uses to obtain the archive embedded into linux binaries, so the
# binary build never needs a container toolchain.
#
# Usage: ./fetch.sh --arch <amd64|arm64> --output <path.tar.zst>
#
# The download base URL can be overridden with CODER_DESKTOP_RUNTIME_URL, for
# example to point at a mirror in an offline build environment.
#
# Exits with status 2 when runtime.lock is not pinned so callers can tell
# "not pinned yet" apart from a failed download.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"

arch=""
output_path=""

args="$(getopt -o "" -l arch:,output: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--arch)
		arch="$2"
		if [[ "$arch" != "amd64" ]] && [[ "$arch" != "arm64" ]]; then
			error "Invalid --arch parameter '$arch', must be 'amd64' or 'arm64'"
		fi
		shift 2
		;;
	--output)
		mkdir -p "$(dirname "$2")"
		output_path="$(realpath "$2")"
		shift 2
		;;
	--)
		shift
		break
		;;
	*)
		error "Unrecognized option: $1"
		;;
	esac
done

if [[ "$#" != 0 ]]; then
	error "Unexpected argument(s): $*"
fi
if [[ "$arch" == "" ]]; then
	error "--arch is required"
fi
if [[ "$output_path" == "" ]]; then
	error "--output is required"
fi

dependencies curl sha256sum

cdroot
lock_file="scripts/desktopruntime/runtime.lock"

lock_value() {
	sed -nE "s/^$1=(.*)$/\1/p" "$lock_file" | head -n1
}

version="$(lock_value version)"
expected_sha="$(lock_value "sha256_${arch}")"
if [[ "$version" == "" ]]; then
	log "desktop runtime is not pinned in $lock_file"
	exit 2
fi
if [[ ! "$expected_sha" =~ ^[0-9a-f]{64}$ ]]; then
	error "sha256_${arch} in $lock_file is missing or malformed"
fi

current_version="$(./scripts/desktopruntime/version.sh)"
if [[ "$version" != "$current_version" ]]; then
	error "runtime.lock pins version $version but the build inputs hash to $current_version; publish the new archives and update the lock"
fi

name="desktop-runtime-linux-${arch}.tar.zst"
base_url="${CODER_DESKTOP_RUNTIME_URL:-https://releases.coder.com/desktop-runtime}"
url="${base_url}/${version}/${name}"

# Reuse an existing download when it already matches.
if [[ -f "$output_path" ]] && [[ "$(sha256sum "$output_path" | cut -d' ' -f1)" == "$expected_sha" ]]; then
	log "$output_path is up to date"
	exit 0
fi

temp_file="$(mktemp "${output_path}.XXXXXX")"
# shellcheck disable=SC2317
cleanup() {
	rm -f "$temp_file"
}
trap cleanup EXIT

log "Downloading $url"
curl \
	--fail \
	--location \
	--silent \
	--show-error \
	--retry 5 \
	--retry-all-errors \
	--output "$temp_file" \
	"$url"

actual_sha="$(sha256sum "$temp_file" | cut -d' ' -f1)"
if [[ "$actual_sha" != "$expected_sha" ]]; then
	error "sha256 mismatch for $name: expected $expected_sha, got $actual_sha"
fi

chmod 0644 "$temp_file"
mv "$temp_file" "$output_path"
trap - EXIT
log "Fetched $output_path ($(stat -c %s "$output_path") bytes)"
