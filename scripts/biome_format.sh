#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 <path-relative-to-site>" >&2
	exit 2
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
target=$1

output_file=$(mktemp)
trap 'rm -f "$output_file"' EXIT

if (
	cd "$repo_root/site"
	pnpm exec biome format --write "$target"
) >"$output_file" 2>&1; then
	cat "$output_file"
	exit 0
fi
status=$?

cat "$output_file" >&2

if [[ $status -eq 127 ]] || grep -q "Could not start dynamically linked executable" "$output_file" || grep -q "NixOS cannot run dynamically linked executables" "$output_file"; then
	echo "WARNING: skipping biome format for '$target' because the biome binary is unavailable in this environment." >&2
	exit 0
fi

exit $status
