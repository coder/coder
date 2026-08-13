# OAuth2 Scope Negotiation

How a client asks for scopes, what the server grants, and why. The rules
described here are implemented by `validateRequestedScope` in
[`authorize.go`](./authorize.go) and `rbac.ScopesCover` in
[`../rbac/scopes.go`](../rbac/scopes.go).

Two separate values decide what a token can do:

- The **allowlist**, stored on the app at registration time, is a ceiling on
  what that app may ever be granted.
- The **request**, sent as the `scope` query parameter on each authorization
  request, is what the client wants this time. It may be narrower than the
  ceiling, never wider.

## Discovering valid scope names

Only names in the curated external catalog (`rbac.IsExternalScope`) may be
requested. RBAC can expand more names than that, and the `api_key_scope` enum
can store more than that; the catalog is a deliberate curation that keeps
internal-only names such as `debug_info:read` from being negotiated by a
client.

The catalog is published in two places, both sourced from
`rbac.ExternalScopeNames()`:

```sh
# RFC 8414 discovery, unauthenticated. See the scopes_supported field.
curl https://coder.example.com/.well-known/oauth-authorization-server

# Authenticated equivalent for first-party UI.
curl -H "Coder-Session-Token: $TOKEN" \
  https://coder.example.com/api/v2/auth/scopes
```

It contains three kinds of name:

| Kind            | Examples                                           | Notes                                           |
|-----------------|----------------------------------------------------|-------------------------------------------------|
| Composite       | `coder:workspaces.access`, `coder:templates.build` | Expand to several `resource:action` permissions |
| Low-level       | `workspace:ssh`, `template:read`, `file:create`    | One permission each                             |
| Wildcard action | `workspace:*`, `template:*`                        | Every action on that resource                   |

`all` and `application_connect` are accepted as backward-compatible aliases
and are stored as `coder:all` and `coder:application_connect`.

## Setting an app's allowlist

For dynamically registered clients (RFC 7591), the allowlist is the `scope`
field at registration:

```sh
curl -X POST https://coder.example.com/oauth2/register \
  -H 'Content-Type: application/json' \
  -d '{
    "client_name": "my-ci-bot",
    "redirect_uris": ["https://ci.example.com/callback"],
    "scope": "coder:workspaces.access"
  }'
```

Admin-created apps store `NULL` instead, which means no allowlist and so no
ceiling. Both `NULL` and `''` are read as the same absent state, so an app
that registered without a `scope` field behaves like an admin-created one.

## Requesting scopes in the authorization request

`scope` is a space-separated, URL-encoded list on `GET /oauth2/authorize`.
Note the endpoint is at the deployment root, not under `/api/v2`:

```text
https://coder.example.com/oauth2/authorize
  ?response_type=code
  &client_id=<client_id>
  &redirect_uri=https%3A%2F%2Fci.example.com%2Fcallback
  &state=xyz123
  &code_challenge=<S256 challenge>
  &code_challenge_method=S256
  &scope=workspace%3Assh%20template%3Aread
```

Three constraints apply to the request as a whole:

- PKCE is mandatory for `response_type=code`, so `code_challenge` is required.
- Unrecognized query parameters are rejected, so the URL cannot carry extras.
- The browser must already hold a Coder session; the endpoint redirects to
  login otherwise.

Both `GET` and `POST /oauth2/authorize` run the same scope negotiation. `GET`
runs it before rendering the consent page, so a request the app can never be
granted fails before the user is asked to approve anything. The page lists
what the negotiation produced, so the permissions a user approves are the
ones the code will carry. An unrestricted grant is described as full access
rather than by name, since `coder:all` states less to a user than the
sentence does.

## What the allowlist accepts

The allowlist bounds authority, not spelling. A requested name is accepted
when every permission it expands to is also granted by the allowlist, whether
or not the allowlist names it. This is what `rbac.ScopesCover` decides.

For an app registered with `coder:workspaces.access`, which expands to
`template:read`, `organization_member:read`, and `workspace:read`,
`workspace:ssh`, `workspace:application_connect`:

| `scope=` sent                    | Result                            | Reason                                                                 |
|----------------------------------|-----------------------------------|------------------------------------------------------------------------|
| *(omitted)*                      | Granted `coder:workspaces.access` | RFC 6749 section 3.3: an omitted scope defaults to the whole allowlist |
| `workspace:ssh`                  | Granted `workspace:ssh`           | Covered by the composite                                               |
| `workspace:ssh template:read`    | Granted both                      | Coverage may draw on several allowlist entries at once                 |
| `coder:workspaces.access`        | Granted as requested              | A scope always covers itself                                           |
| `template:update`                | `invalid_scope`                   | The composite grants `template:read`, never `update`                   |
| `workspace:*`                    | `invalid_scope`                   | The wildcard action is wider than the composite that covers part of it |
| `workspace:ssh workspace:delete` | `invalid_scope`                   | Refused whole rather than trimmed to the covered part                  |
| `openid`                         | `invalid_scope`                   | Not in the external catalog                                            |

The second and third rows are the point of coverage. Under name matching, a
client that only needed SSH had to request the entire composite to get any
token at all, which is the opposite of what least privilege asks for.

An app with no allowlist grants whatever the request names, or
`coder:all` when the request names nothing.

### Rejection reasons

All three reject with RFC 6749 `invalid_scope`, and are distinguished by the
`error_description` text:

| Sentinel              | Meaning                                                                                                                              |
|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------|
| `errUnknownScope`     | The name is not in the external catalog. The client asked for something that does not exist or is internal-only.                     |
| `errScopeNotAllowed`  | The name is real, but the app's allowlist does not carry the authority for it.                                                       |
| `errNoGrantableScope` | The app's allowlist exists but no entry in it survives catalog filtering. The request is not at fault; the app needs re-registering. |

`errNoGrantableScope` is deliberately not treated as an absent allowlist.
Falling back to unrestricted there would grant strictly more than the
allowlist ever permitted.

## How a rejection reaches the client

Once the redirect URI has been exact-matched against the app's registration,
a scope failure redirects to the app's own callback per RFC 6749 section
4.1.2.1, carrying the state back unchanged:

```text
https://ci.example.com/callback
  ?error=invalid_scope
  &error_description=%22template%3Aupdate%22%3A+scope+is+not+in+this+app%27s+allowed+scope+list
  &state=xyz123
```

Rendering the failure on Coder instead would mean the client's error handling
never runs and it cannot correlate the failure with the request that caused
it. Errors raised *before* the redirect URI is validated are shown to the user
instead, since at that point the URI is only whatever the request supplied.

## Token exchange

The negotiated scope is stored on the authorization code and copied to the API
key and refresh token. The client does not restate it:

```sh
curl -X POST https://coder.example.com/oauth2/tokens \
  -d grant_type=authorization_code \
  -d client_id=<client_id> \
  -d client_secret=<secret> \
  -d code=<code> \
  -d code_verifier=<verifier>
```

On refresh, RFC 6749 section 6 says a request with no `scope` keeps the
originally granted scope, which is what the refresh path does today.

## Storage invariants

The negotiated value is written to a `NOT NULL` column carrying
`CHECK (scope <> '')`, so:

- It is never empty alongside a nil error. The unrestricted case returns the
  literal `coder:all` rather than an empty string.
- Names are canonical `api_key_scope` spellings, so aliases are rewritten
  before storage.
- Duplicates are collapsed, since a space-separated scope denotes a set.

## Known gaps

Behavior not yet implemented, listed so the examples above are not read as
describing more than exists:

- The token response does not populate `scope`, though
  `codersdk.OAuth2TokenResponse` has the field. RFC 6749 section 5.1 requires
  it whenever the granted scope differs from the requested one, which is
  exactly the omitted-scope case where an app receives its whole allowlist.
- `POST /oauth2/tokens` parses a `scope` parameter but nothing reads it. The
  refresh path is where it would narrow an existing grant.
