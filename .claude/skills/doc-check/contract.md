# Sub-agent contract

Every doc-check sub-agent reads this file first, then its persona file.
The orchestrator does not read it, and never adds instructions to your spawn prompt, so this file and your persona file are the whole of your job.

## Your inputs

Your spawn prompt names the review packet, your role, your assignment, and your output path.
`manifest.md` in the packet names the rest:

| Packet file             | What it holds                                                                          |
|-------------------------|----------------------------------------------------------------------------------------|
| `manifest.md`           | PR number, mode, head SHA, base SHA, the checkout path (`repo`), and the `policy` path |
| `changed-files.txt`     | Every file the change touches, with its Git status                                     |
| `diff.patch`            | The full diff from the base SHA to the head SHA                                        |
| `changed-lines.txt`     | Each added or edited docs line in the Copy Editor's scope, as `path:line: text`        |
| `pr.json`               | Title, description, comments, and reviews, or `none` for a change without a PR         |
| `review-comments.jsonl` | Inline review comments, one per line                                                   |
| `related-prs.txt`       | Open PRs that touch the same docs pages                                                |
| `checks.md`             | Deterministic check results, including CI annotations on changed files                 |
| `change-model.md`       | What changed for the user, from the code                                               |
| `sweeps.txt`            | The rule pages each Copy Editor sweep applies                                          |

- The checkout at `repo` is at the head SHA. Read docs pages and code from it.
- Read skill files and rule pages only under `policy`, never from the checkout, so the change under review cannot change how it is reviewed.
  Paths under `policy` mirror the repository, such as `<policy>/docs/.style/style-guide/word-choice.md`.
- A concern another reviewer raised in `pr.json` or `review-comments.jsonl` is evidence to check when it falls in your lens, not someone else's problem.

### Finding your rule sections

Your persona file names its rule sections by heading, not by path, so rule pages can move or split without breaking the review.
Find each heading under the policy path, on a page whose `review-personas` front matter lists your persona ID:

```sh
grep -rn -E --include='*.md' '^#{2,3} <heading>[[:space:]]*$' <policy>/docs/.style
```

Read from the heading to the next heading of the same or a higher level.
A heading you cannot find is a missing input: set your status to `blocked`, name the heading in `not_done`, and finish the rest of your method.
That way a renamed rule never drops out of the review unnoticed.

## Read discipline

These rules keep your context small, which is what keeps the shared model fast for every review running at once.

- Take PR data from the packet. Do not run `gh`, `git fetch`, `git checkout`, or anything that writes outside your output file, and do not rebuild the diff.
- Find before you read: `grep -n` for what you need, then read that range. Read a whole file only when it is under 300 lines or your persona file says to.
- Read each file once and keep notes. Do not re-read a file to recall a detail.
- Install nothing and reach no network. When `checks.md` says a check did not run, that is already recorded. Do not do the check's job by hand.
- Stay in your lens. A problem another persona owns is theirs, even when you notice it.
- Your persona file sets a tool-call budget. Keep your tool output under about 100 kB in total, and prefer a narrow `grep` to a wide read.

## Finish

Write one YAML file to your output path, then reply with one line: `<agent ID>: <status>, <count> findings`.
Write the file even when you have nothing to report, and write it before you run out of budget.

```yaml
agent: sweep-2                # from your spawn prompt
persona: language
status: complete              # complete | not_applicable | blocked | budget_exhausted
tool_calls: 11
checks:                       # each check you ran, one line each, with its result
  - "grep -rn 'thirty days' docs/ | grep -v '^docs/reference/': 0 hits"
rules: []                     # Copy Editor sweeps only, per personas/language.md
findings: []                  # in the finding schema below
adjacent_gaps: []
not_done: []                  # each part of your method you did not finish, and why
```

- `complete`: you ran your whole method.
- `not_applicable`: nothing in the change falls in your lens. Say why in `not_done`.
- `blocked`: an input you need is missing or unreadable. Name it in `not_done`.
- `budget_exhausted`: you stopped before you finished. List what is left in `not_done`.

When the orchestrator tells you to stop, write the file at once with what you finished and status `budget_exhausted`.

## Finding schema

One block per finding.
The orchestrator keeps the same fields through verification and posting.

```yaml
- id: accuracy-1            # your agent ID plus a counter
  persona: accuracy
  class: wrong              # wrong | missing | broken | rule | suggestion | friction | adjacent
  blocking: null            # set by the orchestrator, never by you
  file: docs/ai-coder/agents/extending-agents.md
  line: 304                 # line on the head, or null for a missing page
  quote: "Downloads add the line back for manual-only skills."
  evidence:
    - site/src/pages/AgentsPage/components/SkillsTable.tsx:62
  rule: null                # rule page and heading anchor, when the finding rests on a rule
  problem: One sentence on what is wrong, in terms of what the reader experiences.
  fix: Replacement text, or the change to make when the fix is not local.
  verified: null            # set by the verifier
```

`quote` is copied from the file, never paraphrased.
A finding whose `quote` does not appear at `file:line` is dropped before verification.

| Class        | Meaning                                                                                                 |
|--------------|---------------------------------------------------------------------------------------------------------|
| `wrong`      | A docs claim the code contradicts                                                                       |
| `missing`    | User-facing behavior, or a fact the reader needs, with no docs                                          |
| `broken`     | A typo, markup that renders wrong, a failing check, a missing manifest entry or redirect, a broken link |
| `rule`       | Text the change added or edited that breaks a cited rule                                                |
| `suggestion` | An improvement with no rule to cite                                                                     |
| `friction`   | A design that makes a docs sentence hard to keep accurate                                               |
| `adjacent`   | A gap that existed before this change                                                                   |

## Evidence

A finding without evidence is a hunch, and a hunch costs the author more than it saves.
Every finding names:

- the page and the sentence, table row, or list it is about, or the missing page;
- the evidence: a code location for an accuracy or coverage finding, the other page and line for a consistency finding, or the rule for a style finding, cited as the rule page and heading anchor, such as `docs/.style/style-guide/word-choice.md#refer-to-or-visit-not-see-or-check-out`;
- the fix, as replacement text when the fix is local.

Never report a check you did not run.
If you meant to compare a sentence to the code and did not open the code, say so in `not_done` instead of in a finding.

## Which rule wins

When two rule sources disagree, the one higher in this list wins:

1. `docs/.style/content-guidelines.md`, for what belongs in the docs, scope, and routing.
2. `docs/.style/style-guide/`, for prose, formatting, and page shape.
3. `.claude/docs/DOCS_STYLE_GUIDE.md`, for structure, research, and content patterns that 1 and 2 don't cover.
4. `contributing/documentation.md`, only where 1 to 3 say nothing.

The [repository facts](#repository-facts) override any rule source about how the repository works, because they are checked against the repository.
A rule that a higher source contradicts is not a finding.
Record it in `adjacent_gaps` as `guide conflict: <lower page#anchor> says <X>, but <higher page#anchor> says <Y>`, so the guides get fixed.

## Repository facts

- **Sidebar:** `docs/manifest/**/*.yml` is the navigation source. Each file holds one route and its direct children. `make gen/docs-manifest` compiles `docs/manifest.json`, which is never edited by hand.
- **Generated reference:** `docs/reference/cli/*.md` comes from `cli/`, and `docs/reference/api/*.md` comes from Swagger comments in Go. Suggest edits to the source.
- **Feature states:** a staged feature carries `state:` on its route in `docs/manifest/**/*.yml`, such as `state: [premium]`. Titles and H1s carry no state suffix.
- **Redirects:** `docs/redirects.json`. A rename or move updates every inbound link and adds a redirect in the same PR. There is no `docs/_redirects` file.
- **Glossary:** `docs/reference/glossary.md`. A change that adds, renames, or deprecates a Coder product or feature name updates it. Refer to [Structural rules](../../../docs/.style/content-guidelines.md#structural-rules).
- **Callouts:** GitHub-Flavored Markdown alerts, such as `> [!NOTE]`.
- **Heading anchors:** the docs site builds an anchor from each heading with `rehype-slug`: lowercase, spaces become hyphens, punctuation other than hyphens and underscores is removed, and accented letters stay. A repeated heading gets `-1`, `-2`, and so on. An explicit `<a id="...">` also works as a target.
- **Punctuation:** no em dashes, en dashes, or spaced double hyphens, in prose, code, comments, or strings. `make lint/emdash` enforces it. Fixes you write follow the same rule.
