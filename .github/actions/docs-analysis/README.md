# Docs Analysis Action

A composite GitHub Action to analyze documentation changes in pull requests and provide useful metrics and insights.

## Features

- Detects documentation files changed in a PR
- Calculates metrics (files changed, words added/removed)
- Tracks image changes (added, modified, deleted)
- Analyzes document structure (headings, title)
- Identifies the most changed files
- Provides outputs for use in workflows

## Usage

This action analyzes documentation changes to help provide better context and metrics for documentation PRs. 
It only runs on PRs that modify files in the docs directory or markdown files elsewhere in the repo.

### Basic Example

```yaml
- name: Analyze Documentation Changes
  uses: ./.github/actions/docs-analysis
  id: docs-analysis
  with:
    docs-path: 'docs/'
    pr-ref: ${{ github.event.pull_request.head.ref }}
    base-ref: 'main'
```

### Complete Example with Conditionals

```yaml
jobs:
  check-docs-changes:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v3
        with:
          fetch-depth: 0
      
      - name: Analyze Documentation Changes
        uses: ./.github/actions/docs-analysis
        id: docs-analysis
        with:
          docs-path: 'docs/'
          pr-ref: ${{ github.event.pull_request.head.ref }}
          base-ref: 'main'
          significant-words-threshold: '100'
          skip-if-no-docs: 'true'
      
      - name: Create Preview Comment
        if: steps.docs-analysis.outputs.docs-changed == 'true'
        run: |
          echo "Found ${{ steps.docs-analysis.outputs.docs-files-count }} changed docs files"
          echo "Words: +${{ steps.docs-analysis.outputs.words-added }}/-${{ steps.docs-analysis.outputs.words-removed }}"
          
          if [[ "${{ steps.docs-analysis.outputs.images-total }}" != "0" ]]; then
            echo "Images changed: ${{ steps.docs-analysis.outputs.images-total }}"
          fi
          
          if [[ "${{ steps.docs-analysis.outputs.significant-change }}" == "true" ]]; then
            echo "This is a significant docs change!"
          fi
```

## Inputs

| Name | Description | Required | Default |
|------|-------------|----------|---------|
| `docs-path` | Path to the documentation directory | No | `docs/` |
| `pr-ref` | PR reference to analyze | No | `github.event.pull_request.head.ref` |
| `base-ref` | Base reference to compare against | No | `main` |
| `files-changed` | Comma-separated list of files changed (alternative to git diff) | No | `` |
| `max-scan-files` | Maximum number of files to scan | No | `100` |
| `significant-words-threshold` | Threshold for significant text changes | No | `100` |
| `skip-if-no-docs` | Whether to skip if no docs files are changed | No | `true` |

## Outputs

| Name | Description |
|------|-------------|
| `docs-changed` | Whether documentation files were changed (`true`/`false`) |
| `docs-files-count` | Number of documentation files changed |
| `words-added` | Number of words added to documentation |
| `words-removed` | Number of words removed from documentation |
| `images-added` | Number of images added |
| `images-modified` | Number of images modified |
| `images-deleted` | Number of images deleted |
| `images-total` | Total number of images changed |
| `image-names` | Comma-separated list of changed image files |
| `manifest-changed` | Whether manifest.json was changed (`true`/`false`) |
| `format-only` | Whether changes are formatting-only (`true`/`false`) |
| `significant-change` | Whether changes are significant (`true`/`false`) |
| `has-non-docs-changes` | Whether PR contains non-docs changes (`true`/`false`) |
| `most-changed-file` | Path to the most changed file |
| `most-changed-url-path` | URL path for the most changed file |
| `most-significant-image` | Path to the most significant image |
| `doc-structure` | JSON structure of document heading counts |