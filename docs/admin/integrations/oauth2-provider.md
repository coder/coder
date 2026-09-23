---
title: OAuth2 provider
---

> [!NOTE]
> The OAuth2 provider is generally available and off by default.
> Set `CODER_OAUTH2_PROVIDER_ENABLE=true` to turn it on.
> The `oauth2` experiment has been removed.

Coder can act as an OAuth2 authorization server, allowing third-party applications to authenticate users through Coder and access the Coder API on their behalf. This enables integrations where external applications can leverage Coder's authentication and user management.

## Requirements

- Admin privileges in Coder
- `CODER_OAUTH2_PROVIDER_ENABLE=true` set on the control plane
- HTTPS recommended for production deployments

## Enable OAuth2 Provider

The provider is off by default.
While it is off, the OAuth2 endpoints and discovery documents return 404 and the **OAuth2 Applications** page is hidden.
Turn it on with the CLI flag:

```sh
coder server --oauth2-provider-enable
```

Or set the environment variable:

```dotenv
CODER_OAUTH2_PROVIDER_ENABLE=true
```

Or set it in the YAML configuration file:

```yaml
oauth2:
  provider:
    enable: true
```

For Kubernetes deployments that use the Helm chart, add the environment variable to `coder.env` in your values file:

```yaml
coder:
  env:
    - name: CODER_OAUTH2_PROVIDER_ENABLE
      value: "true"
```

Existing applications, secrets, and user authorizations are kept while the provider is off and work again when you turn it on.
Turning the provider off does not invalidate access tokens it already issued.
Those tokens keep authenticating to the regular Coder API while the OAuth2 refresh and revocation endpoints return 404.
Treat the setting as a way to stop new authorizations rather than as a way to revoke access, and revoke the tokens or delete the application before you disable the provider.

## Creating OAuth2 Applications

### Method 1: Web UI

1. Navigate to **Deployment Settings** > **OAuth2 Applications**.
2. On the **Applications** tab, select **Add application**.
3. Fill in the application details:
   - **Name**: Your application name
   - **Callback URL**: `https://yourapp.example.com/callback` (web) or `myapp://callback` (native/desktop)
   - **Icon**: Optional icon URL
   - **Allowed scopes**: Optional. Refer to [Scopes](#scopes).

### Method 2: Management API

Create an application using the Coder API:

```sh
curl -X POST \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Application",
    "redirect_uris": [
      "https://myapp.example.com/callback",
      "http://localhost:8080/callback"
    ],
    "icon": "https://myapp.example.com/icon.png"
  }' \
  "$CODER_URL/api/v2/oauth2-provider/apps"
```

`callback_url` is still accepted and still returned, but it is deprecated: it is equal to the first entry in `redirect_uris`. New scripts should send and read `redirect_uris` instead.

Update an application with `PUT`. Fetch it first and edit the fields you want to change, then send the result back:

```sh
curl -X PUT \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Application",
    "redirect_uris": ["https://myapp.example.com/callback"],
    "icon": "https://myapp.example.com/icon.png"
  }' \
  "$CODER_URL/api/v2/oauth2-provider/apps/$APP_ID"
```

`name` is required on every `PUT`, and `icon` is cleared if you leave it out.
`redirect_uris` replaces the stored list when present and keeps it when omitted.
`scope` is kept when omitted; refer to [Scopes](#scopes) for how to change it.

Add an optional `scope` field to restrict which scopes the application's clients may request.
Refer to [Scopes](#scopes) for how the allowlist is applied and how to change it later.

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

An application may list several `redirect_uris`, whether it registered itself or an admin created it.
A request may present any of them, and the code it receives can only be exchanged with that same URI.
The first entry is the primary callback: it is what the web UI shows for the application, and what a request that omits `redirect_uri` is sent to.
An admin can edit the list through the management API, and a self-registered client can update its own list with its registration access token.
The admin `PUT` also validates the stored name, so a self-registered client whose name has leading or trailing whitespace can only be updated with its registration access token.

## Integration Patterns

### Client Authentication Methods

Coder supports the following OAuth2 client authentication methods at the token endpoint (`/oauth2/tokens`):

- `client_secret_basic` (recommended): HTTP Basic authentication (RFC 6749 §2.3.1). The username is `client_id` and the password is `client_secret`.
- `client_secret_post`: Form-based authentication where `client_id` and `client_secret` are sent in the request body.
- `none`: No client secret. The client is a public client and authenticates with PKCE alone (RFC 7591 §2, OAuth 2.1 §2.1). Available only through [Dynamic Client Registration](#dynamic-client-registration), which is disabled by default, since a client's type is set when it registers and apps created through the admin UI or API are always confidential.

Coder supports both secret-based methods for compatibility; existing integrations using `client_secret_post` do not need to change.

Send `client_secret` in the request body or in the `Authorization` header. `POST /oauth2/tokens` and `POST /oauth2/revoke` reject a `client_secret` value in the URL query string with `invalid_request`, because OAuth 2.1 section 2.4.1 does not allow it there. Refer to ["invalid_request" for `client_secret` in the query string](#invalid_request-for-client_secret-in-the-query-string) for the exceptions and the log line to search for.

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

An application can carry a `scope` allowlist.
The client may then request anything that allowlist covers, and is granted the whole allowlist if it requests nothing.
An application with no allowlist honors any requested scope, and a request that names no scope is granted `coder:all`.

An application registered through [Dynamic Client Registration](#dynamic-client-registration) declares its allowlist in the `scope` field of its registration.
An administrator sets one with the **Allowed scopes** field in the web UI, or with the optional, space-separated `scope` field when [creating an application](../../reference/api/enterprise.md#create-oauth2-application) through the management API.
When [updating an application](../../reference/api/enterprise.md#update-oauth2-application), omit `scope` to keep the current allowlist, send a new value to replace it, or send an empty string to clear it and make the application unrestricted.
The stored value is not checked against the scopes this deployment offers; a name it does not offer fails at authorization, as described under ["invalid_scope" returned to your callback](#invalid_scope-returned-to-your-callback).

Use caution when narrowing the scope allowlist of a self-registered application.
Many clients request every scope Coder advertises in `scopes_supported` rather than selecting specific scopes; MCP clients that rely on discovery commonly work this way.
The allowlist is stored on the application and does not affect that advertised list, so the client continues requesting the full set.
Coder rejects requests that exceed the allowlist rather than trimming their scopes, so every new authorization attempt fails with `invalid_scope`.
Previously issued tokens retain their scopes.
Such a client cannot be restricted through its allowlist: narrowing it breaks the client, and clearing it leaves the application unrestricted.
Only a client that can be configured to request fewer scopes can be narrowed, and whoever operates that client makes the change.
An allowlist set by an administrator also does not hold against the client: the holder of the application's `registration_access_token` can replace or clear it at any time with `PUT /oauth2/clients/{client_id}`.
The web UI warns before saving a narrower allowlist, and the application page indicates whether the application was self-registered or created by an administrator.

The consent page states the scope being granted before the user approves it. A refresh keeps the scope originally granted; a refresh that names a narrower `scope` applies it to the access token it mints, leaving the grant itself unchanged.

## Discovery Endpoints

Coder provides OAuth2 discovery endpoints for programmatic integration:

- **Authorization Server Metadata**: `GET /.well-known/oauth-authorization-server`
- **Protected Resource Metadata**: `GET /.well-known/oauth-protected-resource`

These endpoints return server capabilities and endpoint URLs according to [RFC 8414](https://datatracker.ietf.org/doc/html/rfc8414) and [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728).

`token_endpoint_auth_methods_supported` lists every method the token endpoint accepts, including `none`. It is not gated on [Dynamic Client Registration](#dynamic-client-registration), since existing public clients still exchange tokens when new registrations are disabled. `registration_endpoint` is advertised only while Dynamic Client Registration is enabled, so that field, not this one, tells a client whether it can register a new public client.

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

### OAuth2 endpoints return 404

The provider is off.
Set `CODER_OAUTH2_PROVIDER_ENABLE=true` and restart the server.
Refer to [Enable OAuth2 Provider](#enable-oauth2-provider).

### "Invalid redirect_uri"

Ensure the redirect URI in your request exactly matches one of the redirect URIs registered for your application.
The one exception is the port of a loopback `http://` redirect URI (`localhost`, `127.0.0.1`, `[::1]`), which may differ from the registered one.
Refer to the note under [Client Authentication Methods](#client-authentication-methods).

### "Invalid Callback URL" on the consent page

If you see this error when authorizing, one of the application's registered
redirect URIs is not usable: either it does not parse as a URL, or it uses a
blocked scheme (`javascript:`, `data:`, `file:`, or `ftp:`). The same cause
answers `server_error` on `POST /oauth2/authorize`. Use
`GET /api/v2/oauth2-provider/apps/{app}` to see every registered redirect
URI, then update the application with a corrected `redirect_uris` list as
shown under [Management API](#method-2-management-api). Refer to
[Callback URL schemes](#callback-url-schemes) for which values are accepted.

The `coderd` log records the application ID and the stored value. The response
does not, so a bad URL is never echoed back to a browser.

### "invalid_scope" returned to your callback

The authorization endpoint validates the `scope` parameter. When it cannot
grant what was asked for, it redirects to your registered callback with
`error=invalid_scope` rather than issuing a code. The `error_description`
opens with the requested name that caused the rejection:

- `unknown or unsupported scope`: this deployment does not offer that name to
  OAuth2 clients. It may not exist, or it may exist and be internal-only, which
  no version offers. Read the current list from `scopes_supported` in
  `GET /.well-known/oauth-authorization-server`.
- `scope requests permissions beyond this app's allowed scopes`: the name is supported, but the application's `scope` allowlist does not cover it.
  Request less, or widen the allowlist.
  If the application registered itself and an administrator has since narrowed its allowlist, refer to [Scopes](#scopes).
- `none of the scopes registered for this app are supported by this deployment`: the application's `scope` allowlist names nothing this deployment offers, so no request against it can succeed, including one that omits `scope`.
  Update the allowlist with supported scopes.
  This description stands alone.
  Nothing checks a stored `scope` against the catalog, so the response never echoes it; the `coderd` log records the application ID.

Omitting `scope` requests the application's allowlist, or full access if it has none.

The negotiated scope is recorded on the authorization, shown on the consent
page, and applied to the access token issued when the code is exchanged.

The token endpoint validates a refresh request's `scope` too, and answers
`invalid_scope` in the response body rather than by redirect. See
["invalid_scope" for a refresh that names a scope](#invalid_scope-for-a-refresh-that-names-a-scope).

### "invalid_grant" for a scope the deployment cannot mint

`POST /oauth2/tokens` mints the access token with the scope recorded on the
authorization code, or on the refresh token when refreshing. If that stored
scope names something this deployment cannot mint, the exchange answers HTTP
400 with `error=invalid_grant` and an `error_description` naming the value.

The usual cause is a grant made against a scope the deployment has since
dropped. Authorize again to negotiate a scope it still supports; the stored
scope is not something the client can change by requesting a different one.

The exchange also re-checks the code's scope against the application's
registered `scope`, which can change during the ten minutes a code stays valid.
Two more descriptions can open the `error_description` here:

- `scope is no longer allowed by this app's registered scopes`: the allowlist narrowed after the code was issued and no longer covers the code's scope.
  Authorize again to negotiate a scope within the new allowlist.
- `none of the scopes registered for this app are supported by this deployment`: the allowlist names nothing this deployment offers, so no code against it can be redeemed.
  Update the allowlist with supported scopes.
  As on the authorize endpoint, the stored value stays out of the response.

A coverage comparison this deployment cannot decide answers HTTP 500 with
`error=server_error` and `The requested scope could not be evaluated`; the
scope that could not be compared is in the `coderd` logs, not the response.

An application's `scope` allowlist can change through [Dynamic Client Registration](#dynamic-client-registration), by the application itself, or through the management API, by an administrator.
An application that holds its registration access token can widen its own allowlist again before redeeming a code, so treat this re-check as reflecting the allowlist at redemption time rather than as a constraint on the client.

A refresh is not re-checked against the registration. That is a Coder policy
choice: withdrawing scope from a session already running would break it
mid-flight, so a narrowing takes effect at the next authorization. A refresh
token keeps its granted scope until it expires, which can be up to the
configured refresh lifetime; revoke the token to cut a live session.

Codes issued before the upgrade that added scope columns carry `coder:all`, recorded as an unrestricted grant.
For an application with a narrower `scope` allowlist, those codes are refused with `scope is no longer allowed by this app's registered scopes` until they expire, which takes at most ten minutes.
Authorizing again issues a code within the current allowlist.

### "invalid_scope" for a refresh that names a scope

`POST /oauth2/tokens` answers HTTP 400 with `error=invalid_scope` when a refresh
request names a `scope` the control plane will not grant. This is the token endpoint,
not the authorization endpoint above: there is no redirect, and the error is in
the response body.

A refresh may name a `scope` of its own to give up authority. The narrowing
applies to the access token that refresh mints, and to nothing else. The
refresh token continues to represent the scope the user consented to, so the
ceiling does not move and a later refresh may ask for a different part of the
same grant, or omit `scope` to take the grant whole again.

The request may name any scope the original grant confers **that also appears in
`scopes_supported`**, including a single permission out of a composite scope, so
a token granted `coder:workspaces.access` can refresh down to `workspace:read`
for one call and to `workspace:ssh` for the next. Two descriptions can open the
`error_description`, each opening with the requested name that caused it:

- `scope requests permissions beyond the scope originally granted; a refresh
  cannot widen a grant, so authorize again to obtain a broader one`: the name is
  offered, but the resource owner never granted it.
- `unknown or unsupported scope`: this deployment does not offer that name to
  OAuth2 clients, either because it does not exist or because it is internal.

A refused refresh mints nothing and leaves the refresh token usable, so a client
that asked for too much can retry with less rather than re-authorizing.

Only the resource owner lowers the ceiling, by revoking the token or authorizing
again with less. This is also what OAuth 2.1 section 4.3.3 requires: a rotated
refresh token carries the scope of the one presented.

Narrowing a composite scope to the low-level names you can request may drop
permissions that have no requestable name of their own. `coder:workspaces.create`
confers `organization_member:read`, which a workspace build needs and which
`scopes_supported` does not list, so a token narrowed to the fullest set a client
can name will fail to create a workspace. Refresh without a `scope` to return to
the composite.

### "invalid_client" for a refresh or a revocation

`POST /oauth2/tokens` with `grant_type=refresh_token` and `POST /oauth2/revoke`
answer HTTP 401 with `error=invalid_client` when a confidential client does not
authenticate. The usual causes are a `client_secret` that was omitted, a secret
that belongs to a different client, or a secret that has since been deleted or
rotated. Present the client's current secret, as HTTP Basic or as a form
parameter, following [Refresh Tokens](#refresh-tokens). The refresh token is
not consumed and nothing is revoked by the refusal, so the retry needs no new
authorization. If the secret was deleted, the tokens issued under it were
revoked with it, and the client must authorize again. Public clients have no
secret and never receive this error for omitting one.

### "invalid_request" for `client_secret` in the query string

`POST /oauth2/tokens` and `POST /oauth2/revoke` answer HTTP 400 with
`error=invalid_request` when `client_secret` appears in the URL query string.
OAuth 2.1 section 2.4.1 allows the secret in the request body or the
`Authorization` header only. Send it as a form parameter or as HTTP Basic,
following [Client Authentication Methods](#client-authentication-methods).

The rule covers `client_secret` only. Coder still reads `refresh_token`,
`code`, and the revocation `token` from the query string. Send those in the
request body too, not in the query string.

A `client_secret` with no value, as in `?client_secret=`, counts as absent
under RFC 6749 section 3.2 and is not refused by this rule. `POST /oauth2/revoke`
accepts such a request when the body authenticates. `POST /oauth2/tokens`
answers 400 only when the body also carries a `client_secret`, because it reads
the body copy and the empty URL copy as the same parameter sent twice, and the
error says so. An empty query value alone never causes a 400: a request that
authenticates with HTTP Basic and sends no secret in the body, or one from a
public client, succeeds.

A copy in the body does not excuse one in the URL: the request is refused on
the query string alone, whatever the body holds. The refusal issues no token
and revokes nothing, so the retry needs no new authorization.

`GET /oauth2/authorize` is not rejected when its URL carries `client_secret`,
because RFC 6749 section 3.1 requires that endpoint to ignore parameters it
does not recognize. Coder ignores the value and logs a warning instead.

Each refusal and the authorization warning write a log line containing
`client_secret in the URL query string` with the `app_id`, `remote_addr`, and
`user_agent` of the request. Search the Coder logs for that string to find the
integration that sends the secret in the URL.

The log line records that the parameter was present, not that its value was a
valid secret. The check runs before client authentication, so anyone who knows
the public `client_id` can produce the same line without credentials. Confirm
with the client's owner that their integration sent the request before
rotating.

The check also runs after the `client_id` is resolved. A request with a
missing, malformed, or unknown `client_id` is answered by that lookup first
and writes no such line, so an absent line does not mean no integration is
leaking.

Rotate the secret that was in the URL. It is still valid, and a URL is
recorded by reverse proxies, load balancers, CDN access logs, shell history,
and client libraries. Coder does not log query strings, so an empty result
when you search the Coder logs does not mean the secret stayed private.
Deleting a secret also revokes the tokens issued under it, so the client has
to authorize again.

Earlier releases accepted the parameter in the query string. An integration
that relied on that has to move it into the body or the header.

### "unsupported_response_type" returned to your callback

Coder supports the authorization code flow only, so `response_type=code` is the single accepted value.
`GET /.well-known/oauth-authorization-server` reports it in `response_types_supported`.

Any other value, including the `token` of the implicit grant, redirects to your registered callback with `error=unsupported_response_type`, an `error_description` of `Only response_type=code is supported`, and the `state` you sent.
This holds for both `GET /oauth2/authorize` and `POST /oauth2/authorize`.

Earlier releases answered on Coder instead: `GET` rendered an "Unsupported Response Type" page and `POST` returned a 400 with a JSON body.
An integration that watched for either now has to read the error from its own callback.

### "invalid_request" for `code_challenge_method`

Coder supports the `S256` challenge method only.
`plain` sends the verifier itself as the challenge, so anything that can observe the authorization request can complete the exchange, which is what PKCE exists to prevent.
Omitting the parameter is allowed and means `S256`.

An unsupported method redirects to your registered callback with `error=invalid_request`, an `error_description` that names the method, and the `state` you sent.
This holds for both `GET /oauth2/authorize` and `POST /oauth2/authorize`.

### "invalid_request" for a rejected parameter

Coder validates every authorization parameter before issuing a code, and reports all the failing fields together in one `error_description`.
Each entry reads `field: reason`, and entries are separated by a semicolon and a space.
Common causes are a `code_challenge` outside the 43 to 128 character unreserved set, and any parameter sent more than once.

The rejection redirects to your registered callback with `error=invalid_request`, an `error_description` naming the fields, and the `state` you sent.
This holds for both `GET /oauth2/authorize` and `POST /oauth2/authorize`.
A description longer than 2048 characters is cut short and marked `(truncated)`.

Parameters the endpoint does not read are ignored, as RFC 6749 Section 3.1 requires, so an OIDC `nonce` or a vendor extension does not fail the request.
A misspelled parameter is ignored on the same rule, so what you see is the failure caused by the parameter you meant to send being absent.

Two failures stay on Coder rather than reaching your callback, because in both cases the callback is not yet trustworthy:

- A `redirect_uri` that does not parse, or that does not exactly match one of the redirect URIs registered for the application.
  Redirecting to it would defeat the check that just rejected it, so Coder answers 400 (see ["Invalid redirect_uri"](#invalid-redirect_uri)).
- A `client_id` sent more than once, or one that does not name the application the callback was matched against.
  Coder cannot tell whose registration it is about to redirect to.

Earlier releases answered on Coder for all of these: `GET` rendered an "Invalid Query Parameters" page and `POST` returned a 400 with a JSON body.
An integration that watched for either now has to read the error from its own callback.

### "invalid_request" from `POST /oauth2/tokens` for a repeated parameter

The token endpoint ignores parameters it does not read, as RFC 6749 Section 3.2 requires, so an OIDC `nonce`, a `client_assertion`, or a vendor extension does not fail the exchange.
A misspelled parameter is ignored on the same rule, so what you see is the failure caused by the parameter you meant to send being absent.

A known parameter sent more than once is rejected with a 400 and a JSON body.
The error is `invalid_request`, except for a repeated `grant_type`, which answers `unsupported_grant_type`.

Earlier releases returned 400 `invalid_request` for any parameter the endpoint did not recognize.
An integration that relied on that error to catch a misspelled optional parameter no longer receives it.

### "invalid_target" for a rejected `resource`

`resource` must be an absolute URI without a fragment (RFC 8707).
A value that is not redirects to your registered callback with `error=invalid_target`, an `error_description` naming the field, and the `state` you sent.
`POST /oauth2/token` already answered `invalid_target` for the same value.

If anything else in the request also failed, the answer is `invalid_request` instead, naming every failing field.
Correct them all before retrying: a retry that fixes only `resource` fails again.

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

The out-of-band URN `urn:ietf:wg:oauth:2.0:oob` is accepted from either client type, for clients that display the authorization code for the user to copy rather than receiving it on a redirect. No other URN is accepted.

The following schemes are blocked for security reasons: `javascript:`, `data:`, `file:`, `ftp:`.

Public clients (`token_endpoint_auth_method: none`) additionally cannot register `mailto:`, `tel:`, or `sms:` redirect URIs, since those schemes hand off to another app rather than returning an authorization code to the client. Confidential clients are not subject to this restriction.

A cleartext `http://` redirect URI is accepted only for a local host. A confidential client may use `localhost`, `127.0.0.1`, `::1`, or a `.localhost` subdomain such as `http://app.localhost/callback`. A public client is limited to `localhost`, `127.0.0.1`, and `::1`. Every other host must use `https://`, so that an authorization code is never delivered in the clear. The management API and Dynamic Client Registration apply the same rule, so an administrator cannot store a target that a client could not register for itself.

These rules apply to every entry in `redirect_uris`, not only the first one.

## Security Considerations

- **Use HTTPS**: Always use HTTPS in production to protect tokens in transit
- **Implement PKCE**: PKCE is mandatory for all authorization code clients
  (public and confidential)
- **Validate redirect URLs**: Only register trusted redirect URIs. Dangerous
  schemes (`javascript:`, `data:`, `file:`, `ftp:`) are blocked by the control
  plane, custom URI schemes for native apps (`myapp://`) are permitted, and
  public clients additionally cannot use `mailto:`, `tel:`, or `sms:`
- **Rotate secrets**: Periodically rotate client secrets using the management API
- **Rate limits**: every `/oauth2` endpoint and both `/.well-known` discovery
  endpoints draw on the login rate limit of 60 requests per minute. Each
  endpoint counts on its own, so a caller that exhausts one can still reach the
  others. Requests with no Coder session are counted per IP address, and the
  rest are counted per user. A caller over the limit receives HTTP 429 with a
  `temporarily_unavailable` error body. The limit is fixed. Running the
  deployment with `--dangerous-disable-rate-limits` turns it off, and a user
  with the Owner role can bypass it on a single request with the
  `X-Coder-Bypass-Ratelimit` header
- **Refresh tokens are not self-sufficient**: a confidential client must present
  its `client_secret` to refresh or revoke, so a leaked token alone cannot mint
  new access tokens or end another client's session
- **No CORS on the authorization endpoint**: `/oauth2/authorize` is reached
  only by browser navigation and sends no CORS headers, as OAuth 2.1 requires.
  The token, registration, revocation, and metadata endpoints do allow
  cross-origin requests so that browser-based clients can call them

## Limitations

The current implementation has these limitations:

- No client credentials grant support
- No device authorization grant support (RFC 8628)
- Implicit grant (`response_type=token`) is not supported; OAuth 2.1 deprecated this flow due to token leakage risks, and a request for it redirects to the registered callback with `unsupported_response_type`
- Limited to opaque access tokens (no JWT support)
- An application may register at most 32 redirect URIs of at most 2048 bytes each. An application that stored a longer list before this limit existed keeps working, but it cannot be saved again until the list fits. To fix it, send a `PUT` with a `redirect_uris` list that fits, as shown under [Management API](#method-2-management-api). In the web UI, open the application and remove entries from **Redirect URIs** until the list fits.
- A cleartext `http://` redirect URI to a host that is not local is rejected. Earlier versions accepted one through the management API for a confidential application, although Dynamic Client Registration always refused it. An application that stored one keeps working, but it cannot be saved again until its list uses `https://` or a local host, as described under [Callback URL schemes](#callback-url-schemes).
- A redirect URI with a private-use scheme must name a path or an authority, as in `com.example.app:/callback` or `com.example.app://auth/callback`. The bare form `com.example.app:callback` is rejected. Dynamic Client Registration accepted it in earlier versions. A client that registered one can re-register with one of the other two forms, or an administrator can correct it with the same `PUT`.

The `redirect_uris` list is now the source of truth for an application's callbacks, and its first entry is the primary:

- Earlier versions stored the primary in a separate `callback_url` field. The upgrade migration rewrites every application so the list starts with that value.
- During a rolling upgrade, a replica running an earlier version still writes only the old field when an administrator edits the callback URL. Replicas running the new version read the list instead, so the edit is silently lost: the application keeps its old callback URL and keeps accepting every previous redirect URI, including any the administrator meant to remove.
- Replicas running the new version also show the old callback URL in the form, so saving the application unchanged does not restore the edit.
- Drain replicas running the earlier version before you upgrade. If a callback URL was edited during the upgrade, enter the intended value again and save the application once every replica runs the new version.

A `scope` on a refresh request was parsed and discarded in earlier versions, so a
client sending one wider than its grant refreshed successfully. It is now
enforced, and such a request answers HTTP 400 with `error=invalid_scope`. The
refresh token is not consumed, so a client that drops the parameter or asks for
less recovers without re-authorizing.

Earlier versions did not check `client_secret` on a refresh or at the RFC 7009
revocation endpoint, so a confidential client could refresh or revoke with a
wrong secret or none. Both now authenticate confidential clients exactly as the
authorization code grant does, and a request without a valid secret answers
HTTP 401 with `error=invalid_client`. The refresh token is not consumed and
nothing is revoked, so a client that adds its secret recovers without
re-authorizing. Public clients are unaffected.

Coder now enforces the `scope` an application declared for itself when it self-registered through [Dynamic Client Registration](#dynamic-client-registration).
This affects only deployments that enabled Dynamic Client Registration and have an application that self-registered with a `scope`.
Dynamic Client Registration is disabled by default, so if you never enabled it, nothing changes for you.
Turning it back off does not clear the check: Coder validates the stored `scope` of an existing application whether or not registration is still allowed, so an application that self-registered before you turned the setting off is affected too.

Earlier versions of Coder accepted any `scope` at registration without checking it, and every token for that application had full access.
Coder now treats the registered `scope` as the list of scopes the application is allowed to request, as described under [Scopes](#scopes).
Applications that self-registered without a `scope`, and applications created through the web UI or the management API without one, have no scope list and are not affected; they continue to receive full access.

An affected application fails in the following ways:

- A request for a scope name this deployment does not offer fails with `invalid_scope`.
- If none of the registered names are offered, every authorization fails, even one that leaves `scope` out.
- Authorization codes issued before the upgrade fail at the token endpoint with `invalid_grant` until they expire.

For the full error details, refer to ["invalid_scope" returned to your callback](#invalid_scope-returned-to-your-callback) and ["invalid_grant" for a scope the deployment cannot mint](#invalid_grant-for-a-scope-the-deployment-cannot-mint).

To fix an affected application, the party that holds its `registration_access_token` updates the registration with `PUT /oauth2/clients/{client_id}`, so that `scope` lists only names from `scopes_supported` in `GET /.well-known/oauth-authorization-server`.
If that token is lost, register the application again.
A Coder administrator can also fix it from the management API by [updating the application](../../reference/api/enterprise.md#update-oauth2-application) with a `scope` that lists supported names, or with an empty `scope` to remove the allowlist.
The **Allowed scopes** field on the application page in the web UI makes the same change.

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

- Review the [API Reference](../../reference/api/index.md) for complete endpoint documentation
- Check [External Authentication](../external-auth/index.md) for configuring Coder as an OAuth2 client
- See [Security Best Practices](../security/index.md) for deployment security guidance

## Feedback

Report issues and feedback through [GitHub Issues](https://github.com/coder/coder/issues) with the `oauth2` label.
