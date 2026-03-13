# VS Code iframe embed auth (Experimental)

> [!WARNING]
> This flow is experimental, is behind the `agents` experiment flag, and may
> change without notice.

Use this flow when embedding `/agents/:agentId/embed` inside a VS Code webview
or another iframe host. The parent supplies an existing Coder bearer token once,
then Coder converts it into the normal browser session cookie for follow-up
requests.

## postMessage contract

When the iframe loads signed out, it sends a ready message to the parent:

```js
window.parent.postMessage(
  { type: "coder:vscode-ready", payload: { agentId: "<agent-id>" } },
  "*",
);
```

The parent replies with the bootstrap credential:

```js
iframe.contentWindow.postMessage(
  {
    type: "coder:vscode-auth-bootstrap",
    payload: { token: "<existing-coder-bearer-token>" },
  },
  new URL(iframe.src).origin,
);
```

Notes:

- The iframe only accepts messages from `window.parent`.
- Messages without `payload.token`, non-string tokens, and empty tokens are
  ignored.
- Duplicate bootstrap messages are ignored while the bootstrap request is in
  flight.

## Bootstrap endpoint

`POST /api/experimental/chats/embed-session`

```json
{ "token": "<existing-coder-bearer-token>" }
```

Behavior:

- `400 Bad Request` for missing or malformed JSON input.
- `401 Unauthorized` for invalid or expired tokens.
- `401 Unauthorized` if the validated user is not active.
- `204 No Content` on success.
- Reuses the validated token as the session cookie value. It does not mint a
  separate embed-only session.

## Cookie behavior

The endpoint reuses the standard session cookie name (`coder_session_token`, or
its configured prefixed form) and forces attributes that work for this embed
flow:

- HTTPS: `HttpOnly; Path=/; SameSite=None; Secure`
- HTTP development: `HttpOnly; Path=/; SameSite=Lax`

Plain HTTP uses `SameSite=Lax` because browsers reject `Secure` cookies over
non-HTTPS origins.

## Accepted security tradeoff

This design intentionally lets the iframe handle a real bearer token long enough
to call the bootstrap endpoint. The token is kept in memory only and is not
persisted to `localStorage` or `sessionStorage`, but script running in the
iframe before the POST completes could still read it. That tradeoff is accepted
so the flow can stay simple and compatible with the target webview.

## Browser and webview caveats

`SameSite=None; Secure` is necessary for third-party iframe cookies, but it is
not always sufficient. Some browsers, enterprise policies, and embedded
webviews still block or partition third-party cookies.

When that happens, the bootstrap POST may return `204` but later iframe
requests may not carry the session cookie, especially after refresh. If the host
blocks third-party cookies for the Coder origin, this flow is not reliable until
that policy is relaxed.
