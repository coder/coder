# Coder Development Guidelines

Read [cursor rules](.cursorrules).

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

## Database queries

- MUST DO! Any changes to database - adding queries, modifying queries should be done in the  `coderd\database\queries\*.sql` files. Use `make gen` to generate necessary changes after.
- MUST DO! Queries are grouped in files relating to context - e.g. `prebuilds.sql`, `users.sql`, `provisionerjobs.sql`.
- After making changes to any `coderd\database\queries\*.sql` files you must run `make gen` to generate respective ORM changes.

## Architecture

### Core Components

- **coderd**: Main API service connecting workspaces, provisioners, and users
- **provisionerd**: Execution context for infrastructure-modifying providers
- **Agents**: Services in remote workspaces providing features like SSH and port forwarding
- **Workspaces**: Cloud resources defined by Terraform

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

For building Frontend refer to [this document](docs/contributing/frontend.md)
