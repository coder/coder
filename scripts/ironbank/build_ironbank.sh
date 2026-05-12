#!/usr/bin/env bash

# This script builds the ironbank Docker image of Coder containing the given
# binary. Terraform is compiled from source to control the Go toolchain version
# and address Go stdlib CVEs. Other dependencies will be automatically
# downloaded and cached.
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
dependencies docker sha256sum yq go
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
# coder.tar.gz (which we will make ourselves) and terraform-src.tar.gz
# (which we will build from source below).
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

# Build Terraform from source to control the Go toolchain version.
# This ensures the bundled Terraform binary uses Go 1.25.9+ to address
# Go stdlib CVEs that affect pre-built HashiCorp binaries.
terraform_src="$(yq e '.resources[] | select(.filename == "terraform-src.tar.gz") | [.url, .validation.value] | @tsv' "$manifest_path")"
if [[ -n "$terraform_src" ]]; then
	terraform_src_url="$(echo "$terraform_src" | cut -f1)"
	terraform_src_sha256="$(echo "$terraform_src" | cut -f2)"

	pushd "$(dirname "${BASH_SOURCE[0]}")"
	terraform_src_cache=".terraform-src.tar.gz.${terraform_src_sha256}"
	if [[ ! -f "$terraform_src_cache" ]]; then
		log "Downloading Terraform source"
		curl -sSL "$terraform_src_url" -o "$terraform_src_cache"
	fi

	sum="$(sha256sum "$terraform_src_cache" | cut -d' ' -f1)"
	if [[ "$sum" != "$terraform_src_sha256" ]]; then
		rm "$terraform_src_cache"
		error "Downloaded Terraform source has hash $sum, but expected $terraform_src_sha256"
	fi
	popd

	# Extract and build Terraform from source.
	terraform_build_dir="$(mktemp -d)"
	trap 'rm -rf "$tmpdir" "$terraform_build_dir"' EXIT
	pushd "$(dirname "${BASH_SOURCE[0]}")"
	tar -xzf "$terraform_src_cache" -C "$terraform_build_dir" --strip-components=1
	popd

	# Read the Go version from the Coder project's go.mod to ensure we use
	# the same toolchain version for all binaries in the image.
	coder_go_version="$(head -5 "$(dirname "${BASH_SOURCE[0]}")/../../go.mod" | grep '^go ' | awk '{print $2}')"
	log "Building Terraform from source with Go ${coder_go_version}"

	pushd "$terraform_build_dir"
	GOTOOLCHAIN="go${coder_go_version}" CGO_ENABLED=0 \
		go build \
		-trimpath \
		-ldflags="-s -w -X 'github.com/hashicorp/terraform/version.dev=no'" \
		-o "$tmpdir/terraform" .
	popd

	# Verify the compiled binary uses the expected Go toolchain.
	built_go_version="$(go version "$tmpdir/terraform" | grep -oP 'go[0-9]+\.[0-9]+\.[0-9]+')"
	log "Terraform built with ${built_go_version}"

	# Package as terraform.zip for the Dockerfile.
	pushd "$tmpdir"
	zip terraform.zip terraform
	rm terraform
	popd

	rm -rf "$terraform_build_dir"
else
	error "terraform-src.tar.gz resource not found in hardening_manifest.yaml"
fi

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
