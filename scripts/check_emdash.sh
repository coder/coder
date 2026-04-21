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

# Build the pattern from raw bytes so the script itself does not
# contain literal emdash/endash characters (which would trigger
# the check when the script is in the diff).
emdash=$'\xE2\x80\x94'
endash=$'\xE2\x80\x93'
pattern="${emdash}|${endash}"

scan_all_files() {
	local output
	output=$(git ls-files -z | xargs -0 grep -IEn "$pattern" 2>/dev/null || true)
	if [[ -n "$output" ]]; then
		echo "$output"
		found=1
	else
		found=0
	fi
}

if [[ "$mode" == "all" ]]; then
	scan_all_files
else
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
		# Ensure the base ref is fetchable. CI shallow clones
		# (fetch-depth: 1) may not have the base branch available.
		if ! git rev-parse --verify "$base" >/dev/null 2>&1; then
			ref="${base#origin/}"
			echo "Base ref $base not found locally, fetching $ref..."
			git fetch origin "$ref" --depth=1 2>/dev/null || true
			if ! git rev-parse --verify "$base" >/dev/null 2>&1; then
				echo "ERROR: could not fetch base ref $base."
				exit 1
			fi
		fi

		found=0
		if ! diff_output=$(git diff "$base" -U0 -- . 2>&1); then
			echo "ERROR: git diff against $base failed:"
			echo "$diff_output"
			exit 1
		fi

		if [[ -z "$diff_output" ]]; then
			echo "OK: no changes to check."
			exit 0
		fi

		# Parse the diff to check only added lines for emdash/endash.
		current_file=""
		current_line=0
		while IFS= read -r diff_line; do
			if [[ "$diff_line" =~ ^\+\+\+\ b/(.*) ]]; then
				current_file="${BASH_REMATCH[1]}"
			fi
			# Anchored to hunk header structure to avoid matching
			# digits from trailing function context.
			if [[ "$diff_line" =~ ^@@\ -[0-9,]+\ \+([0-9]+) ]]; then
				current_line=${BASH_REMATCH[1]}
				continue
			fi
			if [[ "$diff_line" =~ ^\+ ]] && [[ ! "$diff_line" =~ ^\+\+\+\ [ab/] ]]; then
				if echo "$diff_line" | grep -Eq "$pattern"; then
					echo "${current_file}:${current_line}:${diff_line:1}"
					found=1
				fi
				((current_line++)) || true
			fi
		done <<<"$diff_output"
	fi
fi

if [[ "$found" -ne 0 ]]; then
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
