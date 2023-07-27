#!/usr/bin/env bash
# Usage: ./deploy-pr.sh  --skip-build
# deploys the current branch to a PR environment and posts login credentials to
# [#pr-deployments](https://codercom.slack.com/archives/C05DNE982E8) Slack channel
# if --skip-build is passed, the build step will be skipped and the last build image will be used
# if --yes or -y is passed, the script will not ask for confirmation before deploying

set -euo pipefail

# ask for user confirmation before deploying also skip confirmation if --yes or -y is passed
if [[ "$*" != *--yes* ]] && [[ "$*" != *-y* ]]; then
	read -p "Are you sure you want to deploy? (y/n) " -n 1 -r
	echo
	if [[ ! $REPLY =~ ^[Yy]$ ]]; then
		exit 1
	fi
fi

branchName=$(gh pr view --json headRefName | jq -r .headRefName)
prNumber=$(gh pr view --json number | jq -r .number)

if [[ "$*" == *--skip-build* ]]; then
	skipBuild=true
	#check if the image exists
	foundTag=$(curl -fsSL https://github.com/coder/coder/pkgs/container/coder-preview | grep -o "$prNumber" | head -n 1) || true
	echo "foundTag is: '${foundTag}'"
	if [[ -z "${foundTag}" ]]; then
		echo "Image not found"
		echo "${prNumber} tag not found in ghcr.io/coder/coder-preview"
		echo "Please remove --skip-build and try again"
		exit 1
	fi
else
	skipBuild=false
fi

## dry run with --dry-run or -n

if [[ "$*" == *--dry-run* ]] || [[ "$*" == *-n* ]]; then
	echo "dry run"
	echo "branchName: ${branchName}"
	echo "prNumber: ${prNumber}"
	echo "skipBuild: ${skipBuild}"
	echo "gh workflow run pr-deploy.yaml --ref "${branchName}" -f pr_number="${prNumber}" -f skip_build="${skipBuild}""
	exit 0
fi

gh workflow run pr-deploy.yaml --ref "${branchName}" -f pr_number="${prNumber}" -f skip_build="${skipBuild}"
