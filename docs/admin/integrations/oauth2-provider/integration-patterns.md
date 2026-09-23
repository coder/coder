---
title: OAuth2 provider integration patterns
---

How a client authenticates to Coder's OAuth2 provider and completes an authorization: the supported client authentication methods, the standard authorization code flow, the required PKCE (Proof Key for Code Exchange) flow, and the discovery endpoints a client uses to find them.
For enabling the provider and creating an application, refer to [OAuth2 provider](./index.md).
For scopes, token refresh and revocation, and accepted redirect URI schemes, refer to [Scopes](./scopes.md), [Token management](./token-management.md), and [Callback URL schemes](./callback-url-schemes.md).

## Client Authentication Methods

Coder supports the following OAuth2 client authentication methods at the token endpoint (`/oauth2/tokens`):

- `client_secret_basic` (recommended): HTTP Basic authentication (RFC 6749 §2.3.1). The username is `client_id` and the password is `client_secret`.
- `client_secret_post`: Form-based authentication where `client_id` and `client_secret` are sent in the request body.
- `none`: No client secret. The client is a public client and authenticates with PKCE alone (RFC 7591 §2, OAuth 2.1 §2.1). Available only through [Dynamic Client Registration](./index.md#dynamic-client-registration), which is disabled by default, since a client's type is set when it registers and apps created through the admin UI or API are always confidential.

Coder supports both basic authentication and form-based authentication for compatibility; existing integrations using `client_secret_post` do not need to change.

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
> [Callback URL schemes](./callback-url-schemes.md).

A client's type is fixed when it registers.
An RFC 7592 update that would move a client between public and confidential is rejected with `invalid_client_metadata`, since the client either holds a secret that would stop being required or has none and no way to be issued one.
Switching between `client_secret_basic` and `client_secret_post` is allowed, because both are confidential.
To change type, register a new client.

Clients registered with `token_endpoint_auth_method: none` before Coder honored it are stored as confidential and still require their `client_secret`.
Coder reports `client_secret_basic` for those clients so that what it reports matches what it enforces, and the mismatch clears the next time the client updates its registration using the value Coder reported.

If client authentication fails, the token endpoint returns **HTTP 401** with an OAuth2 `invalid_client` error and a `WWW-Authenticate: Basic realm="coder"` response header.

## Standard OAuth2 Flow

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

## PKCE Flow (Required)

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

## Discovery Endpoints

Coder provides OAuth2 discovery endpoints for programmatic integration:

- **Authorization Server Metadata**: `GET /.well-known/oauth-authorization-server`
- **Protected Resource Metadata**: `GET /.well-known/oauth-protected-resource`

These endpoints return server capabilities and endpoint URLs according to [RFC 8414](https://datatracker.ietf.org/doc/html/rfc8414) and [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728).

`token_endpoint_auth_methods_supported` lists every method the token endpoint accepts, including `none`. It is not gated on [Dynamic Client Registration](./index.md#dynamic-client-registration), since existing public clients still exchange tokens when new registrations are disabled. `registration_endpoint` is advertised only while Dynamic Client Registration is enabled, so that field, not this one, tells a client whether it can register a new public client.

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

## Learn more

- Review [Scopes](./scopes.md) for how access is bounded
- Review [Token management](./token-management.md) for refresh, revocation, and deletion
- Check [Common issues](./troubleshooting.md) if a request fails
