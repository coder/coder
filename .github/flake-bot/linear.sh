#!/usr/bin/env bash
# shellcheck disable=SC2016  # GraphQL queries use $var GraphQL variables, not shell expansion.
# flake-bot Linear helper.
#
# A small, reviewable wrapper around the Linear GraphQL API so the agent does
# not have to hand-write GraphQL. Authenticates with the LINEAR_ACCESS_KEY
# secret (the same key used by linear/linear-release-action).
#
# Usage:
#   linear.sh team-id <TEAM_KEY>
#   linear.sh label-id <TEAM_KEY> <LABEL_NAME>
#   linear.sh search "<query>"
#   linear.sh get <IDENTIFIER>                      # e.g. ENG-2862
#   linear.sh create --team <KEY> --title <T> --body-file <PATH> \
#                    [--label <NAME>] [--priority <N>]
#   linear.sh comment <IDENTIFIER> --body-file <PATH>
#   linear.sh set-state <IDENTIFIER> <STATE_NAME>   # e.g. Triage
#
# All large text (issue/comment bodies) is passed via --body-file to avoid
# shell-quoting problems with multi-line Markdown. JSON is built with jq so
# values are always safely escaped.
set -euo pipefail

API="https://api.linear.app/graphql"

die() {
	echo "linear.sh: $*" >&2
	exit 1
}

command -v jq >/dev/null || die "jq is required"
command -v curl >/dev/null || die "curl is required"

# gql <query> <variables-json>
# Performs a GraphQL request and prints the .data object. Exits non-zero and
# prints the raw response if Linear returns an "errors" array.
gql() {
	[[ -n "${LINEAR_ACCESS_KEY:-}" ]] || die "LINEAR_ACCESS_KEY is not set"
	local query="$1" variables="${2:-}" body resp
	[[ -n "$variables" ]] || variables='{}'
	body="$(jq -cn --arg q "$query" --argjson v "$variables" '{query:$q, variables:$v}')"
	resp="$(curl -sS -X POST "$API" \
		-H "Authorization: ${LINEAR_ACCESS_KEY}" \
		-H "Content-Type: application/json" \
		--data "$body")"
	if [[ "$(jq 'has("errors")' <<<"$resp")" == "true" ]]; then
		echo "$resp" >&2
		die "GraphQL error"
	fi
	jq '.data' <<<"$resp"
}

team_id() {
	local key="$1" id
	id="$(gql 'query($key:String!){teams(filter:{key:{eq:$key}}){nodes{id}}}' \
		"$(jq -cn --arg key "$key" '{key:$key}')" |
		jq -r '.teams.nodes[0].id // empty')"
	[[ -n "$id" ]] || die "no team with key '$key'"
	echo "$id"
}

label_id() {
	local key="$1" name="$2" tid
	tid="$(team_id "$key")"
	gql 'query($id:String!){team(id:$id){labels(first:250){nodes{id name}}}}' \
		"$(jq -cn --arg id "$tid" '{id:$id}')" |
		jq -r --arg n "$name" \
			'.team.labels.nodes[] | select(.name|ascii_downcase==($n|ascii_downcase)) | .id' |
		head -n1
}

search() {
	local q="$1"
	gql 'query($q:String!){issues(filter:{title:{containsIgnoreCase:$q}},first:25){nodes{id identifier title url state{name type} team{key} assignee{name}}}}' \
		"$(jq -cn --arg q "$q" '{q:$q}')" |
		jq '.issues.nodes'
}

# Resolve an ENG-1234 style identifier to the issue node (id + metadata).
get() {
	local ident="$1" key="${1%%-*}" num="${1##*-}"
	[[ "$key" != "$ident" && -n "$num" ]] || die "get: expected an identifier like ENG-123"
	gql 'query($k:String!,$n:Float!){issues(filter:{team:{key:{eq:$k}},number:{eq:$n}}){nodes{id identifier title url state{name type} team{key} assignee{name}}}}' \
		"$(jq -cn --arg k "$key" --argjson n "$num" '{k:$k,n:$n}')" |
		jq '.issues.nodes[0] // empty'
}

issue_uuid() {
	local ident="$1" id
	id="$(get "$ident" | jq -r '.id // empty')"
	[[ -n "$id" ]] || die "no issue with identifier '$ident'"
	echo "$id"
}

create() {
	local team="" title="" body_file="" label="" priority=""
	while [[ $# -gt 0 ]]; do
		case "$1" in
		--team) team="$2"; shift 2 ;;
		--title) title="$2"; shift 2 ;;
		--body-file) body_file="$2"; shift 2 ;;
		--label) label="$2"; shift 2 ;;
		--priority) priority="$2"; shift 2 ;;
		*) die "create: unknown arg '$1'" ;;
		esac
	done
	[[ -n "$team" && -n "$title" && -n "$body_file" ]] ||
		die "create requires --team, --title, --body-file"
	[[ -f "$body_file" ]] || die "create: body file '$body_file' not found"

	local tid lid="" desc input
	tid="$(team_id "$team")"
	if [[ -n "$label" ]]; then
		lid="$(label_id "$team" "$label" || true)"
		[[ -n "$lid" ]] || echo "linear.sh: warning: no '$label' label on $team" >&2
	fi
	desc="$(cat "$body_file")"

	input="$(jq -cn \
		--arg teamId "$tid" \
		--arg title "$title" \
		--arg desc "$desc" \
		'{teamId:$teamId, title:$title, description:$desc}')"
	if [[ -n "$lid" ]]; then
		input="$(jq -c --arg l "$lid" '. + {labelIds:[$l]}' <<<"$input")"
	fi
	if [[ -n "$priority" ]]; then
		input="$(jq -c --argjson p "$priority" '. + {priority:$p}' <<<"$input")"
	fi

	gql 'mutation($input:IssueCreateInput!){issueCreate(input:$input){success issue{identifier url}}}' \
		"$(jq -cn --argjson input "$input" '{input:$input}')" |
		jq '.issueCreate.issue'
}

comment() {
	local ident="$1"; shift
	local body_file=""
	while [[ $# -gt 0 ]]; do
		case "$1" in
		--body-file) body_file="$2"; shift 2 ;;
		*) die "comment: unknown arg '$1'" ;;
		esac
	done
	[[ -n "$body_file" && -f "$body_file" ]] ||
		die "comment requires --body-file <existing path>"

	local id body
	id="$(issue_uuid "$ident")"
	body="$(cat "$body_file")"
	gql 'mutation($input:CommentCreateInput!){commentCreate(input:$input){success comment{url}}}' \
		"$(jq -cn --arg issueId "$id" --arg body "$body" '{input:{issueId:$issueId, body:$body}}')" |
		jq '.commentCreate.comment'
}

set_state() {
	local ident="$1" state="$2" key="${1%%-*}" tid sid iid
	iid="$(issue_uuid "$ident")"
	tid="$(team_id "$key")"
	sid="$(gql 'query($id:String!){team(id:$id){states(first:100){nodes{id name}}}}' \
		"$(jq -cn --arg id "$tid" '{id:$id}')" |
		jq -r --arg n "$state" '.team.states.nodes[] | select(.name|ascii_downcase==($n|ascii_downcase)) | .id' | head -n1)"
	[[ -n "$sid" ]] || die "no state named '$state'"
	gql 'mutation($id:String!,$sid:String!){issueUpdate(id:$id,input:{stateId:$sid}){success}}' \
		"$(jq -cn --arg id "$iid" --arg sid "$sid" '{id:$id, sid:$sid}')" |
		jq '.issueUpdate.success'
}

usage() {
	# Print the leading comment header (skip shebang + shellcheck directive),
	# stopping before the first real command.
	sed -n '3,/^set -euo pipefail/p' "$0" | sed '/^set -euo pipefail/d; s/^# \{0,1\}//'
}

cmd="${1:-}"
[[ -n "$cmd" ]] || { usage; exit 1; }
shift || true
case "$cmd" in
team-id) team_id "$@" ;;
label-id) label_id "$@" ;;
search) search "$@" ;;
get) get "$@" ;;
create) create "$@" ;;
comment) comment "$@" ;;
set-state) set_state "$@" ;;
-h | --help | help) usage ;;
*) die "unknown command '$cmd' (try --help)" ;;
esac
