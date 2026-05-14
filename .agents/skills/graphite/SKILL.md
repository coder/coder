---
name: graphite
description: "Use this skill when an agent needs to produce, update, or review a stack of dependent pull requests with the Graphite CLI (`gt`). Covers installation, authentication, stack nomenclature, navigation, absorb/modify, reorder, sync, restack, submit, and conflict handling, with a focus on non-interactive flags safe for automated agents."
---

# Graphite CLI (`gt`)

## When to use this skill

Use this skill when the task calls for:

- breaking a large change into a series of small, dependent PRs;
- updating an existing stack in response to review feedback while keeping
  each PR scoped to one concern;
- routing fixes to the correct PR level instead of piling everything on
  the tip;
- syncing a stack after the bottom PR merges to trunk;
- inspecting and modifying parent / child relationships between branches.

If the change is genuinely one PR, do not use Graphite. A flat
`git checkout -b ... && git push` flow is simpler and avoids stack
metadata drift.

`coder/coder` policy (`docs/about/contributing/CONTRIBUTING.md`) is to
keep PRs under ~1000 lines and split larger work into a series. A
Graphite stack is one way to manage that series.

## Nomenclature

| Term | Meaning |
|------|---------|
| **Trunk** | Base branch (usually `main`). Configured once with `gt init`. |
| **Stack** | Ordered chain of branches: `trunk → A → B → C`. Each branch is a real Git branch. |
| **Parent / child** | Each branch has exactly one parent (closer to trunk) and 0..N children (closer to the tip). |
| **Upstack** | Everything above the current branch, toward the tip. |
| **Downstack** | Everything below the current branch, toward trunk. |
| **Top / bottom** | The branch farthest from / closest to trunk in the current stack. |
| **Restack** | Rebase children onto the new tip of their parent after the parent changed. |
| **Submit** | Push the stack and create or update one PR per branch. |

`gt` is a thin wrapper around `git`. Branches are real Git branches and
commits are real Git commits. Graphite stores parent / child metadata
in the local repo and uses it to orchestrate rebases and PR submission.
Anything `gt` does not recognise falls through to `git`.

A stack of N PRs is **not** the same as one squashed PR. Reviewers see
each branch as its own PR, with its own diff against its parent, and
each PR can be merged independently. Plan the cut points up front so
each branch is conceptually coherent and individually builds and passes
tests (this is required by `coder/coder` policy).

## Installation

Install one of:

```bash
# npm, cross-platform, recommended for agents
npm install -g @withgraphite/graphite-cli@stable

# Homebrew, macOS or Linux
brew install withgraphite/tap/graphite
```

Confirm:

```bash
gt --version
```

On Windows, run `gt` from WSL. There is no native Windows build.

## Authentication

`gt` requires a personal CLI token that can only be obtained through a
browser flow tied to a human Graphite account. An agent cannot complete
it on its own.

Before running any other `gt` command, check for an existing token:

```bash
test -s ~/.graphite_user_config && echo authed || echo missing
```

If the file is missing or empty, **stop and surface these instructions
to the human verbatim**, then wait for confirmation before continuing:

> Graphite CLI is not authenticated in this workspace. To unblock me:
>
> 1. Open <https://app.graphite.dev/activate> in your browser.
> 2. Sign in if needed and copy the CLI auth token.
> 3. In the workspace, run:
>
>    ```bash
>    gt auth --token <PASTE_TOKEN_HERE>
>    ```
>
> 4. Reply when done and I will resume.

Do not try to invoke `gt auth` interactively (without `--token`); it
opens a browser flow that also needs a human.

The token is stored at `~/.graphite_user_config`. On Coder workspaces
that path lives inside the persistent home volume, so the token
survives workspace stops and rebuilds as long as the volume is not
destroyed.

If a later `gt submit` fails with a re-auth URL, stop and surface the
same instructions again.

## Per-repository init

Once per clone:

```bash
# Interactive
gt init

# Non-interactive
gt init --trunk main --reset
```

Trunk only needs to be set once. `gt init --reset` re-detects trunk
and wipes stale tracking metadata, which is useful when adopting
Graphite in an existing repo.

## Agent-safe defaults

For automation, pass these global flags whenever you can:

```bash
gt --no-interactive <cmd>   # fail instead of prompting
gt --quiet <cmd>            # suppress chatter; implies --no-interactive
```

Prefer command flags that bypass editors and prompts. The most
important pairs:

| Goal | Bad (interactive) | Good (non-interactive) |
|------|-------------------|------------------------|
| Create branch | `gt create` | `gt create <name> -am "msg"` |
| Amend tip commit | `gt modify -a` | `gt modify -am "msg"` (or omit `-m` to keep message) |
| Submit stack | `gt submit` | `gt submit --stack --no-edit --publish` (or `--draft`) |
| Resolve missing reviewers prompt | `gt submit -r` | `gt submit -r alice,bob` |

Do not invoke `gt reorder`, `gt downstack edit`, `gt split` (without
`--by-file`), or `gt modify --interactive-rebase` from automation.
They open `$EDITOR` and will hang.

## Creating a stack

Start from trunk, build upward. Each `gt create` makes a new branch
on top of the current one and commits staged changes.

```bash
gt checkout main           # or: gt trunk
gt sync                    # pull trunk, prune merged

# PR 1
# edit files
gt create feat/api -am "feat: add users API"

# PR 2 stacks on PR 1
# edit files
gt create feat/ui -am "feat(site): list users in admin panel"

# PR 3 stacks on PR 2
# edit files
gt create feat/docs -am "docs: document users admin panel"

gt log short               # confirm the stack
```

If you omit `<name>`, Graphite derives one from the commit message
(`gt create -am "msg"`).

## Inspecting state

```bash
gt log              # tree of all tracked stacks
gt log short        # compact tree, parseable for scripts (alias: gt ls)
gt log long         # tree with commit details (alias: gt ll)
gt log --stack      # only the current stack
```

For programmatic checks:

- `gt log short` prints one branch per line and marks the current
  branch. Use it as a source of truth when deciding whether work is
  already stacked.
- `git status` still works. A Graphite-driven rebase shows up as a
  normal `REBASE` state.
- `gt continue` exits non-zero if there is nothing to continue. Use
  it as a probe for an in-progress rebase.

## Navigating the stack

```bash
gt up [n]           # move N branches toward the tip (default 1)
gt down [n]         # move N branches toward trunk
gt top              # jump to the topmost branch in the current stack
gt bottom           # jump to the bottommost branch above trunk
gt trunk            # check out trunk
gt checkout <name>  # check out a specific branch (alias: gt co)
```

Pass an explicit branch name to `gt checkout` rather than relying on
the interactive picker.

## Modifying commits at the right level

`gt modify` is the stacked replacement for `git commit` and
`git commit --amend`. It edits the current branch's tip commit and
then restacks every descendant so children stay based on the new tip.

```bash
# Amend the current branch's tip commit
gt modify -a                  # stage all, keep message, no editor
gt modify -am "new message"   # stage all, new message, no editor

# Create a NEW commit on the current branch (do not amend)
gt modify -c -am "follow-up commit"

# Amend a downstack branch without checking it out
gt modify --into feat/api -am "fix typo in API"
```

`--into` only accepts branches that are downstack of the current
branch. After it runs, Graphite restacks every descendant.

Never use `git commit` or `git commit --amend` inside a stack.
Bare-git amends do not update Graphite metadata, do not restack
children, and silently desynchronise the stack.

## Absorbing changes into the correct level

`gt absorb` is the most useful command for agents fixing review
feedback on a multi-PR stack. It takes the currently staged hunks,
walks the stack downward, and routes each hunk into the most recent
commit that last touched the surrounding lines. Hunks with no clear
target are left unstaged.

```bash
gt absorb              # interactive confirmation, hunks routed automatically
gt absorb -a           # stage all tracked changes first
gt absorb -d           # dry-run, print the planned routing
gt absorb -p           # patch-style hunk selection
gt absorb -f           # skip the confirmation prompt
```

Recommended agent recipe:

```bash
gt absorb -a -d   # dry-run, inspect routing
gt absorb -a -f   # apply
```

When `gt absorb` cannot place a hunk, fall back to
`gt down` / `gt up` to the right branch, then
`gt modify -am "..."` to amend that branch directly.

## Syncing with remote / trunk

```bash
gt sync                  # fetch trunk, delete merged branches, restack survivors
gt sync --restack        # also force a restack pass
gt --no-interactive sync # take default actions instead of prompting
```

`gt sync` understands squash-merges by patch ID and will offer to
delete the corresponding local branches. After deletion it restacks
the remaining branches in your stack onto the new trunk tip.

Run `gt sync` at the start of every session and after merging the
bottom of a stack.

If parents or commits have changed in a way `gt sync` did not pick
up, run a targeted restack:

```bash
gt restack                  # restack upstack from the current branch (default)
gt restack --downstack      # only ancestors of the current branch
gt restack --upstack        # only descendants (default)
gt restack --only           # just the current branch
gt restack --branch <name>  # run as if checked out on <name>
```

## Reordering, moving, splitting, squashing, folding

```bash
# Interactive reorder. Opens $EDITOR. Not agent-safe.
gt reorder
gt reorder --stack   # include every branch up through the stack tip

# Move the current branch (and its upstack) onto a different parent
gt move --onto <new-parent>

# Squash all commits on the current branch into one
gt squash

# Fold the current branch into its parent and restack descendants
gt fold

# Split the current branch by file glob. Non-interactive.
gt split --by-file '<glob>'

# Other split modes are interactive. Avoid in automation.
# gt split --by-commit
# gt split --by-hunk
```

`gt downstack edit` is an older alias for opening the reorder
editor scoped from trunk through the current branch. Use
`gt reorder` in new automation.

## Branch metadata

```bash
gt track                  # adopt the current Git branch into Graphite; prompts for parent
gt untrack <name>         # stop tracking a branch
gt rename <new-name>      # rename current branch and update Graphite metadata
gt delete <name>          # delete the branch and restack its children onto its parent
```

If a `git checkout` lands you on a branch unknown to Graphite, run
`gt track` to set its parent. Always prefer `gt rename` and
`gt delete` over raw `git branch -m` / `git branch -D` so child
pointers stay correct.

## Submitting the stack

```bash
gt submit                                  # submit current branch and its downstack
gt submit --stack                          # submit the whole stack (alias: gt ss)
gt submit --stack --draft --no-edit        # safest default for agents
gt submit --stack --publish --no-edit      # mark new PRs ready for review
gt submit --stack --update-only            # only push branches with existing PRs
gt submit --dry-run                        # preview without pushing
gt submit -r alice,bob                     # reviewers (skip the prompt)
gt submit --merge-when-ready               # enqueue for auto-merge
gt submit --always                         # force-update PRs even with no diff change
```

Notes:

- `gt submit` validates the stack is restacked before pushing. If it
  refuses, run `gt restack` first.
- `gt submit` blocks force-pushes that would overwrite remote work
  that has changed since the last submit. Resolve with `gt sync`
  rather than `git push --force`.
- The Graphite web app at <https://app.graphite.dev> understands stack
  ordering and is the recommended place to merge. `gt merge` exists
  for CLI merges but covers fewer cases.
- `coder/coder` defaults to creating PRs as drafts. Combine
  `--stack --draft --no-edit` unless the user explicitly asks for
  ready-for-review.

After submission you still need to monitor CI per the
`pull-requests` skill (`gh pr checks <PR_NUMBER> --watch`).

## Conflict handling

When `gt restack`, `gt sync`, `gt modify --into`, `gt move`, or
`gt reorder` triggers a rebase that conflicts:

1. Graphite stops and prints the conflicting paths.
2. Edit the files to resolve.
3. Stage the resolutions: `gt add -A` (preferred) or `git add <paths>`.
4. Resume: `gt continue`.
5. If you want to bail out completely: `gt abort`.

Repeat steps 2-4 for each remaining conflict in the rebase.

Never run `git rebase --continue` or `git rebase --abort` during a
Graphite-driven rebase. Mixing them desynchronises Graphite's
metadata. If that happens, run `gt restack --branch <name>` from a
clean tree to repair.

## Tips for agents

### Prefer small stacks over large single PRs

A stack of focused PRs is easier to review, easier to revert, and
keeps each PR under the `coder/coder` size guideline. Plan the cut
points before writing code so each PR is conceptually coherent.

### Idempotent submission

`gt submit --stack --update-only --no-edit --always` is the safest
re-run after a fix. It updates existing PRs only, never silently
creates new ones, and force-updates branches even when the diff is
unchanged (useful after fixing CI metadata).

### Detecting stack state

```bash
# Is the current branch tracked by Graphite?
# gt log --stack exits non-zero on an untracked branch.
if gt log --stack >/dev/null 2>&1; then echo tracked; else echo untracked; fi

# Is a Graphite-driven rebase in progress?
# Graphite uses native git rebase under the hood, so check git's state:
if [ -d .git/rebase-merge ] || [ -d .git/rebase-apply ]; then
  echo rebasing
fi
```

### Common failure modes

- **Dirty working tree.** Most `gt` commands refuse to run with
  uncommitted changes. Use `gt absorb -a` to route them into existing
  commits, or stash with `git stash` before navigating.
- **Detached HEAD.** Run `gt checkout <branch>` to attach again.
- **Untracked branch after `git checkout`.** Run `gt track`.
- **Missing trunk.** Run `gt init --trunk <branch>`.
- **Auth expired or missing.** `gt submit` prints a re-auth URL.
  Stop and surface the auth instructions from the "Authentication"
  section to the human; do not try to complete the flow yourself.
- **Squash-merged but still local.** `gt sync` detects this by patch
  ID. If the PR was force-rebased before squash-merge, the heuristic
  may miss it. Use `gt delete <branch>` manually.

### When to drop down to plain `git`

- Reading content, diffs, blame, log: use `git` directly.
- Pushing without creating a PR: `git push` is fine.
- Anything that rewrites history (`git rebase -i`, `git reset --hard`,
  `git cherry-pick`): avoid inside a stack. If you must, run
  `gt restack` from a clean tree afterward to repair metadata.

### Interaction with `coder/coder` git hooks

`gt modify`, `gt create`, and `gt submit` run the same `pre-commit`
and `pre-push` hooks as plain `git`. Wait for them to finish even
when the command appears to hang. Do not pass `--no-verify`; the
`coder/coder` policy in `AGENTS.md` forbids it.

If `pre-push` flags a problem after `gt submit`, fix locally, run
`gt modify -am "..."` (or `gt absorb`), then re-run `gt submit`.

## Cheat sheet

| Task | Command |
|------|---------|
| Init repo | `gt init --trunk main` |
| Auth | `gt auth --token <token>` |
| New branch + commit | `gt create <name> -am "msg"` |
| View stack | `gt log short` |
| Navigate | `gt up` / `gt down` / `gt top` / `gt bottom` |
| Amend tip | `gt modify -am "msg"` |
| New commit on current branch | `gt modify -c -am "msg"` |
| Amend downstack branch | `gt modify --into <branch> -am "msg"` |
| Route hunks to right commit | `gt absorb -a -d` then `gt absorb -a -f` |
| Pull trunk, prune merged | `gt sync` |
| Rebase children after edit | `gt restack` |
| Resolve conflict | edit, `gt add -A`, `gt continue` |
| Abort rebase | `gt abort` |
| Move branch onto new parent | `gt move --onto <branch>` |
| Reorder (interactive) | `gt reorder` |
| Split by file | `gt split --by-file '<glob>'` |
| Squash branch | `gt squash` |
| Fold into parent | `gt fold` |
| Rename | `gt rename <new>` |
| Delete | `gt delete <name>` |
| Adopt plain git branch | `gt track` |
| Submit stack (draft) | `gt submit --stack --draft --no-edit` |
| Submit stack (ready) | `gt submit --stack --publish --no-edit` |
| Update existing PRs only | `gt submit --stack --update-only --no-edit` |
| Dry-run submit | `gt submit --stack --dry-run` |

## References

- Command reference: <https://graphite.com/docs/command-reference>
- CLI overview: <https://graphite.dev/docs/graphite-cli>
- Auth activation: <https://app.graphite.dev/activate>
- Web app (merging stacks): <https://app.graphite.dev>
- Modifying a stack guide: <https://github.com/withgraphite/docs/blob/main/guides/graphite-cli/modifying-a-stack.md>
- `coder/coder` PR-size policy: `docs/about/contributing/CONTRIBUTING.md`
- Related skill: `.agents/skills/pull-requests/SKILL.md` for PR
  description and CI follow-up conventions.
