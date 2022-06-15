#!/usr/bin/env bash

# This script prints the image tag to use for the given arch and version
# combination.
#
# Usage: ./image_tag.sh [--arch amd64] [--version 1.2.3]
#
# The --arch parameter accepts a Golang arch specification. If not specified,
# the image tag for the multi-arch image will be returned instead.
#
# If no version is specified, defaults to the version from ./version.sh. If the
# supplied version is "latest", no `v` prefix will be added to the tag.
#
# The returned tag will be sanitized to remove invalid characters like the plus
# sign.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

arch=""
version=""

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

# Remove the "v" prefix because we don't want to add it twice.
version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

image="${CODER_IMAGE_BASE:-ghcr.io/coder/coder}"
tag="v$version"
if [[ "$version" == "latest" ]]; then
	tag="latest"
fi
if [[ "$arch" != "" ]]; then
	tag+="-$arch"
fi

# Dev versions contain plus signs which are illegal characters in Docker tags.
tag="${tag//+/-}"
echo "$image:$tag"
