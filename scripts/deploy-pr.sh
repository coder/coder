#!/usr/bin/env bash
# Usage: ./deploy-pr.sh
# deploys the current branch to a PR environment and posts login credentials to [#pr-deployments](https://codercom.slack.com/archives/C05DNE982E8) Slack channel
branchName=$(gh pr view --json headRefName | jq -r .headRefName)
if [[ "$branchName" == "main" ]]; then
    # get commit sha --short
    commitSha=$(git rev-parse --short HEAD)
    gh workflow run pr-deploy.yaml /
        --ref $branchName /
        -f pr_number=${commitSha}
else
    prNumber=$(gh pr view --json number | jq -r .number)
    gh workflow run pr-deploy.yaml /
    --ref $branchName /
    -f pr_number=${prNumber}
