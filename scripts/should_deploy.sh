#!/usr/bin/env bash

# This script determines if a commit in either the main branch or a
# `release/x.y` branch should be deployed to dogfood, and which channel
# it maps to.
#
# Channel mapping:
#   main           → dogfood  (dev.coder.com, always)
#   release/X.Y    → stable   (stable.coder.com, highest published minor)
#   release/ESR    → esr      (esr.coder.com, from .github/esr-version)
#
# RC deployments (rc.coder.com) are handled separately by deploy.yaml
# when the latest tag on main is an RC tag. This script does not output
# "rc" — main always maps to dogfood.
#
# Stable is derived programmatically from published vX.Y.0 tags.
# ESR is configured manually via .github/esr-version.
#
# To avoid masking unrelated failures, this script returns 0 in all
# cases and prints one of the following to stdout:
#   DEPLOY <channel>   — deploy to this channel
#   NOOP               — do not deploy

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

branch_name=$(git branch --show-current)

# main always deploys to dogfood.
if [[ "$branch_name" == "main" ]]; then
	log "Branch 'main' maps to channel 'dogfood'"
	echo "DEPLOY dogfood"
	exit 0
fi

# Must be a release branch.
if [[ ! "$branch_name" =~ ^release/([0-9]+)\.([0-9]+)$ ]]; then
	log "Branch '$branch_name' is not a supported branch for dogfood deploy"
	echo "NOOP"
	exit 0
fi
branch_version="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
log "Current branch '$branch_name' (version $branch_version)"

# Find stable: the highest minor version with a published vX.Y.0 tag.
# Exclude rc, dev, and pre-release tags.
stable_version=$(
	git tag -l 'v[0-9]*.[0-9]*.0' |
		grep -vE '(rc|dev|-|\+)' |
		sort -V | tail -n1 |
		sed 's/^v//; s/\.0$//'
)

if [[ -z "$stable_version" ]]; then
	log "No published vX.Y.0 tags found, cannot determine channels"
	echo "NOOP"
	exit 0
fi

# ESR: read from config file.
esr_version=""
esr_config=".github/esr-version"
if [[ -f "$esr_config" ]]; then
	esr_version=$(tr -d '[:space:]' <"$esr_config")
fi

log "Channel mapping: stable=$stable_version esr=${esr_version:-(none)}"

if [[ "$branch_version" == "$stable_version" ]]; then
	log "VERDICT: DEPLOY stable"
	echo "DEPLOY stable"
elif [[ -n "$esr_version" && "$branch_version" == "$esr_version" ]]; then
	log "VERDICT: DEPLOY esr"
	echo "DEPLOY esr"
else
	log "VERDICT: NOOP (branch $branch_version not mapped to any channel)"
	echo "NOOP"
fi
