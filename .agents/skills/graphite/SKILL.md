---
name: graphite
description: "Use when an agent needs to produce or update a stack of dependent pull requests with the Graphite CLI (`gt`). Covers install, auth, nomenclature, navigation, modify/absorb, reorder, sync/restack, submit, conflict handling, and non-interactive flags."
---

# Graphite CLI (`gt`)

## When to use

Producing or updating a series of small, dependent PRs (a stack). If
the change fits one PR, use plain git. `coder/coder` policy
(`docs/about/contributing/CONTRIBUTING.md`) caps PRs at ~1000 lines
and asks for stacks beyond that.

## Nomenclature

| Term | Meaning |
|------|---------|
| Trunk | Base branch (usually `main`). Set once via `gt init`. |
| Stack | Ordered chain `trunk → A → B → C`. Each link is a real Git branch. |
| Parent / child | Each branch has 1 parent (toward trunk) and 0..N children (toward tip). |
| Upstack / downstack | Above / below the current branch. |
| Top / bottom | Tip-most / trunk-most branch in the current stack. |
| Restack | Rebase descendants onto a changed parent. |
| Submit | Push the stack; create or update one PR per branch. |

`gt` wraps `git`. Branches and commits are real Git objects; Graphite
stores parent/child metadata locally. Unknown subcommands fall through
to `git`.

## Install

```bash
npm install -g @withgraphite/graphite-cli@stable   # cross-platform
brew install withgraphite/tap/graphite             # macOS / Linux
```

Windows: run from WSL.

## Authentication

The CLI token must be minted through a browser flow tied to a human
Graphite account. An agent cannot complete it.

```bash
test -s ~/.graphite_user_config && echo authed || echo missing
```

If missing, stop and surface this to the human:

> Graphite CLI is not authenticated. Open
> <https://app.graphite.dev/activate>, copy the pre-filled command,
> run `gt auth --token <TOKEN>` in this workspace, then reply when
> done.

The token persists in `~/.graphite_user_config` (the Coder home
volume), so it survives workspace stops and rebuilds. Repeat the
prompt if `gt submit` later asks for re-auth. Do not run bare
`gt auth`; it opens a browser.

## Setup per clone

```bash
gt init --trunk main        # non-interactive
```

## Agent-safe defaults

Pass `--no-interactive` (or `--quiet`) globally when you can. Always
prefer flag forms that bypass `$EDITOR` and prompts:

| Goal | Use |
|------|-----|
| Create branch | `gt create <name> -am "msg"` |
| Amend tip commit | `gt modify -am "msg"` (omit `-m` to keep message) |
| Submit stack (draft) | `gt submit --stack --draft --no-edit` |
| Submit stack (ready) | `gt submit --stack --publish --no-edit` |
| Reviewers | `gt submit -r alice,bob` |

Do not invoke `gt reorder`, `gt downstack edit`, interactive
`gt split`, or `gt modify --interactive-rebase` from automation;
they all open `$EDITOR`.

## Creating a stack

```bash
gt trunk && gt sync
# edit, then for each PR:
gt create feat/api -am "feat: add users API"
# edit, then:
gt create feat/ui  -am "feat(site): list users"
gt log short                  # confirm
```

`gt create -am "msg"` derives the branch name from the message.

## Inspecting state

```bash
gt log              # full tree
gt log short        # compact, current branch marked (alias: gt ls)
gt log --stack      # current stack only
```

Probes:

```bash
# Tracked? Exits non-zero on an untracked branch.
gt log --stack >/dev/null 2>&1 && echo tracked || echo untracked

# Mid-rebase? Graphite uses native git rebase.
[ -d .git/rebase-merge ] || [ -d .git/rebase-apply ] && echo rebasing
```

## Navigating

```bash
gt up [n] / gt down [n]           # toward tip / trunk
gt top / gt bottom                # ends of current stack
gt trunk                          # check out trunk
gt checkout <name>                # explicit; avoid the interactive picker
```

## Modify and absorb

`gt modify` replaces `git commit` and `git commit --amend` inside a
stack. Never use bare-git equivalents; they desync metadata.

```bash
gt modify -am "msg"             # amend tip commit + restack descendants
gt modify -c -am "msg"          # new commit on current branch
gt modify --into <branch> -am "msg"   # amend a downstack branch
```

`gt absorb` routes staged hunks into the most recent commit that
touched the surrounding lines. Best tool for routing review fixes to
the correct PR level:

```bash
gt absorb -a -d   # dry-run, inspect routing
gt absorb -a -f   # apply
```

Unplaceable hunks stay unstaged. Fall back to `gt up`/`gt down` then
`gt modify -am ...`.

## Sync and restack

```bash
gt sync                         # fetch trunk, prune merged, restack survivors
gt --no-interactive sync        # take default actions, no prompts
gt restack                      # restack upstack from current branch (default)
gt restack --downstack          # only ancestors
gt restack --branch <name>      # as if checked out on <name>
```

`gt sync` detects squash-merges by patch ID and offers to delete the
corresponding local branches. Run it at session start and after
merging the bottom of a stack.

## Reorder, move, split, squash, fold

```bash
gt reorder                    # INTERACTIVE; not agent-safe
gt move --onto <new-parent>   # move current branch + upstack
gt split --by-file '<glob>'   # non-interactive split
gt squash                     # collapse current branch's commits
gt fold                       # merge current branch into parent
```

`gt split --by-commit` and `--by-hunk` are interactive; avoid in
automation.

## Branch metadata

```bash
gt track                # adopt a plain git branch (prompts for parent)
gt rename <new-name>    # use this, not git branch -m
gt delete <name>        # use this, not git branch -D; children re-parent
```

If `git checkout` lands you on an untracked branch, run `gt track`.

## Submitting

```bash
gt submit --stack --draft --no-edit     # default for agents
gt submit --stack --publish --no-edit   # ready for review (only if asked)
gt submit --stack --update-only         # never create new PRs
gt submit --stack --dry-run             # preview
gt submit --always                      # force-update unchanged branches
```

Notes:

- Submit validates the stack is restacked. If it refuses, run
  `gt restack` first.
- It also blocks unsafe force-pushes. Resolve via `gt sync`, not
  `git push --force`.
- Merge through the Graphite web app (<https://app.graphite.dev>); it
  understands stack order. `gt merge` exists but covers fewer cases.
- `coder/coder` defaults to draft PRs. After submitting, follow CI per
  the `pull-requests` skill (`gh pr checks <N> --watch`).

Idempotent re-submit after a fix:
`gt submit --stack --update-only --no-edit --always`.

## Conflicts

When `gt restack`, `gt sync`, `gt modify --into`, `gt move`, or
`gt reorder` conflicts:

1. Resolve files.
2. `gt add -A`
3. `gt continue` (or `gt abort` to bail out).

Never run `git rebase --continue` / `--abort` inside a Graphite
rebase; mixing them desyncs metadata. Repair with
`gt restack --branch <name>` from a clean tree.

## Common failure modes

- **Dirty tree.** Use `gt absorb -a` or `git stash` first.
- **Detached HEAD.** `gt checkout <branch>`.
- **Untracked branch.** `gt track`.
- **Missing trunk.** `gt init --trunk <branch>`.
- **Auth expired.** Surface the auth prompt above; do not retry.
- **Squash-merge missed by sync.** `gt delete <branch>` manually.

## When to drop down to plain `git`

- Read-only operations (`git log`, `diff`, `blame`, `status`).
- `git push` of a non-stack branch.
- History-rewriting commands (`rebase -i`, `reset --hard`,
  `cherry-pick`) inside a stack: avoid. If unavoidable, follow with
  `gt restack` from a clean tree.

## `coder/coder` git hooks

`gt modify`, `gt create`, and `gt submit` run the repo's `pre-commit`
and `pre-push` hooks. Wait them out; do not pass `--no-verify`
(`AGENTS.md` forbids it). On hook failure, fix locally and re-run
`gt modify -am ...` or `gt absorb`, then re-submit.

## References

- Command reference: <https://graphite.com/docs/command-reference>
- Auth activation: <https://app.graphite.dev/activate>
- Web app (merging): <https://app.graphite.dev>
- Related skill: `.agents/skills/pull-requests/SKILL.md`
