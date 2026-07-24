#!/usr/bin/env bash

# Print the pinned mise SHA256 checksum for a version and release target.

set -euo pipefail

if [[ "$#" -ne 3 ]]; then
	echo "usage: $0 <checksums.toml> <mise-version> <target>" >&2
	exit 1
fi

checksums_file="$1"
mise_version="$2"
target="$3"

awk -F= -v version="${mise_version}" -v target="${target}" '
	$0 == "[\"" version "\"]" { in_table = 1; next }
	/^\[/ { in_table = 0 }
	in_table {
		key = $1
		gsub(/^[[:space:]]+|[[:space:]]+$/, "", key)
		if (key == target) {
			value = $2
			gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
			gsub(/^"|"$/, "", value)
			print value
			exit
		}
	}
' "${checksums_file}"
