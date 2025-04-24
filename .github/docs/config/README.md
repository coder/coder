# Documentation Tool Configuration

This directory contains configuration files for the various documentation validation tools used in our workflow.

## Files

- `.markdownlint.yml`: Configuration for markdownlint rules
- `markdown-link-check.json`: Configuration for markdown-link-check tool

## Integration with MegaLinter

These configuration files are used by MegaLinter's documentation flavor. MegaLinter provides a standardized way to run various linters with consistent configuration and reporting.

## Markdownlint Configuration

The `.markdownlint.yml` file controls the rules for markdown linting, including:

- Heading structure and formatting
- List formatting and consistency
- Permitted HTML elements
- Line length requirements
- Code block formatting

## Link Checking Configuration

The `markdown-link-check.json` file configures link validation:

- URL patterns to ignore (anchors, mailto, etc.)
- URL replacement patterns
- Timeout and retry settings
- Valid status codes

## Local Usage

To run these tools locally:

```bash
# Run markdown linting
make lint/markdown

# Run full documentation validation (including links)
make lint/docs
```

## Adding or Modifying Rules

When modifying configuration files:

1. Test changes locally first
2. Document any significant changes in the PR
3. Consider impacts on existing documentation
4. Run the test script to validate integration: `.github/docs/testing/test-megalinter.sh`