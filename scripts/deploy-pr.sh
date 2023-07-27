#!/usr/bin/env bash
# Usage: ./deploy-pr.sh  --skip-build
# deploys the current branch to a PR environment and posts login credentials to 
# [#pr-deployments](https://codercom.slack.com/archives/C05DNE982E8) Slack channel
# if --skip-build is passed, the build step will be skipped and the last build image will be used

set -euox pipefail

branchName=$(gh pr view --json headRefName | jq -r .headRefName)

if [[ "$branchName" == "main" ]]; then
    prNumber=$(git rev-parse --short HEAD)
else
    prNumber=$(gh pr view --json number | jq -r .number)
fi

# if --skip-build is passed, the build job will be skipped and the last built image will be used
if [[ "$*" == *--skip-build* ]]; then
    skipBuild=true
    #check if the image exists
    foundTag=$(curl -fsSL https://github.com/coder/coder/pkgs/container/coder-preview | grep -o $prNumber | head -n 1)
    if [ -z "$foundTag" ]; then
        echo "Image not found"
        echo "$prNumber tag not found in ghcr.io/coder/coder-preview"
        echo "Please remove --skip-build and try again"
        exit 1
    fi
else
    skipBuild=false
fi

gh workflow run pr-deploy.yaml --ref $branchName -f pr_number=$imageTag -f image_tag=$imageTag -f skip_build=$skipBuild
