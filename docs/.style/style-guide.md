# Coder documentation style guide

This is the canonical style guide for the Coder documentation. It is the
source of truth that the Vale rules in `docs/.style/styles/Coder/` enforce.

Status: scaffold. Sections below are populated by the rule-specific
tickets in the
[Docs style guide](https://linear.app/codercom/project/docs-style-guide-7828445b9afc)
project; this page starts as a table of contents and grows as those
tickets land.

## How to use this guide

- **Contributors**: read the section that matches what you are writing.
  Each rule notes the Vale rule ID, if any, so you can reproduce the
  warning locally.
- **Reviewers**: cite the section in a review comment. Reviews are easier
  when the guidance is in one place.
- **AI agents**: read this page in full before editing anything under
  `docs/`. The Coder Agents and Claude Code guides
  ([`AGENTS.md`](../../AGENTS.md),
  [`.claude/docs/DOCS_STYLE_GUIDE.md`](../../.claude/docs/DOCS_STYLE_GUIDE.md))
  link here.

## Voice and tone

To be filled in by rule-specific tickets. Planned coverage:

- Active voice
- Second person
- Plural nouns and pronouns where number is uncertain
- Product voice (`stop` over `kill`, `turn off` over `disable` in
  user-facing copy) - see
  [DOCS-183](https://linear.app/codercom/issue/DOCS-183)
- Limiting "we" - see
  [DOCS-35](https://linear.app/codercom/issue/DOCS-35)

## Word choice

To be filled in by rule-specific tickets. Planned coverage:

- Inclusive language substitutions - see
  [DOCS-182](https://linear.app/codercom/issue/DOCS-182)
- HashiCorp casing - see
  [DOCS-34](https://linear.app/codercom/issue/DOCS-34)
- Dev Container terminology - see
  [DOCS-33](https://linear.app/codercom/issue/DOCS-33)
- "Setup" vs "set up" and Quickstart casing - see
  [DOCS-36](https://linear.app/codercom/issue/DOCS-36)
- "Next steps" vs "Learn more" - see
  [DOCS-37](https://linear.app/codercom/issue/DOCS-37)
- Weasel words - see
  [DOCS-42](https://linear.app/codercom/issue/DOCS-42)

## Capitalization and punctuation

To be filled in by rule-specific tickets. Planned coverage:

- Sentence case in titles and headings
- General capitalization policy - see
  [DOCS-38](https://linear.app/codercom/issue/DOCS-38)
- Em-dash and en-dash ban (use comma, semicolon, or period) - see
  [DOCS-44](https://linear.app/codercom/issue/DOCS-44), origin tracked
  in [DOCS-181](https://linear.app/codercom/issue/DOCS-181)

## Formatting

To be filled in by rule-specific tickets. Planned coverage:

- Bold for UI elements
- Italics for parameter names and version variables
- Code font for user input, command-line utility names, filenames,
  environment variables, HTTP verbs and status codes, placeholder
  variables
- Code blocks with explicit language fences - see
  [DOCS-43](https://linear.app/codercom/issue/DOCS-43) for MD040

## Vale enforcement

The repo-root `.vale.ini` configures Vale to read styles from
`docs/.style/styles/`. The starter configuration combines:

- Google's developer-docs base style
- A curated subset of `alex` (inclusive-language)
- A curated subset of `write-good` (wordiness)
- Coder-specific custom rules in `docs/.style/styles/Coder/`

See [DOCS-40](https://linear.app/codercom/issue/DOCS-40) for the rationale
behind the cherry-picked base styles and the severity policy.

## Editor setup

To be filled in by
[DOCS-178](https://linear.app/codercom/issue/DOCS-178). Will cover VS
Code, Cursor, JetBrains, and Neovim.

## Third-party references

When this guide does not cover something, consult:

| Type of guidance     | Reference                                                                               |
|----------------------|-----------------------------------------------------------------------------------------|
| Spelling             | [Merriam-Webster](https://www.merriam-webster.com/)                                     |
| Style - nontechnical | [The Chicago Manual of Style](https://www.chicagomanualofstyle.org/home.html)           |
| Style - technical    | [Microsoft Writing Style Guide](https://learn.microsoft.com/en-us/style-guide/welcome/) |
