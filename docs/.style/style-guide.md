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

## Headings

To be filled in by rule-specific tickets. Planned coverage:

- Sentence case in titles and headings - see
  [DOCS-38](https://linear.app/codercom/issue/DOCS-38)

### Gerund headings

**Rule**: `Coder.GerundHeading` (warning).

Avoid leading a heading with a gerund (an -ing word: "Installing,"
"Configuring," "Setting up"). Two alternatives almost always read more
cleanly:

1. **Imperative** for task headings. Use the bare verb instead of the
   -ing form when the section is a step or how-to.
2. **Noun** for concept headings. Use the noun form when the section
   describes a thing rather than an action.

| Avoid                        | Prefer (imperative)        | Prefer (noun)       |
|------------------------------|----------------------------|---------------------|
| Installing Coder             | Install Coder              | Installation        |
| Configuring authentication   | Configure authentication   | Authentication      |
| Setting up your workspace    | Set up your workspace      | Workspace setup     |
| Managing workspace schedules | Manage workspace schedules | Workspace schedules |

The right choice depends on the page. Imperative reads well in tutorials
and reference sections that walk through tasks. Noun reads well in
overview, conceptual, and feature-list pages. Concept-noun gerunds like
"Logging," "Monitoring," "Networking," and "Troubleshooting" are flagged
by the rule. Promote them to imperative ("Monitor your deployment") or
convert them fully to nouns ("Network architecture") when the rewrite
reads better; leave them as-is when the gerund-form is the established
term and the alternatives feel forced.

The rule fires on the first word of any heading or title that ends in
`-ing` and starts with a capital letter. A small exception list covers
non-gerund words that happen to end in `-ing` (`Bring`, `String`,
`Spring`, `King`, `Ring`, `Sting`, `Sing`, `Thing`, `Wing`). To silence a
specific instance that the exception list does not cover, wrap the
heading with `<!-- vale Coder.GerundHeading = NO -->` and
`<!-- vale Coder.GerundHeading = YES -->`. Add a justifying comment.

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

### Running Vale locally

See [`docs/.style/README.md`](README.md#running-vale-locally) for the
make target, the `--no-exit` rationale, and the rule-set pointer.

### Severity policy (v1)

Rule severity sits at a level that reflects two things together: the
rule's false-positive rate against real Coder docs and the gravity of the
rule. Low FPs plus high gravity argues for `error`; lower gravity or more
judgment calls argue for `warning` or `suggestion`.

v1 lands most rules at `warning` and the wordiness rules at `suggestion`.
A rule promotes to `error` only when (a) its false-positive rate against
real content is effectively zero and (b) the existing-content violation
count for that rule is also zero.

Vale exits non-zero only on error-level alerts, regardless of
`MinAlertLevel`. The Makefile invokes Vale with `--no-exit` so the
baseline error count from un-overridden Google rules does not fail CI;
real failures (missing binary, bad config) still propagate. Drop
`--no-exit` once the baseline error count is zero.

### Active rule set

The curated set lives in `.vale.ini`'s inline comments. Run
`make lint/prose` to see it in action. The high-level shape:

- **Google** as the base, with a handful of disables and softer levels
  on high-volume rules.
- **write-good** for wordiness, with passive voice and E-Prime off.
- **alex** loaded a la carte for the inclusive-language checks that do
  not fire on technical vocabulary.
- **Coder** for custom rules. Empty in v1; rules land through the
  rule-specific tickets in this project (see
  `docs/.style/styles/Coder/README.md`).

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
