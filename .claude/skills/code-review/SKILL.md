---
name: code-review
description: Reviews code changes for bugs, security issues, and quality problems
---

# Code Review Skill

Review code changes in coder/coder and identify bugs, security issues, and
quality problems.

## Workflow

1. **Get the code changes** - Use the method provided in the prompt, or if none
   specified:
   - For a PR: `gh pr diff <PR_NUMBER> --repo coder/coder`
   - For local changes: `git diff main` or `git diff --staged`

2. **Read full files** before commenting - verify issues actually exist at the
   lines you're reviewing

3. **Analyze for issues** - Focus on what could break production

4. **Report findings** - Use the method provided in the prompt, or summarize
   directly

## Severity Levels

- **🔴 CRITICAL**: Security vulnerabilities, auth bypass, data corruption,
  crashes
- **🟡 IMPORTANT**: Logic bugs, race conditions, resource leaks, unhandled
  errors
- **🔵 NITPICK**: Minor improvements, style issues, portability concerns

## What to Look For

- **Security**: Auth bypass, injection, data exposure, improper access control
- **Correctness**: Logic errors, off-by-one, nil/null handling, error paths
- **Concurrency**: Race conditions, deadlocks, missing synchronization
- **Resources**: Leaks, unclosed handles, missing cleanup
- **Error handling**: Swallowed errors, missing validation, panic paths

## What NOT to Comment On

- Style that matches existing Coder patterns (check AGENTS.md first)
- Code that already exists unchanged
- Theoretical issues without concrete impact
- Changes unrelated to the PR's purpose

## Coder-Specific Patterns

### Authorization Context

```go
// Public endpoints needing system access
dbauthz.AsSystemRestricted(ctx)

// Authenticated endpoints with user context - just use ctx
api.Database.GetResource(ctx, id)
```

### Error Handling

```go
// OAuth2 endpoints use RFC-compliant errors
writeOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_grant", "description")

// Regular endpoints use httpapi
httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{...})
```

### Shell Scripts

`set -u` only catches UNDEFINED variables, not empty strings:

```sh
unset VAR; echo ${VAR}         # ERROR with set -u
VAR=""; echo ${VAR}            # OK with set -u (empty is fine)
VAR="${INPUT:-}"; echo ${VAR}  # OK - always defined
```

GitHub Actions context variables (`github.*`, `inputs.*`) are always defined.

## Review Quality

- Explain **impact** ("causes crash when X" not "could be better")
- Make observations **actionable** with specific fixes
- Read the **full context** before commenting on a line
- Check **AGENTS.md** for project conventions before flagging style
