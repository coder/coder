---
name: frontend-deep-review
description: Parallel multi-reviewer audit of a frontend diff under site/ against numbered checkpoints distilled from human review comments on this repository's frontend PRs. Use before opening or updating a PR that touches site/, or when asked to review someone else's frontend PR.
---

# Frontend Deep Review

Reviews a `site/` diff the way this repository's frontend maintainers do.
Every checkpoint in `roles/` exists because maintainers flagged the pattern
in code review, usually many times; the Evidence line on each checkpoint
records how often. The goal is to catch and fix those findings before a
human reviewer posts them.

The review is split into eight reviewer roles so it can run as parallel
sub-agents. Each role owns a numbered checkpoint set:

| Role file | Prefix | Owns | Comments |
|---|---|---|---|
| `roles/simplicity-and-dead-code.md` | SD | single-use constants, wrappers, dead code, hand-rolled primitives, readability | 149, the largest group |
| `roles/comments-and-docs.md` | CD | comment quality, doc placement, missing explanations | 124 |
| `roles/tests-and-stories.md` | TS | Storybook `play`, Vitest assertions, fixtures, test hygiene | 110 |
| `roles/react-patterns.md` | RP | effects, refs, hooks, memoization, render helpers, component size, forms | 91 |
| `roles/reuse-and-organization.md` | RO | reuse before building, duplication, file placement, PR scope and size | 73 |
| `roles/styling-and-accessibility.md` | SA | Tailwind discipline, layout correctness, abstraction level, a11y | 63 |
| `roles/types-and-naming.md` | TN | optionality, coercions, generated types, type conventions, names, copy | 59 |
| `roles/data-flow.md` | DF | React Query, mutations, query keys, UI states, browser storage | 50 |

Counts are top-level inline review comments on `site/` paths from April to
September 2026 (671 comments across 187 PRs). A comment counts toward two
roles when it makes two points.

## When to run

- Before creating a PR whose diff touches `site/`.
- Before pushing a significant new round of commits to a frontend PR.
- When asked to review a frontend PR authored by someone else (report mode).
- Skip when the diff touches nothing under `site/`.

This skill complements, not replaces, `frontend-review` (the FE1 to FE10
contract in `.claude/docs/FRONTEND_PATTERNS.md`). The FE rules are the
short canonical contract; these checkpoints are the long tail of what
reviewers actually post. `pnpm check`, `pnpm lint`, `pnpm format`, and the
affected tests still run separately (see `site/AGENTS.md`).

Where the checkpoints and the FE text disagree, the checkpoints follow the
reviewers:

- TS1 and TS5: a story's `play` only drives the component into the state the
  visual snapshot captures; assertions belong in Vitest, as FE1 in
  `site/AGENTS.md` states.
- DF3: reviewers are split on `mutate` with callbacks versus
  `await mutateAsync()`; the checkpoint asks for one style per file rather
  than enforcing FE7's preference.
- TS3 over TN4 on fixture names: a new `mockX` fixture beside pre-existing
  unprefixed ones is the convention arriving, not a second style. Ask for
  the prefix on the new fixture only.

## Workflow

### 1. Scope the diff

```sh
BASE=$(git merge-base HEAD origin/main 2>/dev/null || git merge-base HEAD main)
git diff --stat "$BASE" -- site/ | tail -1
git diff --name-only "$BASE" -- site/
```

For a PR authored by someone else, use `gh pr diff <n> --name-only` and
`gh pr diff <n>` instead, and check out the head branch so reviewers can
read whole files.

Record three things every reviewer needs:

- the review target (branch, commit range, or PR number),
- the changed file list,
- the PR's stated purpose (title and description, or the user's summary).

If the diff is over roughly 1000 added lines, note it now: RO6 will fail and
the fix is to split the PR, which changes how the rest of the review is
reported.

### 2. Spawn the reviewers

Spawn all eight roles in parallel as read-only sub-agents that share this
checkout. Use the prompt in `reviewer-prompt.md`, filling in the role file,
the target, and the file list. Every role runs on every frontend diff; a
role with nothing to flag reports "No findings" quickly. If your harness
cannot spawn sub-agents, work through the role files one at a time using the
same prompt.

Each reviewer returns its findings in its final message. Do not ask
read-only reviewers to write files.
### 3. Collect and cross-check

Read each reviewer's report one at a time and build a single finding list.
Then:

- Deduplicate. The same hunk often trips several checkpoints (a single-use
  constant with a restating comment is SD1 and CD1). Keep both IDs on one
  finding.
- Resolve conflicts. If two roles disagree (for example RO3 wants duplicated
  JSX extracted while SD7 calls the extraction premature, or TN4 reads a new
  `mockX` fixture as a second naming style while TS3 requires the prefix),
  decide using the role text and record the decision.
- Drop findings without `file:line` evidence, and findings about untouched
  pre-existing code unless the PR is a refactor of exactly that code.

### 4. Report

Print one verdict table with every checkpoint, grouped by role:

```text
CD1 FAIL  site/src/pages/FooPage/FooPage.tsx:42   comment restates the assignment below it
CD2 PASS
CD3 N/A
...
RO6 FAIL  (whole PR)                                +1420/-80 lines; split frontend from backend
```

FAIL lines carry every finding (repeat the ID for multiple findings) with
the severity the role assigns: `blocking`, `should-fix`, or `nit`.

### 5. Fix or post

- Own branch (self-review mode): fix every `blocking` and `should-fix`
  finding with the smallest correct change, then re-run only the roles that
  failed. Fix `nit` findings when the fix is trivial; otherwise list them.
  Stop when the table is all PASS or N/A, or every remaining FAIL has a
  one-line justification that will go in the PR description.
- Someone else's PR (report mode): post findings as inline review comments
  in the register human reviewers use (short, specific, with the fix).
  Group nits into the review summary. Never post a finding you cannot point at a line.

## Severity

Severity mirrors how reviewers treated the pattern, not how hard it is
to fix. Each role file states the default severity per checkpoint. A
reviewer may raise severity when the consequence is user-visible (a flicker,
a swallowed error) and lower it when the pattern is pre-existing and merely
touched.

## Mapping to FE1 to FE10

| FE rule | Checkpoints that expand it |
|---|---|
| FE1 Storybook visual states, Vitest behavior | TS1, TS4, TS5, TS7 |
| FE2 No loose types | TN1, TN2, TN3 |
| FE3 Reuse, single purpose, no dead code | RO1, RO2, RO3, RO6, SD1, SD2, SD4 |
| FE4 Comments earn their place | CD1 to CD6 |
| FE5 UI states, never clobber user state | DF5, DF6 |
| FE6 Accessibility | SA6 |
| FE7 React Query discipline | DF1 to DF4 |
| FE8 Effects last resort | RP1, RP2, RP3 |
| FE9 Fixtures and mocks | TS3, TS6 |
| FE10 Tests assert observable behavior | TS2 |

Checkpoints with no FE parent (SD3, SD5, SD6, SD7, RP4 to RP8, DF7, RO4,
RO5, TN4 to TN6, SA1 to SA5) are conventions reviewers enforce that the
contract does not yet spell out. When one of them proves consistently
useful, propose adding it to `FRONTEND_PATTERNS.md`.
