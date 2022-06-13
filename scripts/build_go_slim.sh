#!/usr/bin/env bash

# This script builds multiple "slim" Go binaries for Coder with the given OS and
# architecture combinations. This wraps ./build_go_matrix.sh.
#
# Usage: ./build_go_slim.sh [--version 1.2.3+devel.abcdef] [--output dist/] os1:arch1,arch2 os2:arch1 os1:arch3
#
# If no OS:arch combinations are provided, nothing will happen and no error will
# be returned. If no version is specified, defaults to the version from
# ./version.sh
#
# The --output parameter differs from ./build_go_matrix.sh, in that it does not
# accept variables such as `{os}` and `{arch}` and only accepts a directory
# ending with `/`.
#
# The built binaries are additionally copied to the site output directory so
# they can be packaged into non-slim binaries correctly.

set -euo pipefail
shopt -s nullglob
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version=""
output_path=""

args="$(getopt -o "" -l version:,output: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--version)
		version="$2"
		shift 2
		;;
	--output)
		output_path="$2"
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

# Check dependencies
if ! command -v go; then
	error "The 'go' binary is required."
fi

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

# Verify the output path.
if [[ "$output_path" == "" ]]; then
	# Input paths are relative, so we don't cdroot at the top, but for this case
	# we want it to be relative to the root.
	cdroot
	mkdir -p dist
	output_path="$(realpath "dist/coder-slim_{version}_{os}_{arch}")"
elif [[ "$output_path" != */ ]] || [[ "$output_path" == *"{"* ]]; then
	error "The output path '$output_path' cannot contain variables and must end with a slash"
else
	mkdir -p "$output_path"
	output_path="$(realpath "${output_path}coder-slim_{version}_{os}_{arch}")"
fi

./scripts/build_go_matrix.sh \
	--version "$version" \
	--output "$output_path" \
	--slim \
	"$@"

cdroot
dest_dir="./site/out/bin"
mkdir -p "$dest_dir"
dest_dir="$(realpath "$dest_dir")"

# Copy the binaries to the site directory.
cd "$(dirname "$output_path")"
for f in ./coder-slim_*; do
	f="${f#./}"
	dest="$dest_dir/${f//-slim_$version/}"
	cp "$f" "$dest"
done
