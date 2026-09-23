---
title: OAuth2 provider callback URL schemes
---

Which redirect URI schemes and hosts Coder accepts, for both admin-created
and self-registered applications.

Custom URI schemes (`myapp://`, `vscode://`, `jetbrains://`, etc.) are fully supported for native and desktop applications. The OS routes the redirect back to the registered application without requiring a running HTTP server.

The out-of-band URN `urn:ietf:wg:oauth:2.0:oob` is accepted from either client type, for clients that display the authorization code for the user to copy rather than receiving it on a redirect. No other URN is accepted.

The following schemes are blocked for security reasons: `javascript:`, `data:`, `file:`, `ftp:`.

Public clients (`token_endpoint_auth_method: none`) additionally cannot register `mailto:`, `tel:`, or `sms:` redirect URIs, since those schemes hand off to another app rather than returning an authorization code to the client. Confidential clients are not subject to this restriction.

A cleartext `http://` redirect URI is accepted only for a local host. A confidential client may use `localhost`, `127.0.0.1`, `::1`, or a `.localhost` subdomain such as `http://app.localhost/callback`. A public client is limited to `localhost`, `127.0.0.1`, and `::1`. Every other host must use `https://`, so that an authorization code is never delivered in the clear. The management API and Dynamic Client Registration apply the same rule, so an administrator cannot store a target that a client could not register for itself.

These rules apply to every entry in `redirect_uris`, not only the first one.

## Learn more

- Check [Common issues](./troubleshooting.md) for "Invalid Callback URL" and related errors
