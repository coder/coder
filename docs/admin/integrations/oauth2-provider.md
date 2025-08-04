# OAuth2 Provider (Experimental)

> [!WARNING]
> The OAuth2 provider functionality is currently **experimental and unstable**. This feature:
>
> - Is subject to breaking changes without notice
> - May have incomplete functionality
> - Is not recommended for production use
> - Requires the `oauth2` experiment flag to be enabled
>
> Use this feature for development and testing purposes only.

Coder can act as an OAuth2 authorization server, allowing third-party applications to authenticate users through Coder and access the Coder API on their behalf. This enables integrations where external applications can leverage Coder's authentication and user management.

## Requirements

- Admin privileges in Coder
- OAuth2 experiment flag enabled
- HTTPS recommended for production deployments

## Enable OAuth2 Provider

Add the `oauth2` experiment flag to your Coder server:

```bash
coder server --experiments oauth2
```

Or set the environment variable:

```env
CODER_EXPERIMENTS=oauth2
```

## Creating OAuth2 Applications

### Method 1: Web UI

1. Navigate to **Deployment Settings** → **OAuth2 Applications**
2. Click **Create Application**
3. Fill in the application details:
   - **Name**: Your application name
   - **Callback URL**: `https://yourapp.example.com/callback`
   - **Icon**: Optional icon URL

### Method 2: Management API

Create an application using the Coder API:

```bash
curl -X POST \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Application",
    "callback_url": "https://myapp.example.com/callback",
    "icon": "https://myapp.example.com/icon.png"
  }' \
  "$CODER_URL/api/v2/oauth2-provider/apps"
```

Generate a client secret:

```bash
curl -X POST \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/oauth2-provider/apps/$APP_ID/secrets"
```

## Integration Patterns

### Standard OAuth2 Flow

1. **Authorization Request**: Redirect users to Coder's authorization endpoint:

   ```url
   https://coder.example.com/oauth2/authorize?
     client_id=your-client-id&
     response_type=code&
     redirect_uri=https://yourapp.example.com/callback&
     state=random-string
   ```

2. **Token Exchange**: Exchange the authorization code for an access token:

   ```bash
   curl -X POST \
     -H "Content-Type: application/x-www-form-urlencoded" \
     -d "grant_type=authorization_code" \
     -d "code=$AUTH_CODE" \
     -d "client_id=$CLIENT_ID" \
     -d "client_secret=$CLIENT_SECRET" \
     -d "redirect_uri=https://yourapp.example.com/callback" \
     "$CODER_URL/oauth2/tokens"
   ```

3. **API Access**: Use the access token to call Coder's API:

   ```bash
   curl -H "Authorization: Bearer $ACCESS_TOKEN" \
     "$CODER_URL/api/v2/users/me"
   ```

### PKCE Flow (Public Clients)

For mobile apps and single-page applications, use PKCE for enhanced security:

1. Generate a code verifier and challenge:

   ```bash
   CODE_VERIFIER=$(openssl rand -base64 96 | tr -d "=+/" | cut -c1-128)
   CODE_CHALLENGE=$(echo -n $CODE_VERIFIER | openssl dgst -sha256 -binary | base64 | tr -d "=+/" | cut -c1-43)
   ```

2. Include PKCE parameters in the authorization request:

   ```url
   https://coder.example.com/oauth2/authorize?
     client_id=your-client-id&
     response_type=code&
     code_challenge=$CODE_CHALLENGE&
     code_challenge_method=S256&
     redirect_uri=https://yourapp.example.com/callback
   ```

3. Include the code verifier in the token exchange:

   ```bash
   curl -X POST \
     -d "grant_type=authorization_code" \
     -d "code=$AUTH_CODE" \
     -d "client_id=$CLIENT_ID" \
     -d "code_verifier=$CODE_VERIFIER" \
     "$CODER_URL/oauth2/tokens"
   ```

## Discovery Endpoints

Coder provides OAuth2 discovery endpoints for programmatic integration:

- **Authorization Server Metadata**: `GET /.well-known/oauth-authorization-server`
- **Protected Resource Metadata**: `GET /.well-known/oauth-protected-resource`

These endpoints return server capabilities and endpoint URLs according to [RFC 8414](https://datatracker.ietf.org/doc/html/rfc8414) and [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728).

## Token Management

### Refresh Tokens

Refresh an expired access token:

```bash
curl -X POST \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token" \
  -d "refresh_token=$REFRESH_TOKEN" \
  -d "client_id=$CLIENT_ID" \
  -d "client_secret=$CLIENT_SECRET" \
  "$CODER_URL/oauth2/tokens"
```

### Revoke Access

Revoke all tokens for an application:

```bash
curl -X DELETE \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/oauth2/tokens?client_id=$CLIENT_ID"
```

## Testing and Development

Coder provides comprehensive test scripts for OAuth2 development:

```bash
# Navigate to the OAuth2 test scripts
cd scripts/oauth2/

# Run the full automated test suite
./test-mcp-oauth2.sh

# Create a test application for manual testing
eval $(./setup-test-app.sh)

# Run an interactive browser-based test
./test-manual-flow.sh

# Clean up when done
./cleanup-test-app.sh
```

For more details on testing, see the [OAuth2 test scripts README](../../../scripts/oauth2/README.md).

## Common Issues

### "OAuth2 experiment not enabled"

Add `oauth2` to your experiment flags: `coder server --experiments oauth2`

### "Invalid redirect_uri"

Ensure the redirect URI in your request exactly matches the one registered for your application.

### "PKCE verification failed"

Verify that the `code_verifier` used in the token request matches the one used to generate the `code_challenge`.

## Security Considerations

- **Use HTTPS**: Always use HTTPS in production to protect tokens in transit
- **Implement PKCE**: Use PKCE for all public clients (mobile apps, SPAs)
- **Validate redirect URLs**: Only register trusted redirect URIs for your applications
- **Rotate secrets**: Periodically rotate client secrets using the management API

## Limitations

As an experimental feature, the current implementation has limitations:

- No scope system - all tokens have full API access
- No client credentials grant support
- Limited to opaque access tokens (no JWT support)

## Standards Compliance

This implementation follows established OAuth2 standards including [RFC 6749](https://datatracker.ietf.org/doc/html/rfc6749) (OAuth2 core), [RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636) (PKCE), and related specifications for discovery and client registration.

## Next Steps

- Review the [API Reference](../../reference/api/index.md) for complete endpoint documentation
- Check [External Authentication](../external-auth/index.md) for configuring Coder as an OAuth2 client
- See [Security Best Practices](../security/index.md) for deployment security guidance

## Feedback

This is an experimental feature under active development. Please report issues and feedback through [GitHub Issues](https://github.com/coder/coder/issues) with the `oauth2` label.
