# Vale Configuration for Coder Documentation

This directory contains the Vale configuration for linting Coder's documentation style. The configuration is based on the Google developer documentation style guide and includes additional Coder-specific terminology rules.

## Configuration

- `.vale.ini`: Main configuration file that sets up Vale
- `styles/`: Directory containing style files and rules
  - `Coder/`: Custom Coder-specific style rules
    - `Terms.yml`: Coder-specific terminology and preferred terms

## Usage

This Vale configuration is integrated into the docs shared GitHub Action. When a PR includes documentation changes, Vale automatically runs and provides style feedback in the PR comment.

To test Vale locally:

1. Install Vale: https://vale.sh/docs/vale-cli/installation/
2. Run Vale on specific files:
   ```
   vale --config=.github/vale/.vale.ini path/to/file.md
   ```

## Rule Sets

The configuration uses these rule sets:

1. **Google**: Style rules from Google's developer documentation style guide
2. **Write-good**: General style suggestions for clear, concise writing
3. **Coder**: Custom rules specific to Coder documentation and terminology

## References

- [Vale documentation](https://vale.sh/docs/)
- [Google developer documentation style guide](https://developers.google.com/style)