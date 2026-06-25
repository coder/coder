# Coder documentation style guide

This is the canonical style guide for the Coder documentation. It is the
source of truth that the Vale rules in `docs/.style/styles/Coder/` enforce.

Status: scaffold. Sections below are populated by follow-up PRs; this
page starts as a table of contents and grows as those PRs land.

## How to use this guide

This page is a scaffold while follow-up PRs land. Sections marked "To be
filled in" are placeholders. For anything not yet covered, see the
public summary at
[`docs/about/contributing/documentation.md`](../about/contributing/documentation.md).

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

To be filled in by follow-up PRs. Planned coverage:

- Active voice
- Second person
- Plural nouns and pronouns where number is uncertain
- Product voice (`stop` over `kill`, `turn off` over `disable` in
  user-facing copy)
- Limiting "we"

## Word choice

To be filled in by follow-up PRs. Planned coverage:

- Inclusive-language substitutions
- HashiCorp casing
- Dev Container terminology
- "Setup" vs "set up" and Quickstart casing
- "Next steps" vs "Learn more"
- Weasel words

## Capitalization and punctuation

To be filled in by follow-up PRs. Planned coverage:

- Sentence case in titles and headings
- General capitalization policy
- Em-dash and en-dash ban (use comma, semicolon, or period)

## Formatting

To be filled in by follow-up PRs. Planned coverage:

- Bold for UI elements
- Italics for parameter names and version variables
- Code font for user input, command-line utility names, filenames,
  environment variables, HTTP verbs and status codes, placeholder
  variables
- Code blocks with explicit language fences

## Vale enforcement

The repo-root `.vale.ini` configures Vale to read styles from
`docs/.style/styles/`. The starter configuration combines:

- Google's developer-docs base style
- A curated subset of `alex` (inclusive-language)
- A curated subset of `write-good` (wordiness)
- Coder-specific custom rules in `docs/.style/styles/Coder/`

The rationale for the cherry-picked base styles and the severity
policy lives in `.vale.ini`'s inline comments. Run `make lint/prose`
to reproduce the baseline locally.

## Editor setup

To be filled in by a follow-up PR. Will cover VS Code, Cursor,
JetBrains, and Neovim.

## Relationship to `docs/about/contributing/documentation.md`

A public-facing prose summary lives today at
[`docs/about/contributing/documentation.md`](../about/contributing/documentation.md).
A follow-up PR will redirect that page to this guide; until then,
follow the public summary for anything the scaffolded sections above do
not yet cover. New prose rules land here; the public page is frozen
pending the redirect.

## Third-party references

When this guide does not cover something, consult:

| Type of guidance    | Reference                                                                               |
|---------------------|-----------------------------------------------------------------------------------------|
| Spelling            | [Merriam-Webster](https://www.merriam-webster.com/)                                     |
| Style, nontechnical | [The Chicago Manual of Style](https://www.chicagomanualofstyle.org/home.html)           |
| Style, technical    | [Microsoft Writing Style Guide](https://learn.microsoft.com/en-us/style-guide/welcome/) |
