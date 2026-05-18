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
dependencies docker sha256sum yq go zip git
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

# Build Terraform from source so the binary is compiled with the same Go
# toolchain as Coder (>= 1.25.9), avoiding CVEs present in older toolchains.
terraform_version="$(yq e '.args.TERRAFORM_VERSION' "$(dirname "${BASH_SOURCE[0]}")/hardening_manifest.yaml")"
if [[ -z "$terraform_version" || "$terraform_version" == "null" ]]; then
	error "TERRAFORM_VERSION not found in hardening_manifest.yaml"
fi
log "Building Terraform $terraform_version from source with $(go version)..."
terraform_srcdir="$(mktemp -d)"
trap 'rm -rf "$terraform_srcdir" "$tmpdir"' EXIT
git clone --depth 1 --branch "v${terraform_version}" https://github.com/hashicorp/terraform.git "$terraform_srcdir"
pushd "$terraform_srcdir"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o terraform .
popd
(
	cd "$terraform_srcdir"
	zip "$tmpdir/terraform.zip" terraform
)
rm -rf "$terraform_srcdir"
log "Terraform $terraform_version built successfully."

# Download all resources in the hardening_manifest.yaml file except for
# coder.tar.gz (which we build ourselves) and terraform-src.tar.gz (we build
# Terraform from source above).
manifest_path="$(dirname "${BASH_SOURCE[0]}")/hardening_manifest.yaml"
resources="$(yq e '.resources[] | select(.filename != "coder.tar.gz" and .filename != "terraform-src.tar.gz") | [.filename, .url, .validation.value] | @tsv' "$manifest_path")"
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
	--build-arg BASE_IMAGE=ubi9/ubi-minimal \
	--build-arg BASE_TAG=9.6 \
	--build-arg TERRAFORM_CODER_PROVIDER_VERSION="$terraform_coder_provider_version" \
	-t "$image_tag" \
	. >&2
popd

echo "$image_tag"
