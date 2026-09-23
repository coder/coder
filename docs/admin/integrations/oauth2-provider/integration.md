---
title: OAuth2 provider integration reference
---

This page is the client-integration reference for Coder's OAuth2 provider: the
supported client authentication methods, the authorization code and PKCE
flows, scopes, discovery endpoints, token lifecycle operations, and which
redirect URI schemes are accepted. For enabling the provider and creating an
application, see [OAuth2 provider](./index.md).

## Integration Patterns

### Client Authentication Methods

Coder supports the following OAuth2 client authentication methods at the token endpoint (`/oauth2/tokens`):

- `client_secret_basic` (recommended): HTTP Basic authentication (RFC 6749 §2.3.1). The username is `client_id` and the password is `client_secret`.
- `client_secret_post`: Form-based authentication where `client_id` and `client_secret` are sent in the request body.
- `none`: No client secret. The client is a public client and authenticates with PKCE alone (RFC 7591 §2, OAuth 2.1 §2.1). Available only through [Dynamic Client Registration](./index.md#dynamic-client-registration), which is disabled by default, since a client's type is set when it registers and apps created through the admin UI or API are always confidential.

Coder supports both secret-based methods for compatibility; existing integrations using `client_secret_post` do not need to change.

Send `client_secret` in the request body or in the `Authorization` header. `POST /oauth2/tokens` and `POST /oauth2/revoke` reject a `client_secret` value in the URL query string with `invalid_request`, because OAuth 2.1 section 2.4.1 does not allow it there. Refer to ["invalid_request" for `client_secret` in the query string](./troubleshooting.md#invalid_request-for-client_secret-in-the-query-string) for the exceptions and the log line to search for.

Public clients suit native, mobile, and CLI applications that cannot keep a secret confidential. Note the redirect URI restrictions below before choosing one.

Opening a public client on the **OAuth2 Applications** page shows no client secrets section, since a public client has no secret to display or generate.

If you use Dynamic Client Registration (RFC 7591) and omit `token_endpoint_auth_method`, clients default to `client_secret_basic`. To request `client_secret_post`, set `token_endpoint_auth_method` to `client_secret_post` in the registration request. To register a public client, set it to `none`: Coder issues no `client_secret`, and the registration response omits that field entirely.

> [!IMPORTANT]
> A public client may use `http://` only with a loopback host (`localhost`, `127.0.0.1`, `[::1]`).
> An `http://` redirect URI to any other host is rejected, so use `https://` instead.
> A confidential client has the same restriction but also accepts `.localhost` subdomains over `http://`.
> Coder ignores the port of an `http://` redirect URI to one of those three loopback hosts, for public and confidential clients alike. RFC 8252 requires this for `127.0.0.1` and `[::1]` so that native apps can choose a port at runtime. Coder applies it to `localhost` too. A `.localhost` subdomain still requires an exact port match.
> Register `http://127.0.0.1/callback` and present whichever port the client is listening on.
>
> Which schemes a redirect URI may use is a separate restriction that
> also differs by client type. See
> [Callback URL schemes](#callback-url-schemes).

A client's type is fixed when it registers.
An RFC 7592 update that would move a client between public and confidential is rejected with `invalid_client_metadata`, since the client either holds a secret that would stop being required or has none and no way to be issued one.
Switching between `client_secret_basic` and `client_secret_post` is allowed, because both are confidential.
To change type, register a new client.

Clients registered with `token_endpoint_auth_method: none` before Coder honored it are stored as confidential and still require their `client_secret`.
Coder reports `client_secret_basic` for those clients so that what it reports matches what it enforces, and the mismatch clears the next time the client updates its registration using the value Coder reported.

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

   **Confidential client**

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

   **Public client (`token_endpoint_auth_method: none`)**

   Send `client_id` in the form body and omit `client_secret` entirely. The code
   verifier is the only proof of possession, and must satisfy RFC 7636 §4.1
   (43-128 characters from `[A-Za-z0-9-._~]`).

   ```sh
   curl -X POST \
     -H "Content-Type: application/x-www-form-urlencoded" \
     -d "grant_type=authorization_code" \
     -d "code=$AUTH_CODE" \
     -d "client_id=$CLIENT_ID" \
     -d "code_verifier=$CODE_VERIFIER" \
     -d "redirect_uri=https://yourapp.example.com/callback" \
     "$CODER_URL/oauth2/tokens"
   ```

## Scopes

An access token is bounded by the scope negotiated when the user authorized it, on top of that user's own permissions. A token can never do more than its user can.

Scope names come from the same vocabulary as [API key scopes](../../users/sessions-tokens.md#api-key-scopes): individual `resource:action` names such as `workspace:ssh`, and `coder:` composites such as `coder:workspaces.access` that stand for a set of them. `coder:all` records an unrestricted grant.

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

An application can carry a `scope` allowlist.
The client may then request anything that allowlist covers, and is granted the whole allowlist if it requests nothing.
An application with no allowlist honors any requested scope, and a request that names no scope is granted `coder:all`.

An application registered through [Dynamic Client Registration](./index.md#dynamic-client-registration) declares its allowlist in the `scope` field of its registration.
An administrator sets one with the optional, space-separated `scope` field when [creating an application](../../../reference/api/enterprise.md#create-oauth2-application) through the management API.
When [updating an application](../../../reference/api/enterprise.md#update-oauth2-application), omit `scope` to keep the current allowlist, send a new value to replace it, or send an empty string to clear it and make the application unrestricted.
The stored value is not checked against the scopes this deployment offers; a name it does not offer fails at authorization, as described under ["invalid_scope" returned to your callback](./troubleshooting.md#invalid_scope-returned-to-your-callback).
The web UI does not yet set the allowlist.

The consent page states the scope being granted before the user approves it. A refresh keeps the scope originally granted; a refresh that names a narrower `scope` applies it to the access token it mints, leaving the grant itself unchanged.

## Discovery Endpoints

Coder provides OAuth2 discovery endpoints for programmatic integration:

- **Authorization Server Metadata**: `GET /.well-known/oauth-authorization-server`
- **Protected Resource Metadata**: `GET /.well-known/oauth-protected-resource`

These endpoints return server capabilities and endpoint URLs according to [RFC 8414](https://datatracker.ietf.org/doc/html/rfc8414) and [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728).

`token_endpoint_auth_methods_supported` lists every method the token endpoint accepts, including `none`. It is not gated on [Dynamic Client Registration](./index.md#dynamic-client-registration), since existing public clients still exchange tokens when new registrations are disabled. `registration_endpoint` is advertised only while Dynamic Client Registration is enabled, so that field, not this one, tells a client whether it can register a new public client.

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

**Option C: Public client (`none`)**

```sh
curl -X POST \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token" \
  -d "refresh_token=$REFRESH_TOKEN" \
  -d "client_id=$CLIENT_ID" \
  "$CODER_URL/oauth2/tokens"
```

### Revoke a Token

Revoke one refresh token or access token through the
[RFC 7009](https://datatracker.ietf.org/doc/html/rfc7009) endpoint that
`revocation_endpoint` advertises. A confidential client authenticates as it
does on a refresh, with HTTP Basic as below or with `client_id` and
`client_secret` form fields as in the refresh examples above. An omitted or
wrong secret answers HTTP 401 with `error=invalid_client`:

```sh
curl -X POST \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=$REFRESH_TOKEN" \
  "$CODER_URL/oauth2/revoke"
```

A public client sends `client_id` alone. Revoking a refresh token also ends the
access token issued with it. A successful revocation returns HTTP 200, but that
response does not confirm that the token existed or belonged to your client. A
confidential client that fails to authenticate receives HTTP 401 with
`error=invalid_client` and nothing is revoked.

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

## Callback URL schemes

Custom URI schemes (`myapp://`, `vscode://`, `jetbrains://`, etc.) are fully supported for native and desktop applications. The OS routes the redirect back to the registered application without requiring a running HTTP server.

The out-of-band URN `urn:ietf:wg:oauth:2.0:oob` is accepted from either client type, for clients that display the authorization code for the user to copy rather than receiving it on a redirect. No other URN is accepted.

The following schemes are blocked for security reasons: `javascript:`, `data:`, `file:`, `ftp:`.

Public clients (`token_endpoint_auth_method: none`) additionally cannot register `mailto:`, `tel:`, or `sms:` redirect URIs, since those schemes hand off to another app rather than returning an authorization code to the client. Confidential clients are not subject to this restriction.

A cleartext `http://` redirect URI is accepted only for a local host. A confidential client may use `localhost`, `127.0.0.1`, `::1`, or a `.localhost` subdomain such as `http://app.localhost/callback`. A public client is limited to `localhost`, `127.0.0.1`, and `::1`. Every other host must use `https://`, so that an authorization code is never delivered in the clear. The management API and Dynamic Client Registration apply the same rule, so an administrator cannot store a target that a client could not register for itself.

These rules apply to every entry in `redirect_uris`, not only the first one.

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

For more details on testing, see the [OAuth2 test scripts README](../../../../scripts/oauth2/README.md).

## Standards Compliance

This implementation follows established OAuth2 standards including
[RFC 6749](https://datatracker.ietf.org/doc/html/rfc6749) (OAuth2 core),
[RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636) (PKCE), and the
[OAuth 2.1 draft](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1-16).
Coder enforces OAuth 2.1 requirements including mandatory PKCE for all
authorization code grants, exact redirect URI string matching with the
[RFC 8252](https://datatracker.ietf.org/doc/html/rfc8252) loopback port
exception, rejection of the implicit grant, and CSRF protections on consent
pages.

## Next Steps

- Review [Common issues](./troubleshooting.md) if a request fails
- Review [Security considerations and limitations](./security.md)
- Review the [API Reference](../../../reference/api/index.md) for complete endpoint documentation
