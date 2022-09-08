#!/usr/bin/env bash

# This script merges Coder Docker images of different architectures together
# into the specified target image+tag, or the arch-less image tag returned by
# ./image_tag.sh.
#
# Usage: ./build_docker_multiarch.sh [--version 1.2.3] [--target image:tag] [--push] image1:tag1 image2:tag2
#
# The supplied images must already be pushed to the registry or this will fail.
# Also, the source images cannot be in a different registry than the target
# image.
#
# If no version is specified, defaults to the version from ./version.sh.
#
# If no target tag is supplied, the arch-less image tag returned by
# ./image_tag.sh will be used.
#
# If the --push parameter is supplied, all supplied tags will be pushed.
#
# Returns the merged image tag.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version=""
target=""
push=0

args="$(getopt -o "" -l version:,target:,push -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--version)
		version="$2"
		shift 2
		;;
	--target)
		target="$2"
		shift 2
		;;
	--push)
		push=1
		shift
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

if [[ "$#" == 0 ]]; then
	error "At least one argument must be provided to this script, $# were supplied"
fi

# Check dependencies
dependencies docker

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

if [[ "$target" == "" ]]; then
	target="$(execrelative ./image_tag.sh --version "$version")"
fi

create_args=()
for image_tag in "$@"; do
	create_args+=(--amend "$image_tag")
done

# Sadly, manifests don't seem to support labels.
log "--- Creating multi-arch Docker image ($target)"
docker manifest create \
	"$target" \
	"${create_args[@]}"

if [[ "$push" == 1 ]]; then
	log "--- Pushing multi-arch Docker image ($target)"
	docker manifest push "$target"
fi

echo "$target"
