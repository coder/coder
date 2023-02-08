#!/usr/bin/env bash

# This script builds the ironbank Docker image of Coder containing the given
# binary. Other dependencies will be automatically downloaded and cached.
#
# Usage: ./build_ironbank.sh --target image_tag path/to/coder

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"

image_tag=""

args="$(getopt -o "" -l target: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--target)
		image_tag="$2"
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

if [[ "$image_tag" == "" ]]; then
	error "The --image-tag parameter is required"
fi

# Check dependencies
dependencies docker sha256sum yq
if [[ $(yq --version) != *" v4."* ]]; then
	error "yq version 4 is required"
fi

if [[ "$#" != 1 ]]; then
	error "Exactly one argument must be provided to this script, $# were supplied"
fi
if [[ ! -f "$1" ]]; then
	error "File '$1' does not exist or is not a regular file"
fi
input_file="$(realpath "$1")"

# Make temporary dir for Docker build context.
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
pushd "$(dirname "${BASH_SOURCE[0]}")"
cp Dockerfile "$tmpdir/"
cp terraform-filesystem-mirror.tfrc "$tmpdir/"
popd

# Create a coder.tar.gz file.
execrelative ../archive.sh \
	--format tar.gz \
	--os linux \
	--output "$tmpdir/coder.tar.gz" \
	"$input_file"

# Download all resources in the hardening_manifest.yaml file except for
# coder.tar.gz (which we will make ourselves).
manifest_path="$(dirname "${BASH_SOURCE[0]}")/hardening_manifest.yaml"
resources="$(yq e '.resources[] | select(.filename != "coder.tar.gz") | [.filename, .url, .validation.value] | @tsv' "$manifest_path")"
while read -r line; do
	filename="$(echo "$line" | cut -f1)"
	url="$(echo "$line" | cut -f2)"
	sha256_hash="$(echo "$line" | cut -f3)"

	pushd "$(dirname "${BASH_SOURCE[0]}")"
	target=".${filename}.${sha256_hash}"
	if [[ ! -f "$target" ]]; then
		log "Downloading $filename"
		curl -sSL "$url" -o "$target"
	fi

	sum="$(sha256sum "$target" | cut -d' ' -f1)"
	if [[ "$sum" != "$sha256_hash" ]]; then
		rm "$target"
		error "Downloaded $filename has hash $sum, but expected $sha256_hash"
	fi
	cp "$target" "$tmpdir/$filename"
	popd
done <<<"$resources"

terraform_coder_provider_version="$(yq e '.args.TERRAFORM_CODER_PROVIDER_VERSION' "$manifest_path")"
if [[ "$terraform_coder_provider_version" == "" ]]; then
	error "TERRAFORM_CODER_PROVIDER_VERSION not found in hardening_manifest.yaml"
fi

# Build the image.
pushd "$tmpdir"
docker build \
	--build-arg BASE_REGISTRY=registry.access.redhat.com \
	--build-arg BASE_IMAGE=ubi8/ubi-minimal \
	--build-arg BASE_TAG=8.7 \
	--build-arg TERRAFORM_CODER_PROVIDER_VERSION="$terraform_coder_provider_version" \
	-t "$image_tag" \
	. >&2
popd

echo "$image_tag"
