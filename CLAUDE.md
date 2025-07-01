# Coder Development Guidelines

Read [cursor rules](.cursorrules).

## Quick Start Checklist for New Features

### Before Starting

- [ ] Run `git pull` to ensure you're on latest code
- [ ] Check if feature touches database - you'll need migrations
- [ ] Check if feature touches audit logs - update `enterprise/audit/table.go`

## Development Server

### Starting Development Mode

- Use `./scripts/develop.sh` to start Coder in development mode
- This automatically builds and runs with `--dev` flag and proper access URL
- Do NOT manually run `make build && ./coder server --dev` - use the script instead

## Build/Test/Lint Commands

### Main Commands

- `make build` or `make build-fat` - Build all "fat" binaries (includes "server" functionality)
- `make build-slim` - Build "slim" binaries
- `make test` - Run Go tests
- `make test RUN=TestFunctionName` or `go test -v ./path/to/package -run TestFunctionName` - Test single
- `make test-postgres` - Run tests with Postgres database
- `make test-race` - Run tests with Go race detector
- `make test-e2e` - Run end-to-end tests
- `make lint` - Run all linters
- `make fmt` - Format all code
- `make gen` - Generates mocks, database queries and other auto-generated files

### Frontend Commands (site directory)

- `pnpm build` - Build frontend
- `pnpm dev` - Run development server
- `pnpm check` - Run code checks
- `pnpm format` - Format frontend code
- `pnpm lint` - Lint frontend code
- `pnpm test` - Run frontend tests

## Code Style Guidelines

### Go

- Follow [Effective Go](https://go.dev/doc/effective_go) and [Go's Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofumpt` for formatting
- Create packages when used during implementation
- Validate abstractions against implementations
- **Test packages**: Use `package_test` naming (e.g., `identityprovider_test`) for black-box testing

### Error Handling

- Use descriptive error messages
- Wrap errors with context
- Propagate errors appropriately
- Use proper error types
- (`xerrors.Errorf("failed to X: %w", err)`)

### Naming

- Use clear, descriptive names
- Abbreviate only when obvious
- Follow Go and TypeScript naming conventions

### Comments

- Document exported functions, types, and non-obvious logic
- Follow JSDoc format for TypeScript
- Use godoc format for Go code

## Commit Style

- Follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)
- Format: `type(scope): message`
- Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`
- Keep message titles concise (~70 characters)
- Use imperative, present tense in commit titles

## Database Work

### Migration Guidelines

1. **Create migration files**:
   - Location: `coderd/database/migrations/`
   - Format: `{number}_{description}.{up|down}.sql`
   - Number must be unique and sequential
   - Always include both up and down migrations

2. **Update database queries**:
   - MUST DO! Any changes to database - adding queries, modifying queries should be done in the `coderd/database/queries/*.sql` files
   - MUST DO! Queries are grouped in files relating to context - e.g. `prebuilds.sql`, `users.sql`, `oauth2.sql`
   - After making changes to any `coderd/database/queries/*.sql` files you must run `make gen` to generate respective ORM changes

3. **Handle nullable fields**:
   - Use `sql.NullString`, `sql.NullBool`, etc. for optional database fields
   - Set `.Valid = true` when providing values
   - Example:

     ```go
     CodeChallenge: sql.NullString{
         String: params.codeChallenge,
         Valid:  params.codeChallenge != "",
     }
     ```

4. **Audit table updates**:
   - If adding fields to auditable types, update `enterprise/audit/table.go`
   - Add each new field with appropriate action (ActionTrack, ActionIgnore, ActionSecret)
   - Run `make gen` to verify no audit errors

5. **In-memory database (dbmem) updates**:
   - When adding new fields to database structs, ensure `dbmem` implementation copies all fields
   - Check `coderd/database/dbmem/dbmem.go` for Insert/Update methods
   - Missing fields in dbmem can cause tests to fail even if main implementation is correct

### Database Generation Process

1. Modify SQL files in `coderd/database/queries/`
2. Run `make gen`
3. If errors about audit table, update `enterprise/audit/table.go`
4. Run `make gen` again
5. Run `make lint` to catch any remaining issues

## Architecture

### Core Components

- **coderd**: Main API service connecting workspaces, provisioners, and users
- **provisionerd**: Execution context for infrastructure-modifying providers
- **Agents**: Services in remote workspaces providing features like SSH and port forwarding
- **Workspaces**: Cloud resources defined by Terraform

### Adding New API Endpoints

1. **Define types** in `codersdk/` package
2. **Add handler** in appropriate `coderd/` file
3. **Register route** in `coderd/coderd.go`
4. **Add tests** in `coderd/*_test.go` files
5. **Update OpenAPI** by running `make gen`

## Sub-modules

### Template System

- Templates define infrastructure for workspaces using Terraform
- Environment variables pass context between Coder and templates
- Official modules extend development environments

### RBAC System

- Permissions defined at site, organization, and user levels
- Object-Action model protects resources
- Built-in roles: owner, member, auditor, templateAdmin
- Permission format: `<sign>?<level>.<object>.<id>.<action>`

### Database

- PostgreSQL 13+ recommended for production
- Migrations managed with `migrate`
- Database authorization through `dbauthz` package

## Frontend

The frontend is contained in the site folder.

For building Frontend refer to [this document](docs/about/contributing/frontend.md)

## Common Patterns

### OAuth2/Authentication Work

- Types go in `codersdk/oauth2.go` or similar
- Handlers go in `coderd/oauth2.go` or `coderd/identityprovider/`
- Database fields need migration + audit table updates
- Always support backward compatibility

## OAuth2 Development

### OAuth2 Provider Implementation

When working on OAuth2 provider features:

1. **OAuth2 Spec Compliance**:
   - Follow RFC 6749 for token responses
   - Use `expires_in` (seconds) not `expiry` (timestamp) in token responses
   - Return proper OAuth2 error format: `{"error": "code", "error_description": "details"}`

2. **Error Response Format**:
   - Create OAuth2-compliant error responses for token endpoint
   - Use standard error codes: `invalid_client`, `invalid_grant`, `invalid_request`
   - Avoid generic error responses for OAuth2 endpoints

3. **Testing OAuth2 Features**:
   - Use scripts in `./scripts/oauth2/` for testing
   - Run `./scripts/oauth2/test-mcp-oauth2.sh` for comprehensive tests
   - Manual testing: use `./scripts/oauth2/test-manual-flow.sh`

4. **PKCE Implementation**:
   - Support both with and without PKCE for backward compatibility
   - Use S256 method for code challenge
   - Properly validate code_verifier against stored code_challenge

5. **UI Authorization Flow**:
   - Use POST requests for consent, not GET with links
   - Avoid dependency on referer headers for security decisions
   - Support proper state parameter validation

### OAuth2 Error Handling Pattern

```go
// Define specific OAuth2 errors
var (
    errInvalidPKCE = xerrors.New("invalid code_verifier")
)

// Use OAuth2-compliant error responses
type OAuth2Error struct {
    Error            string `json:"error"`
    ErrorDescription string `json:"error_description,omitempty"`
}

// Return proper OAuth2 errors
if errors.Is(err, errInvalidPKCE) {
    writeOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_grant", "The PKCE code verifier is invalid")
    return
}
```

### Testing Patterns

- Use table-driven tests for comprehensive coverage
- Mock external dependencies
- Test both positive and negative cases
- Use `testutil.WaitLong` for timeouts in tests

## Testing Scripts

### OAuth2 Test Scripts

Located in `./scripts/oauth2/`:

- `test-mcp-oauth2.sh` - Full automated test suite
- `setup-test-app.sh` - Create test OAuth2 app
- `cleanup-test-app.sh` - Remove test app
- `generate-pkce.sh` - Generate PKCE parameters
- `test-manual-flow.sh` - Manual browser testing

Always run the full test suite after OAuth2 changes:

```bash
./scripts/oauth2/test-mcp-oauth2.sh
```

## Troubleshooting

### Common Issues

1. **"Audit table entry missing action"** - Update `enterprise/audit/table.go`
2. **"package should be X_test"** - Use `package_test` naming for test files
3. **SQL type errors** - Use `sql.Null*` types for nullable fields
4. **Missing newlines** - Ensure files end with newline character
5. **Tests passing locally but failing in CI** - Check if `dbmem` implementation needs updating
6. **OAuth2 endpoints returning wrong error format** - Ensure OAuth2 endpoints return RFC 6749 compliant errors
