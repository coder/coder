#!/usr/bin/env bash

# This script builds a Docker image of Coder containing the given binary, for
# the given architecture. Only linux binaries are supported at this time.
#
# Usage: ./build_docker.sh --arch amd64 --tags v1.2.4-rc.1-arm64,v1.2.3+devel.abcdef [--version 1.2.3] [--push]
#
# The --arch parameter is required and accepts a Golang arch specification. It
# will be automatically mapped to a suitable architecture that Docker accepts.
#
# The image will be built and tagged against all supplied tags. At least one tag
# must be supplied. All tags will be sanitized to remove invalid characters like
# plus signs.
#
# If no version is specified, defaults to the version from ./version.sh.
#
# If the --push parameter is supplied, all supplied tags will be pushed.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

image="ghcr.io/coder/coder"
arch=""
tags_str=""
version=""
push=0

args="$(getopt -o "" -l arch:,tags:,version:,push -- "$@")"
eval set -- "$args"
while true; do
    case "$1" in
    --arch)
        arch="$2"
        shift 2
        ;;
    --tags)
        tags_str="$2"
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

tags=()
for tag in $(echo "$tags_str" | tr "," "\n"); do
    # Docker images don't support plus signs, which devel versions may contain.
    tag="${tag//+/-}"
    tags+=("$tag")
done
if [[ "${#tags[@]}" == 0 ]]; then
    error "At least one tag must be supplied through --tags"
fi

if [[ "$#" != 1 ]]; then
    error "Exactly one argument must be provided to this script, $# were supplied"
fi
if [[ ! -f "$1" ]]; then
    error "File '$1' does not exist or is not a regular file"
fi
input_file="$(realpath "$1")"

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
    version="$(execrelative ./version.sh)"
fi

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
    "--platform=$arch"
    "--label=org.opencontainers.image.title=Coder"
    "--label=org.opencontainers.image.description=A tool for provisioning self-hosted development environments with Terraform."
    "--label=org.opencontainers.image.url=https://github.com/coder/coder"
    "--label=org.opencontainers.image.source=https://github.com/coder/coder"
    "--label=org.opencontainers.image.version=$version"
    "--label=org.opencontainers.image.licenses=AGPL-3.0"
)
for tag in "${tags[@]}"; do
    build_args+=(--tag "$image:$tag")
done

echo "--- Building Docker image for $arch"
docker buildx build "${build_args[@]}" .

cdroot
rm -rf "$temp_dir"

if [[ "$push" == 1 ]]; then
    echo "--- Pushing Docker images for $arch"
    for tag in "${tags[@]}"; do
        echo "Pushing $image:$tag"
        docker push "$image:$tag"
    done
fi
