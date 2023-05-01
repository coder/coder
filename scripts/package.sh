#!/usr/bin/env bash

# This script creates a Linux package for the given binary.
#
# ./package.sh --arch amd64 --format "(apk|deb|rpm)" --output "path/to/coder.apk" [--version 1.2.3] path/to/coder
#
# If no version is specified, defaults to the version from ./version.sh.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

arch=""
format=""
output_path=""
version=""

args="$(getopt -o "" -l arch:,format:,output:,version: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--arch)
		arch="$2"
		shift 2
		;;
	--format)
		format="$2"
		shift 2
		;;
	--output)
		mkdir -p "$(dirname "$2")"
		output_path="$(realpath "$2")"
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
if [[ "$format" != "apk" ]] && [[ "$format" != "deb" ]] && [[ "$format" != "rpm" ]]; then
	error "--format is a required parameter and must be one of 'apk', 'deb', or 'rpm'"
fi
if [[ "$output_path" == "" ]]; then
	error "--output is a required parameter"
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
# be hardlinked to.
cdroot
temp_dir="$(TMPDIR="$(dirname "$input_file")" mktemp -d)"
ln "$input_file" "$temp_dir/coder"
ln "$(realpath coder.env)" "$temp_dir/"
ln "$(realpath scripts/linux-pkg/coder-workspace-proxy.service)" "$temp_dir/"
ln "$(realpath scripts/linux-pkg/coder.service)" "$temp_dir/"
ln "$(realpath scripts/linux-pkg/nfpm.yaml)" "$temp_dir/"
ln "$(realpath scripts/linux-pkg/preinstall.sh)" "$temp_dir/"

pushd "$temp_dir"
GOARCH="$arch" CODER_VERSION="$version" nfpm package \
	-f nfpm.yaml \
	-p "$format" \
	-t "$output_path" \
	1>&2
popd

rm -rf "$temp_dir"
