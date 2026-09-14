#!/usr/bin/env bash
# Regression tests for the NUL-delimited diff parser in deploy-docs.yaml.
# The workflow runs `git diff --name-status -z` into $DIFF_FILE and feeds
# the result through an awk script that emits <path>\t<status> lines.
# jq then slurps those lines into a JSON array. This script exercises
# the awk parser against synthetic NUL-delimited inputs so we can
# verify path escaping, rename handling, and unknown-status-code
# behavior without spinning up the full workflow.
#
# Keep `parse_diff` and `build_json_array` below in sync with
# deploy-docs.yaml. The workflow comment "Tested in
# test-deploy-docs-diff.sh" is the contract.
#
# Test inputs are passed to the parser as file paths (not via shell
# variables) because bash strips NUL bytes from command substitutions
# and parameter values. Each test writes its synthetic diff to a tmp
# file before invoking the parser, which is also how the workflow
# itself feeds the parser ($DIFF_FILE).

set -euo pipefail

TMPDIR_SELF="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_SELF"' EXIT

# parse_diff replicates the awk block in deploy-docs.yaml so we can
# exercise it without running the full workflow. Reads NUL-delimited
# `git diff --name-status -z` output from $1 and emits
# <path>\t<status> lines on stdout. Unknown status codes log a warning
# to stderr and consume the path field so the record alignment stays
# correct.
parse_diff() {
	awk -v RS='\0' '
		function emit(path, status) {
			printf "%s\t%s\n", path, status
		}
		{
			code = substr($0, 1, 1)
			if (code == "A") { getline; emit($0, "added"); next }
			if (code == "M") { getline; emit($0, "modified"); next }
			if (code == "T") { getline; emit($0, "modified"); next }
			if (code == "D") { getline; emit($0, "deleted"); next }
			if (code == "R") {
				# R<similarity>\0<old>\0<new>\0
				getline old_path
				getline new_path
				emit(new_path, "renamed")
				next
			}
			if ($0 != "") {
				unknown_code = $0
				getline unknown_path
				printf "::warning::Unknown git diff status %s for %s; skipping.\n", unknown_code, unknown_path > "/dev/stderr"
			}
		}
	' "$1"
}

# build_json_array mirrors the jq slurp in deploy-docs.yaml. Reads
# <path>\t<status> lines from $1 and emits a compact JSON array.
build_json_array() {
	jq -Rcn '
		[ inputs
		  | split("\t")
		  | { path: .[0], status: .[1] }
		]
	' <"$1"
}

# write_nul_input writes a NUL-delimited diff to a fresh tmp file and
# echoes the file path. Args become NUL-delimited records.
write_nul_input() {
	local f
	f="$(mktemp -p "$TMPDIR_SELF")"
	# Cannot use a single printf %s\0 list because bash's printf will
	# happily emit literal NULs, but the surrounding command
	# substitution does not strip NULs from file descriptors, only
	# from variables. Write directly to the file.
	local arg
	for arg in "$@"; do
		printf '%s\0' "$arg"
	done >"$f"
	printf '%s' "$f"
}

failures=0
section=""

start_section() {
	section="$1"
	echo
	echo "--- $section ---"
}

assert_parse() {
	local description="$1"
	local input_file="$2"
	local expected="$3"
	local actual
	actual="$(parse_diff "$input_file" 2>/dev/null)"
	if [ "$actual" = "$expected" ]; then
		echo "PASS: $description"
	else
		echo "FAIL: $description"
		echo "  expected: $(printf '%s' "$expected" | cat -A)"
		echo "  actual:   $(printf '%s' "$actual" | cat -A)"
		failures=$((failures + 1))
	fi
}

assert_json() {
	local description="$1"
	local input_file="$2"
	local expected="$3"
	local parsed
	parsed="$(mktemp -p "$TMPDIR_SELF")"
	parse_diff "$input_file" 2>/dev/null >"$parsed"
	local actual
	actual="$(build_json_array "$parsed")"
	if [ "$actual" = "$expected" ]; then
		echo "PASS: $description"
	else
		echo "FAIL: $description"
		echo "  expected: $expected"
		echo "  actual:   $actual"
		failures=$((failures + 1))
	fi
}

assert_warns() {
	local description="$1"
	local input_file="$2"
	local needle="$3"
	local stderr_out
	stderr_out="$(parse_diff "$input_file" 2>&1 >/dev/null)"
	if printf '%s' "$stderr_out" | grep -q -- "$needle"; then
		echo "PASS: $description"
	else
		echo "FAIL: $description"
		echo "  needle:   $needle"
		echo "  stderr:   $stderr_out"
		failures=$((failures + 1))
	fi
}

assert_count_matches_emitter() {
	# Verify count derivation cannot diverge from the emitter output.
	# This is the structural guarantee DEREM-21 calls out: counter and
	# emitter must agree by construction. Here that means
	# `wc -l < parsed` always equals the number of <path>\t<status>
	# lines emitted, even when the input contains unknown codes.
	local description="$1"
	local input_file="$2"
	local expected_count="$3"
	local actual_count
	actual_count="$(parse_diff "$input_file" 2>/dev/null | wc -l | tr -d ' ')"
	if [ "$actual_count" = "$expected_count" ]; then
		echo "PASS: $description (count=$actual_count)"
	else
		echo "FAIL: $description"
		echo "  expected count: $expected_count"
		echo "  actual count:   $actual_count"
		failures=$((failures + 1))
	fi
}

# ---------------------------------------------------------------
start_section "Status codes (covers DEREM-3 awk rewrite)"
# ---------------------------------------------------------------

assert_parse "single added file" \
	"$(write_nul_input 'A' 'docs/added.md')" \
	$'docs/added.md\tadded'

assert_parse "single modified file" \
	"$(write_nul_input 'M' 'docs/modified.md')" \
	$'docs/modified.md\tmodified'

assert_parse "type-changed treated as modified" \
	"$(write_nul_input 'T' 'docs/typechange.md')" \
	$'docs/typechange.md\tmodified'

assert_parse "single deleted file" \
	"$(write_nul_input 'D' 'docs/deleted.md')" \
	$'docs/deleted.md\tdeleted'

assert_parse "rename indexes the new path" \
	"$(write_nul_input 'R100' 'docs/old.md' 'docs/new.md')" \
	$'docs/new.md\trenamed'

assert_parse "multiple mixed records" \
	"$(write_nul_input 'A' 'docs/a.md' 'M' 'docs/b.md' 'D' 'docs/c.md')" \
	$'docs/a.md\tadded\ndocs/b.md\tmodified\ndocs/c.md\tdeleted'

assert_parse "rename interleaved with simple records" \
	"$(write_nul_input 'A' 'docs/a.md' 'R85' 'docs/old.md' 'docs/new.md' 'D' 'docs/c.md')" \
	$'docs/a.md\tadded\ndocs/new.md\trenamed\ndocs/c.md\tdeleted'

empty_file="$(mktemp -p "$TMPDIR_SELF")"
: >"$empty_file"
assert_parse "empty input emits nothing" "$empty_file" ""

# ---------------------------------------------------------------
start_section "Path escaping (covers DEREM-2 path-injection rewrite)"
# ---------------------------------------------------------------

assert_parse "path with spaces survives" \
	"$(write_nul_input 'M' 'docs/file with space.md')" \
	$'docs/file with space.md\tmodified'

assert_parse "path with double quote survives raw" \
	"$(write_nul_input 'M' 'docs/quote".md')" \
	$'docs/quote".md\tmodified'

assert_parse "path with backslash survives raw" \
	"$(write_nul_input 'M' 'docs/back\slash.md')" \
	$'docs/back\\slash.md\tmodified'

# Tab inside a path: the parser is line-based, so a tab character
# inside the path field will be preserved verbatim through awk; jq's
# split on tab then turns this into a multi-element array. We don't
# defend against this at the parser layer because real-world doc paths
# never contain tabs and git would normally quote-escape them anyway.
# Capture the current behavior so a future change is visible.
assert_parse "tab in path preserved raw by parser" \
	"$(write_nul_input 'M' $'docs/has\ttab.md')" \
	$'docs/has\ttab.md\tmodified'

assert_json "jq escapes double quote in JSON output" \
	"$(write_nul_input 'M' 'docs/quote".md')" \
	'[{"path":"docs/quote\".md","status":"modified"}]'

assert_json "jq escapes backslash in JSON output" \
	"$(write_nul_input 'M' 'docs/back\slash.md')" \
	'[{"path":"docs/back\\slash.md","status":"modified"}]'

assert_json "jq emits empty array for empty input" "$empty_file" "[]"

# ---------------------------------------------------------------
start_section "Unknown status codes (DEREM-21 structural guarantee)"
# ---------------------------------------------------------------

# This is the exact case the reviewer reproduced. Old design diverged:
# counter awk said 2, emitter awk said 1. New design has a single awk
# whose output is the source of truth for both.
assert_parse "unknown code consumes its path, valid record after is preserved" \
	"$(write_nul_input 'X' 'docs/a.md' 'M' 'docs/real.md')" \
	$'docs/real.md\tmodified'

assert_warns "unknown code emits a workflow warning" \
	"$(write_nul_input 'X' 'docs/a.md' 'M' 'docs/real.md')" \
	'::warning::Unknown git diff status X for docs/a.md'

assert_count_matches_emitter "count matches emitter when an unknown code is skipped" \
	"$(write_nul_input 'X' 'docs/a.md' 'M' 'docs/real.md')" \
	"1"

assert_count_matches_emitter "count matches emitter for a clean batch" \
	"$(write_nul_input 'A' 'docs/a.md' 'M' 'docs/b.md' 'D' 'docs/c.md')" \
	"3"

assert_count_matches_emitter "rename counts as one record, not two" \
	"$(write_nul_input 'R100' 'docs/old.md' 'docs/new.md')" \
	"1"

assert_count_matches_emitter "all unknown produces zero" \
	"$(write_nul_input 'X' 'docs/a.md' 'Y' 'docs/b.md')" \
	"0"

# ---------------------------------------------------------------
start_section "Sanity checks"
# ---------------------------------------------------------------

# 50-file boundary at the parser layer. The cap-at-50 decision lives
# above this parser in the workflow, but the parser must handle the
# boundary input correctly regardless.
big_input="$(mktemp -p "$TMPDIR_SELF")"
{
	for i in $(seq 1 50); do
		printf 'M\0docs/big-%02d.md\0' "$i"
	done
} >"$big_input"
assert_count_matches_emitter "50 records parse to 50 lines" "$big_input" "50"

if [ "$failures" -gt 0 ]; then
	echo
	echo "$failures test(s) failed."
	exit 1
fi

echo
echo "All tests passed."
