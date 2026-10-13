---
name: frontend-deep-review
description: Parallel multi-reviewer audit of a frontend diff under site/ against numbered checkpoints distilled from human review comments on this repository's frontend PRs. Use before opening or updating a PR that touches site/, or when asked to review someone else's frontend PR.
---

# Frontend Deep Review

Reviews a `site/` diff the way this repository's frontend maintainers do.
Every checkpoint in `roles/` is a pattern maintainers flagged in code
review. The goal is to catch and fix those findings before a human reviewer
posts them. Identifiers quoted in a checkpoint's examples come from those
reviews, and some have since been renamed or removed: the pattern is what
a checkpoint checks, not the name.

The review is split into eight reviewer roles so it can run as parallel
sub-agents. Each role owns a numbered checkpoint set:

| Role file | Prefix | Owns |
|---|---|---|
| `roles/simplicity-and-dead-code.md` | SD | single-use constants, wrappers, dead code, hand-rolled primitives, readability |
| `roles/comments-and-docs.md` | CD | comment quality, doc placement, missing explanations |
| `roles/tests-and-stories.md` | TS | Storybook `play`, Vitest assertions, fixtures, test hygiene |
| `roles/react-patterns.md` | RP | effects, refs, hooks, memoization, render helpers, component size, forms |
| `roles/reuse-and-organization.md` | RO | reuse before building, duplication, file placement, PR scope and size |
| `roles/styling-and-accessibility.md` | SA | Tailwind discipline, layout correctness, abstraction level, a11y |
| `roles/types-and-naming.md` | TN | optionality, coercions, generated types, type conventions, names, copy |
| `roles/data-flow.md` | DF | React Query, mutations, query keys, UI states, browser storage |

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

The root `AGENTS.md`, `site/AGENTS.md`, and `FRONTEND_PATTERNS.md` outrank
the checkpoints. When a checkpoint contradicts them, the repository rule
wins: reviewers apply it and report the conflict so the role file can be
corrected. Passing every checkpoint does not replace those files: the
author still follows the rules no checkpoint covers, such as JSDoc on
exported symbols.

## Workflow

### 1. Scope the diff

```sh
if [ "$(git rev-parse --is-shallow-repository)" = true ]; then git fetch --unshallow origin; fi
BASE_REF=$(gh pr view --json baseRefName --jq .baseRefName) || BASE_REF=main
if git fetch origin "$BASE_REF"; then BASE=$(git merge-base HEAD FETCH_HEAD); else BASE=$(git merge-base HEAD "$BASE_REF"); fi
echo "$BASE"
git diff --stat "$BASE" -- site/ | tail -1
git diff --name-only "$BASE" -- site/
git ls-files --others --exclude-standard -- site/
```

`BASE_REF` is the branch this one merges into: the open PR's base, or
`main` when the branch has no PR yet. `gh` exits 1 both when it finds no PR
and when the lookup itself fails, so read its message: only
`no pull requests found` means no PR. On any other error
(`HTTP 401: Bad credentials`, a network failure), fix it and run the block
again instead of reviewing against `main`. For a stack slice without a PR,
set `BASE_REF` to the parent branch, so the diff holds only this slice;
when that parent exists only locally, the fetch fails and the merge base
comes from the local branch. The merge base reads `FETCH_HEAD`, not
`origin/$BASE_REF`, because a single-branch clone (any `--depth` clone, by
default) does not update that ref when it fetches. The first line fetches
full history in a shallow clone, where `git merge-base` cannot reach the
fork point.

`git diff <commit>` compares that commit with the working tree, so it
includes staged and unstaged edits, but not untracked files. Add the files
the `git ls-files` line prints to the changed file list, marked
`(new, untracked)`, so reviewers read them whole.

For a PR authored by someone else, do not check out its branch. A checkout
runs Git hooks from the author's tree (this repository's hooks live in
`scripts/githooks/`, `post-checkout` included), and reading files from the
worktree follows any symlink the PR adds and mixes in your local edits.
Fetch the PR and pin its revision once instead, so every reviewer reads the
same commits even if the author pushes during the review:

```sh
if [ "$(git rev-parse --is-shallow-repository)" = true ]; then git fetch --unshallow origin; fi
BASE_REF=$(gh pr view <n> --json baseRefName --jq .baseRefName)
git fetch origin "pull/<n>/head"
HEAD_SHA=$(git rev-parse FETCH_HEAD)
git fetch origin "$BASE_REF"
RULES_SHA=$(git rev-parse FETCH_HEAD)
BASE=$(git merge-base "$HEAD_SHA" "$RULES_SHA")
echo "$BASE $HEAD_SHA $RULES_SHA"
git diff --stat "$BASE" "$HEAD_SHA" -- site/ | tail -1
git diff --name-only "$BASE" "$HEAD_SHA" -- site/
```

Reviewers then read the PR only as Git objects at the head SHA, and what
it deletes at `BASE`. They read
the rules (the reviewer prompt, the role files, and the three rule files
above) at `RULES_SHA`, the base branch tip, so the PR cannot rewrite the
rules it is reviewed against and they are the rules it would merge under.
`BASE`, the merge base, only scopes the diff. Send the prompt from that
revision
(`git show <rules-sha>:.claude/skills/frontend-deep-review/reviewer-prompt.md`),
filled as it describes. Follow this workflow and its role list from
`RULES_SHA` too: when
`git diff --quiet <rules-sha> -- .claude/skills/frontend-deep-review/`
reports a difference, read `SKILL.md` and list `roles/` at `RULES_SHA`
(`git show`, `git ls-tree`) and follow those. When the base branch
predates this skill, use the local copies throughout. The PR's file names
are author-controlled too: single-quote any you put in a command, such as
the path of an inline comment.

Record three things every reviewer needs:

- the review target: the exact command that prints the diff, with the
  SHAs that `echo` printed (a sub-agent's shell has none of these
  variables): `git diff <sha> -- site/` for your own branch, or
  `git diff <base-sha> <head-sha> -- site/` for someone else's PR,
- the changed file list,
- the PR's stated purpose (title and description, or the user's summary).

A diff over roughly 1000 added lines is a reason to check RO6 closely, not
a verdict. RO6 fails when the diff mixes concerns that could ship
separately (infrastructure, its consumers, refactors, bug fixes). It passes
when the change is cohesive, or when generated types, fixtures, and stories
account for most of the size.

### 2. Spawn the reviewers

Run the eight roles as read-only sub-agents that share this checkout, as
many at once as the harness allows. Queue the rest and start each one when
a slot frees. Use the prompt in `reviewer-prompt.md`, filling all four
placeholders: the role file, the target, the file list, and the PR's stated
purpose. Every role runs on every frontend diff; a role with nothing to
flag reports "No findings" quickly. If your harness cannot spawn
sub-agents, work through the role files one at a time using the same
prompt.

Each reviewer returns its findings in its final message. Do not ask
read-only reviewers to write files.

### 3. Collect and cross-check

Read each reviewer's report one at a time and build a single finding list.
Then:

- Deduplicate. The same hunk often trips several checkpoints (a single-use
  constant with a restating comment is SD1 and CD1). Keep both IDs on one
  finding.
- Resolve conflicts. If two roles disagree (for example RO3 wants duplicated
  JSX extracted while SD7 calls the extraction premature), decide using the
  role text and record the decision.
- Adjudicate every "Outside my role" note against the owning role's
  checkpoint, or against the cited rule when no checkpoint covers it. Add a
  confirmed note to the finding list (a broken rule as a `should-fix`
  finding under the rule file's name); drop the rest and list each with its
  reason below the verdict table.
- Copy each reviewer's "Conflicts with repository rules" entries below the
  verdict table: each one names a checkpoint whose role file needs
  correcting.
- Drop findings without `file:line` evidence, except PR-scope findings
  such as RO6, which cite `(whole PR)`. A line the diff removes, cited as
  `file:line (deleted)` with the old file's line, is evidence: removing the
  only invalidation or error branch is a finding. Also drop findings about
  untouched pre-existing code unless the PR is a refactor of exactly that
  code.

### 4. Report

Print one verdict table with every checkpoint, grouped by role:

```text
CD1 FAIL  site/src/pages/FooPage/FooPage.tsx:42   comment restates the assignment below it
CD2 PASS
CD3 N/A
...
RO6 FAIL  (whole PR)                                new storage module ships with its consumers and a bug fix; split by concern
```

FAIL lines carry every finding (repeat the ID for multiple findings) with
the severity the role assigns: `blocking`, `should-fix`, or `nit`. Findings
under a rule file's name follow the table in the same format.

### 5. Fix or post

- Own branch (self-review mode): fix every `blocking` and `should-fix`
  finding with the smallest correct change, then re-run every role that
  failed and every role whose checkpoints cover code the fixes changed (an
  extraction for RO3 can add TN or RP findings). When unsure, re-run all
  eight. Before each re-run, repeat the step 1 commands and rebuild the
  prompts from the new file list, so files the fixes added get reviewed
  too. When the fixes touch `site/src/`, also run the `frontend-review`
  audit before those re-runs, because these roles do not check FE1 to
  FE10 on their own, and skip its closing handoff back to this skill. Its
  collection step diffs against `main`, so give it
  `git diff <sha> -- site/src` with the SHA step 1 printed instead, plus
  the `(new, untracked)` files under `site/src/` in the rebuilt file list,
  which that diff omits. Its
  fixes then reach the re-runs; if a later fix changes files again, re-run
  the roles that cover them. Fix `nit` findings when the fix is trivial;
  otherwise list them.
  Stop when no `blocking` or `should-fix` FAIL remains, as `site/AGENTS.md`
  requires. A finding you judge wrong is not a FAIL to justify: drop it,
  and list it with the reason below the verdict table, next to the step 3
  conflicts, so the role file can be corrected.
- Someone else's PR (report mode): post findings as inline review comments
  on the pinned head SHA, so their lines match what reviewers read, in the
  register human reviewers use (short, specific, with the fix). Post a
  `(deleted)` finding on the removed line, on the left side of the diff.
  Before posting, check the PR head again
  (`gh pr view <n> --json headRefOid --jq .headRefOid`): if it is no
  longer `HEAD_SHA`, review the new head instead of posting findings for a
  superseded commit.
  Group nits and PR-scope findings such as RO6 into the review summary.
  Never post any other finding you cannot point at a line.

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
| FE3 Reuse, single purpose, no dead code | RO1, RO2, RO3, RO5, RO6, RP5, SD1, SD2, SD4 |
| FE4 Comments earn their place | CD1 to CD6 |
| FE5 UI states, never clobber user state | DF5, DF6 |
| FE6 Accessibility | SA6 |
| FE7 React Query discipline | DF1 to DF4 |
| FE8 Effects last resort | RP1, RP2, RP3 |
| FE9 Fixtures and mocks | TS3, TS6 |
| FE10 Tests assert observable behavior | TS2 |

Checkpoints with no FE parent (SD3, SD5, SD6, SD7, RP4, RP6 to RP8, DF7,
RO4, TN4 to TN6, SA1 to SA5) are conventions reviewers enforce that the
contract does not yet spell out. When one of them proves consistently
useful, propose adding it to `FRONTEND_PATTERNS.md`.
