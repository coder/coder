#!/usr/bin/env bash

# This script builds a Docker image of Coder containing the given binary, for
# the given architecture. Only linux binaries are supported at this time.
#
# Usage: ./build_docker.sh --arch amd64 [--version 1.2.3] [--target image_tag] [--build-base image_tag] [--push] path/to/coder
#
# The --arch parameter is required and accepts a Golang arch specification. It
# will be automatically mapped to a suitable architecture that Docker accepts
# before being passed to `docker buildx build`.
#
# The --build-base parameter is optional and specifies to build the base image
# in Dockerfile.base instead of pulling a copy from the registry. The string
# value is the tag to use for the built image (not pushed). This also consumes
# $CODER_IMAGE_BUILD_BASE_TAG for easily forcing a fresh build in CI.
#
# The default base image can be controlled via $CODER_BASE_IMAGE_TAG.
#
# The image will be built and tagged against the image tag returned by
# ./image_tag.sh unless a --target parameter is supplied.
#
# If no version is specified, defaults to the version from ./version.sh.
#
# If the --push parameter is supplied, the image will be pushed.
#
# Prints the image tag on success.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

DEFAULT_BASE="${CODER_BASE_IMAGE_TAG:-ghcr.io/coder/coder-base:latest}"

arch=""
image_tag=""
build_base="${CODER_IMAGE_BUILD_BASE_TAG:-}"
version=""
push=0

args="$(getopt -o "" -l arch:,target:,build-base:,version:,push -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--arch)
		arch="$2"
		shift 2
		;;
	--target)
		image_tag="$2"
		shift 2
		;;
	--version)
		version="$2"
		shift 2
		;;
	--build-base)
		build_base="$2"
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

if [[ "$image_tag" == "" ]]; then
	image_tag="$(execrelative ./image_tag.sh --arch "$arch" --version="$version")"
fi

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
orig_arch="$arch"
if [[ "${arch_map[$arch]+exists}" != "" ]]; then
	arch="${arch_map[$arch]}"
fi

# Make temporary dir where all source files intended to be in the image will be
# hardlinked from.
cdroot
temp_dir="$(TMPDIR="$(dirname "$input_file")" mktemp -d)"
ln "$input_file" "$temp_dir/coder"
ln ./scripts/Dockerfile.base "$temp_dir/"
ln ./scripts/Dockerfile "$temp_dir/"

cd "$temp_dir"

export DOCKER_BUILDKIT=1

base_image="$DEFAULT_BASE"
if [[ "$build_base" != "" ]]; then
	log "--- Building base Docker image for $arch ($build_base)"
	docker build \
		--platform "$arch" \
		--build-arg "ARCH=${orig_arch}" \
		--tag "$build_base" \
		--no-cache \
		-f Dockerfile.base \
		. 1>&2

	base_image="$build_base"
else
	docker pull --platform "$arch" "$base_image" 1>&2
fi

log "--- Building Docker image for $arch ($image_tag)"

docker build \
	--platform "$arch" \
	--build-arg "BASE_IMAGE=$base_image" \
	--build-arg "CODER_VERSION=$version" \
	--no-cache \
	--tag "$image_tag" \
	-f Dockerfile \
	. 1>&2

cdroot
rm -rf "$temp_dir"

if [[ "$push" == 1 ]]; then
	log "--- Pushing Docker image for $arch ($image_tag)"
	docker push "$image_tag" 1>&2
fi

echo "$image_tag"
