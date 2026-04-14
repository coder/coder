#!/usr/bin/env bats

# Tests for fetch-pr-context.sh using mocked gh responses.
# No network access required — all gh calls are intercepted by a
# stub script that returns known JSON fixtures.

SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
FETCH_SCRIPT="$SCRIPT_DIR/fetch-pr-context.sh"

setup() {
	TEST_TMPDIR="$(mktemp -d)"
	MOCK_BIN="$TEST_TMPDIR/bin"
	mkdir -p "$MOCK_BIN"

	# Create the gh stub. It inspects arguments to decide which
	# fixture to return.
	cat > "$MOCK_BIN/gh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail

# Join all arguments for pattern matching.
args="$*"

# gh pr view ... --json number,title,...
if [[ "$args" == *"pr view"* && "$args" == *"--json"* ]]; then
	# Extract the --json fields to decide which fixture to return.
	json_fields="${args##*--json }"
	json_fields="${json_fields%% *}"

	if [[ "$json_fields" == *"number,title"* ]]; then
		# PR metadata.
		cat <<'EOF'
{
  "number": 42,
  "title": "fix: improve widget handling",
  "body": "This PR fixes widget handling.",
  "author": {"login": "testuser"},
  "state": "OPEN",
  "baseRefName": "main",
  "headRefName": "fix-widgets",
  "url": "https://github.com/test/repo/pull/42",
  "headRefOid": "abc123def456",
  "baseRefOid": "000111222333"
}
EOF
	elif [[ "$json_fields" == *"reviews"* ]]; then
		cat <<'EOF'
{
  "reviews": [
    {
      "id": "R_100",
      "author": {"login": "reviewer1"},
      "state": "COMMENTED",
      "body": "Looks good, one P2 finding.",
      "submittedAt": "2025-01-15T10:00:00Z"
    },
    {
      "id": "R_101",
      "author": {"login": "reviewer2"},
      "state": "APPROVED",
      "body": "",
      "submittedAt": "2025-01-15T11:00:00Z"
    }
  ]
}
EOF
	elif [[ "$json_fields" == *"comments"* ]]; then
		cat <<'EOF'
{
  "comments": [
    {
      "id": "IC_200",
      "author": {"login": "testuser"},
      "body": "Thanks for the review!",
      "createdAt": "2025-01-15T12:00:00Z",
      "updatedAt": "2025-01-15T12:00:00Z"
    }
  ]
}
EOF
	elif [[ "$json_fields" == *"commits"* ]]; then
		cat <<'EOF'
{
  "commits": [
    {
      "oid": "abc123",
      "messageHeadline": "fix: improve widget handling",
      "authors": [{"login": "testuser"}],
      "committedDate": "2025-01-14T09:00:00Z"
    },
    {
      "oid": "def456",
      "messageHeadline": "fix: address review feedback",
      "authors": [{"login": "testuser"}],
      "committedDate": "2025-01-15T14:00:00Z"
    }
  ]
}
EOF
	fi
	exit 0
fi

# gh api --paginate repos/.../pulls/.../comments
if [[ "$args" == *"api"* && "$args" == *"/comments"* && "$args" != *"graphql"* ]]; then
	cat <<'EOF'
[
  {
    "id": 300,
    "node_id": "PRRC_300",
    "pull_request_review_id": 100,
    "user": {"login": "reviewer1"},
    "path": "widget.go",
    "line": 42,
    "side": "RIGHT",
    "start_line": null,
    "body": "**P2** Missing nil check",
    "in_reply_to_id": null,
    "created_at": "2025-01-15T10:01:00Z",
    "updated_at": "2025-01-15T10:01:00Z"
  },
  {
    "id": 301,
    "node_id": "PRRC_301",
    "pull_request_review_id": 100,
    "user": {"login": "testuser"},
    "path": "widget.go",
    "line": 42,
    "side": "RIGHT",
    "start_line": null,
    "body": "Good catch, will fix.",
    "in_reply_to_id": 300,
    "created_at": "2025-01-15T12:30:00Z",
    "updated_at": "2025-01-15T12:30:00Z"
  },
  {
    "id": 400,
    "node_id": "PRRC_400",
    "pull_request_review_id": 100,
    "user": {"login": "reviewer1"},
    "path": "handler.go",
    "line": 10,
    "side": "RIGHT",
    "start_line": null,
    "body": "**P3** Consider renaming this",
    "in_reply_to_id": null,
    "created_at": "2025-01-15T10:02:00Z",
    "updated_at": "2025-01-15T10:02:00Z"
  }
]
EOF
	exit 0
fi

# gh api graphql — review threads
if [[ "$args" == *"api"* && "$args" == *"graphql"* ]]; then
	cat <<'EOF'
{
  "data": {
    "repository": {
      "pullRequest": {
        "reviewThreads": {
          "pageInfo": {"hasNextPage": false, "endCursor": null},
          "nodes": [
            {
              "id": "PRT_thread_300",
              "isResolved": false,
              "comments": {"nodes": [{"databaseId": 300}]}
            },
            {
              "id": "PRT_thread_400",
              "isResolved": true,
              "comments": {"nodes": [{"databaseId": 400}]}
            }
          ]
        }
      }
    }
  }
}
EOF
	exit 0
fi

# gh repo view (for repo inference — shouldn't be called in tests
# since we always pass --repo, but just in case).
if [[ "$args" == *"repo view"* ]]; then
	echo '{"nameWithOwner": "test/repo"}'
	exit 0
fi

echo "gh stub: unrecognized call: $args" >&2
exit 1
STUB
	chmod +x "$MOCK_BIN/gh"

	# Prepend mock bin to PATH so fetch-pr-context.sh picks it up.
	export PATH="$MOCK_BIN:$PATH"
}

teardown() {
	rm -rf "$TEST_TMPDIR"
}

@test "output has all required top-level keys" {
	run "$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$TEST_TMPDIR/out.json"
	[ "$status" -eq 0 ]
	result="$(jq 'has("pr") and has("reviews") and has("review_comments") and has("issue_comments") and has("commits")' "$TEST_TMPDIR/out.json")"
	[ "$result" = "true" ]
}

@test "PR metadata fields are non-null" {
	"$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$TEST_TMPDIR/out.json"
	[ "$(jq '.pr.number != null' "$TEST_TMPDIR/out.json")" = "true" ]
	[ "$(jq '.pr.title != null' "$TEST_TMPDIR/out.json")" = "true" ]
	[ "$(jq '.pr.author != null' "$TEST_TMPDIR/out.json")" = "true" ]
	[ "$(jq '.pr.url != null' "$TEST_TMPDIR/out.json")" = "true" ]
	[ "$(jq '.pr.base_sha != null' "$TEST_TMPDIR/out.json")" = "true" ]
	[ "$(jq '.pr.head_sha != null' "$TEST_TMPDIR/out.json")" = "true" ]
	[ "$(jq '.pr.base_ref != null' "$TEST_TMPDIR/out.json")" = "true" ]
	[ "$(jq '.pr.head_ref != null' "$TEST_TMPDIR/out.json")" = "true" ]
	[ "$(jq '.pr.state != null' "$TEST_TMPDIR/out.json")" = "true" ]
}

@test "PR metadata values are correctly mapped" {
	"$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$TEST_TMPDIR/out.json"
	[ "$(jq -r '.pr.number' "$TEST_TMPDIR/out.json")" = "42" ]
	[ "$(jq -r '.pr.title' "$TEST_TMPDIR/out.json")" = "fix: improve widget handling" ]
	[ "$(jq -r '.pr.author' "$TEST_TMPDIR/out.json")" = "testuser" ]
	[ "$(jq -r '.pr.head_ref' "$TEST_TMPDIR/out.json")" = "fix-widgets" ]
	[ "$(jq -r '.pr.base_ref' "$TEST_TMPDIR/out.json")" = "main" ]
}

@test "resolved threads are excluded from review_comments" {
	"$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$TEST_TMPDIR/out.json"
	# Comment 400 (handler.go:10) is in a resolved thread — should be excluded.
	count_400="$(jq '[.review_comments[] | select(.id == 400)] | length' "$TEST_TMPDIR/out.json")"
	[ "$count_400" -eq 0 ]
	# Comments 300 and 301 (unresolved thread) should survive.
	count_unresolved="$(jq '[.review_comments[] | select(.id == 300 or .id == 301)] | length' "$TEST_TMPDIR/out.json")"
	[ "$count_unresolved" -eq 2 ]
}

@test "threading preserved — in_reply_to_id links exist for replies" {
	"$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$TEST_TMPDIR/out.json"
	# Comment 301 replies to 300.
	reply_to="$(jq '.review_comments[] | select(.id == 301) | .in_reply_to_id' "$TEST_TMPDIR/out.json")"
	[ "$reply_to" = "300" ]
	# Comment 300 is a root (null in_reply_to_id).
	root="$(jq '.review_comments[] | select(.id == 300) | .in_reply_to_id' "$TEST_TMPDIR/out.json")"
	[ "$root" = "null" ]
}

@test "thread_id is populated from GraphQL data" {
	"$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$TEST_TMPDIR/out.json"
	# Comment 300 is root of thread PRT_thread_300.
	thread_id="$(jq -r '.review_comments[] | select(.id == 300) | .thread_id' "$TEST_TMPDIR/out.json")"
	[ "$thread_id" = "PRT_thread_300" ]
	# Comment 301 replies to 300, so should also get PRT_thread_300.
	thread_id_reply="$(jq -r '.review_comments[] | select(.id == 301) | .thread_id' "$TEST_TMPDIR/out.json")"
	[ "$thread_id_reply" = "PRT_thread_300" ]
}

@test "reviews are correctly shaped" {
	"$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$TEST_TMPDIR/out.json"
	[ "$(jq '.reviews | length' "$TEST_TMPDIR/out.json")" = "2" ]
	[ "$(jq -r '.reviews[0].author' "$TEST_TMPDIR/out.json")" = "reviewer1" ]
	[ "$(jq -r '.reviews[0].state' "$TEST_TMPDIR/out.json")" = "COMMENTED" ]
	[ "$(jq -r '.reviews[1].state' "$TEST_TMPDIR/out.json")" = "APPROVED" ]
}

@test "issue comments are correctly shaped" {
	"$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$TEST_TMPDIR/out.json"
	[ "$(jq '.issue_comments | length' "$TEST_TMPDIR/out.json")" = "1" ]
	[ "$(jq -r '.issue_comments[0].author' "$TEST_TMPDIR/out.json")" = "testuser" ]
	[ "$(jq -r '.issue_comments[0].body' "$TEST_TMPDIR/out.json")" = "Thanks for the review!" ]
}

@test "commits are correctly shaped" {
	"$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$TEST_TMPDIR/out.json"
	[ "$(jq '.commits | length' "$TEST_TMPDIR/out.json")" = "2" ]
	[ "$(jq -r '.commits[0].sha' "$TEST_TMPDIR/out.json")" = "abc123" ]
	[ "$(jq -r '.commits[0].author' "$TEST_TMPDIR/out.json")" = "testuser" ]
}

@test "--output flag writes to specified file" {
	outfile="$TEST_TMPDIR/specific-output.json"
	run "$FETCH_SCRIPT" --pr 42 --repo "test/repo" --output "$outfile"
	[ "$status" -eq 0 ]
	[ -f "$outfile" ]
	# Must be valid JSON with correct structure.
	jq . "$outfile" >/dev/null
	result="$(jq 'has("pr") and has("reviews")' "$outfile")"
	[ "$result" = "true" ]
}

@test "stdout output when --output is not provided" {
	run bash -c "'$FETCH_SCRIPT' --pr 42 --repo 'test/repo' 2>/dev/null"
	[ "$status" -eq 0 ]
	# Output should be valid JSON.
	echo "$output" | jq . >/dev/null
	result="$(echo "$output" | jq 'has("pr")')"
	[ "$result" = "true" ]
}
