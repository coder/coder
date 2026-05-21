#!/usr/bin/env bash
# Runs protoc into a temporary directory, then atomically moves each
# generated file to the source tree. This prevents interrupted builds
# from leaving truncated or deleted .pb.go files.
#
# Usage: atomic_protoc.sh [protoc flags...] ./path/to/file.proto

set -euo pipefail

mkdir -p _gen
tmpdir=$(mktemp -d -p _gen)
trap 'rm -rf "$tmpdir"' EXIT

# Rewrite --go_out=. and --go-drpc_out=. to point at tmpdir.
args=()
for arg in "$@"; do
	case "$arg" in
	--go_out=.) args+=("--go_out=$tmpdir") ;;
	--go-drpc_out=.) args+=("--go-drpc_out=$tmpdir") ;;
	*) args+=("$arg") ;;
	esac
done

protoc "${args[@]}"

# Move all generated .go files from tmpdir back to the source tree.
find "$tmpdir" -name '*.go' -print0 | while IFS= read -r -d '' f; do
	dest="${f#"$tmpdir"/}"
	mv "$f" "$dest"
done
