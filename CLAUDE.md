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
   - **Use helper scripts**:
     - `./coderd/database/migrations/create_migration.sh "migration name"` - Creates new migration files
     - `./coderd/database/migrations/fix_migration_numbers.sh` - Renumbers migrations to avoid conflicts
     - `./coderd/database/migrations/create_fixture.sh "fixture name"` - Creates test fixtures for migrations

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

### In-Memory Database Testing

When adding new database fields:

- **CRITICAL**: Update `coderd/database/dbmem/dbmem.go` in-memory implementations
- The `Insert*` functions must include ALL new fields, not just basic ones
- Common issue: Tests pass with real database but fail with in-memory database due to missing field mappings
- Always verify in-memory database functions match the real database schema after migrations

Example pattern:

```go
// In dbmem.go - ensure ALL fields are included
code := database.OAuth2ProviderAppCode{
    ID:                  arg.ID,
    CreatedAt:           arg.CreatedAt,
    // ... existing fields ...
    ResourceUri:         arg.ResourceUri,         // New field
    CodeChallenge:       arg.CodeChallenge,       // New field
    CodeChallengeMethod: arg.CodeChallengeMethod, // New field
}
```

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

## RFC Compliance Development

### Implementing Standard Protocols

When implementing standard protocols (OAuth2, OpenID Connect, etc.):

1. **Fetch and Analyze Official RFCs**:
   - Always read the actual RFC specifications before implementation
   - Use WebFetch tool to get current RFC content for compliance verification
   - Document RFC requirements in code comments

2. **Default Values Matter**:
   - Pay close attention to RFC-specified default values
   - Example: RFC 7591 specifies `client_secret_basic` as default, not `client_secret_post`
   - Ensure consistency between database migrations and application code

3. **Security Requirements**:
   - Follow RFC security considerations precisely
   - Example: RFC 7592 prohibits returning registration access tokens in GET responses
   - Implement proper error responses per protocol specifications

4. **Validation Compliance**:
   - Implement comprehensive validation per RFC requirements
   - Support protocol-specific features (e.g., custom schemes for native OAuth2 apps)
   - Test edge cases defined in specifications

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

6. **RFC 8707 Resource Indicators**:
   - Store resource parameters in database for server-side validation (opaque tokens)
   - Validate resource consistency between authorization and token requests
   - Support audience validation in refresh token flows
   - Resource parameter is optional but must be consistent when provided

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

## Testing Best Practices

### Avoiding Race Conditions

1. **Unique Test Identifiers**:
   - Never use hardcoded names in concurrent tests
   - Use `time.Now().UnixNano()` or similar for unique identifiers
   - Example: `fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())`

2. **Database Constraint Awareness**:
   - Understand unique constraints that can cause test conflicts
   - Generate unique values for all constrained fields
   - Test name isolation prevents cross-test interference

### RFC Protocol Testing

1. **Compliance Test Coverage**:
   - Test all RFC-defined error codes and responses
   - Validate proper HTTP status codes for different scenarios
   - Test protocol-specific edge cases (URI formats, token formats, etc.)

2. **Security Boundary Testing**:
   - Test client isolation and privilege separation
   - Verify information disclosure protections
   - Test token security and proper invalidation

## Code Navigation and Investigation

### Using Go LSP Tools (STRONGLY RECOMMENDED)

**IMPORTANT**: Always use Go LSP tools for code navigation and understanding. These tools provide accurate, real-time analysis of the codebase and should be your first choice for code investigation.

When working with the Coder codebase, leverage Go Language Server Protocol tools for efficient code navigation:

1. **Find function definitions** (USE THIS FREQUENTLY):

   ```none
   mcp__go-language-server__definition symbolName
   ```

   - Example: `mcp__go-language-server__definition getOAuth2ProviderAppAuthorize`
   - Example: `mcp__go-language-server__definition ExtractAPIKeyMW`
   - Quickly jump to function implementations across packages
   - **Use this when**: You see a function call and want to understand its implementation
   - **Tip**: Include package prefix if symbol is ambiguous (e.g., `httpmw.ExtractAPIKeyMW`)

2. **Find symbol references** (ESSENTIAL FOR UNDERSTANDING IMPACT):

   ```none
   mcp__go-language-server__references symbolName
   ```

   - Example: `mcp__go-language-server__references APITokenFromRequest`
   - Locate all usages of functions, types, or variables
   - Understand code dependencies and call patterns
   - **Use this when**: Making changes to understand what code might be affected
   - **Critical for**: Refactoring, deprecating functions, or understanding data flow

3. **Get symbol information** (HELPFUL FOR TYPE INFO):

   ```none
   mcp__go-language-server__hover filePath line column
   ```

   - Example: `mcp__go-language-server__hover /Users/thomask33/Projects/coder/coderd/httpmw/apikey.go 560 25`
   - Get type information and documentation at specific positions
   - **Use this when**: You need to understand the type of a variable or return value

4. **Edit files using LSP** (WHEN MAKING TARGETED CHANGES):

   ```none
   mcp__go-language-server__edit_file filePath edits
   ```

   - Make precise edits using line numbers
   - **Use this when**: You need to make small, targeted changes to specific lines

5. **Get diagnostics** (ALWAYS CHECK AFTER CHANGES):

   ```none
   mcp__go-language-server__diagnostics filePath
   ```

   - Check for compilation errors, unused imports, etc.
   - **Use this when**: After making changes to ensure code is still valid

### LSP Tool Usage Priority

**ALWAYS USE THESE TOOLS FIRST**:

- **Use LSP `definition`** instead of manual searching for function implementations
- **Use LSP `references`** instead of grep when looking for function/type usage
- **Use LSP `hover`** to understand types and signatures
- **Use LSP `diagnostics`** after making changes to check for errors

**When to use other tools**:

- **Use Grep for**: Text-based searches, finding patterns across files, searching comments
- **Use Bash for**: Running tests, git commands, build operations
- **Use Read tool for**: Reading configuration files, documentation, non-Go files

### Investigation Strategy (LSP-First Approach)

1. **Start with route registration** in `coderd/coderd.go` to understand API endpoints
2. **Use LSP `definition` lookup** to trace from route handlers to actual implementations
3. **Use LSP `references`** to understand how functions are called throughout the codebase
4. **Follow the middleware chain** using LSP tools to understand request processing flow
5. **Check test files** for expected behavior and error patterns
6. **Use LSP `diagnostics`** to ensure your changes don't break compilation

### Common LSP Workflows

**Understanding a new feature**:

1. Use `grep` to find the main entry point (e.g., route registration)
2. Use LSP `definition` to jump to handler implementation
3. Use LSP `references` to see how the handler is used
4. Use LSP `definition` on each function call within the handler

**Making changes to existing code**:

1. Use LSP `references` to understand the impact of your changes
2. Use LSP `definition` to understand the current implementation
3. Make your changes using `Edit` or LSP `edit_file`
4. Use LSP `diagnostics` to verify your changes compile correctly
5. Run tests to ensure functionality still works

**Debugging issues**:

1. Use LSP `definition` to find the problematic function
2. Use LSP `references` to trace how the function is called
3. Use LSP `hover` to understand parameter types and return values
4. Use `Read` to examine the full context around the issue

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
7. **OAuth2 tests failing but scripts working** - Check in-memory database implementations in `dbmem.go`
8. **Resource indicator validation failing** - Ensure database stores and retrieves resource parameters correctly
9. **PKCE tests failing** - Verify both authorization code storage and token exchange handle PKCE fields
10. **Race conditions in tests** - Use unique identifiers instead of hardcoded names
11. **RFC compliance failures** - Verify against actual RFC specifications, not assumptions
12. **Authorization context errors in public endpoints** - Use `dbauthz.AsSystemRestricted(ctx)` pattern
13. **Default value mismatches** - Ensure database migrations match application code defaults
14. **Bearer token authentication issues** - Check token extraction precedence and format validation
15. **URI validation failures** - Support both standard schemes and custom schemes per protocol requirements
16. **Log message formatting errors** - Use lowercase, descriptive messages without special characters

## Systematic Debugging Approach

### Multi-Issue Problem Solving

When facing multiple failing tests or complex integration issues:

1. **Identify Root Causes**:
   - Run failing tests individually to isolate issues
   - Use LSP tools to trace through call chains
   - Check both compilation and runtime errors

2. **Fix in Logical Order**:
   - Address compilation issues first (imports, syntax)
   - Fix authorization and RBAC issues next
   - Resolve business logic and validation issues
   - Handle edge cases and race conditions last

3. **Verification Strategy**:
   - Test each fix individually before moving to next issue
   - Use `make lint` and `make gen` after database changes
   - Verify RFC compliance with actual specifications
   - Run comprehensive test suites before considering complete

### Authorization Context Patterns

Common patterns for different endpoint types:

```go
// Public endpoints needing system access (OAuth2 registration)
app, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)

// Authenticated endpoints with user context
app, err := api.Database.GetOAuth2ProviderAppByClientID(ctx, clientID)

// System operations in middleware
roles, err := db.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), userID)
```

## Protocol Implementation Checklist

### OAuth2/Authentication Protocol Implementation

Before completing OAuth2 or authentication feature work:

- [ ] Verify RFC compliance by reading actual specifications
- [ ] Implement proper error response formats per protocol
- [ ] Add comprehensive validation for all protocol fields
- [ ] Test security boundaries and token handling
- [ ] Update RBAC permissions for new resources
- [ ] Add audit logging support if applicable
- [ ] Create database migrations with proper defaults
- [ ] Update in-memory database implementations
- [ ] Add comprehensive test coverage including edge cases
- [ ] Verify linting and formatting compliance
- [ ] Test both positive and negative scenarios
- [ ] Document protocol-specific patterns and requirements
