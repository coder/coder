#!/usr/bin/env bash
# Updates SRI hashes for flake.nix.

set -eu

cd "$(dirname "${BASH_SOURCE[0]}")/.."

OUT=$(mktemp -d -t nar-hash-XXXXXX)

echo "Downloading Go modules..."
GOPATH="$OUT" go mod download
echo "Calculating SRI hash..."
HASH=$(go run tailscale.com/cmd/nardump --sri "$OUT/pkg/mod/cache/download")
sudo rm -rf "$OUT"

sed -i "s#\(vendorHash = \"\)[^\"]*#\1${HASH}#" ./flake.nix
