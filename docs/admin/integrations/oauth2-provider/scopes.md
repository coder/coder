---
title: OAuth2 provider scopes
---

This page is for a developer choosing which scope to request, and for a Coder deployment administrator restricting which scopes an application may request.
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

## Learn more

- Refer to ["invalid_scope" for a refresh that names a scope](./troubleshooting.md#invalid_scope-for-a-refresh-that-names-a-scope) for how a refresh can narrow the access token it mints
- Check [Troubleshooting](./troubleshooting.md) for scope-related errors
