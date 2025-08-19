# Copilot Coding Agent Instructions for Coder

## Quick Start for Agents

For most tasks, use this workflow:

1. **Simple Go builds**: `go build -o build/coder cmd/coder/main.go` (works without full dependency setup)
2. **Test specific functionality**: `export PATH=$PATH:$HOME/go/bin && go test -v ./package -run TestSpecific -short`
3. **Format code**: `go fmt ./...` (basic formatting without full make fmt)
4. **Validate changes**: `go vet ./...` and `go build ./...`

For full development setup, see commands below.

## Project Overview

**Coder** is a platform for **self-hosted cloud development environments** that enables organizations to provision secure, consistent development workspaces in any cloud infrastructure. The platform uses Terraform to define infrastructure, connects through Wireguard tunnels, and automatically manages resource lifecycle.

### Key Architecture Components

- **coderd**: Main API service, web dashboard, and HTTP endpoints
- **provisionerd**: Infrastructure provisioning service (executes Terraform)
- **agents**: Services running inside workspaces (SSH, port forwarding, apps)
- **CLI**: Command-line interface for developers and administrators
- **Frontend**: React/TypeScript web UI located in `site/` directory
- **Templates**: Terraform/OpenTofu definitions for workspace infrastructure

### Repository Scale & Technologies

- **~1,400 Go files** across backend services and CLI
- **Go 1.24+** required, uses modern Go features and patterns
- **PostgreSQL database** with custom `dbauthz` authorization layer
- **TypeScript/React frontend** with Vite build system
- **Terraform provider** for infrastructure management
- **OAuth2 server** implementation for authentication
- **Enterprise features** in `enterprise/` directory

## Essential Build & Development Commands

### Core Development Workflow

| Command | Purpose | Notes |
|---------|---------|-------|
| `./scripts/develop.sh` | **Primary development command** | ⚠️ Use this instead of manual builds |
| `make build` | Build fat binaries (includes frontend) | Default production build |
| `make build-slim` | Build slim binaries (no embedded frontend) | Faster for backend-only changes |
| `make test` | Run full Go test suite | Can be slow (~10+ minutes) |
| `make test RUN=TestName` | Run specific test | Much faster for targeted testing |
| `make test-postgres` | Run tests with PostgreSQL | Required for database changes |
| `make test-race` | Run with Go race detector | Critical for concurrency changes |
| `make lint` | **Always run after code changes** | Includes Go, TypeScript, shell, examples |
| `make fmt` | Format all code (Go, TS, shell, docs) | Auto-fixes formatting issues |
| `make gen` | **Required after database changes** | Regenerates DB code and audit tables |
| `make clean` | Clean build artifacts | Use when builds behave unexpectedly |

### Direct Go Build (Alternative)

If Makefile dependencies are missing, you can build directly:
```bash
go build -o build/coder cmd/coder/main.go
./build/coder --help  # Test the binary
```

### Frontend Development

```bash
cd site/
pnpm install    # Install dependencies
pnpm dev        # Development server
pnpm build      # Production build  
pnpm test       # Run frontend tests
pnpm check      # TypeScript and linting checks
pnpm format     # Format frontend code
```

### Database Development Pattern

**CRITICAL**: Always follow this sequence for database changes:

1. Modify `coderd/database/queries/*.sql` files
2. Run `make gen`
3. If audit errors occur: update `enterprise/audit/table.go`
4. Run `make gen` again
5. Test with `make test-postgres`

### Required Tools & Dependencies

- **Go 1.24+**: Check with `go version`
- **pnpm 10.14.0**: Frontend package manager, install with `npm install -g pnpm@10.14.0`
- **Node.js 20+**: JavaScript runtime
- **gotestsum**: Install with `go install gotest.tools/gotestsum@latest` and ensure `$HOME/go/bin` is in PATH
- **sqlc**: Required for database code generation, install from [sqlc.dev](https://sqlc.dev/)
- **PostgreSQL**: Use `make test-postgres-docker` for local testing
- **Terraform**: For template development and testing
- **golangci-lint**: Will be downloaded automatically by make lint

## Common Build Issues & Solutions

### Version Warnings
```
INFO(version.sh): It appears you've checked out a fork or shallow clone...
```
**Solution**: This is normal in forks and doesn't affect functionality.

### Frontend Build Failures
```
Error: write EPIPE ... pnpm install
```
**Solutions**: 
- Run `pnpm install` directly in `site/` directory first
- The error often occurs with verbose output being truncated - check if packages actually installed successfully
- Check Node.js version compatibility (requires 20+)
- Clear `site/node_modules` and reinstall if needed
- Use `pnpm install --silent` to reduce output volume

### Test Dependencies Missing
```
bash: gotestsum: command not found
```
**Solution**: Install and add to PATH:
```bash
go install gotest.tools/gotestsum@latest
export PATH=$PATH:$HOME/go/bin
```

### Database Code Generation Requires sqlc
```
./coderd/database/generate.sh: line 21: sqlc: command not found
```
**Solution**: Install sqlc from [sqlc.dev](https://sqlc.dev/)

### Database Code Generation Errors
- Always run `make gen` after modifying SQL files
- Update `enterprise/audit/table.go` if audit errors occur
- Use `make test-postgres` to validate database changes

## Project Layout & Key Locations

### Backend Services
- `cmd/coder/main.go` - Main CLI entry point
- `coderd/` - API server implementation
- `provisionerd/` - Infrastructure provisioning service  
- `agent/` - Workspace agent implementation
- `cli/` - Command-line interface implementation

### Database & Storage
- `coderd/database/` - Database layer and queries
- `coderd/database/queries/` - SQL query definitions  
- `coderd/database/migrations/` - Database schema migrations
- `enterprise/audit/table.go` - Audit log definitions

### Frontend Application
- `site/src/` - React/TypeScript source code
- `site/static/` - Static assets
- `site/package.json` - Frontend dependencies and scripts

### Templates & Examples  
- `examples/templates/` - Example Terraform templates
- `provisioner/` - Terraform provider integration
- `provisionersdk/` - SDK for custom provisioners

### Configuration & Testing
- `.golangci.yaml` - Go linting configuration
- `Makefile` - Primary build orchestration
- `scripts/` - Build and development scripts
- `testutil/` - Shared testing utilities

### Enterprise Features
- `enterprise/` - Premium features and functionality
- Requires proper license configuration for testing

## Validation Workflows

### Before Committing Changes

1. **Format code**: `make fmt`
2. **Run linters**: `make lint` 
3. **Test affected areas**: `make test RUN=TestRelevantFunction`
4. **Check database changes**: `make gen` (if applicable)
5. **Build verification**: `make build` or `make build-slim`

### GitHub CI Pipeline

The CI runs these checks automatically:
- Go linting with `golangci-lint`
- Frontend linting with Biome
- Shell script validation with `shellcheck`
- Full test suite on multiple platforms
- Database migration testing
- Example template validation

### Manual Testing Workflow

1. Start development server: `./scripts/develop.sh`
2. Access web UI at `http://localhost:3000`
3. Test CLI commands: `./build/coder_*_linux_amd64 --help`
4. Provision test workspace with Docker template

## Critical Development Patterns

### Most Common File Locations for Changes

- **CLI commands**: `cli/*.go` - add new CLI subcommands here
- **API endpoints**: `coderd/*.go` - REST API implementation  
- **Database queries**: `coderd/database/queries/*.sql` - SQL definitions
- **Frontend components**: `site/src/components/` - React components
- **Frontend pages**: `site/src/pages/` - Main application pages
- **Templates**: `examples/templates/` - Example infrastructure templates
- **Documentation**: `docs/` - User and admin documentation

### Authorization Context
```go
// System context for internal operations
app, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)

// User context for authenticated endpoints  
app, err := api.Database.GetOAuth2ProviderAppByClientID(ctx, clientID)
```

### OAuth2 Error Handling
```go
// Always use RFC-compliant error responses
writeOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_grant", "description")
```

### Test Naming for Concurrency
```go
// Use unique identifiers to prevent race conditions
name := fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())
```

### Timing Issues
- **Never use `time.Sleep`** to resolve timing issues
- Use [github.com/coder/quartz](https://github.com/coder/quartz) for proper time handling
- Leverage `testutil.Context` for test timeouts

## AI & Agent Features

Coder supports AI coding agents through **Coder Tasks** (beta):
- Tasks run in isolated Coder workspaces
- Support for Claude Code, Aider, and MCP-compatible agents
- Template modules available in [Coder Registry](https://registry.coder.com)
- Documentation in `docs/ai-coder/`

### Key Files at Repository Root

- `cmd/coder/main.go` - Main CLI application entry point
- `Makefile` - Primary build orchestration (40+ targets)
- `go.mod` - Go module definition (1,400+ Go files)
- `package.json` - Root package.json for documentation tooling
- `CLAUDE.md` - Existing development guidelines for AI agents
- `.golangci.yaml` - Go linting configuration
- `docker-compose.yaml` - Local development database setup

### Key Directories by Size & Importance

- `coderd/` - **Main API server** (~100 files, most changes happen here)
- `site/` - **Frontend application** (React/TypeScript, ~800 files)
- `cli/` - **Command-line interface** (~50 files)
- `agent/` - **Workspace agent services** (~30 files)
- `docs/` - **Documentation** (~200 markdown files)
- `examples/` - **Template examples** (Terraform/Docker templates)
- `enterprise/` - **Premium features** (requires license)
- `scripts/` - **Build and development scripts** (~50 shell scripts)

---

**Trust these instructions** and only search the codebase if information is incomplete or incorrect. This guide covers the essential patterns for productive development on the Coder platform.