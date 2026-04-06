#!/usr/bin/env bash
# Usage: ./scripts/backport-pr.sh [--dry-run] <release-version> <pr-number>
#
# Backports a merged PR to a release branch by cherry-picking its merge commit
# and opening a new PR targeting the release branch.
#
# Examples:
#   ./scripts/backport-pr.sh 2.30 23969
#   ./scripts/backport-pr.sh --dry-run 2.30 23969

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

dry_run=0

# Parse flags.
while [[ $# -gt 0 ]]; do
	case "$1" in
	--dry-run | -n)
		dry_run=1
		shift
		;;
	-*)
		error "Unknown flag: $1"
		;;
	*)
		break
		;;
	esac
done

if [[ $# -lt 2 ]]; then
	echo "Usage: $0 [--dry-run] <release-version> <pr-number>" >&2
	echo "  e.g. $0 2.30 23969" >&2
	exit 1
fi

release_version="$1"
pr_number="$2"
release_branch="release/${release_version}"

dependencies gh jq git

# Authenticate with GitHub.
gh_auth

# Validate that the PR exists and is merged.
log "Fetching PR #${pr_number}..."
pr_json=$(gh pr view "$pr_number" --json mergeCommit,title,number,state,headRefName,url)

pr_state=$(echo "$pr_json" | jq -r '.state')
if [[ "$pr_state" != "MERGED" ]]; then
	error "PR #${pr_number} is not merged (state: ${pr_state})."
fi

merge_commit=$(echo "$pr_json" | jq -r '.mergeCommit.oid')
pr_title=$(echo "$pr_json" | jq -r '.title')
pr_url=$(echo "$pr_json" | jq -r '.url')

if [[ -z "$merge_commit" || "$merge_commit" == "null" ]]; then
	error "Could not determine merge commit for PR #${pr_number}."
fi

log "PR:            #${pr_number} - ${pr_title}"
log "Merge commit:  ${merge_commit}"
log "Release branch: ${release_branch}"

# Make sure we have the latest refs.
maybedryrun "$dry_run" git fetch origin

# Validate the release branch exists on the remote.
if ! git rev-parse "origin/${release_branch}" >/dev/null 2>&1; then
	error "Release branch '${release_branch}' does not exist on origin."
fi

backport_branch="backport/${pr_number}-to-${release_version}"
log "Backport branch: ${backport_branch}"

if [[ "$dry_run" == 1 ]]; then
	log ""
	log "DRYRUN: Would cherry-pick ${merge_commit} onto ${release_branch} via branch ${backport_branch}"
	log "DRYRUN: Would create PR targeting ${release_branch}"
	exit 0
fi

# Check for uncommitted changes that would block checkout.
if ! git diff-index --quiet HEAD --; then
	error "You have uncommitted changes. Please commit or stash them first."
fi

# Create the backport branch from the release branch.
log "Creating branch ${backport_branch} from origin/${release_branch}..."
git checkout -b "$backport_branch" "origin/${release_branch}"

# Cherry-pick the merge commit.
log "Cherry-picking ${merge_commit}..."
if ! git cherry-pick -x "$merge_commit"; then
	log ""
	log "Cherry-pick failed due to conflicts."
	log "Resolve the conflicts, then run:"
	log "  git cherry-pick --continue"
	log "  git push origin ${backport_branch}"
	log "  gh pr create --base ${release_branch} --head ${backport_branch} --title \"chore: backport #${pr_number} to ${release_version}\" --body \"Backport of ${pr_url}\""
	log ""
	log "Or abort with: git cherry-pick --abort && git checkout - && git branch -D ${backport_branch}"
	exit 1
fi

# Push the backport branch.
log "Pushing ${backport_branch}..."
git push origin "$backport_branch"

# Create the PR.
log "Creating PR..."
backport_pr_url=$(gh pr create \
	--draft \
	--label "cherry-pick/v${release_version}" \
	--base "$release_branch" \
	--head "$backport_branch" \
	--title "chore: backport #${pr_number} to ${release_version}" \
	--body "$(
		cat <<EOF
Backport of ${pr_url}

Original PR: #${pr_number} — ${pr_title}
Merge commit: ${merge_commit}
EOF
	)")

log ""
log "Backport PR created: ${backport_pr_url}"

# Return to previous branch.
git checkout -
