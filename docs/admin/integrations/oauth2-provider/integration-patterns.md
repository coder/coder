---
title: OAuth2 provider integration patterns
---

This page covers how a client proves who it is and completes a login: the supported ways to prove identity, the standard login steps, the required PKCE (Proof Key for Code Exchange) step, and how a client finds the right web addresses to use.

- [OAuth2 provider](./index.md): turn on the provider and create an application
- [Scopes](./scopes.md): control what a client is allowed to access
- [Token management](./token-management.md): refresh, cancel, or delete access
- [Callback URL schemes](./callback-url-schemes.md): which redirect addresses are allowed

## Client authentication methods

Coder supports the following OAuth2 client authentication methods at the token endpoint (`/oauth2/tokens`):

- `client_secret_basic` (recommended): HTTP Basic authentication (RFC 6749 §2.3.1). The username is `client_id` and the password is `client_secret`.
- `client_secret_post`: Form-based authentication where `client_id` and `client_secret` are sent in the request body.
- `none`: No client secret.
  The client is a public client and authenticates with PKCE alone (RFC 7591 §2, OAuth 2.1 §2.1).
  Available only through [Dynamic Client Registration](./index.md#dynamic-client-registration), which is disabled by default, since a client's type is set when it registers and apps created through the admin UI or API are always confidential.

Coder supports both methods, so existing integrations using `client_secret_post` don't need to switch to `client_secret_basic`.

Send `client_secret` in the request body or in the `Authorization` header.
`POST /oauth2/tokens` and `POST /oauth2/revoke` reject a `client_secret` value in the URL query string with `invalid_request`, because OAuth 2.1 section 2.4.1 does not allow it there.
Refer to ["invalid_request" for `client_secret` in the query string](./troubleshooting.md#invalid_request-for-client_secret-in-the-query-string) for the exceptions and the log line to search for.

Public clients suit native, mobile, and CLI applications that cannot keep a secret confidential. Note the redirect URI restrictions below before choosing one.

Opening a public client on the **OAuth2 Applications** page shows no client secrets section, since a public client has no secret to display or generate.

If you use Dynamic Client Registration (RFC 7591) and omit `token_endpoint_auth_method`, clients default to `client_secret_basic`. To request `client_secret_post`, set `token_endpoint_auth_method` to `client_secret_post` in the registration request. To register a public client, set it to `none`: Coder issues no `client_secret`, and the registration response omits that field entirely.

> [!IMPORTANT]
> Which redirect URI schemes and hosts a client may register, including the loopback host and port rules for public and confidential clients, is a separate restriction from client authentication.
> Refer to [Callback URL schemes](./callback-url-schemes.md).

A client's type is fixed when it registers.
An RFC 7592 update that would move a client between public and confidential is rejected with `invalid_client_metadata`, since the client either holds a secret that would stop being required or has none and no way to be issued one.
Switching between `client_secret_basic` and `client_secret_post` is allowed, because both are confidential.
To change type, register a new client.

Clients registered with `token_endpoint_auth_method: none` before Coder honored it are stored as confidential and still require their `client_secret`.
Coder reports `client_secret_basic` for those clients so that what it reports matches what it enforces, and the mismatch clears the next time the client updates its registration using the value Coder reported.

If client authentication fails, the token endpoint returns **HTTP 401** with an OAuth2 `invalid_client` error and a `WWW-Authenticate: Basic realm="coder"` response header.

## Authorization code flow

Every authorization code flow requires PKCE (Proof Key for Code Exchange).
Coder enforces PKCE in compliance with the OAuth 2.1 specification, for both public and confidential clients.

> [!NOTE]
> `code_verifier` and `code_challenge` must each be 43-128 characters from the unreserved character set `[A-Za-z0-9-._~]` (RFC 7636 §4.1).
> A value outside these bounds is rejected with an `invalid_request` error, at the token endpoint for `code_verifier` and at the authorization endpoint for `code_challenge`.

1. Generate a code verifier and challenge:

   ```sh
   CODE_VERIFIER=$(openssl rand -base64 96 | tr -d '\n' | tr '+/' '-_' | tr -d '=')
   CODE_CHALLENGE=$(echo -n $CODE_VERIFIER | openssl dgst -sha256 -binary | base64 | tr -d "=" | tr '+/' '-_')
   ```

2. Redirect users to Coder's authorization endpoint:

   ```txt
   https://coder.example.com/oauth2/authorize?
     client_id=your-client-id&
     response_type=code&
     code_challenge=$CODE_CHALLENGE&
     code_challenge_method=S256&
     redirect_uri=https://yourapp.example.com/callback&
     state=random-string
   ```

3. Exchange the authorization code for an access token, using the code verifier (refer to [Client authentication methods](#client-authentication-methods) for how to authenticate):

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

   Send `client_id` in the form body and omit `client_secret` entirely.
   The code verifier is the only proof of possession, and must satisfy RFC 7636 §4.1 (43-128 characters from `[A-Za-z0-9-._~]`).

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

4. Use the access token to call Coder's API:

   ```sh
   curl -H "Authorization: Bearer $ACCESS_TOKEN" \
     "$CODER_URL/api/v2/users/me"
   ```

## Discovery endpoints

Coder provides OAuth2 discovery endpoints for programmatic integration:

- **Authorization Server Metadata**: `GET /.well-known/oauth-authorization-server`
- **Protected Resource Metadata**: `GET /.well-known/oauth-protected-resource`

These endpoints return server capabilities and endpoint URLs according to [RFC 8414](https://datatracker.ietf.org/doc/html/rfc8414) and [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728).

`token_endpoint_auth_methods_supported` lists every method the token endpoint accepts, including `none`.
It is not gated on [Dynamic Client Registration](./index.md#dynamic-client-registration), since existing public clients still exchange tokens when new registrations are disabled.
`registration_endpoint` is advertised only while Dynamic Client Registration is enabled, so that field, not this one, tells a client whether it can register a new public client.

## Standards compliance

This implementation follows established OAuth2 standards including [RFC 6749](https://datatracker.ietf.org/doc/html/rfc6749) (OAuth2 core), [RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636) (PKCE), and the [OAuth 2.1 draft](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1-16).
Coder enforces OAuth 2.1 requirements including mandatory PKCE for all authorization code grants, exact redirect URI string matching with the [RFC 8252](https://datatracker.ietf.org/doc/html/rfc8252) loopback port exception, rejection of the implicit grant, and CSRF protections on consent pages.

## Learn more

- Review [Scopes](./scopes.md) for how access is bounded
- Review [Token management](./token-management.md) for refresh, revocation, and deletion
- Check [Troubleshooting](./troubleshooting.md) if a request fails
