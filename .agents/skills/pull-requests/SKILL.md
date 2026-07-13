---
name: pull-requests
description: "Guide for creating, updating, and following up on pull requests in the Coder repository. Use when asked to open a PR, update a PR, rewrite a PR description, or follow up on CI/check failures."
---

# Pull Request Skill

Create and maintain accurate Coder pull requests. Read the full diff against the base branch before writing or updating PR prose.

## Title

Use `type(scope): message`. Follow the commit-message rules in [`CONTRIBUTING.md`](../../../docs/about/contributing/CONTRIBUTING.md#commit-messages).

- Use one of: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
- Use an imperative, present-tense message.
- Keep the title concise, about 70 characters when practical.
- Use a scope only when it is a real filesystem path that contains every changed file.
- Use a broader path or omit the scope for cross-cutting changes.

Examples:

- `feat: add tracing to aibridge`
- `perf(coderd/database): add workspace app status index`
- `refactor(site): remove redundant app status sorting`

## Description

Default to one or two short paragraphs:

1. State what changed.
2. Explain the problem, reason, or user-visible impact when it is not obvious.

For changes that need more context, use only the sections that help:

```markdown
## Summary
[What changed]

## Problem
[What was wrong or missing]

## Fix
[How this change addresses it]
```

Include when relevant:

- Related work with `Closes`, `Fixes`, `Depends on`, or `Refs`.
- Measured performance context.
- Migration or operational warnings.
- Visual evidence for UI changes.
- Upstream links for dependency or generated updates.

Omit test plans, benefits sections, marketing language, and low-level implementation detail unless it is necessary to review the change. Do not fabricate impact or evidence.

Let GitHub soft-wrap body prose. Add manual line breaks only for Markdown structure. Unwrap commit-message paragraphs before reusing them in a PR body.

## Lifecycle

1. Confirm the current branch is not `main` or `master`.
2. Check for an existing PR:

   ```sh
   gh pr list --head "$(git branch --show-current)" --author @me --json number --jq '.[0].number // empty'
   ```

3. If a PR exists, verify `gh pr view` resolves to that PR and update it. Otherwise, create a draft with `gh pr create --draft` unless the user explicitly asks for ready-for-review.
4. Keep the title and description aligned with the complete PR diff after every substantive push.
5. Do not mark ready, merge, auto-merge, or push to `origin/main` or `origin/master` without explicit instruction.
6. Run the validation and hooks required by [`AGENTS.md`](../../../AGENTS.md). Never bypass hooks.

## CI follow-up

Always watch checks after pushing. Do not push and walk away.

```sh
gh pr checks <PR_NUMBER> --watch
```

For failures:

1. Identify the failed run from `gh pr checks`.
2. Read logs with `gh run view <RUN_ID> --log-failed`.
3. Fix the issue locally and run the relevant validation.
4. Push the fix, refresh the PR description if the diff changed materially, and watch checks again.

Use `gh pr view <PR_NUMBER> --json statusCheckRollup` only when a programmatic status summary is needed.
