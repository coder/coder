# Vale Style Checking for Coder Documentation

This directory contains configuration files and custom style rules for Vale, a syntax-aware linter for prose.

## Integration with MegaLinter

Vale is integrated into our documentation workflow through MegaLinter's documentation flavor. This approach provides:

1. Standardized configuration and execution
2. Consistent reporting format
3. Integration with other linting tools

## Configuration

The primary Vale configuration is in `.vale.ini`, which:

- Sets style baselines and alert levels
- Configures which types of content to ignore
- Defines which style rules to apply

## Custom Style Rules

We maintain several custom style rule sets:

### Coder Style

Located in `styles/Coder/`:

- `Headings.yml`: Ensures proper heading capitalization
- `Terms.yml`: Enforces consistent terminology
- `RepeatedWords.yml`: Catches repeated words like "the the"
- `SentenceLength.yml`: Warns about overly long sentences

### GitLab Style

We also include selected rules from GitLab's style guide in `styles/GitLab/`.

## Using Vale Locally

### Installation

```bash
# macOS
brew install vale

# Linux
curl -sfL https://github.com/errata-ai/vale/releases/download/v2.30.0/vale_2.30.0_Linux_64-bit.tar.gz | tar -xz -C ~/.local/bin vale
chmod +x ~/.local/bin/vale

# Windows (with Chocolatey)
choco install vale
```

### Running Vale

```bash
# Option 1: Use make target (preferred)
make lint/docs-style

# Option 2: Run Vale directly
vale --config=.github/docs/vale/.vale.ini docs/

# Option 3: Check specific files
vale --config=.github/docs/vale/.vale.ini path/to/file.md
```

## Suppressing Vale Warnings

To suppress Vale warnings in specific sections of a document:

```markdown
<!-- vale Coder.SentenceLength = NO -->
This is a very long sentence that would normally trigger the sentence length rule but has been explicitly exempted for a good reason such as a technical requirement or quotation.
<!-- vale Coder.SentenceLength = YES -->
```

To disable Vale for an entire file, add this to the frontmatter:

```markdown
---
vale: false
---
```

## Adding New Rules

1. Create a new YAML file in the appropriate style directory
2. Follow the [Vale package format](https://vale.sh/docs/topics/packages/)
3. Test your rule with `vale --config=.github/docs/vale/.vale.ini --glob='*.md' /path/to/test/file.md`
4. Update this README with information about your new rule