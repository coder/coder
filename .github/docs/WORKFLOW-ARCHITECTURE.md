# Documentation Workflow Architecture

This document explains the documentation workflow architecture, which handles validation and preview processes for Coder's documentation.

## Architecture Overview

The documentation workflow system is built on a "pipeline" architecture, leveraging industry-standard tools and GitHub Actions best practices:

```
┌─ Workflow Entry Points ───────────────────────────────────────────────────────┐
│                                                                               │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  ┌───────────┐ │
│  │   PR Preview    │  │    Post-Merge   │  │     Weekly      │  │  CI Check │ │
│  │   Workflow      │  │    Validation   │  │     Checks      │  │  Workflow │ │
│  │ docs-preview.yml│  │  docs-ci.yml    │  │weekly-docs.yml  │  │docs-ci.yml│ │
│  │                 │  │                 │  │                 │  │           │ │
│  │ • Runs on PR    │  │ • Runs after    │  │ • Runs weekly   │  │ • Runs on │ │
│  │   creation/update│  │   merges to main│  │   on schedule  │  │   PR      │ │
│  │ • Generates     │  │ • Checks links  │  │ • Comprehensive │  │ • Basic   │ │
│  │   preview links │  │   only          │  │   validation    │  │   checks  │ │
│  │ • Validates docs│  │ • Falls back to │  │ • Planned: issues│  │ • Fast    │ │
│  │ • Posts comments│  │   original doc  │  │   for problems  │  │   feedback│ │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘  └─────┬─────┘ │
│           │                    │                    │                  │       │
└───────────┼────────────────────┼────────────────────┼──────────────────┼───────┘
            │                    │                    │                  │
            └──────────┬─────────┴──────────┬─────────┴──────────┬──────┘
                       │                    │                    │
                       ▼                    ▼                    ▼
┌─ Unified Reusable Workflow ─────────────────────────────────────────────────────┐
│                                                                                 │
│  docs-unified.yaml                                                              │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ Configuration System                                                       │  │
│  │ ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐               │  │
│  │ │ PR Preset  │ │Post Preset │ │Weekly Preset│ │ CI Preset  │               │  │
│  │ └────────────┘ └────────────┘ └────────────┘ └────────────┘               │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ Validation Pipeline                                                        │  │
│  │ ┌────────────────┐  ┌────────────────┐  ┌────────────────┐                │  │
│  │ │ MegaLinter     │  │ Cross-reference│  │ Result Reporting│                │  │
│  │ │ Documentation  │  │ Validation     │  │ and Comments    │                │  │
│  │ └────────────────┘  └────────────────┘  └────────────────┘                │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

## Key Components

### 1. Workflow Entry Points

- **docs-preview.yaml**: Quick previews for PR changes
- **docs-link-check.yaml**: Post-merge validation focusing on link integrity
- **weekly-docs.yaml**: Scheduled comprehensive checks
- **docs-ci.yaml**: Fast syntax checking for CI processes

### 2. Unified Reusable Workflow

The `docs-unified.yaml` workflow serves as a central orchestration point:

- Provides standardized configuration presets
- Manages file change detection using tj-actions/changed-files
- Runs validation tools through MegaLinter's documentation flavor
- Processes and reports validation results 

### 3. MegaLinter Integration

The [MegaLinter Documentation Flavor](https://megalinter.io/latest/flavors/documentation/) provides standardized validation:

- **markdownlint**: Checks markdown syntax and formatting
- **Vale**: Validates writing style and terminology
- **markdown-link-check**: Verifies links are valid

### 4. Cross-Reference Validation

Custom validation for internal document references:

- Checks references to deleted files
- Validates internal document links
- Verifies image references exist

## Configuration Files

### 1. Markdownlint Configuration

Located at `.github/docs/config/.markdownlint.yml`:

- Controls markdown formatting and style rules
- Sets document structure requirements
- Configures allowed HTML elements

### 2. Vale Configuration

Located at `.github/docs/vale/.vale.ini`:

- Defines style guide rules
- Sets alert levels and scopes
- Controls terminology enforcement

### 3. Link Checking Configuration

Located at `.github/docs/config/markdown-link-check.json`:

- Defines URL patterns to ignore
- Sets timeout and retry parameters
- Configures URL replacement patterns

## Configuration Presets

The workflow system includes standardized configuration presets for common scenarios:

### PR Preset

```yaml
# For doc changes in PRs
preset: 'pr'  
```

- Full validation with preview URLs
- PR comments with direct links to changed files
- Doesn't fail on validation errors

### Post-Merge Preset

```yaml
# For recent changes to main branch
preset: 'post-merge'
```

- Focuses on link and cross-reference validation
- Notifies about broken links (issue creation planned)
- No preview generation

### Weekly Preset

```yaml
# For scheduled comprehensive checks
preset: 'weekly'
```

- Comprehensive link checking of all documentation
- Strict validation with failure on errors
- Planned feature: GitHub issues with detailed diagnostics

### CI Preset

```yaml
# For fast checks during CI
preset: 'ci'
```

- Rapid syntax and format checking
- Minimal dependency requirements
- Fails fast on errors

## Key Features

### 1. MegaLinter Documentation Flavor

- Standardized, well-maintained validation tools
- Configuration-driven rather than implementation-heavy
- Comprehensive reporting and diagnostics
- Support for multiple validation tools in one step

### 2. File Detection with tj-actions/changed-files

- Reliable detection of changed files
- Support for various file patterns
- Integration with GitHub's API

### 3. Vale Style Checking

- Consistent terminology enforcement
- Writing style validation
- Custom rules for Coder documentation

### 4. Two-Stage PR Comments

- Initial comment with preview links while validation runs
- Updated comment with comprehensive validation results
- Status indicators and direct links to documentation

## Usage Examples

### Basic Usage with Preset

```yaml
jobs:
  docs-check:
    uses: ./.github/workflows/docs-unified.yaml
    with:
      preset: 'pr'
```

### Using Preset with Overrides

```yaml
jobs:
  docs-check:
    uses: ./.github/workflows/docs-unified.yaml
    with:
      preset: 'pr'
      check-links: false  # Skip link checking for faster results
      notification-channels: 'slack'  # Add Slack notifications
```

### Full Custom Configuration

```yaml
jobs:
  docs-check:
    uses: ./.github/workflows/docs-unified.yaml
    with:
      # Explicitly configure everything
      lint-markdown: true
      check-format: true
      check-links: true
      check-cross-references: true
      lint-vale: true
      generate-preview: true
      post-comment: true
      create-issues: false
      fail-on-error: false
      notification-channels: 'github-issue'
      issue-labels: 'documentation,urgent'
```

## PR Comment Features

When running with `post-comment: true` (included in the PR preset), the workflow posts a comprehensive PR comment containing:

### 1. Status Overview

A summary of all validation checks with clear status indicators (✅, ⚠️, ❌).

### 2. Preview Links

Direct links to preview your documentation changes, including:
- Main documentation
- Installation guide
- Getting Started guide
- Links to specific changed files

### 3. Validation Stats

Detailed statistics about the validation run:
- Number of files checked
- Percentage of successful validations
- Processing time

## Design Decisions

### Why MegaLinter?

We chose MegaLinter for several reasons:

1. **Standardization**: Uses common tool configurations
2. **Maintenance**: Regularly updated with security patches
3. **Performance**: Optimized for GitHub Actions environment
4. **Documentation**: Well-documented configuration options
5. **Extensibility**: Easy to add new linters when needed

### Pipeline vs. Composite Action

We moved from a composite action to a pipeline architecture because:

1. **Isolation**: Each validation step is independent
2. **Error Handling**: Better control of failures and reporting
3. **Flexibility**: Easier to add or remove validation steps
4. **Transparency**: Clear workflow progression visible in GitHub UI

### tj-actions/changed-files for File Detection

We chose tj-actions/changed-files for reliable file detection:

1. **Reliability**: Consistent results across different GitHub event types
2. **Flexibility**: Supports multiple file patterns and exclusions
3. **Maintained**: Regular updates and good support
4. **Performance**: Efficient detection without complex custom logic

## Troubleshooting

### Workflow Issues

1. **File Detection Failures**
   - Check the workflow logs for changed file detection results
   - Verify that files match the expected patterns
   - Files outside the monitored patterns will not trigger validation

2. **MegaLinter Issues**
   - Check the MegaLinter output for specific error messages
   - Verify configuration file paths are correct
   - Check for syntax issues in configuration files

3. **Preview URL Issues**
   - Branch names with slashes will have slashes converted to dashes
   - Use the direct links provided in the PR comment
   - Check if the docs preview server is properly configured

### Local Validation

To run validation locally before creating a PR:

```bash
# Check markdown formatting
make lint/markdown

# Run comprehensive validation
make lint/docs

# Run Vale style checking (if installed)
make lint/docs-style

# Format markdown tables
make fmt/markdown
```

## Future Improvements

1. **Automated Issue Creation**
   - Implement GitHub issue creation for persistent problems
   - Tag appropriate teams based on issue category

2. **Documentation Quality Metrics**
   - Track documentation quality over time
   - Generate reports on common issues

3. **Integration with AI Tools**
   - Implement automated fix suggestions using AI
   - Add content quality analysis beyond syntax and style