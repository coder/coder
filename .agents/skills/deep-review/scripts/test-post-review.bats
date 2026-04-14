#!/usr/bin/env bats

# Tests for post-review.sh

SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
POST_REVIEW="$SCRIPT_DIR/post-review.sh"

# A simple diff used across multiple tests.
# Positions:
#   @@ header       = position 1
#   " package main" = position 2 (line 1)
#   " "             = position 3 (line 2)
#   " func hello()" = position 4 (line 3)
#   "+    fmt..."   = position 5 (line 4)
#   "     return"   = position 6 (line 5)
#   " }"            = position 7 (line 6)
DIFF='diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -1,5 +1,6 @@
 package main
 
 func hello() {
+    fmt.Println("hello")
     return
 }'

setup() {
	TEST_TMPDIR="$(mktemp -d)"
	DIFF_FILE="$TEST_TMPDIR/pr.diff"
	echo "$DIFF" >"$DIFF_FILE"
}

teardown() {
	rm -rf "$TEST_TMPDIR"
}

@test "--dry-run outputs valid JSON with computed positions" {
	cat >"$TEST_TMPDIR/input.json" <<'EOF'
{
  "event": "COMMENT",
  "body": "Review summary",
  "comments": [
    {
      "path": "test.go",
      "line": 4,
      "body": "**P1** This line is suspicious."
    }
  ]
}
EOF

	run "$POST_REVIEW" --pr 1 --repo owner/repo --input "$TEST_TMPDIR/input.json" \
		--diff "$DIFF_FILE" --dry-run
	[ "$status" -eq 0 ]

	# First line of output is the review payload — must be valid JSON.
	payload="$(echo "$output" | head -n1)"
	echo "$payload" | jq . >/dev/null

	# Must have a position field (not line).
	[ "$(echo "$payload" | jq '.comments[0].position')" != "null" ]
	[ "$(echo "$payload" | jq '.comments[0].line // empty')" = "" ]
}

@test "line-to-position translation matches expected value" {
	# Line 4 in our diff is the added line "+    fmt.Println..."
	# which is at position 5.
	cat >"$TEST_TMPDIR/input.json" <<'EOF'
{
  "event": "COMMENT",
  "body": "Checking positions",
  "comments": [
    {
      "path": "test.go",
      "line": 4,
      "body": "Comment on the added line."
    }
  ]
}
EOF

	run "$POST_REVIEW" --pr 1 --repo owner/repo --input "$TEST_TMPDIR/input.json" \
		--diff "$DIFF_FILE" --dry-run
	[ "$status" -eq 0 ]

	payload="$(echo "$output" | head -n1)"
	[ "$(echo "$payload" | jq '.comments[0].position')" = "5" ]
}

@test "unmappable line falls back to file-level comment" {
	cat >"$TEST_TMPDIR/input.json" <<'EOF'
{
  "event": "COMMENT",
  "body": "Fallback test",
  "comments": [
    {
      "path": "test.go",
      "line": 999,
      "body": "This line is way out of range."
    }
  ]
}
EOF

	run "$POST_REVIEW" --pr 1 --repo owner/repo --input "$TEST_TMPDIR/input.json" \
		--diff "$DIFF_FILE" --dry-run
	[ "$status" -eq 0 ]

	# The output may contain a WARNING line on stderr (captured by bats).
	# Extract the first line that starts with '{' to get the JSON payload.
	payload="$(echo "$output" | grep '^{'  | head -n1)"

	# Position should be 1 (fallback).
	[ "$(echo "$payload" | jq '.comments[0].position')" = "1" ]

	# Body should be prefixed with [Line 999].
	body="$(echo "$payload" | jq -r '.comments[0].body')"
	[[ "$body" == "[Line 999] "* ]]
}

@test "replies are separated from review comments in output" {
	cat >"$TEST_TMPDIR/input.json" <<'EOF'
{
  "event": "COMMENT",
  "body": "Review with replies",
  "comments": [
    {
      "path": "test.go",
      "line": 4,
      "body": "Inline comment."
    }
  ],
  "replies": [
    {
      "in_reply_to_id": 456,
      "body": "Reply to existing comment."
    }
  ]
}
EOF

	run "$POST_REVIEW" --pr 1 --repo owner/repo --input "$TEST_TMPDIR/input.json" \
		--diff "$DIFF_FILE" --dry-run
	[ "$status" -eq 0 ]

	# First line is the review payload.
	review="$(echo "$output" | head -n1)"
	[ "$(echo "$review" | jq -r '.event')" = "COMMENT" ]
	[ "$(echo "$review" | jq '.comments | length')" = "1" ]

	# Second line is the reply action.
	reply="$(echo "$output" | sed -n '2p')"
	[ "$(echo "$reply" | jq -r '.action')" = "reply" ]
	[ "$(echo "$reply" | jq '.in_reply_to_id')" = "456" ]
	[ "$(echo "$reply" | jq -r '.body')" = "Reply to existing comment." ]
}

@test "missing required fields exits non-zero" {
	# Missing body.
	cat >"$TEST_TMPDIR/input.json" <<'EOF'
{
  "event": "COMMENT",
  "comments": []
}
EOF

	run "$POST_REVIEW" --pr 1 --repo owner/repo --input "$TEST_TMPDIR/input.json" \
		--diff "$DIFF_FILE" --dry-run
	[ "$status" -ne 0 ]
}

@test "empty comments array posts review body only" {
	cat >"$TEST_TMPDIR/input.json" <<'EOF'
{
  "event": "COMMENT",
  "body": "All good, no inline comments.",
  "comments": []
}
EOF

	run "$POST_REVIEW" --pr 1 --repo owner/repo --input "$TEST_TMPDIR/input.json" \
		--diff "$DIFF_FILE" --dry-run
	[ "$status" -eq 0 ]

	payload="$(echo "$output" | head -n1)"
	echo "$payload" | jq . >/dev/null

	# Review body is present.
	[ "$(echo "$payload" | jq -r '.body')" = "All good, no inline comments." ]

	# No comments key in the payload.
	[ "$(echo "$payload" | jq 'has("comments")')" = "false" ]
}

@test "resolve_thread_ids included in --dry-run output" {
	cat >"$TEST_TMPDIR/input.json" <<'EOF'
{
  "event": "COMMENT",
  "body": "Resolving threads.",
  "resolve_thread_ids": ["thread-node-id-1", "thread-node-id-2"]
}
EOF

	run "$POST_REVIEW" --pr 1 --repo owner/repo --input "$TEST_TMPDIR/input.json" \
		--diff "$DIFF_FILE" --dry-run
	[ "$status" -eq 0 ]

	# First line is the review payload.
	# Lines 2 and 3 should be resolve_thread actions.
	resolve1="$(echo "$output" | sed -n '2p')"
	resolve2="$(echo "$output" | sed -n '3p')"

	[ "$(echo "$resolve1" | jq -r '.action')" = "resolve_thread" ]
	[ "$(echo "$resolve1" | jq -r '.thread_id')" = "thread-node-id-1" ]

	[ "$(echo "$resolve2" | jq -r '.action')" = "resolve_thread" ]
	[ "$(echo "$resolve2" | jq -r '.thread_id')" = "thread-node-id-2" ]
}
