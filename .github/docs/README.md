# Coder Documentation GitHub Actions

This directory contains GitHub Actions, configurations, and workflows for Coder's documentation.

## Directory Structure

- `actions/docs-setup`: Common setup action for documentation workflows
- `actions/docs-shared`: Phase-based composite action providing core documentation functionality
- `vale`: Configuration and style rules for Vale documentation linting
- `.lycheeignore`: Configuration patterns for lychee link checking

## Available Workflows

### Reusable Workflow

The `docs-unified.yaml` workflow provides a reusable workflow that can be called from other workflows. This combines all documentation checks in one workflow with optimized concurrent execution:

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
      check-cross-references: true
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
4. **Cross-Reference Validation**: Detects broken references when files or headings are changed/removed
5. **Markdown Linting**: Ensures proper markdown formatting with markdownlint-cli2
6. **Markdown Table Format Checking**: Checks (but doesn't apply) markdown table formatting
7. **PR Comments**: Creates or updates PR comments with preview links and validation results
8. **Post-Merge Validation**: Ensures documentation quality after merges to main
9. **Issue Creation**: Automatically creates GitHub issues for broken links
10. **Optimized Concurrent Execution**: Phases-based structure for parallel validation
11. **Unified Result Reporting**: Aggregates results from all validators into a single JSON structure

## Workflow Architecture

The documentation workflow is designed for maximum efficiency using a phase-based approach:

### Phase 1: Setup and Environment Validation
- Security configuration
- Directory validation
- Environment setup (Node.js, PNPM, Vale)

### Phase 2: File Analysis
- Identify changed documentation files
- Parse files into different formats for processing
- Check for manifest.json changes

### Phase 3: Concurrent Validation
- All validation steps run in parallel:
  - Markdown linting
  - Table formatting validation
  - Link checking
  - Vale style checking
  - Cross-reference validation

### Phase 4: Preview Generation
- Generate preview URLs for documentation changes
- Build links to new documentation

### Phase 5: Results Aggregation
- Collect results from all validation steps
- Normalize into a unified JSON structure
- Calculate success metrics and statistics
- Generate summary badge

### Phase 6: PR Comment Management
- Find existing comments or create new ones
- Format results in a user-friendly way
- Provide actionable guidance for fixing issues

## Unified Results Reporting

The workflow now aggregates all validation results into a single JSON structure:

```json
[
  {
    "name": "markdown-lint",
    "status": "success|failure",
    "output": "Raw output from the validation tool",
    "guidance": "Human-readable guidance on how to fix",
    "fix_command": "Command to run to fix the issue"
  },
  // Additional validation results...
]
```

### Benefits of Unified Reporting:

1. **Consistency**: All validation tools report through the same structure
2. **Integration**: JSON output can be easily consumed by other tools or dashboards
3. **Statistics**: Automatic calculation of pass/fail rates and success percentages
4. **Diagnostics**: All validation results in one place for easier debugging
5. **Extensibility**: New validators can be added with the same reporting format

## Formatting Local Workflow

For formatting markdown tables, run the local command:

```bash
make fmt/markdown
```

The GitHub Actions workflow only checks formatting and reports issues but doesn't apply changes.

## Examples

See the `docs-reusable-example.yaml` workflow for a complete example that demonstrates both the reusable workflow and direct action usage with:

1. Concurrent validation
2. Improved error reporting
3. Phase-based organization
4. Performance optimizations