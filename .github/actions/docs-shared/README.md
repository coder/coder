# Docs Shared Action

A composite GitHub action that provides shared functionality for docs-related workflows. This action unifies the common patterns across documentation linting, formatting, preview link generation, and PR commenting.

## Features

- Detects changes in documentation files using `tj-actions/changed-files`
- Provides linting and formatting for markdown files
- Generates preview links for documentation changes
- Creates or updates PR comments with preview links
- Handles special analysis of manifest.json changes
- Includes security hardening measures
- Provides detailed outputs for use in workflows

## Security Features

- Uses secure file permissions with `umask 077`
- Clears potentially harmful environment variables
- Input validation and sanitization
- Can work with harden-runner actions

## Usage

```yaml
- name: Process Documentation
  id: docs-shared
  uses: ./.github/actions/docs-shared
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    docs-dir: docs
    include-md-files: "true"
    check-links: "true"
    lint-markdown: "true"
    format-markdown: "true"
    generate-preview: "true"
    post-comment: "true"
    pr-number: "${{ github.event.pull_request.number }}"
    fail-on-error: "true"
```

## Inputs

| Input            | Description                                         | Required | Default |
|------------------|-----------------------------------------------------|----------|---------|
| github-token     | GitHub token for API operations                     | Yes      | -       |
| docs-dir         | Path to the docs directory                          | No       | docs    |
| include-md-files | Whether to include all markdown files (not just docs) | No     | false   |
| check-links      | Whether to check links in markdown files            | No       | false   |
| lint-markdown    | Whether to lint markdown files                      | No       | false   |
| format-markdown  | Whether to check markdown formatting                | No       | false   |
| generate-preview | Whether to generate preview links                   | No       | false   |
| post-comment     | Whether to post a PR comment with results           | No       | false   |
| pr-number        | PR number for commenting                            | No       | ""      |
| fail-on-error    | Whether to fail the workflow on errors              | No       | true    |

## Outputs

| Output                | Description                                       |
|-----------------------|---------------------------------------------------|
| has_changes           | Boolean indicating if documentation files changed |
| changed_files         | JSON array of changed documentation files         |
| formatted_changed_files | Markdown-formatted list of changed files with links |
| preview_url           | Documentation preview URL                         |
| manifest_changed      | Boolean indicating if manifest.json changed       |
| has_new_docs          | Boolean indicating if new docs were added         |
| new_docs              | List of newly added docs formatted for comment    |
| preview_links         | List of preview links for newly added docs        |
| lint_results          | Results from linting                             |
| format_results        | Results from format checking                     |
| link_check_results    | Results from link checking                       |

## Example

See the [docs-shared-example.yaml](./.github/workflows/docs-shared-example.yaml) workflow for a complete example of how to use this action.