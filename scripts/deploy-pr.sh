#!/usr/bin/env bash
# Usage: ./deploy-pr.sh  [--skip-build -s] [--dry-run -n] [--yes -y]
# deploys the current branch to a PR environment and posts login credentials to
# [#pr-deployments](https://codercom.slack.com/archives/C05DNE982E8) Slack channel

set -euo pipefail

# default settings
skipBuild=false
dryRun=false
confirm=true
experiments=""

# parse arguments
while (("$#")); do
	case "$1" in
	-s | --skip-build)
		skipBuild=true
		shift
		;;
	-n | --dry-run)
		dryRun=true
		shift
		;;
	-e | --experiments)
		if [ -n "$2" ] && [ "${2:0:1}" != "-" ]; then
			experiments="$2"
			shift
		else
			echo "Error: Argument for $1 is missing" >&2
			exit 1
		fi
		shift
		;;
	-y | --yes)
		confirm=false
		shift
		;;
	--)
		shift
		break
		;;
	--*)
		echo "Error: Unsupported flag $1" >&2
		exit 1
		;;
	*)
		shift
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
	echo "experiments: ${experiments}"
	exit 0
fi

echo "branchName: ${branchName}"
echo "prNumber: ${prNumber}"
echo "skipBuild: ${skipBuild}"
echo "experiments: ${experiments}"

gh workflow run pr-deploy.yaml --ref "${branchName}" -f "pr_number=${prNumber}" -f "skip_build=${skipBuild}" -f "experiments=${experiments}"
