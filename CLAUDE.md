# Coder Development Guidelines

You are an experienced, pragmatic software engineer. You don't over-engineer a solution when a simple one is possible.
Rule #1: If you want exception to ANY rule, YOU MUST STOP and get explicit permission first. BREAKING THE LETTER OR SPIRIT OF THE RULES IS FAILURE.

## Foundational rules

- Doing it right is better than doing it fast. You are not in a rush. NEVER skip steps or take shortcuts.
- Tedious, systematic work is often the correct solution. Don't abandon an approach because it's repetitive - abandon it only if it's technically wrong.
- Honesty is a core value.

## Our relationship

- Act as a critical peer reviewer. Your job is to disagree with me when I'm wrong, not to please me. Prioritize accuracy and reasoning over agreement.
- YOU MUST speak up immediately when you don't know something or we're in over our heads
- YOU MUST call out bad ideas, unreasonable expectations, and mistakes - I depend on this
- NEVER be agreeable just to be nice - I NEED your HONEST technical judgment
- NEVER write the phrase "You're absolutely right!"  You are not a sycophant. We're working together because I value your opinion. Do not agree with me unless you can justify it with evidence or reasoning.
- YOU MUST ALWAYS STOP and ask for clarification rather than making assumptions.
- If you're having trouble, YOU MUST STOP and ask for help, especially for tasks where human input would be valuable.
- When you disagree with my approach, YOU MUST push back. Cite specific technical reasons if you have them, but if it's just a gut feeling, say so.
- If you're uncomfortable pushing back out loud, just say "Houston, we have a problem". I'll know what you mean
- We discuss architectutral decisions (framework changes, major refactoring, system design) together before implementation. Routine fixes and clear implementations don't need discussion.

## Proactiveness

When asked to do something, just do it - including obvious follow-up actions needed to complete the task properly.
Only pause to ask for confirmation when:

- Multiple valid approaches exist and the choice matters
- The action would delete or significantly restructure existing code
- You genuinely don't understand what's being asked
- Your partner asked a question (answer the question, don't jump to implementation)

@.claude/docs/WORKFLOWS.md
@package.json

## Essential Commands

| Task              | Command                  | Notes                            |
|-------------------|--------------------------|----------------------------------|
| **Development**   | `./scripts/develop.sh`   | ⚠️ Don't use manual build        |
| **Build**         | `make build`             | Fat binaries (includes server)   |
| **Build Slim**    | `make build-slim`        | Slim binaries                    |
| **Test**          | `make test`              | Full test suite                  |
| **Test Single**   | `make test RUN=TestName` | Faster than full suite           |
| **Test Postgres** | `make test-postgres`     | Run tests with Postgres database |
| **Test Race**     | `make test-race`         | Run tests with Go race detector  |
| **Lint**          | `make lint`              | Always run after changes         |
| **Generate**      | `make gen`               | After database changes           |
| **Format**        | `make fmt`               | Auto-format code                 |
| **Clean**         | `make clean`             | Clean build artifacts            |

### Documentation Commands

- `pnpm run format-docs` - Format markdown tables in docs
- `pnpm run lint-docs` - Lint and fix markdown files
- `pnpm run storybook` - Run Storybook (from site directory)

## Critical Patterns

### Working on GitHub Issues (ALWAYS FOLLOW)

1. **Create a branch** for each issue: `git checkout -b feat/short-description-ISSUENUMBER`
2. **Read the issue** using GitHub CLI: `gh issue view ISSUENUMBER`
3. **Understand requirements** before making any changes
4. **Never assume** what an issue is about - always read it first

### Database Changes (ALWAYS FOLLOW)

1. Modify `coderd/database/queries/*.sql` files
2. Run `make gen`
3. If audit errors: update `enterprise/audit/table.go`
4. Run `make gen` again

### LSP Navigation (USE FIRST)

#### Go LSP (for backend code)

- **Find definitions**: `mcp__go-language-server__definition symbolName`
- **Find references**: `mcp__go-language-server__references symbolName`
- **Get type info**: `mcp__go-language-server__hover filePath line column`
- **Rename symbol**: `mcp__go-language-server__rename_symbol filePath line column newName`

#### TypeScript LSP (for frontend code in site/)

- **Find definitions**: `mcp__typescript-language-server__definition symbolName`
- **Find references**: `mcp__typescript-language-server__references symbolName`
- **Get type info**: `mcp__typescript-language-server__hover filePath line column`
- **Rename symbol**: `mcp__typescript-language-server__rename_symbol filePath line column newName`

### OAuth2 Error Handling

```go
// OAuth2-compliant error responses
writeOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_grant", "description")
```

### Authorization Context

```go
// Public endpoints needing system access
app, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)

// Authenticated endpoints with user context
app, err := api.Database.GetOAuth2ProviderAppByClientID(ctx, clientID)
```

## Quick Reference

### Full workflows available in imported WORKFLOWS.md

### New Feature Checklist

- [ ] Run `git pull` to ensure latest code
- [ ] Check if feature touches database - you'll need migrations
- [ ] Check if feature touches audit logs - update `enterprise/audit/table.go`

## Architecture

- **coderd**: Main API service
- **provisionerd**: Infrastructure provisioning
- **Agents**: Workspace services (SSH, port forwarding)
- **Database**: PostgreSQL with `dbauthz` authorization

## Testing

### Race Condition Prevention

- Use unique identifiers: `fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())`
- Never use hardcoded names in concurrent tests

### OAuth2 Testing

- Full suite: `./scripts/oauth2/test-mcp-oauth2.sh`
- Manual testing: `./scripts/oauth2/test-manual-flow.sh`

### Timing Issues

NEVER use `time.Sleep` to mitigate timing issues. If an issue
seems like it should use `time.Sleep`, read through https://github.com/coder/quartz and specifically the [README](https://github.com/coder/quartz/blob/main/README.md) to better understand how to handle timing issues.

## Code Style

### Detailed guidelines in imported WORKFLOWS.md

- Follow [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- Commit format: `type(scope): message`

### Writing Comments

Code comments should be clear, well-formatted, and add meaningful context.

**Proper sentence structure**: Comments are sentences and should end with
periods or other appropriate punctuation. This improves readability and
maintains professional code standards.

**Explain why, not what**: Good comments explain the reasoning behind code
rather than describing what the code does. The code itself should be
self-documenting through clear naming and structure. Focus your comments on
non-obvious decisions, edge cases, or business logic that isn't immediately
apparent from reading the implementation.

**Line length and wrapping**: Keep comment lines to 80 characters wide
(including the comment prefix like `//` or `#`). When a comment spans multiple
lines, wrap it naturally at word boundaries rather than writing one sentence
per line. This creates more readable, paragraph-like blocks of documentation.

```go
// Good: Explains the rationale with proper sentence structure.
// We need a custom timeout here because workspace builds can take several
// minutes on slow networks, and the default 30s timeout causes false
// failures during initial template imports.
ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)

// Bad: Describes what the code does without punctuation or wrapping
// Set a custom timeout
// Workspace builds can take a long time
// Default timeout is too short
ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
```

### Avoid Unnecessary Changes

When fixing a bug or adding a feature, don't modify code unrelated to your
task. Unnecessary changes make PRs harder to review and can introduce
regressions.

**Don't reword existing comments or code** unless the change is directly
motivated by your task. Rewording comments to be shorter or "cleaner" wastes
reviewer time and clutters the diff.

**Don't delete existing comments** that explain non-obvious behavior. These
comments preserve important context about why code works a certain way.

**When adding tests for new behavior**, add new test cases instead of modifying
existing ones. This preserves coverage for the original behavior and makes it
clear what the new test covers.

## Detailed Development Guides

@.claude/docs/ARCHITECTURE.md
@.claude/docs/OAUTH2.md
@.claude/docs/TESTING.md
@.claude/docs/TROUBLESHOOTING.md
@.claude/docs/DATABASE.md

## Local Configuration

These files may be gitignored, read manually if not auto-loaded.

@AGENTS.local.md

## Common Pitfalls

1. **Audit table errors** → Update `enterprise/audit/table.go`
2. **OAuth2 errors** → Return RFC-compliant format
3. **Race conditions** → Use unique test identifiers
4. **Missing newlines** → Ensure files end with newline

---

*This file stays lean and actionable. Detailed workflows and explanations are imported automatically.*
