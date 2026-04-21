---
name: pull-requests
description: >
  Guide for creating, updating, and following up on pull requests in the
  aibridge repository. Use when asked to open a PR, update a PR, rewrite
  a PR description, or follow up on CI/check failures.
---

# Pull Request Skill

## When to Use This Skill

Use this skill when asked to:

- Create a pull request for the current branch.
- Update an existing PR branch or description.
- Rewrite a PR body.
- Follow up on CI or check failures for an existing PR.

## References

- PR conventions and local validation: `AGENTS.md` (Essential Commands and
  Commit Style sections)

## Lifecycle Rules

1. **Check for an existing PR** before creating a new one:

   ```
   gh pr list --head "$(git branch --show-current)" --author @me --json number --jq '.[0].number // empty'
   ```

   If that returns a number, update that PR. If it returns empty output,
   create a new one.

2. **Check you are not on main.** If the current branch is `main` or `master`,
   create a feature branch before doing PR work.

3. **Default to draft.** Use `gh pr create --draft` unless the user explicitly
   asks for ready-for-review.

4. **Keep description aligned with the full diff.** Re-read the diff against
   the base branch before writing or updating the title and body. Describe the
   entire PR diff, not just the last commit.

5. **Never auto-merge.** Do not merge or mark ready for review unless the user
   explicitly asks.

6. **Never push to main or master.**

## PR Title Format

Follow the commit style from `AGENTS.md`:

```
type(scope): message
```

Examples:
- `feat(mcp): add tool allowlist filtering`
- `fix(intercept/messages): handle empty streaming chunks`
- `refactor(provider): extract common auth logic`

## PR Description

- Start with a one-line summary of **what** and **why**.
- List key changes as bullet points.
- Note any breaking changes or migration steps.
- Do not fabricate or embellish — describe only what the diff contains.

## CI / Checks Follow-up

**Always watch CI checks after pushing.** Do not push and walk away.

After pushing:

- Monitor CI with `gh pr checks <PR_NUMBER> --watch`.
- Use `gh pr view <PR_NUMBER> --json statusCheckRollup` for programmatic check
  status.

If checks fail:

1. Find the failed run ID from the `gh pr checks` output.
2. Read the logs with `gh run view <run-id> --log-failed`.
3. Fix the problem locally.
4. Run `make test` and `make fmt` before pushing the fix.
5. Push the fix.

## Pre-Push Validation

Before pushing, always run:

```bash
make fmt
make test
```

## What Not to Do

- Do not reference or call helper scripts that do not exist in this repository.
- Do not auto-merge or mark ready for review without explicit user request.
- Do not push to `origin/main` or `origin/master`.
- Do not skip local validation before pushing.
- Do not fabricate or embellish PR descriptions.
- Do not use `--no-verify` on git operations.
