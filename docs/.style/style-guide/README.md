# Coder documentation style guide

This is the canonical prose style guide for the Coder documentation.
It tells you *how* to write the words that go in the docs.
For decisions about what belongs in the docs and what doesn't, refer to [`content-guidelines.md`](../content-guidelines.md).

Each rule on the linked pages is a policy decision the Coder docs team has made.
Where a Vale rule already enforces the policy, the rule name is listed in a parenthetical so you can reproduce the warning locally.
Where the rule is documentation-only, the parenthetical says so.
The doctrine for adding Vale rules lives in the [Vale doctrine README](../README.md).

## How to use this guide

- **Contributors**: read the section that matches what you're writing.
  Each rule includes a brief rationale and **Do** / **Don't** examples.
- **Reviewers**: cite the section in a review comment.
  Reviews are easier when the guidance lives in one place.
- **AI agents**: read every section before editing anything under `docs/`.
  The Coder Agents and Claude Code guides ([`AGENTS.md`](../../../AGENTS.md), [`.claude/docs/DOCS_STYLE_GUIDE.md`](../../../.claude/docs/DOCS_STYLE_GUIDE.md)) link here.

## Sections

| Page                                                                  | Covers                                                                                                                                                                                                      |
|-----------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Audience and scope](./audience-and-scope.md)                         | One audience per page; one outcome per page; declare both up front; Coder personas                                                                                                                          |
| [Voice and tone](./voice-and-tone.md)                                 | Second person; no first-person singular; "we" as the company, not the software; active voice; present tense; sentence and paragraph budgets; verbs over noun forms; pronoun referents                       |
| [Procedural writing](./procedural-writing.md)                         | One instruction per step; condition before instruction; step length; callouts inform, steps instruct; warnings state the consequence                                                                        |
| [Word choice](./word-choice.md)                                       | Canonical brand and product names; one term per concept; "refer to" over "see"; "select" over "click"; weasel words; plain English for product actions; keep internal-only references out of published docs |
| [Accessibility and inclusion](./accessibility-and-inclusion.md)       | WCAG target; inclusive pronouns and substitutions; descriptive link text; alt text; page descriptions; heading structure; reading level                                                                     |
| [Capitalization and punctuation](./capitalization-and-punctuation.md) | Sentence-case headings; no gerund leads; no em-dashes; commas; US-style quotation                                                                                                                           |
| [Formatting](./formatting.md)                                         | Bold for UI; italics for emphasis; code font for identifiers; language fences on code blocks; callouts; tabs; lists; tables; links; images; screenshots sparingly                                           |
| [Numbers, units, and dates](./numbers-units-and-dates.md)             | Digits for 6 and higher, either way for 0-5; "more than X"/"X or more" always spelled out below 6; non-breaking space between number and unit; `Month Day, Year` dates; 12-hour time with AM/PM             |
| [Editor setup](./editor-setup.md)                                     | Vale editor integration for VS Code, Cursor, JetBrains, and Neovim (placeholder)                                                                                                                            |

## Conventions for editing Coder docs

These conventions apply to every Markdown file under `docs/`.
The style guide subpages dogfood them so contributors can see the rules in action.

### One sentence per line

Source lines in Coder documentation follow a one-sentence-per-line policy.
Each sentence sits on its own Markdown source line.
Sentences aren't split across lines, and lines don't wrap to a fixed column width.

The rendered Markdown joins lines inside a paragraph back together, so the source line breaks don't appear in the rendered output.
Reviewers reading the diff do encounter them, and the per-sentence lines make diffs land cleanly at the sentence level.

`markdownlint`'s `MD013` (line length) is already disabled, so the convention is editorial.
Configure editors that auto-wrap on save to leave the source alone.

#### Incremental adoption

The Coder docs corpus predates this convention.
Much of the existing prose still wraps to a fixed column width or runs on a single long line.
Some paragraphs on the other pages of this style guide still carry semantic line breaks (sembr) from earlier commits.
The convention is adopted incrementally.

A contributor who edits any line inside a paragraph reformats the entire paragraph to one sentence per line as part of the same edit.
The contributor doesn't reformat surrounding paragraphs they didn't otherwise touch.

For this rule, a bullet item, a numbered list entry, and a blockquote line are each their own paragraph.
Headings, fenced code blocks, and tables are out of scope: headings are single lines by convention, code blocks render their source verbatim, and `markdown-table-formatter` governs table rows.

### The style guide doesn't use "see" for navigation

The [Word choice page](./word-choice.md) bans "see" as a navigational verb across all docs.
The style guide itself follows the rule: "refer to" for formal cross-references, "check out" for informal pointers in tutorial-style passages, "visit" for external URLs.
Reserve "see" for the rare case where the prose describes what a reader observes in the product UI.

## Vale enforcement

The repo-root `.vale.ini` loads only the Coder rule package by default.
Third-party rules from Google, alex, and write-good aren't enabled until a per-rule PR brings each back in.

Each enabled rule lands via a dedicated PR that:

1. Cleans the corpus to zero baseline findings.
2. Adds the rule line in `.vale.ini` at the rule author's chosen severity.
3. Adds the corresponding section to the appropriate subpage of this guide.

Severity is a deliberate per-rule choice among 3 tiers:

- `error` blocks merge in CI.
  Use for hard policy where any violation is wrong.
- `warning` surfaces an annotation without failing CI.
  Use for strong guidance with legitimate human-judgment exceptions.
- `suggestion` surfaces a `notice` annotation.
  Use for soft guidance where the right fix is contextual.

The full doctrine, including the false-positive policy, lives in the [Vale doctrine README](../README.md).
This guide is itself exempt from the Coder rules: it demonstrates the violations those rules ban, so the repo-root `.vale.ini` clears `BasedOnStyles` for `docs/.style/style-guide/**`.
Zero baseline is measured over `docs/` excluding `docs/.style/style-guide/`.
Run `make lint/prose` to reproduce the baseline locally.

## Relationship to `docs/about/contributing/documentation.md`

A public-facing prose summary lives today at [`docs/about/contributing/documentation.md`](../../about/contributing/documentation.md).
A follow-up PR will redirect that page to this guide.
Until then, follow the public summary for anything the subpages of this guide don't cover.
New prose rules land here.
The public page is frozen pending the redirect.

## Third-party references

When this guide doesn't cover something, consult:

| Type of guidance          | Reference                                                                               |
|---------------------------|-----------------------------------------------------------------------------------------|
| Spelling                  | [Merriam-Webster](https://www.merriam-webster.com/)                                     |
| Style, nontechnical       | [The Chicago Manual of Style](https://www.chicagomanualofstyle.org/home.html)           |
| Style, technical          | [Microsoft Writing Style Guide](https://learn.microsoft.com/en-us/style-guide/welcome/) |
| Style, developer-focused  | [Google developer documentation style guide](https://developers.google.com/style)       |
| Style, procedural writing | [ASD-STE100 Simplified Technical English](https://www.asd-ste100.org/) (Issue 9, 2025)  |
