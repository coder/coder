# OAuth2 Development Guide

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

## OAuth2 Provider Implementation

### OAuth2 Spec Compliance

1. **Follow RFC 6749 for token responses**
   - Use `expires_in` (seconds) not `expiry` (timestamp) in token responses
   - Return proper OAuth2 error format: `{"error": "code", "error_description": "details"}`

2. **Error Response Format**
   - Create OAuth2-compliant error responses for token endpoint
   - Use standard error codes: `invalid_client`, `invalid_grant`, `invalid_request`
   - Avoid generic error responses for OAuth2 endpoints

### PKCE Implementation

- Support both with and without PKCE for backward compatibility
- Use S256 method for code challenge
- Properly validate code_verifier against stored code_challenge

### UI Authorization Flow

- Use POST requests for consent, not GET with links
- Avoid dependency on referer headers for security decisions
- Support proper state parameter validation

### RFC 8707 Resource Indicators

- Store resource parameters in database for server-side validation (opaque tokens)
- Validate resource consistency between authorization and token requests
- Support audience validation in refresh token flows
- Resource parameter is optional but must be consistent when provided

## OAuth2 Error Handling Pattern

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

## Testing OAuth2 Features

### Test Scripts

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

### RFC Protocol Testing

1. **Compliance Test Coverage**:
   - Test all RFC-defined error codes and responses
   - Validate proper HTTP status codes for different scenarios
   - Test protocol-specific edge cases (URI formats, token formats, etc.)

2. **Security Boundary Testing**:
   - Test client isolation and privilege separation
   - Verify information disclosure protections
   - Test token security and proper invalidation

## Common OAuth2 Issues

1. **OAuth2 endpoints returning wrong error format** - Ensure OAuth2 endpoints return RFC 6749 compliant errors
2. **Resource indicator validation failing** - Ensure database stores and retrieves resource parameters correctly
3. **PKCE tests failing** - Verify both authorization code storage and token exchange handle PKCE fields
4. **RFC compliance failures** - Verify against actual RFC specifications, not assumptions
5. **Authorization context errors in public endpoints** - Use `dbauthz.AsSystemRestricted(ctx)` pattern
6. **Default value mismatches** - Ensure database migrations match application code defaults
7. **Bearer token authentication issues** - Check token extraction precedence and format validation
8. **URI validation failures** - Support both standard schemes and custom schemes per protocol requirements

## Authorization Context Patterns

```go
// Public endpoints needing system access (OAuth2 registration)
app, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)

// Authenticated endpoints with user context
app, err := api.Database.GetOAuth2ProviderAppByClientID(ctx, clientID)

// System operations in middleware
roles, err := db.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), userID)
```

## OAuth2/Authentication Work Patterns

- Types go in `codersdk/oauth2.go` or similar
- Handlers go in `coderd/oauth2.go` or `coderd/identityprovider/`
- Database fields need migration + audit table updates
- Always support backward compatibility

## Protocol Implementation Checklist

Before completing OAuth2 or authentication feature work:

- [ ] Verify RFC compliance by reading actual specifications
- [ ] Implement proper error response formats per protocol
- [ ] Add comprehensive validation for all protocol fields
- [ ] Test security boundaries and token handling
- [ ] Update RBAC permissions for new resources
- [ ] Add audit logging support if applicable
- [ ] Create database migrations with proper defaults
- [ ] Add comprehensive test coverage including edge cases
- [ ] Verify linting compliance
- [ ] Test both positive and negative scenarios
- [ ] Document protocol-specific patterns and requirements
