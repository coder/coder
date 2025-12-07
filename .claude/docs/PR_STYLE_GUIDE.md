# Pull Request Description Style Guide

This guide documents the PR description style used in the Coder repository, based on analysis of recent merged PRs. This is specifically for PR descriptions - see [CONTRIBUTING.md](../../docs/about/contributing/CONTRIBUTING.md) for general PR guidelines (size limits, review process, etc.).

## PR Title Format

Follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) format:

```
type(scope): brief description
```

**Common types:**
- `feat`: New features
- `fix`: Bug fixes
- `refactor`: Code refactoring without behavior change
- `perf`: Performance improvements
- `docs`: Documentation changes
- `chore`: Dependency updates, tooling changes

**Examples:**
- `feat: add tracing to aibridge`
- `fix: move contexts to appropriate locations`
- `perf(coderd/database): add index on workspace_app_statuses.app_id`
- `docs: fix swagger tags for license endpoints`
- `refactor(site): remove redundant client-side sorting of app statuses`

## PR Description Structure

### Default Pattern: Keep It Concise

Most PRs use a simple 1-2 paragraph format:

```markdown
[Brief statement of what changed]

[One sentence explaining technical details or context if needed]
```

**Example from PR #21100:**
```markdown
Previously, when a devcontainer config file was modified, the dirty
status was updated internally but not broadcast to websocket listeners.

Add `broadcastUpdatesLocked()` call in `markDevcontainerDirty` to notify
websocket listeners immediately when a config file changes.
```

**Example from PR #21085:**
```markdown
Changes from https://github.com/coder/aibridge/pull/71/
```

**Example from PR #21077:**
```markdown
Removes references to adding database replicas from the scaling docs,
as Coder only allows a single connection URL. These passages where added in error.
```

### For Complex Changes: Use "Summary", "Problem", "Fix"

Only use structured sections when the change requires significant explanation:

```markdown
## Summary
Brief overview of the change

## Problem
Detailed explanation of the issue being addressed

## Fix
How the solution works
```

**Example from PR #21101:**
```markdown
## Summary
Change `@Tags` from `Organizations` to `Enterprise` for POST /licenses...

## Problem
The license API endpoints were inconsistently tagged...

## Fix
Simply updated the `@Tags` annotation from `Organizations` to `Enterprise`...
```

### For Large Refactors: Lead with Context

When rewriting significant documentation or code, start with the problems being fixed:

```markdown
This PR rewrites [component] for [reason].

The previous [component] had [specific issues]: [details].

[What changed]: [specific improvements made].

[Additional changes]: [context].

Refs #[issue-number]
```

**Example from PR #21080** (dev containers docs rewrite):
- Started with "This PR rewrites the dev containers documentation for GA readiness"
- Listed specific inaccuracies being fixed
- Explained organizational changes
- Referenced related issue

## What to Include

### Always Include

1. **Link Related Work**
   - `Closes https://github.com/coder/internal/issues/XXX`
   - `Depends on #XXX`
   - `Fixes: https://github.com/coder/aibridge/issues/XX`
   - `Refs #XXX` (for general reference)

2. **Performance Context** (when relevant)
   ```markdown
   Each query took ~30ms on average with 80 requests/second to the cluster,
   resulting in ~5.2 query-seconds every second.
   ```

3. **Migration Warnings** (when relevant)
   ```markdown
   **NOTE**: This migration creates an index on `workspace_app_statuses`.
   For deployments with heavy task usage, this may take a moment to complete.
   ```

4. **Visual Evidence** (for UI changes)
   ```markdown
   <img width="1281" height="425" alt="image" src="..." />
   ```

### Never Include

- ❌ **Test plans** - Testing is handled through code review and CI
- ❌ **"Benefits" sections** - Benefits should be clear from the description
- ❌ **Implementation details** - Keep it high-level
- ❌ **Marketing language** - Stay technical and factual
- ❌ **Bullet lists of features** (unless it's a large refactor that needs enumeration)

## Special Patterns

### Simple Chore PRs

For straightforward updates (dependency bumps, minor fixes):

```markdown
Changes from [link to upstream PR/issue]
```

Or:

```markdown
Reference:
[link explaining why this change is needed]
```

### Bug Fixes

Start with the problem, then explain the fix:

```markdown
[What was broken and why it matters]

[What you changed to fix it]
```

### Dependency Updates

Dependabot PRs are auto-generated - don't try to match their verbose style for manual updates. Instead use:

```markdown
Changes from https://github.com/upstream/repo/pull/XXX/
```

## Attribution Footer

For AI-generated PRs, end with:

```markdown
🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

## Key Principles

1. **Be concise** - Default to 1-2 paragraphs unless complexity demands more
2. **Be technical** - Explain what and why, not detailed how
3. **Link everything** - Issues, PRs, upstream changes, Notion docs
4. **Show impact** - Metrics for performance, screenshots for UI, warnings for migrations
5. **No test plans** - Code review and CI handle testing
6. **No benefits sections** - Benefits should be obvious from the technical description

## Examples by Category

### Performance Improvements
See PR #21099 - includes query timing metrics and explains the index solution

### Bug Fixes
See PR #21100 - describes broken behavior then the fix in two sentences

### Documentation
See PR #21080 - long form explaining inaccuracies and improvements for major rewrite
See PR #21077 - one sentence for simple correction

### Features
See PR #21106 - simple statement of what was added and dependencies

### Refactoring
See PR #21102 - explains why client-side sorting is now redundant

### Configuration
See PR #21083 - adds guidelines with issue reference
