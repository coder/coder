---
title: OAuth2 provider security and limitations
---

Security guidance for deploying the OAuth2 provider, and the current
implementation's limitations. For enabling the provider and creating an
application, see [OAuth2 provider](./index.md).

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

- The web UI cannot set or change a scope allowlist; declare one at [Dynamic Client Registration](./index.md#dynamic-client-registration) or set it through the management API, as described under [Scopes](./scopes.md)
- No client credentials grant support
- No device authorization grant support (RFC 8628)
- Implicit grant (`response_type=token`) is not supported; OAuth 2.1 deprecated this flow due to token leakage risks, and a request for it redirects to the registered callback with `unsupported_response_type`
- Limited to opaque access tokens (no JWT support)
- An application may register at most 32 redirect URIs of at most 2048 bytes each. To fix an application over that limit, send a `PUT` with a `redirect_uris` list that fits, as shown under [Management API](./index.md#method-2-management-api), or open the application in the web UI and remove entries from **Redirect URIs** until the list fits.
- A cleartext `http://` redirect URI to a host that is not local is rejected; use `https://` or a local host, as described under [Callback URL schemes](./callback-url-schemes.md).
- A redirect URI with a private-use scheme must name a path or an authority, as in `com.example.app:/callback` or `com.example.app://auth/callback`. The bare form `com.example.app:callback` is rejected.
