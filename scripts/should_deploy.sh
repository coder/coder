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

# Step 2: Find the latest released branch (has a v<x.y>.0 tag), then check
# whether the next release branch exists. If it does, that branch is the
# frozen deploy target. This avoids coupling to any RC tagging convention.
latest_released=""
for branch in $(echo "$release_branches" | tac); do
	version=${branch#release/}
	if git rev-parse "refs/tags/v${version}.0" >/dev/null 2>&1; then
		latest_released=$branch
		log "Latest released branch: $branch (v${version}.0 exists)"
		break
	fi
done

if [[ -z "$latest_released" ]]; then
	log "No released branch found, falling back to main"
else
	# The frozen branch is the one immediately after the latest released
	# branch in version order, if it exists.
	frozen=$(echo "$release_branches" | grep -A1 "^${latest_released}$" | tail -n1)
	if [[ "$frozen" != "$latest_released" ]]; then
		log "Frozen release branch: $frozen"
		deploy_branch=$frozen
	else
		log "No frozen release branch found after $latest_released, falling back to main"
	fi
fi
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
