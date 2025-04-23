# Coder Documentation GitHub Actions

This directory contains GitHub Actions, configurations, and workflows for Coder's documentation.

## Directory Structure

- `actions/docs-shared`: Composite action providing core documentation functionality
- `actions/docs-preview`: Preview link generation for documentation changes
- `vale`: Configuration and style rules for Vale documentation linting
- `.linkspector.yml`: Configuration for link checking

## Available Workflows

### Reusable Workflow

The `docs-unified.yaml` workflow provides a reusable workflow that can be called from other workflows. This combines all documentation checks in one workflow:

```yaml
jobs:
  docs-validation:
    name: Validate Documentation
    uses: ./.github/workflows/docs-unified.yaml
    permissions:
      contents: read
      pull-requests: write
    with:
      lint-markdown: true
      check-format: true
      check-links: true
      lint-vale: true
      generate-preview: true
      post-comment: true
      fail-on-error: false
```

### Post-Merge Link Checking

The `docs-link-check.yaml` workflow runs after merges to main and on a weekly schedule to check for broken links and create GitHub issues automatically:

- Runs after merges to main that affect documentation
- Runs weekly on Monday mornings
- Creates GitHub issues with broken link details
- Sends Slack notifications when issues are found

## Features

1. **Documentation Preview**: Generates preview links for documentation changes
2. **Vale Style Checking**: Enforces consistent terminology and style
3. **Link Validation**: Checks for broken links in documentation
4. **Markdown Linting**: Ensures proper markdown formatting with markdownlint-cli2
5. **Markdown Table Format Checking**: Checks (but doesn't apply) markdown table formatting
6. **PR Comments**: Creates or updates PR comments with preview links and validation results
7. **Post-Merge Validation**: Ensures documentation quality after merges to main
8. **Issue Creation**: Automatically creates GitHub issues for broken links

## Formatting Local Workflow

For formatting markdown tables, run the local command:

```bash
make fmt/markdown
```

The GitHub Actions workflow only checks formatting and reports issues but doesn't apply changes.

## Examples

See the `docs-reusable-example.yaml` workflow for a complete example that demonstrates both the reusable workflow and direct action usage.