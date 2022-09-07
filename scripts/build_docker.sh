#!/usr/bin/env bash

# This script builds a Docker image of Coder containing the given binary, for
# the given architecture. Only linux binaries are supported at this time.
#
# Usage: ./build_docker.sh --arch amd64 [--version 1.2.3] [--push] path/to/coder
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
	[armv7]="linux/arm/v7"
)
if [[ "${arch_map[$arch]+exists}" != "" ]]; then
	arch="${arch_map[$arch]}"
fi

# Make temporary dir where all source files intended to be in the image will be
# hardlinked from.
cdroot
temp_dir="$(TMPDIR="$(dirname "$input_file")" mktemp -d)"
ln "$input_file" "$temp_dir/coder"
ln Dockerfile "$temp_dir/"

cd "$temp_dir"

log "--- Building Docker image for $arch ($image_tag)"

# Pull the base image, copy the /etc/group and /etc/passwd files out of it, and
# add the coder group and user. We have to do this in a separate step instead of
# using the RUN directive in the Dockerfile because you can't use RUN if you're
# building the image for a different architecture than the host.
docker pull --platform "$arch" alpine:latest 1>&2

temp_container_id="$(docker create --platform "$arch" alpine:latest)"
docker cp "$temp_container_id":/etc/group ./group 1>&2
docker cp "$temp_container_id":/etc/passwd ./passwd 1>&2
docker rm "$temp_container_id" 1>&2

echo "coder:x:1000:coder" >>./group
echo "coder:x:1000:1000::/:/bin/sh" >>./passwd
mkdir ./empty-dir

docker buildx build \
	--platform "$arch" \
	--build-arg "CODER_VERSION=$version" \
	--tag "$image_tag" \
	. 1>&2

cdroot
rm -rf "$temp_dir"

if [[ "$push" == 1 ]]; then
	log "--- Pushing Docker image for $arch ($image_tag)"
	docker push "$image_tag" 1>&2
fi

echo "$image_tag"
