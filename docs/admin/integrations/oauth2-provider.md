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

```sh
coder server --experiments oauth2
```

Or set the environment variable:

```dotenv
CODER_EXPERIMENTS=oauth2
```

## Creating OAuth2 Applications

### Method 1: Web UI

1. Navigate to **Deployment Settings** > **OAuth2 Applications**.
2. On the **Applications** tab, select **Add application**.
3. Fill in the application details:
   - **Name**: Your application name
   - **Callback URL**: `https://yourapp.example.com/callback` (web) or `myapp://callback` (native/desktop)
   - **Icon**: Optional icon URL

### Method 2: Management API

Create an application using the Coder API:

```sh
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

```sh
curl -X POST \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/oauth2-provider/apps/$APP_ID/secrets"
```

## Dynamic Client Registration

Dynamic Client Registration ([RFC 7591](https://datatracker.ietf.org/doc/html/rfc7591)) lets a client register itself against `/oauth2/register` instead of an admin creating the application manually. It's **disabled by default**; an owner must turn it on before any client can self-register.

Change the setting in the web UI:

1. Navigate to **Deployment Settings** > **OAuth2 Applications**.
2. Select the **Settings** tab.
3. Select **Enable** or **Disable** next to **Dynamic Client Registration**.

Enabling asks you to confirm first.
Disabling does not.
The tab is linkable directly at `https://$CODER_ACCESS_URL/deployment/oauth2-provider/apps?tab=settings`.

Viewing the tab requires permission to view deployment configuration, and changing the setting requires permission to edit it.
Without edit permission the button is present but inactive, and the page says why.

Check or change the setting with the CLI:

```sh
coder oauth2-provider dcr enable
coder oauth2-provider dcr disable
```

Or with the management API:

```sh
curl -X PUT \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dynamic_client_registration_enabled": true}' \
  "$CODER_URL/api/v2/oauth2-provider/settings"
```

```sh
curl -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/oauth2-provider/settings"
```

Disabling only blocks *new* self-registrations. Applications that already
registered while it was enabled keep authorizing and exchanging tokens
normally; disabling does not revoke or otherwise affect them.

## Integration Patterns

### Client Authentication Methods

Coder supports the following OAuth2 client authentication methods at the token endpoint (`/oauth2/tokens`):

- `client_secret_basic` (recommended): HTTP Basic authentication (RFC 6749 §2.3.1). The username is `client_id` and the password is `client_secret`.
- `client_secret_post`: Form-based authentication where `client_id` and `client_secret` are sent in the request body.

Coder supports both methods for compatibility; existing integrations using `client_secret_post` do not need to change.

If you use Dynamic Client Registration (RFC 7591) and omit `token_endpoint_auth_method`, clients default to `client_secret_basic`. To request `client_secret_post`, set `token_endpoint_auth_method` to `client_secret_post` in the registration request.

If client authentication fails, the token endpoint returns **HTTP 401** with an OAuth2 `invalid_client` error and a `WWW-Authenticate: Basic realm="coder"` response header.

### Standard OAuth2 Flow

1. **Authorization Request**: Redirect users to Coder's authorization endpoint:

   ```txt
   https://coder.example.com/oauth2/authorize?
     client_id=your-client-id&
     response_type=code&
     redirect_uri=https://yourapp.example.com/callback&
     state=random-string
   ```

2. **Token Exchange**: Exchange the authorization code for an access token.

   **Option A: HTTP Basic authentication (`client_secret_basic`, recommended)**

   ```sh
   curl -X POST \
     -u "$CLIENT_ID:$CLIENT_SECRET" \
     -H "Content-Type: application/x-www-form-urlencoded" \
     -d "grant_type=authorization_code" \
     -d "code=$AUTH_CODE" \
     -d "redirect_uri=https://yourapp.example.com/callback" \
     "$CODER_URL/oauth2/tokens"
   ```

   **Option B: Form parameters (`client_secret_post`)**

   ```sh
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

   ```sh
   curl -H "Authorization: Bearer $ACCESS_TOKEN" \
     "$CODER_URL/api/v2/users/me"
   ```

> [!NOTE]
> The PKCE flow below is the **required** integration path. The example
> above is shown for reference but omits the mandatory `code_challenge`
> parameter. See [PKCE Flow](#pkce-flow-required) for the complete flow.

### PKCE Flow (Required)

PKCE is **required** for all OAuth2 authorization code flows. Coder enforces
PKCE in compliance with the OAuth 2.1 specification. Both public and
confidential clients must include PKCE parameters:

> [!NOTE]
> `code_verifier` and `code_challenge` must each be 43-128 characters from
> the unreserved character set `[A-Za-z0-9-._~]` (RFC 7636 §4.1). A value
> outside these bounds is rejected with an `invalid_request` error, at the
> token endpoint for `code_verifier` and at the authorization endpoint for
> `code_challenge`.

1. Generate a code verifier and challenge:

   ```sh
   CODE_VERIFIER=$(openssl rand -base64 96 | tr -d '\n' | tr '+/' '-_' | tr -d '=')
   CODE_CHALLENGE=$(echo -n $CODE_VERIFIER | openssl dgst -sha256 -binary | base64 | tr -d "=" | tr '+/' '-_')
   ```

2. Include PKCE parameters in the authorization request:

   ```txt
   https://coder.example.com/oauth2/authorize?
     client_id=your-client-id&
     response_type=code&
     code_challenge=$CODE_CHALLENGE&
     code_challenge_method=S256&
     redirect_uri=https://yourapp.example.com/callback
   ```

3. Include the code verifier in the token exchange (see [Client Authentication Methods](#client-authentication-methods)):

   ```sh
   curl -X POST \
     -u "$CLIENT_ID:$CLIENT_SECRET" \
     -H "Content-Type: application/x-www-form-urlencoded" \
     -d "grant_type=authorization_code" \
     -d "code=$AUTH_CODE" \
     -d "code_verifier=$CODE_VERIFIER" \
     -d "redirect_uri=https://yourapp.example.com/callback" \
     "$CODER_URL/oauth2/tokens"
   ```

## Scopes

An access token is bounded by the scope negotiated when the user authorized it, on top of that user's own permissions. A token can never do more than its user can.

Scope names come from the same vocabulary as [API key scopes](../users/sessions-tokens.md#api-key-scopes): individual `resource:action` names such as `workspace:ssh`, and `coder:` composites such as `coder:workspaces.access` that stand for a set of them. `coder:all` records an unrestricted grant.

A client asks for a scope with the `scope` parameter on the authorization request, space separated:

```txt
https://coder.example.com/oauth2/authorize?
  client_id=your-client-id&
  response_type=code&
  scope=coder:workspaces.access&
  code_challenge=$CODE_CHALLENGE&
  code_challenge_method=S256&
  redirect_uri=https://yourapp.example.com/callback
```

An application registered through [Dynamic Client Registration](#dynamic-client-registration) can declare a `scope` field, which acts as an allowlist. The client may then request anything that allowlist covers, and is granted the whole allowlist if it requests nothing. Applications created through the web UI or the management API declare no allowlist, so any requested scope is honored and a request that names no scope is granted `coder:all`.

The consent page states the scope being granted before the user approves it, and refreshing a token keeps the scope originally granted.

## Discovery Endpoints

Coder provides OAuth2 discovery endpoints for programmatic integration:

- **Authorization Server Metadata**: `GET /.well-known/oauth-authorization-server`
- **Protected Resource Metadata**: `GET /.well-known/oauth-protected-resource`

These endpoints return server capabilities and endpoint URLs according to [RFC 8414](https://datatracker.ietf.org/doc/html/rfc8414) and [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728).

## Token Management

### Refresh Tokens

Refresh an expired access token.

**Option A: HTTP Basic authentication (`client_secret_basic`)**

```sh
curl -X POST \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token" \
  -d "refresh_token=$REFRESH_TOKEN" \
  "$CODER_URL/oauth2/tokens"
```

**Option B: Form parameters (`client_secret_post`)**

```sh
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

```sh
curl -X DELETE \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/oauth2/tokens?client_id=$CLIENT_ID"
```

This ends existing sessions but leaves the application registered, so it can authorize again.

### Delete an Application

Deleting an application is a separate operation from revoking its tokens.
It removes the registration itself, so the client cannot authorize again without being registered anew.

In the web UI, navigate to **Deployment Settings** > **OAuth2 Applications**, select the application on the **Applications** tab, then select **Delete**.
This requires permission to delete OAuth2 applications.

Or with the management API:

```sh
curl -X DELETE \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/oauth2-provider/apps/$APP_ID"
```

This is also how you remove clients that registered themselves while dynamic client registration was enabled.
Turning the setting off stops new registrations; it does not remove the ones already there.

## Testing and Development

Coder provides comprehensive test scripts for OAuth2 development:

```sh
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

### "Invalid Callback URL" on the consent page

If you see this error when authorizing, the registered callback URL uses a
blocked scheme (`javascript:`, `data:`, `file:`, or `ftp:`). Update the
application's callback URL to a valid scheme (see
[Callback URL schemes](#callback-url-schemes)).

### "PKCE verification failed"

Verify that the `code_verifier` used in the token request matches the one used to generate the `code_challenge`.

### "public clients may not use the mailto/tel/sms scheme"

This error appears during client registration when a public client
(`token_endpoint_auth_method: none`) registers a redirect URI using the
`mailto:`, `tel:`, or `sms:` scheme. These schemes hand off to a mail
client, dialer, or SMS app instead of returning control to the
application that started the flow, so a public client registered with
one of them could never complete authorization. Register a redirect URI
the client can actually receive control on instead, such as a custom
scheme (`myapp://callback`) or a loopback HTTP address.

## Callback URL schemes

Custom URI schemes (`myapp://`, `vscode://`, `jetbrains://`, etc.) are fully supported for native and desktop applications. The OS routes the redirect back to the registered application without requiring a running HTTP server.

The following schemes are blocked for security reasons: `javascript:`, `data:`, `file:`, `ftp:`.

Public clients (`token_endpoint_auth_method: none`) additionally cannot register `mailto:`, `tel:`, or `sms:` redirect URIs, since those schemes hand off to another app rather than returning an authorization code to the client. Confidential clients are not subject to this restriction.

## Security Considerations

- **Use HTTPS**: Always use HTTPS in production to protect tokens in transit
- **Implement PKCE**: PKCE is mandatory for all authorization code clients
  (public and confidential)
- **Validate redirect URLs**: Only register trusted redirect URIs. Dangerous
  schemes (`javascript:`, `data:`, `file:`, `ftp:`) are blocked by the server,
  custom URI schemes for native apps (`myapp://`) are permitted, and public
  clients additionally cannot use `mailto:`, `tel:`, or `sms:`
- **Rotate secrets**: Periodically rotate client secrets using the management API

## Limitations

As an experimental feature, the current implementation has limitations:

- A scope allowlist can only be declared at [Dynamic Client Registration](#dynamic-client-registration); applications created through the web UI or the management API cannot restrict which scopes a client may request
- A client cannot narrow the token's scope on refresh; the `scope` parameter is ignored and the refreshed token always keeps the scope originally granted
- No client credentials grant support
- Implicit grant (`response_type=token`) is not supported; OAuth 2.1
  deprecated this flow due to token leakage risks, and requests return
  `unsupported_response_type`
- Limited to opaque access tokens (no JWT support)

## Standards Compliance

This implementation follows established OAuth2 standards including
[RFC 6749](https://datatracker.ietf.org/doc/html/rfc6749) (OAuth2 core),
[RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636) (PKCE), and the
[OAuth 2.1 draft](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1-12).
Coder enforces OAuth 2.1 requirements including mandatory PKCE for all
authorization code grants, exact redirect URI string matching, rejection
of the implicit grant, and CSRF protections on consent pages.

## Next Steps

- Review the [API Reference](../../reference/api/index.md) for complete endpoint documentation
- Check [External Authentication](../external-auth/index.md) for configuring Coder as an OAuth2 client
- See [Security Best Practices](../security/index.md) for deployment security guidance

## Feedback

This is an experimental feature under active development. Please report issues and feedback through [GitHub Issues](https://github.com/coder/coder/issues) with the `oauth2` label.
