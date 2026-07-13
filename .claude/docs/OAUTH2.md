# OAuth2 Development Guide

Implement OAuth2 and OpenID Connect behavior from the applicable RFCs, not from
memory or assumptions. Preserve protocol error shapes and authorization
boundaries even when nearby non-protocol handlers use different conventions.

## RFC-Compliant Errors

OAuth2 endpoints must return the error format required by the relevant RFC:

```json
{"error":"invalid_request","error_description":"details"}
```

- Use standard codes such as `invalid_client`, `invalid_grant`, and
  `invalid_request`.
- Use `writeOAuth2Error(...)` for OAuth2 endpoint failures.
- Do not replace protocol errors with generic API error responses.
- Match the RFC's HTTP status, required fields, optional fields, and disclosure
  rules.

```go
if errors.Is(err, errInvalidPKCE) {
    writeOAuth2Error(
        ctx,
        rw,
        http.StatusBadRequest,
        "invalid_grant",
        "The PKCE code verifier is invalid",
    )
    return
}
```

Verify default values against the RFC and keep database defaults consistent
with application behavior. For example, RFC 7591 defaults
`token_endpoint_auth_method` to `client_secret_basic`.

## Public Endpoints and Database Authorization

A public endpoint has no user authorization context. When it must read or write
system-owned OAuth2 state, use `dbauthz.AsSystemRestricted(ctx)`:

```go
app, err := api.Database.GetOAuth2ProviderAppByClientID(
    dbauthz.AsSystemRestricted(ctx),
    clientID,
)
```

Do not use an unrestricted system context. Authenticated endpoints should keep
the caller context unless an established authorization boundary requires
otherwise.

## Protocol Safeguards

- PKCE must validate the stored challenge and method against the verifier. Use
  S256 where required and preserve compatibility for flows that legitimately do
  not use PKCE.
- Consent actions use POST and do not depend on the `Referer` header for
  authorization decisions.
- Validate `state` according to the flow.
- RFC 8707 resource indicators are optional, but authorization, token, and
  refresh flows must preserve and validate them consistently when present.
- Do not disclose registration access tokens in registration GET responses.
- Support URI schemes allowed by the applicable native-app specification.

Database schema, query, generation, and audit changes follow
[Database Development Patterns](DATABASE.md).

## Tests

Cover protocol-defined errors, HTTP statuses, defaults, PKCE, resource
indicators, client isolation, token invalidation, and information disclosure.

Exact scripts in `scripts/oauth2/`:

```sh
./scripts/oauth2/test-mcp-oauth2.sh
./scripts/oauth2/test-manual-flow.sh
```

Supporting scripts:

- `setup-test-app.sh`
- `cleanup-test-app.sh`
- `generate-pkce.sh`

Run the automated script after OAuth2 changes. Use the manual flow when browser
redirects or consent behavior changed.

## Common Failures

- Wrong error envelope: route the failure through `writeOAuth2Error(...)` and
  verify its RFC status and code.
- Existing client reported as `invalid_client`: inspect the endpoint's database
  authorization context.
- PKCE failure: verify challenge and method persistence in both authorization
  code creation and token exchange.
- Resource mismatch: trace the value through authorization, storage, token, and
  refresh operations.
- Default mismatch: compare the RFC, migration default, generated model, and
  application fallback.
