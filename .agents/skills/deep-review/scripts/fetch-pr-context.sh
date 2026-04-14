#!/usr/bin/env bash

# Fetch all PR context (metadata, reviews, inline comments, issue comments,
# commits) into a single structured JSON file. Resolved inline review
# threads are excluded from the output.
#
# Usage: ./fetch-pr-context.sh --pr <number> [--repo <owner/repo>] [--output <path>]

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../../../scripts/lib.sh"

dependencies gh jq

pr=""
repo=""
output=""

args="$(getopt -o "" -l pr:,repo:,output: -- "$@")"
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
	--output)
		output="$2"
		shift 2
		;;
	--)
		shift
		break
		;;
	*)
		error "Unrecognized option: $1"
		;;
	esac
done

if [[ -z "$pr" ]]; then
	error "--pr is a required parameter"
fi

# Infer repo from the current git context if not provided.
if [[ -z "$repo" ]]; then
	repo="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
	log "Inferred repo: $repo"
fi

# Split repo into owner and reponame for GraphQL queries.
owner="${repo%%/*}"
reponame="${repo##*/}"

log "Fetching PR #$pr from $repo..."

# 1. Fetch PR metadata.
log "  Fetching PR metadata..."
pr_json="$(gh pr view "$pr" --repo "$repo" \
	--json number,title,body,author,state,baseRefName,headRefName,url,headRefOid,baseRefOid)"
pr_json="$(echo "$pr_json" | jq '{
	number: .number,
	title: .title,
	body: .body,
	author: .author.login,
	state: .state,
	base_sha: .baseRefOid,
	head_sha: .headRefOid,
	base_ref: .baseRefName,
	head_ref: .headRefName,
	url: .url
}')"

# 2. Fetch reviews.
log "  Fetching reviews..."
reviews_json="$(gh pr view "$pr" --repo "$repo" --json reviews | jq '[
	.reviews[] | {
		id: .id,
		author: .author.login,
		state: .state,
		body: .body,
		submitted_at: .submittedAt
	}
]')"

# 3. Fetch issue comments.
log "  Fetching issue comments..."
issue_comments_json="$(gh pr view "$pr" --repo "$repo" --json comments | jq '[
	.comments[] | {
		id: .id,
		author: .author.login,
		body: .body,
		created_at: .createdAt,
		updated_at: .updatedAt
	}
]')"

# 4. Fetch inline review comments (paginated via gh api).
log "  Fetching inline review comments..."
review_comments_raw="$(gh api --paginate "repos/$repo/pulls/$pr/comments" | jq '[
	.[] | {
		id: .id,
		node_id: .node_id,
		review_id: .pull_request_review_id,
		author: .user.login,
		path: .path,
		line: .line,
		side: .side,
		start_line: .start_line,
		body: .body,
		in_reply_to_id: .in_reply_to_id,
		created_at: .created_at,
		updated_at: .updated_at
	}
]')"

# 5. Fetch resolved thread status via GraphQL with pagination.
log "  Fetching review thread resolution status..."
all_threads="[]"
has_next_page="true"
cursor="null"

while [[ "$has_next_page" == "true" ]]; do
	if [[ "$cursor" == "null" ]]; then
		after_clause=""
	else
		after_clause=", after: $cursor"
	fi

	graphql_result="$(gh api graphql -f query="
		query {
			repository(owner: \"$owner\", name: \"$reponame\") {
				pullRequest(number: $pr) {
					reviewThreads(first: 100${after_clause}) {
						pageInfo { hasNextPage endCursor }
						nodes {
							id
							isResolved
							comments(first: 1) {
								nodes { databaseId }
							}
						}
					}
				}
			}
		}
	")"

	page_threads="$(echo "$graphql_result" | jq '.data.repository.pullRequest.reviewThreads.nodes')"
	all_threads="$(jq -n --argjson a "$all_threads" --argjson b "$page_threads" '$a + $b')"

	has_next_page="$(echo "$graphql_result" | jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.hasNextPage')"
	cursor="$(echo "$graphql_result" | jq '.data.repository.pullRequest.reviewThreads.pageInfo.endCursor')"
done

# 6. Build resolved set and thread ID mapping from GraphQL results.
#    resolved_set: root comment databaseId values where isResolved == true
#    thread_map: root comment databaseId → thread GraphQL node ID
resolved_set="$(echo "$all_threads" | jq '[
	.[] | select(.isResolved == true) | .comments.nodes[0].databaseId
] | map(select(. != null))')"

thread_map="$(echo "$all_threads" | jq '[
	.[] | {
		key: (.comments.nodes[0].databaseId | tostring),
		value: .id
	} | select(.key != "null")
] | from_entries')"

# 7. Filter inline review comments — exclude resolved threads.
#    Also add thread_id to each surviving comment.
log "  Filtering resolved threads..."
review_comments_json="$(jq -n \
	--argjson comments "$review_comments_raw" \
	--argjson resolved "$resolved_set" \
	--argjson thread_map "$thread_map" '
	# Build a set from the resolved array for O(1) lookup.
	($resolved | [.[] | tostring] | INDEX(.[]; .)) as $resolved_set |
	[
		$comments[] |
		# Determine root comment ID for this comment.
		(if .in_reply_to_id == null then .id else .in_reply_to_id end) as $root_id |
		# Skip if the root comment is in the resolved set.
		select(($root_id | tostring) as $rid | $resolved_set[$rid] == null) |
		# Add thread_id from the thread map.
		. + { thread_id: ($thread_map[$root_id | tostring] // null) }
	]
')"

# 8. Fetch commits.
log "  Fetching commits..."
commits_json="$(gh pr view "$pr" --repo "$repo" --json commits | jq '[
	.commits[] | {
		sha: .oid,
		message: .messageHeadline,
		author: .authors[0].login,
		date: .committedDate
	}
]')"

# 9. Assemble final JSON.
log "  Assembling output..."
final_json="$(jq -n \
	--argjson pr "$pr_json" \
	--argjson reviews "$reviews_json" \
	--argjson review_comments "$review_comments_json" \
	--argjson issue_comments "$issue_comments_json" \
	--argjson commits "$commits_json" \
	'{
		pr: $pr,
		reviews: $reviews,
		review_comments: $review_comments,
		issue_comments: $issue_comments,
		commits: $commits
	}')"

# 10. Write output.
if [[ -n "$output" ]]; then
	echo "$final_json" >"$output"
	log "Output written to $output"
else
	echo "$final_json"
fi
