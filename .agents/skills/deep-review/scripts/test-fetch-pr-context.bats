#!/usr/bin/env bats

# Integration tests for fetch-pr-context.sh.
# These tests require network access and run against real PRs from
# coder/coder.

SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
FETCH_SCRIPT="$SCRIPT_DIR/fetch-pr-context.sh"

# PR 24310: merged, 8 reviews, 7 commits, 3 resolved threads (all
# review comments filtered out). Good for resolved-thread filtering.
# PR 24300: open, 10 reviews, 7 commits, 9 unresolved threads with
# replies, 1 issue comment. Good for general structure and threading.
TEST_REPO="coder/coder"

setup() {
	TEST_TMPDIR="$(mktemp -d)"
	# Cache directory shared across tests in a single bats run. We
	# fetch each PR at most once and reuse the cached JSON.
	CACHE_DIR="${BATS_TMPDIR}/fetch-pr-cache"
	mkdir -p "$CACHE_DIR"
}

teardown() {
	rm -rf "$TEST_TMPDIR"
}

# Helper: fetch a PR once, caching the result. Subsequent calls for
# the same PR reuse the cached file.
fetch_cached() {
	local pr="$1"
	local cached="$CACHE_DIR/pr-${pr}.json"
	if [[ ! -f "$cached" ]]; then
		"$FETCH_SCRIPT" --pr "$pr" --repo "$TEST_REPO" --output "$cached"
	fi
	echo "$cached"
}

@test "output has all required top-level keys" {
	local cached
	cached="$(fetch_cached 24300)"
	result="$(jq 'has("pr") and has("reviews") and has("review_comments") and has("issue_comments") and has("commits")' "$cached")"
	[ "$result" = "true" ]
}

@test "PR metadata fields are non-null" {
	local cached
	cached="$(fetch_cached 24300)"
	[ "$(jq '.pr.number != null' "$cached")" = "true" ]
	[ "$(jq '.pr.title != null' "$cached")" = "true" ]
	[ "$(jq '.pr.author != null' "$cached")" = "true" ]
	[ "$(jq '.pr.url != null' "$cached")" = "true" ]
	[ "$(jq '.pr.base_sha != null' "$cached")" = "true" ]
	[ "$(jq '.pr.head_sha != null' "$cached")" = "true" ]
	[ "$(jq '.pr.base_ref != null' "$cached")" = "true" ]
	[ "$(jq '.pr.head_ref != null' "$cached")" = "true" ]
	[ "$(jq '.pr.state != null' "$cached")" = "true" ]
}

@test "resolved threads are excluded from review_comments" {
	# PR 24310 has 3 review threads, ALL resolved. After filtering
	# the output should contain zero review comments.
	local cached
	cached="$(fetch_cached 24310)"
	count="$(jq '.review_comments | length' "$cached")"
	[ "$count" -eq 0 ]
	# Sanity: the PR should still have reviews and commits to prove
	# the fetch actually worked.
	[ "$(jq '.reviews | length' "$cached")" -gt 0 ]
	[ "$(jq '.commits | length' "$cached")" -gt 0 ]
}

@test "threading preserved — in_reply_to_id links exist" {
	# PR 24300 has unresolved threads with reply comments.
	local cached
	cached="$(fetch_cached 24300)"
	total="$(jq '.review_comments | length' "$cached")"
	replies="$(jq '[.review_comments[] | select(.in_reply_to_id != null)] | length' "$cached")"
	# The PR should have some comments overall.
	[ "$total" -gt 0 ]
	# At least one reply should be present.
	[ "$replies" -gt 0 ]
}

@test "--output flag writes to specified file" {
	local outfile="$TEST_TMPDIR/out.json"
	run "$FETCH_SCRIPT" --pr 24300 --repo "$TEST_REPO" --output "$outfile"
	[ "$status" -eq 0 ]
	[ -f "$outfile" ]
	# Must be valid JSON.
	jq . "$outfile" >/dev/null
	# Must have all top-level keys.
	result="$(jq 'has("pr") and has("reviews") and has("review_comments") and has("issue_comments") and has("commits")' "$outfile")"
	[ "$result" = "true" ]
}

@test "invalid PR number exits non-zero" {
	run "$FETCH_SCRIPT" --pr 999999999 --repo "$TEST_REPO"
	[ "$status" -ne 0 ]
}
