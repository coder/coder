#!/usr/bin/env bash

# This script posts a structured review to a GitHub PR. It accepts a
# JSON file describing the review body, inline comments (with source
# file line numbers — not diff positions), replies to existing comments,
# and thread IDs to resolve. Diff position computation is handled
# internally via diff-position-map.sh.
#
# Usage:
#   ./post-review.sh --pr 1234 --input review.json [--repo owner/repo] \
#       [--diff pr.diff] [--dry-run]

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../../../scripts/lib.sh"

dependencies gh jq

pr=""
repo=""
input=""
diff_file=""
dry_run=0

args="$(getopt -o "" -l pr:,repo:,input:,diff:,dry-run -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--pr)
		pr="$2"
		shift 2
		;;
	--repo)
		repo="$2"
		shift 2
		;;
	--input)
		input="$2"
		shift 2
		;;
	--diff)
		diff_file="$2"
		shift 2
		;;
	--dry-run)
		dry_run=1
		shift
		;;
	--)
		shift
		break
		;;
	*)
		error "Unexpected argument: $1"
		;;
	esac
done

if [[ -z "$pr" ]]; then
	error "--pr is required"
fi
if [[ -z "$input" ]]; then
	error "--input is required"
fi
if [[ ! -f "$input" ]]; then
	error "Input file does not exist: $input"
fi

# Infer repo if not provided.
if [[ -z "$repo" ]]; then
	repo="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
fi

# Validate input JSON.
event="$(jq -r '.event // empty' "$input")"
if [[ -z "$event" ]]; then
	error "Input JSON must contain an 'event' field"
fi
if [[ "$event" != "COMMENT" ]]; then
	error "Only 'COMMENT' event is supported (got '$event')"
fi

body="$(jq -r '.body // empty' "$input")"
if [[ -z "$body" ]]; then
	error "Input JSON must contain a non-empty 'body' field"
fi

# Set up temp directory for intermediate files.
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

# Get the diff.
if [[ -z "$diff_file" ]]; then
	diff_file="$tmpdir/pr.diff"
	gh pr diff "$pr" --repo "$repo" >"$diff_file"
fi

# Compute the position map via sibling script.
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
position_map="$("$SCRIPT_DIR/diff-position-map.sh" --diff "$diff_file")"

# Extract comments array (default to empty).
comments="$(jq -c '.comments // []' "$input")"
num_comments="$(echo "$comments" | jq 'length')"

# Build the review comments array with positions.
review_comments="[]"
for ((i = 0; i < num_comments; i++)); do
	path="$(echo "$comments" | jq -r ".[$i].path")"
	line="$(echo "$comments" | jq -r ".[$i].line")"
	comment_body="$(echo "$comments" | jq -r ".[$i].body")"

	# Look up position from the map.
	position="$(echo "$position_map" | jq --arg path "$path" --arg line "$line" '.[$path][$line] // empty')"

	if [[ -n "$position" ]]; then
		# Position found — add as inline comment.
		review_comments="$(echo "$review_comments" | jq \
			--arg path "$path" \
			--argjson position "$position" \
			--arg body "$comment_body" \
			'. + [{"path": $path, "position": $position, "body": $body}]')"
	else
		# Line not in diff — fall back to file-level comment at position 1.
		log "WARNING: Line $line of $path not found in diff, falling back to file-level comment"
		fallback_body="[Line $line] $comment_body"
		review_comments="$(echo "$review_comments" | jq \
			--arg path "$path" \
			--arg body "$fallback_body" \
			'. + [{"path": $path, "position": 1, "body": $body}]')"
	fi
done

# Build the review payload.
if [[ "$(echo "$review_comments" | jq 'length')" -gt 0 ]]; then
	jq -n \
		--arg event "$event" \
		--arg body "$body" \
		--argjson comments "$review_comments" \
		'{"event": $event, "body": $body, "comments": $comments}' \
		>"$tmpdir/review_payload.json"
else
	jq -n \
		--arg event "$event" \
		--arg body "$body" \
		'{"event": $event, "body": $body}' \
		>"$tmpdir/review_payload.json"
fi

# Extract replies and resolve_thread_ids (default to empty arrays).
replies="$(jq -c '.replies // []' "$input")"
num_replies="$(echo "$replies" | jq 'length')"

resolve_thread_ids="$(jq -c '.resolve_thread_ids // []' "$input")"
num_resolve="$(echo "$resolve_thread_ids" | jq 'length')"

if [[ "$dry_run" == 1 ]]; then
	# Print the review payload as compact JSON (one line).
	jq -c . "$tmpdir/review_payload.json"

	# Print each reply as a separate compact JSON object.
	for ((i = 0; i < num_replies; i++)); do
		reply_id="$(echo "$replies" | jq ".[$i].in_reply_to_id")"
		reply_body="$(echo "$replies" | jq -r ".[$i].body")"
		jq -cn \
			--arg action "reply" \
			--argjson in_reply_to_id "$reply_id" \
			--arg body "$reply_body" \
			'{"action": $action, "in_reply_to_id": $in_reply_to_id, "body": $body}'
	done

	# Print each thread resolution as compact JSON.
	for ((i = 0; i < num_resolve; i++)); do
		thread_id="$(echo "$resolve_thread_ids" | jq -r ".[$i]")"
		jq -cn \
			--arg action "resolve_thread" \
			--arg thread_id "$thread_id" \
			'{"action": $action, "thread_id": $thread_id}'
	done

	exit 0
fi

# Post the review.
gh api -X POST "repos/$repo/pulls/$pr/reviews" --input "$tmpdir/review_payload.json"

# Post replies.
for ((i = 0; i < num_replies; i++)); do
	reply_id="$(echo "$replies" | jq ".[$i].in_reply_to_id")"
	reply_body="$(echo "$replies" | jq -r ".[$i].body")"
	gh api -X POST "repos/$repo/pulls/$pr/comments" \
		-f "body=$reply_body" -F "in_reply_to=$reply_id"
done

# Resolve threads via GraphQL.
for ((i = 0; i < num_resolve; i++)); do
	thread_id="$(echo "$resolve_thread_ids" | jq -r ".[$i]")"
	# shellcheck disable=SC2016
	gh api graphql -f query='mutation($id: ID!) {
		resolveReviewThread(input: {threadId: $id}) {
			thread { isResolved }
		}
	}' -f id="$thread_id"
done
