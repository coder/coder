#!/usr/bin/env bash

# This script determines if a commit in either the main branch or a
# `release/x.y` branch should be deployed to dogfood.
#
# To avoid masking unrelated failures, this script will return 0 in either case,
# and will print `DEPLOY` or `NOOP` to stdout.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

deploy_branch=main

# Determine the current branch name and check that it is one of the supported
# branch names.
branch_name=$(git branch --show-current)

if [[ "$branch_name" != "main" && ! "$branch_name" =~ ^release/[0-9]+\.[0-9]+$ ]]; then
	error "Current branch '$branch_name' is not a supported branch name for dogfood, must be 'main' or 'release/x.y'"
fi
log "Current branch '$branch_name'"

# Determine the remote name
remote=$(git remote -v | grep coder/coder | awk '{print $1}' | head -n1)
if [[ -z "${remote}" ]]; then
	error "Could not find remote for coder/coder"
fi
log "Using remote '$remote'"

# Step 1: List all release branches and sort them by major/minor so we can find
# the latest release branch.
release_branches=$(
	git branch -r --format='%(refname:short)' |
		grep -E "${remote}/release/[0-9]+\.[0-9]+$" |
		sed "s|${remote}/||" |
		sort -V
)

# As a sanity check, release/2.26 should exist.
if ! echo "$release_branches" | grep "release/2.26" >/dev/null; then
	error "Could not find existing release branches. Did you run 'git fetch -ap ${remote}'?"
fi

# Step 2: Iterate from the latest release branch backwards to find the deploy
# branch. A release branch is the deploy target if its `.0` tag does not yet
# exist (i.e. the release is in progress / frozen). Release branches that only
# carry RC tags (v<x.y>.0-rc.*) are skipped — they are not considered frozen.
for branch in $(echo "$release_branches" | sort -Vr); do
	version=${branch#release/}
	log "Checking release branch: $branch (version: $version)"

	if git rev-parse "refs/tags/v${version}.0" >/dev/null 2>&1; then
		# Final .0 tag exists — this release (and all older ones) are done.
		log "Tag 'v${version}.0' exists, release is complete"
		break
	fi

	# No .0 tag. Check if there are RC tags, which would indicate this is
	# an RC-only branch that we should skip.
	if git tag -l "v${version}.0-rc.*" | grep -q .; then
		log "Branch '$branch' only has RC tags, skipping"
		continue
	fi

	# No .0 tag and no RC tags — this is the frozen release branch.
	log "Branch '$branch' is the frozen release branch"
	deploy_branch=$branch
	break
done
log "Deploy branch: $deploy_branch"

# Finally, check if the current branch is the deploy branch.
log
if [[ "$branch_name" != "$deploy_branch" ]]; then
	log "VERDICT: DO NOT DEPLOY"
	echo "NOOP" # stdout
else
	log "VERDICT: DEPLOY"
	echo "DEPLOY" # stdout
fi
