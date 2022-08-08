#!/usr/bin/env bash

# This script builds multiple "slim" Go binaries for Coder with the given OS and
# architecture combinations. This wraps ./build_go_matrix.sh.
#
# Usage: ./build_go_slim.sh [--version 1.2.3-devel+abcdef] [--output dist/] [--compress 22] os1:arch1,arch2 os2:arch1 os1:arch3
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
#
# When the --compress <level> parameter is provided, the binaries in site/bin
# will be compressed using zstd into site/bin/coder.tar.zst, this helps reduce
# final binary size significantly.

set -euo pipefail
shopt -s nullglob
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version=""
output_path=""
compress=0

args="$(getopt -o "" -l version:,output:,compress: -- "$@")"
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
	--compress)
		compress="$2"
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
dependencies go
if [[ $compress != 0 ]]; then
	dependencies openssl tar zstd zip

	if [[ $compress != [0-9]* ]] || [[ $compress -gt 22 ]] || [[ $compress -lt 1 ]]; then
		error "Invalid value for compress, must in in the range of [1, 22]"
	fi
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
	# Remove ./ prefix
	name="${f#./}"
	# Remove "-slim_$version"
	truncated="${name//-slim_$version/}"
	# Replace underscores with hyphens
	hyphenated="${truncated//_/-}"
	dest="$dest_dir/$hyphenated"
	cp "$f" "$dest"
done

if [[ $compress != 0 ]]; then
	pushd "$dest_dir"
	sha_file=coder.sha1
	sha_dest="$dest_dir/$sha_file"
	log "--- Generating SHA1 for coder-slim binaries ($sha_dest)"
	openssl dgst -r -sha1 coder-* | tee $sha_file
	echo "$sha_dest"
	log
	log

	tar_name=coder.tar.zst
	tar_dest="$dest_dir/$tar_name"
	log "--- Compressing coder-slim binaries using zstd level $compress ($tar_dest)"
	tar cf coder.tar $sha_file coder-*
	rm coder-*
	zstd --force --ultra --long -"${compress}" --rm --no-progress coder.tar -o $tar_name
	echo "$tar_dest"
	log
	log

	popd
fi
