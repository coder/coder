#!/usr/bin/env bash
# Usage: ./deploy-pr.sh [--dry-run -n] [--yes -y] [--experiments -e <experiments>] [--force -f]
# deploys the current branch to a PR environment and posts login credentials to
# [#pr-deployments](https://codercom.slack.com/archives/C05DNE982E8) Slack channel

set -euo pipefail

# default settings
dryRun=false
confirm=true
force=false
experiments=""

# parse arguments
while (("$#")); do
	case "$1" in
	-f | --force)
		force=true
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

if $dryRun; then
	echo "dry run"
	echo "branchName: ${branchName}"
	echo "prNumber: ${prNumber}"
	echo "experiments: ${experiments}"
	echo "force: ${force}"
	exit 0
fi

echo "branchName: ${branchName}"
echo "prNumber: ${prNumber}"
echo "experiments: ${experiments}"
echo "force: ${force}"

gh workflow run pr-deploy.yaml --ref "${branchName}" -f "pr_number=${prNumber}" -f "experiments=${experiments}" -f "force=${force}"
