#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

echo "--- check for emdash/endash characters"

mode="changed"
for arg in "$@"; do
	if [[ "$arg" == "--all" ]]; then
		mode="all"
	fi
done

# grep -P with raw UTF-8 byte sequences avoids locale-dependent
# character class issues (a bare character class can false-positive
# on other multi-byte characters sharing the 0xE2 leading byte).
pattern='\xE2\x80\x94|\xE2\x80\x93'

scan_all_files() {
	found=0
	if git ls-files -z |
		xargs -0 grep -Pn "$pattern" 2>/dev/null; then
		found=1
	fi
}

if [[ "$mode" == "all" ]]; then
	scan_all_files
else
	# Scan only added/modified lines in the current diff.
	base=""
	if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
		base="origin/${GITHUB_BASE_REF}"
	elif git rev-parse --verify origin/main >/dev/null 2>&1; then
		base=$(git merge-base HEAD origin/main 2>/dev/null || echo "origin/main")
	fi

	if [[ -z "$base" ]]; then
		echo "WARNING: no base ref found, scanning all tracked files."
		scan_all_files
	else
		# Get unified diff with no context, then extract only added
		# lines (starting with +) and report file/line locations.
		found=0
		if ! diff_output=$(git diff "$base" -U0 -- . 2>&1); then
			echo "ERROR: git diff against $base failed:"
			echo "$diff_output"
			echo "Falling back to full scan."
			scan_all_files
		elif [[ -z "$diff_output" ]]; then
			echo "OK: no changes to check."
			exit 0
		else
			# Parse diff output to find added lines with emdash/endash.
			current_file=""
			current_line=0
			while IFS= read -r line; do
				# Track which file we're in.
				if [[ "$line" =~ ^\+\+\+\ b/(.*) ]]; then
					current_file="${BASH_REMATCH[1]}"
				fi
				if [[ "$line" =~ ^@@.*\+([0-9]+) ]]; then
					current_line=${BASH_REMATCH[1]}
					continue
				fi
				# Check added lines only.
				if [[ "$line" =~ ^\+ ]] && [[ ! "$line" =~ ^\+\+\+ ]]; then
					if echo "$line" | grep -Pq "$pattern"; then
						# Strip the leading '+' for display.
						echo "${current_file}:${current_line}:${line:1}"
						found=1
					fi
					((current_line++)) || true
				fi
			done <<<"$diff_output"
		fi
	fi
fi

if [[ "$found" -ne 0 ]]; then
	echo ""
	echo "ERROR: Found emdash (U+2014) or endash (U+2013) characters."
	echo ""
	echo "  Do not use emdash or endash characters in code, comments, or docs."
	echo "  Use commas, semicolons, or periods instead. Restructure the sentence"
	echo "  if needed. Do not replace them with ' -- ' either."
	echo ""
	echo "  Example:"
	echo "    Bad:  This is slow [emdash] we should cache it."
	echo "    Good: This is slow. We should cache it."
	echo "    Good: This is slow, so we should cache it."
	echo ""
	exit 1
fi

echo "OK: no emdash or endash characters found."
