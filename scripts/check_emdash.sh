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

# Git exclude_pathspecs excluded from the check. Used in both ls-files and diff comparison.
exclude_pathspecs=(
	":(exclude)aibridge/fixtures/**/*.txtar"
	# Generated CLI golden files embed serpent's emdash-bordered footer.
	":(exclude)cli/testdata/*.golden"
	":(exclude)enterprise/cli/testdata/*.golden"
)

scan_all_files() {
	local output
	output=$(git ls-files -z -- "${exclude_pathspecs[@]}" | xargs -0 grep -IEn "$pattern" 2>/dev/null || true)
	if [[ -n "$output" ]]; then
		echo "$output"
		found=1
	else
		found=0
	fi
}

# resolve_diff_base determines the base commit to diff against.
resolve_diff_base() {
	# CI pull requests: actions/checkout checks out the PR merge commit,
	# whose first parent is the base branch tip. Diff against that parent
	# directly instead of fetching the base branch by name. The base
	# branch is not always available on origin; Graphite stacks, for
	# example, use a synthetic graphite-base/<n> ref that is never pushed.
	if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
		# HEAD^ needs one level of history to resolve. Deepen by one when
		# the checkout is shallow (fetch-depth: 1). This only extends
		# HEAD's own history; it does not fetch a separate base branch.
		if ! git rev-parse --verify --quiet "HEAD^" >/dev/null; then
			git fetch --deepen=1 >/dev/null 2>&1 || true
		fi

		if git rev-parse --verify --quiet "HEAD^" >/dev/null; then
			git rev-parse "HEAD^"
			return
		fi

		echo "WARNING: HEAD has no parent commit; cannot determine PR base." >&2
		return
	fi

	# Local dev: use merge-base with origin/main.
	if git rev-parse --verify origin/main >/dev/null 2>&1; then
		git merge-base HEAD origin/main 2>/dev/null || echo "origin/main"
		return
	fi
}

# scan_diff checks only added lines in the diff for emdash/endash.
scan_diff() {
	local base="$1"

	local diff_output
	if ! diff_output=$(git diff "$base" -U0 -- . "${exclude_pathspecs[@]}" 2>&1); then
		echo "ERROR: git diff against $base failed:" >&2
		echo "$diff_output" >&2
		exit 1
	fi

	if [[ -z "$diff_output" ]]; then
		echo "OK: no changes to check."
		exit 0
	fi

	local current_file="" current_line=0
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
}

if [[ "$mode" == "all" ]]; then
	scan_all_files
else
	base=$(resolve_diff_base) || {
		echo "ERROR: could not determine base ref." >&2
		exit 1
	}
	if [[ -z "$base" ]]; then
		# No base ref is available outside of pull requests, for
		# example push builds on release branches with a shallow clone
		# where neither GITHUB_BASE_REF nor origin/main is present.
		# The diff check has nothing to compare against, and scanning
		# every tracked file would flag pre-existing characters that
		# are unrelated to the change under test. Skip instead; pass
		# --all to force a full-tree scan.
		echo "OK: no base ref found (not a pull request); skipping emdash check."
		echo "    Pass --all to scan every tracked file."
		exit 0
	fi
	found=0
	scan_diff "$base"
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
