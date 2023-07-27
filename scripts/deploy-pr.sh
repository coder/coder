#!/usr/bin/env bash
# Usage: ./deploy-pr.sh  --skip-build
# deploys the current branch to a PR environment and posts login credentials to 
# [#pr-deployments](https://codercom.slack.com/archives/C05DNE982E8) Slack channel
# if --skip-build is passed, the build step will be skipped and the last build image will be used

set -euox pipefail

# if --skip-build is passed, the build step will be skipped and the last build image will be used
if [[ "$*" == *--skip-build* ]]; then
    skipBuild=true
fi

branchName=$(gh pr view --json headRefName | jq -r .headRefName)

if [[ "$branchName" == "main" ]]; then
	gh workflow run pr-deploy.yaml --ref $branchName -f pr_number=$(git rev-parse --short HEAD) -f skip_build=$skipBuild
else
	gh workflow run pr-deploy.yaml --ref $branchName -f pr_number=$(gh pr view --json number | jq -r .number) -f skip_build=$skipBuild
fi
