#!/usr/bin/env bash

set -euo pipefail

# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

COMMIT=$1
if [[ -z "${COMMIT}" ]]; then
	log "Usage: $0 <commit-ref>"
	log ""
	log -n "Example: $0 "
	log $'$(gh pr view <pr-num> --json mergeCommit | jq \'.mergeCommit.oid\' -r)'
	exit 2
fi

REMOTE=$(git remote -v | grep coder/coder | awk '{print $1}' | head -n1)
if [[ -z "${REMOTE}" ]]; then
	error "Could not find remote for coder/coder"
fi

log "It is recommended that you run \`git fetch -ap ${REMOTE}\` to ensure you get a correct result."

RELEASES=$(git branch -r --contains "${COMMIT}" | grep "${REMOTE}" | grep "/release/" | sed "s|${REMOTE}/||" || true)
if [[ -z "${RELEASES}" ]]; then
	log "Commit was not found in any release branch"
else
	log "Commit was found in the following release branches:"
	log "${RELEASES}"
fi
