# Coder Documentation Workflow

This directory contains GitHub Actions, configurations, and workflows for Coder's unified documentation validation system.

## Developer Quick Start Guide

```bash
# Check only changed documentation files (default behavior)
make lint/docs

# Check ALL documentation files
make lint/docs --all

# Format markdown tables and fix common issues
make fmt/markdown

# Run Vale style check (optional, requires Vale installation)
make lint/docs-style
```

The validation system will automatically detect and check only files that have changed in your working directory. This ensures fast feedback during development.

## Directory Structure

- `config`: Configuration files for markdown tools (markdownlint, markdown-link-check)
- `vale`: Configuration and style rules for Vale documentation linting
- `testing`: Test scripts and utilities for workflow validation

## Quick Start

For developers working with documentation, here are the most commonly used commands:

```bash
# Run comprehensive documentation validation (markdown lint + link checking)
make lint/docs

# Run only markdown linting
make lint/markdown

# Run optional style checking with Vale (if installed)
make lint/docs-style

# Fix formatting issues
make fmt/markdown   # Formats tables and markdown styling
```

## Local vs CI Validation

The validation that runs in CI is available locally through the same Makefile targets:

| GitHub Action | Local Command | Description |
|---------------|--------------|-------------|
| Markdown linting | `make lint/markdown` | Checks markdown formatting |
| Link checking | `make lint/docs` | Verifies links aren't broken |
| Vale style checking | `make lint/docs-style` (optional) | Validates documentation style with Vale |
| Cross-reference validation | *Part of CI only* | Checks references between docs |

### Optional Tool Installation

While basic linting works out-of-the-box with node dependencies, additional tools can be installed for more advanced checks:

```bash
# Install Lychee for link checking (recommended)
cargo install lychee

# Install Vale for style checking (optional)
brew install vale

# Node dependencies for markdown formatting (required)
pnpm install
```

# Coder Documentation Workflow System

## Workflow Architecture

The documentation workflow system uses MegaLinter and standardized GitHub Actions to provide a comprehensive validation pipeline:

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
                                       │
                                       ▼
┌─ Local Development Integration ────────────────────────────────────────────────┐
│                                                                               │
│  Makefile targets that mirror workflow functionality:                         │
│  ┌───────────────┐ ┌───────────────┐ ┌───────────────┐ ┌───────────────┐     │
│  │ make lint/docs│ │make fmt/markdown│ │make lint/markdown│ │make lint/docs-style│     │
│  └───────────────┘ └───────────────┘ └───────────────┘ └───────────────┘     │
│                                                                               │
```

## Documentation Workflow Components

### Entry Point Workflows

The system provides four specialized entry points for different validation scenarios:

1. **PR Preview (docs-preview.yaml)**
   - Triggers on PR create/update when docs files change
   - Performs comprehensive validation on documentation changes
   - Generates preview links and posts PR comments with results
   - Skips link checking for faster feedback

2. **Post-Merge Validation (docs-link-check.yaml)**
   - Runs after merges to main branch
   - Lightweight check focused only on link integrity
   - Ensures merged content maintains external link validity

3. **Weekly Check (weekly-docs.yaml)**
   - Scheduled run every Monday at 9 AM
   - Comprehensive validation of documentation health
   - Checks links, cross-references, markdown structure, and formatting
   - Creates issues for persistent problems

4. **CI Check (docs-ci.yaml)**
   - Fast validation for continuous integration
   - Focuses on formatting and structural issues
   - Designed for rapid feedback

### Unified Workflow & Presets

All entry points use the central `docs-unified.yaml` workflow with different preset configurations:

| Preset | Description | Main Validations | When Used |
|--------|-------------|------------------|-----------|
| `pr` | Complete validation for PRs | markdown, formatting, style, cross-references | PRs that modify docs (preview workflow) |
| `post-merge` | Lightweight check after merge | links | After merging to main (catches broken links) |
| `weekly` | Scheduled health check | markdown, formatting, links, cross-references | Weekly cron job (comprehensive check) |
| `ci` | Fast CI validation | markdown, formatting | PR checks (fast feedback) |

## Key Tools and Integrations

### MegaLinter Documentation Flavor

The workflow leverages MegaLinter's documentation flavor to provide comprehensive validation:

- **markdownlint**: Validates markdown syntax and formatting
- **Vale**: Checks documentation style and terminology
- **markdown-link-check**: Verifies links are valid and accessible

Configuration files are stored in standardized locations:
- `.github/docs/config/.markdownlint.yml`: Markdown linting rules
- `.github/docs/vale/.vale.ini`: Vale style configuration
- `.github/docs/config/markdown-link-check.json`: Link checking settings

### Changed Files Detection

The workflow uses tj-actions/changed-files to efficiently detect changed files:

```yaml
# Get changed files
- name: Get changed files
  id: changed-files
  uses: tj-actions/changed-files@v41
  with:
    files: |
      docs/**/*.md
      **/*.md
```

### Cross-Reference Validation

Custom cross-reference validation checks for broken internal links:

- References to deleted files
- Broken internal markdown links
- Missing image references

## Vale Style Checking

The workflow includes Vale style checking that:
- Only examines changed files to improve performance
- Validates documentation against Coder style guidelines
- Uses the errata-ai/vale-action GitHub Action
- Is configured in `.github/docs/vale/` with custom rules

### Vale Style Rules

The following style rules are configured:

| Rule | Description | Severity |
|------|-------------|----------|
| `Coder.Headings` | Ensures proper heading capitalization | warning |
| `Coder.Terms` | Enforces consistent terminology | warning |
| `Coder.RepeatedWords` | Catches repeated words like "the the" | error |
| `Coder.SentenceLength` | Warns about overly long sentences | suggestion |
| `GitLab.*` | Various rules from GitLab style guide | varies |

To suppress a Vale rule for a specific line:

```markdown
<!-- vale Coder.SentenceLength = NO -->
This is a very long sentence that would normally trigger the sentence length rule but has been explicitly exempted for a good reason such as a technical requirement or quotation.
<!-- vale Coder.SentenceLength = YES -->
```

## Workflow Configuration Options

Each workflow entry point can be customized with these key parameters:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `preset` | Predefined configuration bundle | (required) |
| `lint-markdown` | Run markdown linting | (from preset) |
| `check-format` | Validate table formatting | (from preset) |
| `check-links` | Verify link integrity | (from preset) |
| `check-cross-references` | Check documentation cross-references | (from preset) |
| `lint-vale` | Run Vale style validation | (from preset) |
| `generate-preview` | Create preview links | (from preset) |
| `post-comment` | Post results as PR comment | (from preset) |
| `create-issues` | Create GitHub issues for failures | (from preset) |
| `fail-on-error` | Fail workflow on validation errors | (from preset) |

## Using Documentation Validation in Custom Workflows

To use documentation validation in your own workflows:

```yaml
jobs:
  custom-docs-check:
    uses: ./.github/workflows/docs-unified.yaml
    with:
      preset: 'pr'  # Choose a preset based on your needs
      # Optional overrides
      check-links: false  # For faster checks
      notification-channels: 'slack'  # For notifications
```

Available presets:
- `pr`: Full validation with PR comments and preview links
- `post-merge`: Lightweight link checking for merged content
- `weekly`: Comprehensive health check for scheduled runs
- `ci`: Fast validation for continuous integration

The presets provide sensible defaults for each use case, which can be overridden as needed for specific scenarios.