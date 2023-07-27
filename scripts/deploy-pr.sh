#!/usr/bin/env bash
# Usage: ./deploy-pr.sh  [--skip-build -s] [--dry-run -n] [--yes -y]
# deploys the current branch to a PR environment and posts login credentials to
# [#pr-deployments](https://codercom.slack.com/archives/C05DNE982E8) Slack channel

set -euo pipefail

# default settings
skipBuild=false
dryRun=false
confirm=true

# parse arguments
for arg in "$@"; do
	case $arg in
	-s | --skip-build)
		skipBuild=true
		shift # Remove --skip-build from processing
		;;
	-n | --dry-run)
		dryRun=true
		shift # Remove --dry-run from processing
		;;
	-y | --yes)
		confirm=false
		shift # Remove --yes from processing
		;;
	*)
		shift # Remove generic argument from processing
		;;
	esac
done

# confirm if not passed -y or --yes
if $confirm; then
	read -p "Are you sure you want to deploy? (y/n) " -n 1 -r
	echo
	if [[ ! $REPLY =~ ^[Yy]$ ]]; then
		exit 1
	fi
fi

# get branch name and pr number
branchName=$(gh pr view --json headRefName | jq -r .headRefName)
prNumber=$(gh pr view --json number | jq -r .number)

if $skipBuild; then
	#check if the image exists
	foundTag=$(curl -fsSL https://github.com/coder/coder/pkgs/container/coder-preview | grep -o "$prNumber" | head -n 1) || true
	echo "foundTag is: '${foundTag}'"
	if [[ -z "${foundTag}" ]]; then
		echo "Image not found"
		echo "${prNumber} tag not found in ghcr.io/coder/coder-preview"
		echo "Please remove --skip-build and try again"
		exit 1
	fi
fi

if $dryRun; then
	echo "dry run"
	echo "branchName: ${branchName}"
	echo "prNumber: ${prNumber}"
	echo "skipBuild: ${skipBuild}"
	exit 0
fi

gh workflow run pr-deploy.yaml --ref "${branchName}" -f "pr_number=${prNumber}" -f "skip_build=${skipBuild}"
