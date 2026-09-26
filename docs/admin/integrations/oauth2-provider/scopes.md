---
title: OAuth2 provider scopes
---

An access token is bounded by the scope negotiated when the user authorized it, on top of that user's own permissions. A token can never do more than its user can.

Scope names come from the same vocabulary as [API key scopes](../../users/sessions-tokens.md#api-key-scopes): individual `resource:action` names such as `workspace:ssh`, and `coder:` composites such as `coder:workspaces.access` that stand for a set of them.
`coder:all` records an unrestricted grant.

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
An administrator sets one with the **Allowed scopes** field in the web UI, or with the optional, space-separated `scope` field when [creating an application](../../../reference/api/enterprise.md#create-oauth2-application) through the management API.
When [updating an application](../../../reference/api/enterprise.md#update-oauth2-application), omit `scope` to keep the current allowlist, send a new value to replace it, or send an empty string to clear it and make the application unrestricted.
Dynamic Client Registration keeps only the names this deployment offers, both on `POST /oauth2/register` and on `PUT /oauth2/clients/{client_id}`, and returns the stored list in the response.
A `scope` that keeps no offered name is rejected with `400 invalid_client_metadata` and the request is not stored.
An update that resends the stored `scope` unchanged keeps it as stored.
An allowlist set through the web UI or the management API is stored as given; a name it holds that this deployment does not offer fails at authorization, as described under ["invalid_scope" returned to your callback](./troubleshooting.md#invalid_scope-returned-to-your-callback).

Use caution when narrowing the scope allowlist of a self-registered application.
Many clients request every scope Coder advertises in `scopes_supported` rather than selecting specific scopes; MCP clients that rely on discovery commonly work this way.
The allowlist is stored on the application and does not affect that advertised list, so the client continues requesting the full set.
Coder rejects requests that exceed the allowlist rather than trimming their scopes, so every new authorization attempt fails with `invalid_scope`.
Previously issued tokens retain their scopes.
Such a client cannot be restricted through its allowlist: narrowing it breaks the client, and clearing it leaves the application unrestricted.
Only a client that can be configured to request fewer scopes can be narrowed, and whoever operates that client makes the change.
An allowlist set by an administrator also does not hold against the client: the holder of the application's `registration_access_token` can replace or clear it at any time with `PUT /oauth2/clients/{client_id}`.
The web UI warns before saving a narrower allowlist, and the application page indicates whether the application was self-registered or created by an administrator.

The consent page states the scope being granted before the user approves it.
A refresh keeps the scope originally granted; a refresh that names a narrower `scope` applies it to the access token it mints, leaving the grant itself unchanged.

## Learn more

- Refer to ["invalid_scope" for a refresh that names a scope](./troubleshooting.md#invalid_scope-for-a-refresh-that-names-a-scope) for how a refresh can narrow the access token it mints
- Check [Troubleshooting](./troubleshooting.md) for scope-related errors
