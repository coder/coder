# Coder AI Documentation Index

This directory contains comprehensive development guidelines for AI assistants working with the Coder codebase.

## Documentation Structure

### Main Guide

- **`/CLAUDE.md`** - Primary entry point with essential commands, critical patterns, and quick reference
  - Auto-formatting hooks system
  - LSP/MCP server configuration
  - Essential build and test commands
  - Quick reference patterns
  - Architecture overview

### Detailed Guides

- **`WORKFLOWS.md`** - Development workflows and conventions
  - Development server setup
  - Code style guidelines (Go, TypeScript)
  - Database migration workflows
  - Commit conventions
  - LSP-first investigation strategy

- **`OAUTH2.md`** - OAuth2/OIDC implementation patterns
  - RFC compliance requirements
  - OAuth2 error handling
  - PKCE implementation
  - Resource indicators (RFC 8707)
  - Testing scripts and patterns

- **`TESTING.md`** - Testing best practices
  - Race condition prevention
  - Table-driven tests
  - RFC protocol testing
  - Test organization patterns
  - Benchmarking and load testing

- **`TROUBLESHOOTING.md`** - Common issues and solutions
  - Database issues
  - Migration problems
  - OAuth2 errors
  - Authorization context errors
  - Systematic debugging approach

- **`DATABASE.md`** - Database development patterns
  - Migration creation and management
  - Query organization
  - Nullable field handling
  - Audit table updates
  - Authorization patterns

### Frontend Documentation

- **`/site/CLAUDE.md`** - Frontend-specific guidelines
  - TypeScript LSP navigation
  - React component patterns
  - Tailwind CSS best practices
  - MUI → shadcn migration guide
  - Pre-PR checklist

## MCP Servers

Configured in `.mcp.json`:

- **go-language-server**: Go code navigation using gopls
- **typescript-language-server**: TypeScript/React navigation

## Auto-Formatting

Files are automatically formatted via `.claude/settings.json` hooks:

- Hook script: `.claude/scripts/format.sh`
- Triggers on Edit/Write operations
- Supports: Go, TypeScript, Terraform, Shell, Markdown

## Quick Navigation

### For Backend Work
1. Start with `CLAUDE.md` for quick reference
2. Check `WORKFLOWS.md` for development patterns
3. Refer to `DATABASE.md` for schema changes
4. Use `TROUBLESHOOTING.md` for common issues

### For Frontend Work
1. Read `/site/CLAUDE.md` first
2. Use TypeScript LSP tools for navigation
3. Follow Tailwind CSS patterns
4. Migrate away from MUI/Emotion

### For OAuth2/Auth Work
1. Read `OAUTH2.md` thoroughly
2. Verify RFC compliance requirements
3. Use provided test scripts
4. Check authorization context patterns

### For Testing
1. Check `TESTING.md` for patterns
2. Use unique identifiers for concurrent tests
3. Follow table-driven test structure
4. Run appropriate test commands

## Key Principles

1. **LSP First**: Always use LSP tools before manual searching
2. **Make gen**: Always run after database changes
3. **Systematic**: Follow workflows step-by-step
4. **RFC Compliant**: Verify against actual specifications
5. **Auto-format**: Files are auto-formatted on save

## Documentation Updates

This documentation is checked into the repository and should be updated when:

- New critical patterns emerge
- Development workflows change
- Common issues are identified
- Tools or infrastructure changes

Keep `CLAUDE.md` lean and actionable. Move detailed explanations to specific guide files.
