#!/usr/bin/env bash

# This script builds a Docker image of Coder containing the given binary, for
# the given architecture. Only linux binaries are supported at this time.
#
# Usage: ./build_docker.sh --arch amd64 [--version 1.2.3] [--push]
#
# The --arch parameter is required and accepts a Golang arch specification. It
# will be automatically mapped to a suitable architecture that Docker accepts
# before being passed to `docker buildx build`.
#
# The image will be built and tagged against the image tag returned by
# ./image_tag.sh.
#
# If no version is specified, defaults to the version from ./version.sh.
#
# If the --push parameter is supplied, the image will be pushed.
#
# Prints the image tag on success.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

arch=""
version=""
push=0

args="$(getopt -o "" -l arch:,version:,push -- "$@")"
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

if [[ "$arch" == "" ]]; then
	error "The --arch parameter is required"
fi

# Check dependencies
dependencies docker

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

image_tag="$(execrelative ./image_tag.sh --arch "$arch" --version="$version")"

if [[ "$#" != 1 ]]; then
	error "Exactly one argument must be provided to this script, $# were supplied"
fi
if [[ ! -f "$1" ]]; then
	error "File '$1' does not exist or is not a regular file"
fi
input_file="$(realpath "$1")"

# Remap the arch from Golang to Docker.
declare -A arch_map=(
	[amd64]="linux/amd64"
	[arm64]="linux/arm64"
	[arm]="linux/arm/v7"
)
if [[ "${arch_map[$arch]+exists}" != "" ]]; then
	arch="${arch_map[$arch]}"
fi

# Make temporary dir where all source files intended to be in the image will be
# hardlinked from.
cdroot
temp_dir="$(TMPDIR="$(dirname "$input_file")" mktemp -d)"
ln -P "$input_file" "$temp_dir/coder"
ln -P Dockerfile "$temp_dir/"

cd "$temp_dir"

build_args=(
	--platform "$arch"
	--build-arg "CODER_VERSION=$version"
	--tag "$image_tag"
)

log "--- Building Docker image for $arch ($image_tag)"
docker buildx build "${build_args[@]}" . 1>&2

cdroot
rm -rf "$temp_dir"

if [[ "$push" == 1 ]]; then
	log "--- Pushing Docker image for $arch ($image_tag)"
	docker push "$image_tag"
fi

echo -n "$image_tag"
