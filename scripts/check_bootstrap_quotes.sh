#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

echo "--- check bootstrap scripts for single quotes"

files=$(find provisionersdk/scripts -type f -name '*.sh')
found=0
for f in $files; do
	if grep -n "'" "$f"; then
		echo "ERROR: $f contains single quotes (apostrophes)."
		echo "       Bootstrap scripts are inlined via sh -c '...' in templates."
		echo "       Single quotes break this quoting. Use alternative phrasing."
		found=1
	fi
done

if [ "$found" -ne 0 ]; then
	exit 1
fi

echo "OK: no single quotes found in bootstrap scripts."
