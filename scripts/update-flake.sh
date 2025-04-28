#!/usr/bin/env bash
# Updates SRI hashes for flake.nix.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

check_and_install() {
	if ! command -v "$1" &>/dev/null; then
		echo "$1 is not installed. Attempting to install..."
		if ! nix-env -iA nixpkgs."$1"; then
			echo "Failed to install $1. Please install it manually and try again."
			exit 1
		fi
		echo "$1 has been installed successfully."
	fi
}

check_and_install jq
check_and_install nix-prefetch-git

OUT=$(mktemp -d -t nar-hash-XXXXXX)

echo "Downloading Go modules..."
GOPATH="$OUT" go mod download
echo "Calculating SRI hash..."
HASH=$(go run tailscale.com/cmd/nardump --sri "$OUT/pkg/mod/cache/download")
sudo rm -rf "$OUT"

echo "Updating go.mod vendorHash"
sed -i "s#\(vendorHash = \"\)[^\"]*#\1${HASH}#" ./flake.nix

# Update protoc-gen-go sha256
echo "Updating protoc-gen-go sha256..."
PROTOC_GEN_GO_REV=$(nix eval --extra-experimental-features nix-command --extra-experimental-features flakes --raw .#proto_gen_go.rev)
echo "protoc-gen-go version: $PROTOC_GEN_GO_REV"
PROTOC_GEN_GO_SHA256=$(nix-prefetch-git https://github.com/protocolbuffers/protobuf-go --rev "$PROTOC_GEN_GO_REV" | jq -r .hash)
sed -i "s#\(sha256 = \"\)[^\"]*#\1${PROTOC_GEN_GO_SHA256}#" ./flake.nix

make dogfood/coder/nix.hash

echo "Flake updated successfully!"
