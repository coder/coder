#!/usr/bin/env bash
#
# Checks that every migration a pull request adds under
# coderd/database/migrations uses a number strictly greater than the
# highest number already present in the checked-out base branch. Two
# PRs can each pass CI with the same number and then collide on main,
# because CI runs against the base branch as it was when the PR was
# last pushed. Rerunning this check on every push to the base branch
# and reporting through a commit status keeps the merge button
# accurate without rerunning the full test suite.
#
# Usage:
#   check_migration_numbers.sh [--post-status] <pr-number>...
#   check_migration_numbers.sh [--post-status] --all
#
# --all checks every open PR whose base is the current branch and
# only reports PRs that add migrations. It never exits non-zero for
# a failing PR; the commit status is the signal. With explicit PR
# numbers the script exits 1 if any PR fails or cannot be checked.
#
# --post-status writes a commit status named "migration-numbers" to
# each PR's head commit. Set RUN_URL to link the status to a workflow
# run. Requires a token with statuses:write (GH_TOKEN) and a
# repository (GH_REPO or a git remote gh can resolve).
#
# BASE_BRANCH overrides the branch name used to select PRs in --all
# mode and in messages. The migration numbers always come from the
# working tree, so check out that branch too for a real comparison.
set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

dependencies gh jq

readonly STATUS_CONTEXT="migration-numbers"
readonly MIGRATIONS_DIR="coderd/database/migrations"
readonly MIGRATION_RE="^${MIGRATIONS_DIR}/([0-9]{6})_[^/]+\.(up|down)\.sql$"
# The GraphQL files connection returns at most 100 entries per PR.
# Larger PRs fall back to the paginated REST endpoint.
readonly FILES_PAGE=100

post_status=0
all=0
prs=()
for arg in "$@"; do
	case "$arg" in
	--post-status) post_status=1 ;;
	--all) all=1 ;;
	-h | --help)
		# Print the header comment as usage text.
		sed -n '3,/^set -euo/{/^set -euo/!s/^# \{0,1\}//p}' "${BASH_SOURCE[0]}"
		exit 0
		;;
	-*) error "unknown flag: $arg" ;;
	*) prs+=("$arg") ;;
	esac
done
if [[ $all == 1 && ${#prs[@]} -gt 0 ]]; then
	error "--all cannot be combined with PR numbers"
fi
if [[ $all == 0 && ${#prs[@]} -eq 0 ]]; then
	error "no PR numbers given (or use --all)"
fi

repo="${GH_REPO:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}"
base_branch="${BASE_BRANCH:-$(git rev-parse --abbrev-ref HEAD)}"

# Highest migration number on the checked-out base branch. Numbers are
# compared as base-10 integers so leading zeros do not make them octal.
base_max=0
for f in "${MIGRATIONS_DIR}"/[0-9][0-9][0-9][0-9][0-9][0-9]_*.sql; do
	n="$(basename "$f")"
	n=$((10#${n:0:6}))
	if ((n > base_max)); then
		base_max=$n
	fi
done
base_max_str="$(printf '%06d' "$base_max")"
next_free="$(printf '%06d' $((base_max + 1)))"
log "base ${base_branch}: highest migration is ${base_max_str}"

# added_paths_rest prints every file a PR adds, one per line, using
# the paginated REST endpoint. Renamed and copied files count as
# added because the new name is what lands on the base branch.
added_paths_rest() {
	gh api --paginate "repos/${repo}/pulls/$1/files" \
		--jq '.[] | select(.status == "added" or .status == "renamed" or .status == "copied") | .filename'
}

# migration_numbers reads paths on stdin and prints the distinct
# migration numbers among them.
migration_numbers() {
	{ grep -E "$MIGRATION_RE" || true; } |
		sed -E "s#${MIGRATIONS_DIR}/([0-9]{6})_.*#\1#" |
		sort -u
}

# set_status posts a commit status to the PR head. Descriptions are
# capped at 140 characters by the API.
set_status() {
	local sha=$1 state=$2 description=$3
	if [[ $post_status != 1 ]]; then
		return
	fi
	gh api --silent -X POST "repos/${repo}/statuses/${sha}" \
		-f state="$state" \
		-f context="$STATUS_CONTEXT" \
		-f description="${description:0:140}" \
		-f target_url="${RUN_URL:-}"
}

# check_pr evaluates one PR given the migration numbers it adds and
# returns 1 when the check fails. In --all mode, PRs that add no
# migrations are skipped silently because nothing on the base branch
# can change their result.
check_pr() {
	local pr=$1 sha=$2
	shift 2
	local numbers=("$@") bad=() n
	if [[ ${#numbers[@]} -eq 0 ]]; then
		if [[ $all == 0 ]]; then
			log "PR #${pr}: adds no migrations"
			set_status "$sha" success "No new migrations"
		fi
		return 0
	fi
	for n in "${numbers[@]}"; do
		if ((10#$n <= base_max)); then
			bad+=("$n")
		fi
	done
	if [[ ${#bad[@]} -eq 0 ]]; then
		log "PR #${pr}: migrations ${numbers[*]} are newer than ${base_branch}"
		set_status "$sha" success "Migrations ${numbers[*]} are newer than ${base_max_str} on ${base_branch}"
		return 0
	fi
	log "PR #${pr}: migration(s) ${bad[*]} must be greater than ${base_max_str}, the highest on ${base_branch}; renumber to ${next_free} or later"
	set_status "$sha" failure "Migration ${bad[*]} must be > ${base_max_str} on ${base_branch}; renumber to ${next_free}+"
	return 1
}

# check_pr_rest fetches a PR's files through REST and checks it. A
# fetch failure is reported as an error status, never as a pass.
check_pr_rest() {
	local pr=$1 sha=$2 paths numbers
	if ! paths="$(added_paths_rest "$pr")"; then
		log "PR #${pr}: could not list changed files"
		set_status "$sha" error "Could not list changed files; rerun the migration-numbers workflow"
		return 1
	fi
	mapfile -t numbers < <(migration_numbers <<<"$paths")
	check_pr "$pr" "$sha" "${numbers[@]}"
}

failed=0

if [[ $all == 0 ]]; then
	for pr in "${prs[@]}"; do
		sha="$(gh api "repos/${repo}/pulls/${pr}" --jq .head.sha)"
		check_pr_rest "$pr" "$sha" || failed=1
	done
	exit "$failed"
fi

# One GraphQL page lists 100 PRs with their first 100 files each,
# instead of one REST call per open PR. Only PRs targeting this branch
# can collide with it. The dollar names are GraphQL variables.
# shellcheck disable=SC2016
query='
query($owner: String!, $name: String!, $base: String!, $endCursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, baseRefName: $base, first: 100, after: $endCursor) {
      pageInfo { hasNextPage endCursor }
      nodes {
        number
        headRefOid
        files(first: '"$FILES_PAGE"') {
          totalCount
          nodes { path changeType }
        }
      }
    }
  }
}'
while IFS=$'\t' read -r pr sha total paths_json; do
	if ((total > FILES_PAGE)); then
		check_pr_rest "$pr" "$sha" || true
		continue
	fi
	mapfile -t numbers < <(jq -r '.[]' <<<"$paths_json" | migration_numbers)
	check_pr "$pr" "$sha" "${numbers[@]}" || true
done < <(gh api graphql --paginate \
	-F owner="${repo%%/*}" -F name="${repo##*/}" -F base="$base_branch" \
	-f query="$query" \
	--jq '.data.repository.pullRequests.nodes[] |
		[.number, .headRefOid, .files.totalCount,
		 ([.files.nodes[] | select(.changeType == "ADDED" or .changeType == "RENAMED" or .changeType == "COPIED") | .path] | tojson)]
		| @tsv')
