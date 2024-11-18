# Testing and Publishing Coder Templates in CI/CD

<div>
  <a href="https://github.com/matifali" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Muhammad Atif Ali</span>
    <img src="https://github.com/matifali.png" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>
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

Before proceeding, ensure the following:

- **Coder CLI** is installed and configured in your environment.
- **Terraform CLI** is installed and available in your CI environment.
- Access to your **Coder instance** with the appropriate
  [permissions](../admin/users/groups-roles.md#roles).

## Example GitHub Action Workflow

Below is an example workflow for testing and publishing a template using GitHub
Actions. The workflow first validates the Terraform template, pushes the
template to Coder without activating it, tests the template by creating a
workspace, and then promotes the template version to active upon successful
workspace creation.

### Step-by-Step Process

1. **Validate the Terraform template.**
2. **Push the template to Coder without activating it.**
3. **Test the template by creating a workspace.**
4. **Promote the template version to active upon successful workspace
   creation.**

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

      - name: Create a test workspace
        run: |
          coder create -t $TEMPLATE_NAME --template-version ${{ steps.name.outputs.version_name }} test-${{ steps.name.outputs.version_name }} --yes

      - name: Run some example commands
        run: |
          coder config-ssh --yes
          # run some example commands
          coder ssh test-${{ steps.name.outputs.version_name }} -- make build

      - name: Promote template version
        if: success()
        run: |
          coder template version promote --template=$TEMPLATE_NAME --template-version=${{ steps.name.outputs.version_name }} --yes
```
