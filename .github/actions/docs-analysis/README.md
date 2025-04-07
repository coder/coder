# Docs Analysis Action

A composite GitHub Action to analyze documentation changes in pull requests and provide useful metrics and insights.

## Features

- Detects documentation files changed in a PR
- Calculates metrics (files changed, words added/removed)
- Tracks image modifications with detailed reporting
- Analyzes document structure (headings, titles)
- Identifies the most significantly changed files
- Integrates with other doc workflows (weekly checks, PR previews)
- Provides standardized outputs that can be used by any workflow

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

### Integration with tj-actions/changed-files (Recommended)

For optimal performance and reliability, we recommend using with `tj-actions/changed-files`:

```yaml
- uses: tj-actions/changed-files@v45
  id: changed-files
  with:
    files: |
      docs/**
      **.md
    separator: ","

- name: Analyze Documentation Changes
  id: docs-analysis
  uses: ./.github/actions/docs-analysis
  with:
    docs-path: 'docs/'
    changed-files: ${{ steps.changed-files.outputs.all_changed_files }}
    files-pattern: 'docs/**|**.md'
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
      
      - uses: tj-actions/changed-files@v45
        id: changed-files
        with:
          files: |
            docs/**
            **.md
          separator: ","
      
      - name: Analyze Documentation Changes
        uses: ./.github/actions/docs-analysis
        id: docs-analysis
        with:
          docs-path: 'docs/'
          changed-files: ${{ steps.changed-files.outputs.all_changed_files }}
          significant-words-threshold: '100'
          skip-if-no-docs: 'true'
          debug-mode: 'false'
      
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
| `files-pattern` | Glob pattern(s) for documentation files (use vertical bar \| to separate multiple patterns) | No | `**.md\|docs/**` |
| `changed-files` | Comma-separated list of changed files (from tj-actions/changed-files) | No | `` |
| `pr-ref` | PR reference to analyze | No | `github.event.pull_request.head.ref` |
| `base-ref` | Base reference to compare against | No | `main` |
| `files-changed` | Comma-separated list of files changed (legacy input, use `changed-files` instead) | No | `` |
| `max-scan-files` | Maximum number of files to scan | No | `100` |
| `max-files-to-analyze` | Maximum files to analyze in detail (for performance) | No | `20` |
| `throttle-large-repos` | Enable throttling for large repositories | No | `true` |
| `significant-words-threshold` | Threshold for significant text changes | No | `100` |
| `skip-if-no-docs` | Whether to skip if no docs files are changed | No | `true` |
| `debug-mode` | Enable verbose debugging output | No | `false` |
| `use-changed-files-action` | Whether to use tj-actions/changed-files instead of git commands | No | `false` |

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
| `execution-time` | Execution time in seconds |
| `cache-key` | Cache key for this analysis run |

## Security Features

- Stronger input validation with whitelist approach for branch references
- Enhanced path sanitization with traversal detection
- Secure command execution (no eval) in git retry operations
- Error tracing with line numbers for better debugging
- Cross-platform compatibility with fallbacks
- Repository size detection with adaptive throttling
- Python integration for safer JSON handling (with bash fallbacks)
- Performance monitoring with execution metrics

## Performance Optimization

- Configurable document scan limits
- Intelligent throttling for large repositories
- Git performance tuning
- Execution time tracking
- Content-based caching
- Debug mode for troubleshooting

## Examples

### Analyzing Documentation Changes for a PR

```yaml
- name: Analyze Documentation Changes
  uses: ./.github/actions/docs-analysis
  id: docs-analysis
  with:
    docs-path: 'docs/'
```

### Analyzing Non-Git Files

```yaml
- name: Analyze Documentation Files
  uses: ./.github/actions/docs-analysis
  id: docs-analysis
  with:
    files-changed: 'docs/file1.md,docs/file2.md,README.md'
    docs-path: 'docs/'
```

### Debug Mode for Troubleshooting

```yaml
- name: Analyze Documentation with Debug Output
  uses: ./.github/actions/docs-analysis
  id: docs-analysis
  with:
    docs-path: 'docs/'
    debug-mode: 'true'
```

## Unified Documentation Workflows

This action is designed to work seamlessly with Coder's other documentation-related workflows:

### How to Use with docs-ci.yaml

The `docs-ci.yaml` workflow uses this action to analyze documentation changes for linting and formatting:

```yaml
# From .github/workflows/docs-ci.yaml
- uses: tj-actions/changed-files@v45
  id: changed-files
  with:
    files: |
      docs/**
      **.md
    separator: ","

- name: Analyze documentation changes
  id: docs-analysis
  uses: ./.github/actions/docs-analysis
  with:
    docs-path: "docs/"
    changed-files: ${{ steps.changed-files.outputs.all_changed_files }}
    files-pattern: "docs/**|**.md"
```

### How to Use with docs-preview-link.yml

This action can be used in the `docs-preview-link.yml` workflow to analyze documentation changes for preview generation:

```yaml
# Example integration with docs-preview-link.yml
- name: Analyze documentation changes
  id: docs-analysis
  uses: ./.github/actions/docs-analysis
  with:
    docs-path: "docs/"
    pr-ref: ${{ steps.pr_info.outputs.branch_name }}
    base-ref: 'main'
```

### How to Use with weekly-docs.yaml

This action can be used to enhance the weekly documentation checks:

```yaml
# Example integration with weekly-docs.yaml
- name: Analyze documentation structure
  id: docs-analysis
  uses: ./.github/actions/docs-analysis
  with:
    docs-path: "docs/"
    files-pattern: "docs/**"
    max-scan-files: "500"  # Higher limit for full repo scan
```

By using this shared action across all documentation workflows, you ensure consistent analysis, metrics, and reporting for all documentation-related tasks.