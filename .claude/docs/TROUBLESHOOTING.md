# Troubleshooting Guide

## Common Issues

### Database Issues

1. **"Audit table entry missing action"**
   - **Solution**: Update `enterprise/audit/table.go`
   - Add each new field with appropriate action (ActionTrack, ActionIgnore, ActionSecret)
   - Run `make gen` to verify no audit errors

2. **SQL type errors**
   - **Solution**: Use `sql.Null*` types for nullable fields
   - Set `.Valid = true` when providing values
   - Example:

     ```go
     CodeChallenge: sql.NullString{
         String: params.codeChallenge,
         Valid:  params.codeChallenge != "",
     }
     ```

### Testing Issues

3. **"package should be X_test"**
   - **Solution**: Use `package_test` naming for test files
   - Example: `identityprovider_test` for black-box testing

4. **Race conditions in tests**
   - **Solution**: Use unique identifiers instead of hardcoded names
   - Example: `fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())`
   - Never use hardcoded names in concurrent tests

5. **Missing newlines**
   - **Solution**: Ensure files end with newline character
   - Most editors can be configured to add this automatically

### OAuth2 Issues

6. **OAuth2 endpoints returning wrong error format**
   - **Solution**: Ensure OAuth2 endpoints return RFC 6749 compliant errors
   - Use standard error codes: `invalid_client`, `invalid_grant`, `invalid_request`
   - Format: `{"error": "code", "error_description": "details"}`

7. **Resource indicator validation failing**
   - **Solution**: Ensure database stores and retrieves resource parameters correctly
   - Check both authorization code storage and token exchange handling

8. **PKCE tests failing**
    - **Solution**: Verify both authorization code storage and token exchange handle PKCE fields
    - Check `CodeChallenge` and `CodeChallengeMethod` field handling

### RFC Compliance Issues

9. **RFC compliance failures**
    - **Solution**: Verify against actual RFC specifications, not assumptions
    - Use WebFetch tool to get current RFC content for compliance verification
    - Read the actual RFC specifications before implementation

10. **Default value mismatches**
    - **Solution**: Ensure database migrations match application code defaults
    - Example: RFC 7591 specifies `client_secret_basic` as default, not `client_secret_post`

### Authorization Issues

11. **Authorization context errors in public endpoints**
    - **Solution**: Use `dbauthz.AsSystemRestricted(ctx)` pattern
    - Example:

      ```go
      // Public endpoints needing system access
      app, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
      ```

### Authentication Issues

12. **Bearer token authentication issues**
    - **Solution**: Check token extraction precedence and format validation
    - Ensure proper RFC 6750 Bearer Token Support implementation

13. **URI validation failures**
    - **Solution**: Support both standard schemes and custom schemes per protocol requirements
    - Native OAuth2 apps may use custom schemes

### General Development Issues

14. **Log message formatting errors**
    - **Solution**: Use lowercase, descriptive messages without special characters
    - Follow Go logging conventions

## Systematic Debugging Approach

YOU MUST ALWAYS find the root cause of any issue you are debugging
YOU MUST NEVER fix a symptom or add a workaround instead of finding a root cause, even if it is faster.

### Multi-Issue Problem Solving

When facing multiple failing tests or complex integration issues:

1. **Identify Root Causes**:
   - Run failing tests individually to isolate issues
   - Use LSP tools to trace through call chains
   - Read Error Messages Carefully: Check both compilation and runtime errors
   - Reproduce Consistently: Ensure you can reliably reproduce the issue before investigating
   - Check Recent Changes: What changed that could have caused this? Git diff, recent commits, etc.
   - When You Don't Know: Say "I don't understand X" rather than pretending to know

2. **Fix in Logical Order**:
   - Address compilation issues first (imports, syntax)
   - Fix authorization and RBAC issues next
   - Resolve business logic and validation issues
   - Handle edge cases and race conditions last
   - IF your first fix doesn't work, STOP and re-analyze rather than adding more fixes

3. **Verification Strategy**:
   - Always Test each fix individually before moving to next issue
   - Verify Before Continuing: Did your test work? If not, form new hypothesis - don't add more fixes
   - Use `make lint` and `make gen` after database changes
   - Verify RFC compliance with actual specifications
   - Run comprehensive test suites before considering complete

## Debug Commands

### Useful Debug Commands

| Command                                      | Purpose                               |
|----------------------------------------------|---------------------------------------|
| `make lint`                                  | Run all linters                       |
| `make gen`                                   | Generate mocks, database queries      |
| `go test -v ./path/to/package -run TestName` | Run specific test with verbose output |
| `go test -race ./...`                        | Run tests with race detector          |

### LSP Debugging

#### Go LSP (Backend)

| Command                                            | Purpose                      |
|----------------------------------------------------|------------------------------|
| `mcp__go-language-server__definition symbolName`   | Find function definition     |
| `mcp__go-language-server__references symbolName`   | Find all references          |
| `mcp__go-language-server__diagnostics filePath`    | Check for compilation errors |
| `mcp__go-language-server__hover filePath line col` | Get type information         |

#### TypeScript LSP (Frontend)

| Command                                                                    | Purpose                            |
|----------------------------------------------------------------------------|------------------------------------|
| `mcp__typescript-language-server__definition symbolName`                   | Find component/function definition |
| `mcp__typescript-language-server__references symbolName`                   | Find all component/type usages     |
| `mcp__typescript-language-server__diagnostics filePath`                    | Check for TypeScript errors        |
| `mcp__typescript-language-server__hover filePath line col`                 | Get type information               |
| `mcp__typescript-language-server__rename_symbol filePath line col newName` | Rename across codebase             |

## Common Error Messages

### Database Errors

**Error**: `pq: relation "oauth2_provider_app_codes" does not exist`

- **Cause**: Missing database migration
- **Solution**: Run database migrations, check migration files

**Error**: `audit table entry missing action for field X`

- **Cause**: New field added without audit table update
- **Solution**: Update `enterprise/audit/table.go`

### Go Compilation Errors

**Error**: `package should be identityprovider_test`

- **Cause**: Test package naming convention violation
- **Solution**: Use `package_test` naming for black-box tests

**Error**: `cannot use X (type Y) as type Z`

- **Cause**: Type mismatch, often with nullable fields
- **Solution**: Use appropriate `sql.Null*` types

### OAuth2 Errors

**Error**: `invalid_client` but client exists

- **Cause**: Authorization context issue
- **Solution**: Use `dbauthz.AsSystemRestricted(ctx)` for public endpoints

**Error**: PKCE validation failing

- **Cause**: Missing PKCE fields in database operations
- **Solution**: Ensure `CodeChallenge` and `CodeChallengeMethod` are handled

## Prevention Strategies

### Before Making Changes

1. **Read the relevant documentation**
2. **Check if similar patterns exist in codebase**
3. **Understand the authorization context requirements**
4. **Plan database changes carefully**

### During Development

1. **Run tests frequently**: `make test`
2. **Use LSP tools for navigation**: Avoid manual searching
3. **Follow RFC specifications precisely**
4. **Update audit tables when adding database fields**

### Before Committing

1. **Run full test suite**: `make test`
2. **Check linting**: `make lint`
3. **Test with race detector**: `make test-race`

## Getting Help

### Internal Resources

- Check existing similar implementations in codebase
- Use LSP tools to understand code relationships
  - For Go code: Use `mcp__go-language-server__*` commands
  - For TypeScript/React code: Use `mcp__typescript-language-server__*` commands
- Read related test files for expected behavior

### External Resources

- Official RFC specifications for protocol compliance
- Go documentation for language features
- PostgreSQL documentation for database issues

### Debug Information Collection

When reporting issues, include:

1. **Exact error message**
2. **Steps to reproduce**
3. **Relevant code snippets**
4. **Test output (if applicable)**
5. **Environment information** (OS, Go version, etc.)
