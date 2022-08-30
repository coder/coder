#!/usr/bin/env bash

# This script creates Linux packages for the given binary. It will output a
# .rpm, .deb and .apk file in the same directory as the input file with the same
# filename (except the package format suffix).
#
# ./package.sh --arch amd64 [--version 1.2.3] path/to/coder
#
# The --arch parameter is required. If no version is specified, defaults to the
# version from ./version.sh.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version=""
arch=""

args="$(getopt -o "" -l arch:,version: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--arch)
		arch="$2"
		shift 2
		;;
	--version)
		version="$2"
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

if [[ "$arch" == "" ]]; then
	error "--arch is a required parameter"
fi

if [[ "$#" != 1 ]]; then
	error "Exactly one argument must be provided to this script, $# were supplied"
fi
if [[ ! -f "$1" ]]; then
	error "File '$1' does not exist or is not a regular file"
fi
input_file="$(realpath "$1")"

# Check dependencies
dependencies nfpm

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

# armv7 isn't a real architecture, so we need to remap it to armhf.
if [[ "$arch" == "arm" ]] || [[ "$arch" == "armv7" ]]; then
	arch="armhf"
fi

# Make temporary dir where all source files intended to be in the package will
# be hardlinked from.
cdroot
temp_dir="$(TMPDIR="$(dirname "$input_file")" mktemp -d)"
ln "$input_file" "$temp_dir/coder"
ln "$(realpath coder.env)" "$temp_dir/"
ln "$(realpath coder.service)" "$temp_dir/"
ln "$(realpath preinstall.sh)" "$temp_dir/"
ln "$(realpath scripts/nfpm.yaml)" "$temp_dir/"

cd "$temp_dir"

formats=(apk deb rpm)
for format in "${formats[@]}"; do
	output_path="$input_file.$format"
	log "--- Building $format package ($output_path)"

	GOARCH="$arch" CODER_VERSION="$version" nfpm package \
		-f nfpm.yaml \
		-p "$format" \
		-t "$output_path"
done

cdroot
rm -rf "$temp_dir"
