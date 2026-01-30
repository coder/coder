---
name: triage
description: Autonomously implements fixes for pre-classified GitHub issues
---

# Triage Skill

Implement fixes for GitHub issues that have been pre-classified as suitable for
automated implementation (small/medium size, low/medium complexity, sufficient
information).

## Workflow

1. **Read the issue** - Use `gh issue view <NUMBER> --repo coder/coder` to get
   the full issue description and all comments

2. **Understand the problem** - Analyze what's being reported:
   - What is the expected behavior?
   - What is the actual behavior?
   - What are the reproduction steps?

3. **Investigate the codebase** - Find the relevant code:
   - Use LSP tools to find definitions and references
   - Trace the code path that's causing the issue
   - Understand the surrounding context and dependencies

4. **Implement with TDD**:
   - Write failing tests first that demonstrate the bug
   - Implement the minimal fix to make tests pass
   - Verify no regressions with existing tests
   - Run `make lint` to catch style issues

5. **Create PR** - Use `gh pr create` with:
   - Clear title following conventional commits: `fix(scope): description`
   - Description linking to the issue
   - Summary of the fix and how it was tested

## Important Guidelines

- **Minimal changes only** - Fix what's broken, nothing more
- **Follow existing patterns** - Match the code style around you
- **Tests are required** - No PR without tests proving the fix works
- **If blocked, comment** - If you can't proceed, comment on the issue explaining
  what additional information is needed

## What to Look For

### Root Cause Analysis

- **Code path**: What code path leads to the bug?
- **Edge cases**: Is this a corner case or common scenario?
- **Recent changes**: Did a recent commit introduce this?
- **Dependencies**: Is an external dependency involved?

### Solution Quality

- **Minimal change**: Does the fix touch only what's necessary?
- **Test coverage**: Are there tests proving the fix works?
- **No regressions**: Do existing tests still pass?
- **Documentation**: Does the change need doc updates?

## Coder-Specific Patterns

### Testing

```go
// Always use t.Parallel() for concurrent test execution
func TestFeature(t *testing.T) {
    t.Parallel()
    // ...
}

// Use unique identifiers to prevent race conditions
name := fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())

// Use testutil for timeouts
ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
defer cancel()
```

### Database Changes

If the fix requires database changes:

1. Modify `coderd/database/queries/*.sql` files
2. Run `make gen`
3. If audit errors: update `enterprise/audit/table.go`
4. Run `make gen` again

### Error Handling

```go
// Wrap errors with context
if err != nil {
    return xerrors.Errorf("failed to process request: %w", err)
}

// OAuth2 endpoints use RFC-compliant errors
writeOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_grant", "description")
```

### Authorization

```go
// Public endpoints needing system access
dbauthz.AsSystemRestricted(ctx)

// Authenticated endpoints use the existing context
api.Database.GetResource(ctx, id)
```

## Commands Reference

```sh
# View issue details
gh issue view <NUMBER> --repo coder/coder

# Run specific test
make test RUN=TestName

# Run tests with race detector
make test-race RUN=TestName

# Lint code
make lint

# Create PR
gh pr create --title "fix(scope): description" --body "Fixes #NUMBER"
```
