---
title: OAuth2 provider troubleshooting
---

This page is for a developer whose OAuth2 client request failed.
It collects the error conditions the OAuth2 provider returns, matched by the exact `error`, `error_description`, or log line you'll see.
For how the provider works, refer to [OAuth2 provider](./index.md) and [Integration patterns](./integration-patterns.md).

## OAuth2 endpoints return 404

The provider is off.
Set `CODER_OAUTH2_PROVIDER_ENABLE=true` and restart the server.
Refer to [Enable OAuth2 Provider](./index.md#enable-oauth2-provider).

## "Invalid redirect_uri"

Ensure the redirect URI in your request exactly matches one of the redirect URIs registered for your application.
The one exception is the port of a loopback `http://` redirect URI (`localhost`, `127.0.0.1`, `[::1]`), which may differ from the registered one.
Refer to [Callback URL schemes](./callback-url-schemes.md).

## "Invalid Callback URL" on the consent page

If you see this error when authorizing, one of the application's registered redirect URIs is not usable: either it does not parse as a URL, or it uses a blocked scheme (`javascript:`, `data:`, `file:`, or `ftp:`).
The same cause answers `server_error` on `POST /oauth2/authorize`.
Use `GET /api/v2/oauth2-provider/apps/{app}` to see every registered redirect URI, then update the application with a corrected `redirect_uris` list as shown under [Create an application with the API](./index.md#create-an-application-with-the-api).
Refer to [Callback URL schemes](./callback-url-schemes.md) for which values are accepted.

The `coderd` log records the application ID and the stored value. The response
does not, so a bad URL is never echoed back to a browser.

## "invalid_scope" returned to your callback

The authorization endpoint validates the `scope` parameter.
When it cannot grant what was asked for, it redirects to your registered callback with `error=invalid_scope` rather than issuing a code.
The `error_description` opens with the requested name that caused the rejection:

- `unknown or unsupported scope`: this deployment does not offer that name to
  OAuth2 clients. It may not exist, or it may exist and be internal-only, which
  no version offers. Read the current list from `scopes_supported` in
  `GET /.well-known/oauth-authorization-server`.
- `scope requests permissions beyond this app's allowed scopes`: the name is supported, but the application's `scope` allowlist does not cover it.
  Request less, or widen the allowlist.
- `none of the scopes registered for this app are supported by this deployment`: the application's `scope` allowlist names nothing this deployment offers, so no request against it can succeed, including one that omits `scope`.
  Update the allowlist with supported scopes.
  This description stands alone.
  Nothing checks a stored `scope` against the catalog, so the response never echoes it; the `coderd` log records the application ID.

Omitting `scope` requests the application's allowlist, or full access if it has none.

The negotiated scope is recorded on the authorization, shown on the consent
page, and applied to the access token issued when the code is exchanged.

The token endpoint validates a refresh request's `scope` too, and answers `invalid_scope` in the response body rather than by redirect.
Refer to ["invalid_scope" for a refresh that names a scope](#invalid_scope-for-a-refresh-that-names-a-scope).

## "invalid_grant" for a scope the deployment cannot mint

`POST /oauth2/tokens` mints the access token with the scope recorded on the
authorization code, or on the refresh token when refreshing. If that stored
scope names something this deployment cannot mint, the exchange answers HTTP
400 with `error=invalid_grant` and an `error_description` naming the value.

The usual cause is a grant made against a scope the deployment has since
dropped. Authorize again to negotiate a scope it still supports; the stored
scope is not something the client can change by requesting a different one.

The exchange also re-checks the code's scope against the application's
registered `scope`, which can change during the 10 minutes a code stays valid.
Two more descriptions can open the `error_description` here:

- `scope is no longer allowed by this app's registered scopes`: the allowlist narrowed after the code was issued and no longer covers the code's scope.
  Authorize again to negotiate a scope within the new allowlist.
- `none of the scopes registered for this app are supported by this deployment`: the allowlist names nothing this deployment offers, so no code against it can be redeemed.
  Update the allowlist with supported scopes.
  As on the authorize endpoint, the stored value stays out of the response.

A coverage comparison this deployment cannot decide answers HTTP 500 with
`error=server_error` and `The requested scope could not be evaluated`; the
scope that could not be compared is in the `coderd` logs, not the response.

An application's `scope` allowlist can change through [Dynamic Client Registration](./index.md#dynamic-client-registration), by the application itself, or through the management API, by an administrator.
An application that holds its registration access token can widen its own allowlist again before redeeming a code, so treat this re-check as reflecting the allowlist at redemption time rather than as a constraint on the client.

A refresh is not re-checked against the registration. That is a Coder policy
choice: withdrawing scope from a session already running would break it
mid-flight, so a narrowing takes effect at the next authorization. A refresh
token keeps its granted scope until it expires, which can be up to the
configured refresh lifetime; revoke the token to cut a live session.

Codes issued before the upgrade that added scope columns carry `coder:all`, recorded as an unrestricted grant.
For an application with a narrower `scope` allowlist, those codes are refused with `scope is no longer allowed by this app's registered scopes` until they expire, which takes at most 10 minutes.
Authorizing again issues a code within the current allowlist.

## "invalid_scope" for a refresh that names a scope

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

## "invalid_client" for a refresh or a revocation

`POST /oauth2/tokens` with `grant_type=refresh_token` and `POST /oauth2/revoke` answer HTTP 401 with `error=invalid_client` when a confidential client does not authenticate.
The usual causes are a `client_secret` that was omitted, a secret that belongs to a different client, or a secret that has since been deleted or rotated.
Present the client's current secret, as HTTP Basic or as a form parameter, following [Refresh tokens](./token-management.md#refresh-tokens).
The refresh token is not consumed and nothing is revoked by the refusal, so the retry needs no new authorization.
If the secret was deleted, the tokens issued under it were revoked with it, and the client must authorize again.
Public clients have no secret and never receive this error for omitting one.

## "invalid_request" for `client_secret` in the query string

`POST /oauth2/tokens` and `POST /oauth2/revoke` answer HTTP 400 with `error=invalid_request` when `client_secret` appears in the URL query string.
OAuth 2.1 section 2.4.1 allows the secret in the request body or the `Authorization` header only.
Send it as a form parameter or as HTTP Basic, following [Client authentication methods](./integration-patterns.md#client-authentication-methods).

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

## "unsupported_response_type" returned to your callback

Coder supports the authorization code flow only, so `response_type=code` is the single accepted value.
`GET /.well-known/oauth-authorization-server` reports it in `response_types_supported`.

Any other value, including the `token` of the implicit grant, redirects to your registered callback with `error=unsupported_response_type`, an `error_description` of `Only response_type=code is supported`, and the `state` you sent.
This holds for both `GET /oauth2/authorize` and `POST /oauth2/authorize`.

Earlier releases answered on Coder instead: `GET` rendered an "Unsupported Response Type" page and `POST` returned a 400 with a JSON body.
An integration that watched for either now has to read the error from its own callback.

## "invalid_request" for `code_challenge_method`

Coder supports the `S256` challenge method only.
`plain` sends the verifier itself as the challenge, so anything that can observe the authorization request can complete the exchange, which is what PKCE exists to prevent.
Omitting the parameter is allowed and means `S256`.

An unsupported method redirects to your registered callback with `error=invalid_request`, an `error_description` that names the method, and the `state` you sent.
This holds for both `GET /oauth2/authorize` and `POST /oauth2/authorize`.

## "invalid_request" for a rejected parameter

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
  Redirecting to it would defeat the check that just rejected it, so Coder answers 400 (refer to ["Invalid redirect_uri"](#invalid-redirect_uri)).
- A `client_id` sent more than once, or one that does not name the application the callback was matched against.
  Coder cannot tell whose registration it is about to redirect to.

Earlier releases answered on Coder for all of these: `GET` rendered an "Invalid Query Parameters" page and `POST` returned a 400 with a JSON body.
An integration that watched for either now has to read the error from its own callback.

## "invalid_request" from `POST /oauth2/tokens` for a repeated parameter

The token endpoint ignores parameters it does not read, as RFC 6749 Section 3.2 requires, so an OIDC `nonce`, a `client_assertion`, or a vendor extension does not fail the exchange.
A misspelled parameter is ignored on the same rule, so what you see is the failure caused by the parameter you meant to send being absent.

A known parameter sent more than once is rejected with a 400 and a JSON body.
The error is `invalid_request`, except for a repeated `grant_type`, which answers `unsupported_grant_type`.

Earlier releases returned 400 `invalid_request` for any parameter the endpoint did not recognize.
An integration that relied on that error to catch a misspelled optional parameter no longer receives it.

## "invalid_target" for a rejected `resource`

`resource` must be an absolute URI without a fragment (RFC 8707).
A value that is not redirects to your registered callback with `error=invalid_target`, an `error_description` naming the field, and the `state` you sent.
`POST /oauth2/token` already answered `invalid_target` for the same value.

If anything else in the request also failed, the answer is `invalid_request` instead, naming every failing field.
Correct them all before retrying: a retry that fixes only `resource` fails again.

## "PKCE verification failed"

Verify that the `code_verifier` used in the token request matches the one used to generate the `code_challenge`.

## "public clients may not use the mailto/tel/sms scheme"

This error appears during client registration when a public client
(`token_endpoint_auth_method: none`) registers a redirect URI using the
`mailto:`, `tel:`, or `sms:` scheme. These schemes hand off to a mail
client, dialer, or SMS app instead of returning control to the
application that started the flow, so a public client registered with
one of them could never complete authorization. Register a redirect URI
the client can actually receive control on instead, such as a custom
scheme (`myapp://callback`) or a loopback HTTP address.

## Learn more

- Review [OAuth2 provider](./index.md) for enabling the provider and creating an application
- Review [Integration patterns](./integration-patterns.md) for client authentication methods and flows
- Review [Security and limitations](./security.md) for security considerations, current limitations, and upgrade notes
