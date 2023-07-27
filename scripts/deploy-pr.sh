#!/usr/bin/env bash
# Usage: ./deploy-pr.sh
# deploys the current branch to a PR environment and posts login credentials to [#pr-deployments](https://codercom.slack.com/archives/C05DNE982E8) Slack channel
set -euox pipefail

branchName=$(gh pr view --json headRefName | jq -r .headRefName)

if [[ "$branchName" == "main" ]]; then
    gh workflow run pr-deploy.yaml --ref $branchName -f pr_number=$(git rev-parse --short HEAD)
else
    gh workflow run pr-deploy.yaml --ref $branchName -f pr_number=$(gh pr view --json number | jq -r .number)
fi
