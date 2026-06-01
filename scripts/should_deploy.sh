#!/usr/bin/env bash

# This script determines if the current branch should be deployed to dogfood.
#
# To avoid masking unrelated failures, this script will return 0 in either case,
# and will print `DEPLOY` or `NOOP` to stdout.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

branch_name=$(git branch --show-current)

# We no longer deploy release branches to dogfood, and instead test them on the
# stable deployment.
# TODO: once we're happy with the new deployment process, we can remove this
# script and the related GitHub workflow.
if [[ "$branch_name" == "main" ]]; then
	log "VERDICT: DEPLOY"
	echo "DEPLOY" # stdout
else
	log "VERDICT: NOOP"
	echo "NOOP" # stdout
fi
