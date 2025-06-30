#!/usr/bin/env bash

COMMIT=$1
if [[ -z "${COMMIT}" ]]; then
	echo "Usage: $0 <commit-ref>"
	echo ""
	echo -n "Example: $0 "
	echo $'$(gh pr view <pr-num> --json mergeCommit | jq \'.mergeCommit.oid\' -r)'
	exit 2
fi

REMOTE=$(git remote -v | grep coder/coder | awk '{print $1}' | head -n1)
if [[ -z "${REMOTE}" ]]; then
	echo "Could not find remote for coder/coder"
	exit 1
fi

echo "It is recommended that you run \`git fetch -ap ${REMOTE}\` to ensure you get a correct result."

RELEASES=$(git branch -r --contains "${COMMIT}" | grep "${REMOTE}" | grep "/release/" | sed "s|${REMOTE}/||")
if [[ -z "${RELEASES}" ]]; then
	echo "Commit was not found in any release branch"
else
	echo "Commit was found in the following release branches:"
	echo "${RELEASES}"
fi
