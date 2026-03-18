---
name: pull-request
description: Creates a draft pull request following coder's PR conventions. Use when the user asks to "create a PR", "open a pull request", "submit a PR", or "make a pull request".
allowed-tools: Bash(git:*), Bash(gh pr create --draft:*), Bash(gh pr view:*), Read, Glob, Grep
---

# Pull Request Skill

Create a draft pull request following Coder's PR conventions.

## Workflow

1. **Gather changes**

   ```sh
   git log main..HEAD --oneline
   git diff main..HEAD --stat
   ```

2. **Draft the PR title** — `type(scope): description` (~70 chars, CI-linted)
   - Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`
   - Scope must be a real path (directory or file stem) containing all changed files
   - Omit scope when changes span multiple top-level directories
   - Examples: `feat: add tracing to aibridge`, `perf(coderd/database): add index on workspace_app_statuses.app_id`

3. **Draft the PR body** — default to 1–2 paragraphs; use structured sections only for complex changes

4. **Create the PR** — `gh pr create --draft` is the **only permitted command**

## PR Body Patterns

### Default: concise (most PRs)

```markdown
[Brief statement of what changed]

[One sentence of technical detail or context if needed]
```

### Complex changes: Summary / Problem / Fix

Use only when the change requires significant explanation:

```markdown
## Summary
Brief overview

## Problem
Detailed explanation of the issue

## Fix
How the solution works
```

### Large refactors: lead with context

```markdown
This PR rewrites [component] for [reason].

The previous [component] had [specific issues]: [details].

[What changed]: [specific improvements made].

Refs #[issue-number]
```

## What to Include

- **Related work links**: `Closes https://github.com/coder/internal/issues/XXX`, `Depends on #XXX`, `Refs #XXX`
- **Performance context** (when relevant): query timing, request rates, impact numbers
- **Migration warnings** (when relevant): `**NOTE**: This migration creates an index on ...`
- **Screenshots** (for UI changes): `<img width="..." alt="..." src="..." />`

## What NOT to Include

- ❌ Test plans — CI and code review handle testing
- ❌ "Benefits" sections — benefits should be obvious from the description
- ❌ Implementation details — keep it high-level
- ❌ Marketing language — stay technical and factual
- ❌ Bullet lists of features (unless it's a large refactor)

## Attribution Footer

For AI-generated PRs, end with:

```markdown
🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

## HARD CONSTRAINT: Draft PRs Only

```sh
# ONLY permitted form:
gh pr create --draft --title "..." --body "$(cat <<'EOF'
...
EOF
)"
```

**Non-draft PR creation is forbidden.** Never use `gh pr create` without `--draft` unless the user explicitly requests a non-draft PR. After creating, tell the user:

> I've created draft PR #XXXX. Please review the changes and mark it as ready for review when you're satisfied.
