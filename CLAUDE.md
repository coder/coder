# Coder Development Guidelines

@.claude/docs/WORKFLOWS.md
@.cursorrules
@README.md
@package.json

## 🚀 Essential Commands

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

### Frontend Commands (site directory)

- `pnpm build` - Build frontend
- `pnpm dev` - Run development server
- `pnpm check` - Run code checks
- `pnpm format` - Format frontend code
- `pnpm lint` - Lint frontend code
- `pnpm test` - Run frontend tests

### Documentation Commands

- `pnpm run format-docs` - Format markdown tables in docs
- `pnpm run lint-docs` - Lint and fix markdown files
- `pnpm run storybook` - Run Storybook (from site directory)

## 🔧 Critical Patterns

### Database Changes (ALWAYS FOLLOW)

1. Modify `coderd/database/queries/*.sql` files
2. Run `make gen`
3. If audit errors: update `enterprise/audit/table.go`
4. Run `make gen` again

### LSP Navigation (USE FIRST)

- **Find definitions**: `mcp__go-language-server__definition symbolName`
- **Find references**: `mcp__go-language-server__references symbolName`
- **Get type info**: `mcp__go-language-server__hover filePath line column`

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

## 📋 Quick Reference

### Full workflows available in imported WORKFLOWS.md

### New Feature Checklist

- [ ] Run `git pull` to ensure latest code
- [ ] Check if feature touches database - you'll need migrations
- [ ] Check if feature touches audit logs - update `enterprise/audit/table.go`

## 🏗️ Architecture

- **coderd**: Main API service
- **provisionerd**: Infrastructure provisioning
- **Agents**: Workspace services (SSH, port forwarding)
- **Database**: PostgreSQL with `dbauthz` authorization

## 🧪 Testing

### Race Condition Prevention

- Use unique identifiers: `fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())`
- Never use hardcoded names in concurrent tests

### OAuth2 Testing

- Full suite: `./scripts/oauth2/test-mcp-oauth2.sh`
- Manual testing: `./scripts/oauth2/test-manual-flow.sh`

## 🎯 Code Style

### Detailed guidelines in imported WORKFLOWS.md

- Follow [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- Commit format: `type(scope): message`

## 📚 Detailed Development Guides

@.claude/docs/OAUTH2.md
@.claude/docs/TESTING.md
@.claude/docs/TROUBLESHOOTING.md
@.claude/docs/DATABASE.md

## 🚨 Common Pitfalls

1. **Audit table errors** → Update `enterprise/audit/table.go`
2. **OAuth2 errors** → Return RFC-compliant format
3. **Race conditions** → Use unique test identifiers
4. **Missing newlines** → Ensure files end with newline

---

*This file stays lean and actionable. Detailed workflows and explanations are imported automatically.*
