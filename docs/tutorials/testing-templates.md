# Test and Publish Coder Templates Through CI/CD

<div>
  <a href="https://github.com/matifali" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Muhammad Atif Ali</span>
    <img src="https://github.com/matifali.png" alt="matifali" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>

  </a>
</div>
November 15, 2024

---

## Overview

This guide demonstrates how to test and publish Coder templates in a Continuous
Integration (CI) pipeline using the
[coder/setup-action](https://github.com/coder/setup-coder). This workflow
ensures your templates are validated, tested, and promoted seamlessly.

## Prerequisites

- Install and configure Coder CLI in your environment.
- Install Terraform CLI in your CI environment.
- Create a [headless user](../admin/users/headless-auth.md) with the
  [user roles and permissions](../admin/users/groups-roles.md#roles) to manage
  templates and run workspaces.

## Creating the headless user

```shell
coder users create \
  --username machine-user \
  --email machine-user@example.com \
  --login-type none

coder tokens create --user machine-user --lifetime 8760h
# Copy the token and store it in a secret in your CI environment with the name `CODER_SESSION_TOKEN`
```

## Example GitHub Action Workflow

This example workflow tests and publishes a template using GitHub Actions.

The workflow:

1. Validates the Terraform template.
1. Pushes the template to Coder without activating it.
1. Tests the template by creating a workspace.
1. Promotes the template version to active upon successful workspace creation.

### Workflow File

Save the following workflow file as `.github/workflows/publish-template.yaml` in
your repository:

```yaml
name: Test and Publish Coder Template

on:
  push:
    branches:
      - main
  workflow_dispatch:

jobs:
  test-and-publish:
    runs-on: ubuntu-latest
    env:
      TEMPLATE_NAME: "my-template"
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Set up Terraform
        uses: hashicorp/setup-terraform@v2
        with:
          terraform_version: latest

      - name: Set up Coder CLI
        uses: coder/setup-action@v1
        with:
          access_url: "https://coder.example.com"
          coder_session_token: ${{ secrets.CODER_SESSION_TOKEN }}

      - name: Validate Terraform template
        run: terraform validate

      - name: Get short commit SHA to use as template version name
        id: name
        run: echo "version_name=$(git rev-parse --short HEAD)" >> $GITHUB_OUTPUT

      - name: Get latest commit title to use as template version description
        id: message
        run:
          echo "pr_title=$(git log --format=%s -n 1 ${{ github.sha }})" >>
          $GITHUB_OUTPUT

      - name: Push template to Coder
        run: |
          coder templates push $TEMPLATE_NAME --activate=false --name ${{ steps.name.outputs.version_name }} --message "${{ steps.message.outputs.pr_title }}" --yes

      - name: Create a test workspace and run some example commands
        run: |
          coder create -t $TEMPLATE_NAME --template-version ${{ steps.name.outputs.version_name }} test-${{ steps.name.outputs.version_name }} --yes
          coder config-ssh --yes
          # run some example commands
          coder ssh test-${{ steps.name.outputs.version_name }} -- make build

      - name: Delete the test workspace
        if: always()
        run: coder delete test-${{ steps.name.outputs.version_name }} --yes

      - name: Promote template version
        if: success()
        run: |
          coder template version promote --template=$TEMPLATE_NAME --template-version=${{ steps.name.outputs.version_name }} --yes
```
