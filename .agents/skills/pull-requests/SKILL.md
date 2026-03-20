---
name: pull-requests
description: "Guide for creating, updating, and following up on pull requests in the Coder repository. Use when asked to open a PR, update a PR, rewrite a PR description, or follow up on CI/check failures."
---

# Pull Request Skill

## When to Use This Skill

Use this skill when asked to:

- Create a pull request for the current branch.
- Update an existing PR branch or description.
- Rewrite a PR body.
- Follow up on CI or check failures for an existing PR.

## Lifecycle Rules

1. **Reuse existing PRs.** Check whether the current branch already has a PR:

   ```bash
   gh pr view --json number --jq .number
   ```

   - If that returns a PR number, update that PR instead of opening a new one.
   - If no PR exists, create one for the current branch.

2. **Default to draft.** Create new PRs with `gh pr create --draft` unless the
   user explicitly asks for a ready-for-review PR.
3. **Keep description aligned with the full diff.** Re-read the diff against
   the base branch before writing or updating the title and body. Describe the
   entire PR diff, not only the last commit.
4. **Never auto-merge.** Do not merge a PR or mark it ready for review unless
   the user explicitly asks.
5. **Never push to main or master.** Do not push directly to `origin/main` or
   `origin/master`.

## Local Validation

Run local validation before pushing.

- `make pre-commit`
  - Primary validation command.
  - Runs generation, formatting, linting, typos, and build checks.
- `make pre-push`
  - Heavier validation, including tests.
  - Opt-in via the repo allowlist.
- Targeted checks while iterating:
  - `make test RUN=TestName`
  - `make lint`
  - `make fmt`
  - `make gen` when database queries change
- Install git hooks with:

  ```bash
  git config core.hooksPath scripts/githooks
  ```

- Before pushing, always run `make pre-commit` or, at minimum, run both
  `make lint` and `make fmt`.

## PR Title and Description

**Title format:**

```text
type(scope): description
```

Allowed types:

- `feat`
- `fix`
- `docs`
- `style`
- `refactor`
- `perf`
- `test`
- `build`
- `ci`
- `chore`
- `revert`

Scope rules:

- The scope must be a real filesystem path containing all changed files.
- Omit the scope when changes span multiple top-level directories.

Examples:

- `feat(coderd/chatd): add streaming support`
- `fix: move contexts to appropriate locations`

**Description rules:**

- Default to a concise 1-2 paragraph description covering what changed and why.
- For complex changes, use structured sections:

  ```markdown
  ## Summary
  ...

  ## Problem
  ...

  ## Fix
  ...
  ```

- Always link related work with `Closes #XXX`, `Fixes #XXX`, or `Refs #XXX`.
- Include screenshots for UI changes, metrics for performance changes, and
  warnings for migrations.
- Never include test plans, "benefits" sections, detailed implementation
  walkthroughs, or marketing language.

**Attribution footer for AI-generated PRs:**

If you know the tool and model used, add:

```markdown
🤖 Generated with [Tool Name](tool-url)

Co-Authored-By: Model Name <noreply@example.com>
```

If you do not know the specific tool or model, omit the footer.

**PR template:** The repo template at `.github/pull_request_template.md` is
just an HTML comment reminding contributors to review the AI contribution
guidelines. There are no required fields to fill out.

## CI / Checks Follow-up

**Always watch CI checks after pushing.** Do not push and walk away — monitor checks until they pass or fail, and fix failures before reporting success.

After pushing:

- Monitor CI with `gh pr checks <PR_NUMBER>`.
- Use `gh pr view <PR_NUMBER> --json statusCheckRollup` for programmatic check
  status.
- Use `gh pr view <PR_NUMBER>` when you need a quick PR summary.

If checks fail:

1. Read the failing check logs.
2. Fix the problem locally.
3. Run `make pre-commit` again.
4. Push the fix to the same branch.

## What Not to Do

- Do **not** reference or call helper scripts that do not exist in this
  repository.
- Do **not** auto-merge or mark a PR ready for review without an explicit user
  request.
- Do **not** push to `origin/main` or `origin/master`.
- Do **not** skip local validation before pushing.
- Do **not** fabricate or embellish PR descriptions. Keep them factual and
  matched to the actual diff.
