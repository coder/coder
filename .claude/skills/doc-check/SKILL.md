---
name: doc-check
description: Checks if code changes require documentation updates
---

# Documentation Check Skill

Review code changes and determine if documentation updates or new
documentation is needed. This skill decides *whether* a change needs docs;
its counterpart, the [`write-docs` skill](../write-docs/SKILL.md), covers
writing them.

> [!IMPORTANT]
> The **canonical** rules for what belongs in the Coder docs (and what
> doesn't) live in
> [`docs/.style/content-guidelines.md`](../../../docs/.style/content-guidelines.md).
> Read that first. When this skill conflicts with the content
> guidelines, the content guidelines govern.

## Workflow

1. **Get the code changes.** Use the method provided in the prompt, or if
   none specified:
   - For a PR: `gh pr diff <PR_NUMBER> --repo coder/coder`
   - For local changes: `git diff main` or `git diff --staged`
   - For a branch: `git diff main...<branch>`

2. **Triage the diff.** Walk the
   [quick decision checklist](../../../docs/.style/content-guidelines.md#quick-decision-checklist)
   in the content guidelines. Most non-user-facing diffs route out of
   the docs entirely; see [What not to comment on](#what-not-to-comment-on).

3. **Understand the scope.** Consider what changed:
   - Is this user-facing or internal?
   - Does it change behavior, APIs, CLI flags, or configuration?
   - Even for "internal" or "chore" changes, always verify the actual
     diff.

4. **Search the docs.** Find related content in `docs/`.

5. **Decide what's needed.** Consider:
   - Do existing docs need updates to match the code?
   - Is new documentation needed for undocumented features?
   - Or is everything already covered?

6. **Report findings.** Use the method provided in the prompt, or if none
   specified, summarize findings directly.

## What to Check

- **Accuracy**: Does documentation match current code behavior?
- **Completeness**: Are new features or options documented?
- **Examples**: Do code examples still work?
- **CLI/API changes**: Are new flags, endpoints, or options documented?
- **Configuration**: Are new environment variables or settings documented?
- **Breaking changes**: Are migration steps documented if needed?
- **Premium features**: See [Premium feature signaling](#premium-feature-signaling)
  below.
- **Renames or moves**: See [Renames and moves require redirects](#renames-and-moves-require-redirects)
  below.

## What not to comment on

Do not produce sticky-comment suggestions for these classes of change.
They have no user-visible documentation surface.

- **Auto-generated CLI docs** under `docs/reference/cli/`. These are
  generated from Go code under `cli/`; suggest edits to the CLI
  definitions instead.
- **Internal-only refactors** with no user-visible behavior change.
- **Test-only changes** (new tests, refactored tests, fixtures).
- **CI, release, or tooling commits** that don't change user-facing
  surfaces. This includes workflow YAML, Makefile internals, formatter
  configs, and lint configs.
- **Dependency bumps** without behavior changes.
- **Pure code reorganizations** (moves, renames, package restructuring
  with no API or behavior change).
- **Features guarded by an unsafe experiment flag.** Features behind an
  unsafe experiment are not designed for users yet and may be reverted.
  See
  [Experiments versus feature stages](../../../docs/.style/content-guidelines.md#experiments-versus-feature-stages)
  in the content guidelines for the experiment-vs-stage distinction. A
  safe experiment or an Early Access feature does need at least a
  single-page doc, so don't apply this rule to those.

If a diff is a mix of one of the above with a user-facing change, comment
only on the user-facing portion.

## Key Documentation Info

- **`docs/manifest.json`** is the navigation structure; new pages MUST be
  added here.
- **`docs/reference/cli/*.md`** is auto-generated from Go code. Don't
  edit directly.
- **`docs/.style/content-guidelines.md`** is the canonical source for
  what belongs in the docs.

### Premium feature signaling

A page documenting a Premium feature requires **both** of the following.
Missing either one is a defect:

1. The H1 title takes a `(Premium)` suffix. Example:
   `# Template Insights (Premium)`.
2. The page's `docs/manifest.json` entry includes `"state": ["premium"]`.

### No emdash, endash, or ` -- ` as punctuation

This applies in docs prose, code blocks, comments, and string literals.
Use commas, semicolons, or periods, or restructure the sentence. For
numeric ranges, use a plain hyphen (e.g., `0-100`). The rule is enforced
by `make lint/emdash`, but the doc-check skill should also flag
violations it generates or suggests.

### Renames and moves require redirects

Redirects for [coder.com/docs](https://coder.com/docs) are configured in
a separate repo, not in this one. When a doc page is renamed or moved:

1. Update every link that relies on the old location.
2. Add an entry to
   [`coder/coder.com:redirects.json`](https://github.com/coder/coder.com/blob/master/redirects.json)
   that maps the old path to the new one. Open that PR alongside the
   `coder/coder` rename PR.

Do not create a `docs/_redirects` file in this repo; that format isn't
processed by coder.com.

## Coder-specific patterns

### Callouts

Use GitHub-Flavored Markdown alerts:

```markdown
> [!NOTE]
> Additional helpful information.

> [!WARNING]
> Important warning about potential issues.

> [!TIP]
> Helpful tip for users.
```

### CLI Documentation

CLI docs in `docs/reference/cli/` are auto-generated. Don't suggest
editing them directly. Changes should be made in the Go code that
defines the CLI commands (typically the `cli/` directory).

### Code Examples

Use `sh` for shell commands:

```sh
coder server --flag-name value
```
