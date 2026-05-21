---
name: doc-check
description: Checks if code changes require documentation updates
---

# Documentation Check Skill

Review code changes and determine if documentation updates or new documentation
is needed.

## Workflow

1. **Get the code changes** - Use the method provided in the prompt, or if none
   specified:
   - For a PR: `gh pr diff <PR_NUMBER> --repo coder/coder`
   - For local changes: `git diff main` or `git diff --staged`
   - For a branch: `git diff main...<branch>`

2. **Understand the scope** - Consider what changed:
   - Is this user-facing or internal?
   - Does it change behavior, APIs, CLI flags, or configuration?
   - Even for "internal" or "chore" changes, always verify the actual diff

3. **Search the docs** for related content in `docs/`

4. **Decide what's needed**:
   - Do existing docs need updates to match the code?
   - Is new documentation needed for undocumented features?
   - Or is everything already covered?

5. **Report findings** - Use the method provided in the prompt, or if none
   specified, summarize findings directly

## What to Check

- **Accuracy**: Does documentation match current code behavior?
- **Completeness**: Are new features/options documented?
- **Examples**: Do code examples still work?
- **CLI/API changes**: Are new flags, endpoints, or options documented?
- **Configuration**: Are new environment variables or settings documented?
- **Breaking changes**: Are migration steps documented if needed?
- **Premium features**: Should docs indicate `(Premium)` in the title?

## Key Documentation Info

- **`docs/manifest.json`** - Navigation structure; new pages MUST be added here
- **`docs/reference/cli/*.md`** - Auto-generated from Go code, don't edit directly
- **Premium features** - H1 title should include `(Premium)` suffix

## Coder-Specific Patterns

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

CLI docs in `docs/reference/cli/` are auto-generated. Don't suggest editing them
directly. Instead, changes should be made in the Go code that defines the CLI
commands (typically in `cli/` directory).

### Code Examples

Use `sh` for shell commands:

```sh
coder server --flag-name value
```
