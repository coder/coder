# Development Workflows and Guidelines

## Quick Start Checklist for New Features

### Before Starting

- [ ] Run `git pull` to ensure you're on latest code
- [ ] Check if feature touches database - you'll need migrations
- [ ] Check if feature touches audit logs - update `enterprise/audit/table.go`

## Development Server

### Starting Development Mode

- **Use `./scripts/develop.sh` to start Coder in development mode**
- This automatically builds and runs with `--dev` flag and proper access URL
- **⚠️ Do NOT manually run `make build && ./coder server --dev` - use the script instead**

### Development Workflow

1. **Always start with the development script**: `./scripts/develop.sh`
2. **Make changes** to your code
3. **The script will automatically rebuild** and restart as needed
4. **Access the development server** at the URL provided by the script

## Code Style Guidelines

### Go Style

- Follow [Effective Go](https://go.dev/doc/effective_go) and [Go's Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Create packages when used during implementation
- Validate abstractions against implementations
- **Test packages**: Use `package_test` naming (e.g., `identityprovider_test`) for black-box testing

### Error Handling

- Use descriptive error messages
- Wrap errors with context
- Propagate errors appropriately
- Use proper error types
- Pattern: `xerrors.Errorf("failed to X: %w", err)`

## Naming Conventions

- Names MUST tell what code does, not how it's implemented or its history
- Follow Go and TypeScript naming conventions
- When changing code, never document the old behavior or the behavior change
- NEVER use implementation details in names (e.g., "ZodValidator", "MCPWrapper", "JSONParser")
- NEVER use temporal/historical context in names (e.g., "LegacyHandler", "UnifiedTool", "ImprovedInterface", "EnhancedParser")
- NEVER use pattern names unless they add clarity (e.g., prefer "Tool" over "ToolFactory")
- Abbreviate only when obvious

### Comments

- Document exported functions, types, and non-obvious logic
- Follow JSDoc format for TypeScript
- Use godoc format for Go code

## Database Migration Workflows

### Migration Guidelines

1. **Create migration files**:
   - Location: `coderd/database/migrations/`
   - Format: `{number}_{description}.{up|down}.sql`
   - Number must be unique and sequential
   - Always include both up and down migrations

2. **Use helper scripts**:
   - `./coderd/database/migrations/create_migration.sh "migration name"` - Creates new migration files
   - `./coderd/database/migrations/fix_migration_numbers.sh` - Renumbers migrations to avoid conflicts
   - `./coderd/database/migrations/create_fixture.sh "fixture name"` - Creates test fixtures for migrations

3. **Update database queries**:
   - **MUST DO**: Any changes to database - adding queries, modifying queries should be done in the `coderd/database/queries/*.sql` files
   - **MUST DO**: Queries are grouped in files relating to context - e.g. `prebuilds.sql`, `users.sql`, `oauth2.sql`
   - After making changes to any `coderd/database/queries/*.sql` files you must run `make gen` to generate respective ORM changes

4. **Handle nullable fields**:
   - Use `sql.NullString`, `sql.NullBool`, etc. for optional database fields
   - Set `.Valid = true` when providing values

5. **Audit table updates**:
   - If adding fields to auditable types, update `enterprise/audit/table.go`
   - Add each new field with appropriate action (ActionTrack, ActionIgnore, ActionSecret)
   - Run `make gen` to verify no audit errors

### Database Generation Process

1. Modify SQL files in `coderd/database/queries/`
2. Run `make gen`
3. If errors about audit table, update `enterprise/audit/table.go`
4. Run `make gen` again
5. Run `make lint` to catch any remaining issues

## API Development Workflow

### Adding New API Endpoints

1. **Define types** in `codersdk/` package
2. **Add handler** in appropriate `coderd/` file
3. **Register route** in `coderd/coderd.go`
4. **Add tests** in `coderd/*_test.go` files
5. **Update OpenAPI** by running `make gen`

## Testing Workflows

### Test Execution

- Run full test suite: `make test`
- Run specific test: `make test RUN=TestFunctionName`
- Run with Postgres: `make test-postgres`
- Run with race detector: `make test-race`
- Run end-to-end tests: `make test-e2e`

### Test Development

- Use table-driven tests for comprehensive coverage
- Mock external dependencies
- Test both positive and negative cases
- Use `testutil.WaitLong` for timeouts in tests
- Always use `t.Parallel()` in tests

## Commit Style

- Follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)
- Format: `type(scope): message`
- Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`
- Keep message titles concise (~70 characters)
- Use imperative, present tense in commit titles

## Code Navigation and Investigation

### Using LSP Tools (STRONGLY RECOMMENDED)

**IMPORTANT**: Always use LSP tools for code navigation and understanding. These tools provide accurate, real-time analysis of the codebase and should be your first choice for code investigation.

#### Go LSP Tools (for backend code)

1. **Find function definitions** (USE THIS FREQUENTLY):
   - `mcp__go-language-server__definition symbolName`
   - Example: `mcp__go-language-server__definition getOAuth2ProviderAppAuthorize`
   - Quickly jump to function implementations across packages

2. **Find symbol references** (ESSENTIAL FOR UNDERSTANDING IMPACT):
   - `mcp__go-language-server__references symbolName`
   - Locate all usages of functions, types, or variables
   - Critical for refactoring and understanding data flow

3. **Get symbol information**:
   - `mcp__go-language-server__hover filePath line column`
   - Get type information and documentation at specific positions

#### TypeScript LSP Tools (for frontend code in site/)

1. **Find component/function definitions** (USE THIS FREQUENTLY):
   - `mcp__typescript-language-server__definition symbolName`
   - Example: `mcp__typescript-language-server__definition LoginPage`
   - Quickly navigate to React components, hooks, and utility functions

2. **Find symbol references** (ESSENTIAL FOR UNDERSTANDING IMPACT):
   - `mcp__typescript-language-server__references symbolName`
   - Locate all usages of components, types, or functions
   - Critical for refactoring React components and understanding prop usage

3. **Get type information**:
   - `mcp__typescript-language-server__hover filePath line column`
   - Get TypeScript type information and JSDoc documentation

4. **Rename symbols safely**:
   - `mcp__typescript-language-server__rename_symbol filePath line column newName`
   - Rename components, props, or functions across the entire codebase

5. **Check for TypeScript errors**:
   - `mcp__typescript-language-server__diagnostics filePath`
   - Get compilation errors and warnings for a specific file

### Investigation Strategy (LSP-First Approach)

#### Backend Investigation (Go)

1. **Start with route registration** in `coderd/coderd.go` to understand API endpoints
2. **Use Go LSP `definition` lookup** to trace from route handlers to actual implementations
3. **Use Go LSP `references`** to understand how functions are called throughout the codebase
4. **Follow the middleware chain** using LSP tools to understand request processing flow
5. **Check test files** for expected behavior and error patterns

#### Frontend Investigation (TypeScript/React)

1. **Start with route definitions** in `site/src/App.tsx` or router configuration
2. **Use TypeScript LSP `definition`** to navigate to React components and hooks
3. **Use TypeScript LSP `references`** to find all component usages and prop drilling
4. **Follow the component hierarchy** using LSP tools to understand data flow
5. **Check for TypeScript errors** with `diagnostics` before making changes
6. **Examine test files** (`.test.tsx`) for component behavior and expected props

## Troubleshooting Development Issues

### Common Issues

1. **Development server won't start** - Use `./scripts/develop.sh` instead of manual commands
2. **Database migration errors** - Check migration file format and use helper scripts
3. **Audit table errors** - Update `enterprise/audit/table.go` with new fields
4. **OAuth2 compliance issues** - Ensure RFC-compliant error responses

### Debug Commands

- Check linting: `make lint`
- Generate code: `make gen`
- Clean build: `make clean`

## Development Environment Setup

### Prerequisites

- Go (version specified in go.mod)
- Node.js and pnpm for frontend development
- PostgreSQL for database testing
- Docker for containerized testing

### First Time Setup

1. Clone the repository
2. Run `./scripts/develop.sh` to start development server
3. Access the development URL provided
4. Create admin user as prompted
5. Begin development
