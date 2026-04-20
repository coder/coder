#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

echo "--- check for emdash/endash characters"

# Build the pattern from raw bytes so the script itself does not
# contain literal emdash/endash characters (which would trigger
# the check).
emdash=$'\xE2\x80\x94'
endash=$'\xE2\x80\x93'
pattern="${emdash}|${endash}"

output=$(git ls-files -z | xargs -0 grep -IEn "$pattern" 2>/dev/null || true)
if [[ -n "$output" ]]; then
	echo "$output"
	echo ""
	echo "ERROR: Found emdash (U+2014) or endash (U+2013) characters."
	echo ""
	echo "  Do not use emdash or endash in code, comments, string literals,"
	echo "  or documentation. Use commas, semicolons, or periods instead."
	echo "  Restructure the sentence if needed. Do not replace them with"
	echo "  ' -- ' either."
	echo ""
	echo "  Example:"
	echo "    Bad:  This is slow [emdash] we should cache it."
	echo "    Good: This is slow. We should cache it."
	echo "    Good: This is slow, so we should cache it."
	echo ""
	exit 1
fi

echo "OK: no emdash or endash characters found."
