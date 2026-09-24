---
title: OAuth2 provider security and limitations
---

This page is for a Coder deployment administrator running the OAuth2 provider.
It covers security guidance, current limitations, and upgrade notes for pending changes.
For enabling the provider and creating an application, refer to [OAuth2 provider](./index.md).

## Security considerations

- **Use HTTPS**: Always use HTTPS in production to protect tokens in transit
- **Implement PKCE**: PKCE is mandatory for all authorization code clients (public and confidential)
- **Validate redirect URLs**: Only register trusted redirect URIs.
  Dangerous schemes (`javascript:`, `data:`, `file:`, `ftp:`) are blocked by the control plane, custom URI schemes for native apps (`myapp://`) are permitted, and public clients additionally cannot use `mailto:`, `tel:`, or `sms:`
- **Rotate secrets**: Periodically rotate client secrets using the management API
- **Rate limits**: every `/oauth2` endpoint and both `/.well-known` discovery endpoints draw on the login rate limit of 60 requests per minute.
  Each endpoint counts on its own, so a caller that exhausts one can still reach the others.
  Requests with no Coder session are counted per IP address, and the rest are counted per user.
  A caller over the limit receives HTTP 429 with a `temporarily_unavailable` error body.
  The limit is fixed.
  Running the deployment with `--dangerous-disable-rate-limits` turns it off, and a user with the Owner role can bypass it on a single request with the `X-Coder-Bypass-Ratelimit` header
- **Refresh tokens are not self-sufficient**: a confidential client must present its `client_secret` to refresh or revoke, so a leaked token alone cannot mint new access tokens or end another client's session
- **No CORS on the authorization endpoint**: `/oauth2/authorize` is reached only by browser navigation and sends no CORS headers, as OAuth 2.1 requires.
  The token, registration, revocation, and metadata endpoints do allow cross-origin requests so that browser-based clients can call them

## Limitations

The current implementation has these limitations:

- No client credentials grant support
- No device authorization grant support (RFC 8628)
- Implicit grant (`response_type=token`) is not supported; OAuth 2.1 deprecated this flow due to token leakage risks, and a request for it redirects to the registered callback with `unsupported_response_type`
- Limited to opaque access tokens (no JWT support)
- An application may register at most 32 redirect URIs of at most 2048 bytes each.
  An application that stored a longer list before this limit existed keeps working, but it cannot be saved again until the list fits.
  To fix it, send a `PUT` with a `redirect_uris` list that fits, as shown under [Create an application with the API](./index.md#create-an-application-with-the-api).
  In the web UI, open the application and remove entries from **Redirect URIs** until the list fits.
- A cleartext `http://` redirect URI to a host that is not local is rejected.
  Earlier versions accepted one through the management API for a confidential application, although Dynamic Client Registration always refused it.
  An application that stored one keeps working, but it cannot be saved again until its list uses `https://` or a local host, as described under [Callback URL schemes](./callback-url-schemes.md).
- A redirect URI with a private-use scheme must name a path or an authority, as described under [Callback URL schemes](./callback-url-schemes.md).
  Dynamic Client Registration accepted the bare form, such as `com.example.app:callback`, in earlier versions.
  A client that registered one can re-register with an accepted form, or an administrator can correct it with the same `PUT`.

## Upgrade notes

These changes are on `main` and have not shipped in a release yet.
This section will name the version once one ships.

### Upgrade from `callback_url` to `redirect_uris`

The `redirect_uris` list is now the source of truth for an application's callbacks, and its first entry is the primary:

- Earlier versions stored the primary in a separate `callback_url` field. The upgrade migration rewrites every application so the list starts with that value.
- During a rolling upgrade, a replica running an earlier version still writes only the old field when an administrator edits the callback URL. Replicas running the new version read the list instead, so the edit is silently lost: the application keeps its old callback URL and keeps accepting every previous redirect URI, including any the administrator meant to remove.
- Replicas running the new version also show the old callback URL in the form, so saving the application unchanged does not restore the edit.
- Drain replicas running the earlier version before you upgrade. If a callback URL was edited during the upgrade, enter the intended value again and save the application once every replica runs the new version.

### Refresh `scope` narrowing is now enforced

A `scope` on a refresh request was parsed and discarded in earlier versions, so a client sending one wider than its grant refreshed successfully.
It is now enforced, and such a request answers HTTP 400 with `error=invalid_scope`.
The refresh token is not consumed, so a client that drops the parameter or asks for less recovers without re-authorizing.

### Refresh and revocation now require `client_secret`

Earlier versions did not check `client_secret` on a refresh or at the RFC 7009 revocation endpoint, so a confidential client could refresh or revoke with a wrong secret or none.
Both now authenticate confidential clients exactly as the authorization code grant does, and a request without a valid secret answers HTTP 401 with `error=invalid_client`.
The refresh token is not consumed and nothing is revoked, so a client that adds its secret recovers without re-authorizing.
Public clients are unaffected.

### Dynamic Client Registration `scope` enforcement

Coder now enforces the `scope` an application declared for itself when it self-registered through [Dynamic Client Registration](./index.md#dynamic-client-registration).
This affects only deployments that enabled Dynamic Client Registration and have an application that self-registered with a `scope`.
Dynamic Client Registration is disabled by default, so if you never enabled it, nothing changes for you.
Turning it back off does not clear the check: Coder validates the stored `scope` of an existing application whether or not registration is still allowed, so an application that self-registered before you turned the setting off is affected too.

Earlier versions of Coder accepted any `scope` at registration without checking it, and every token for that application had full access.
Coder now treats the registered `scope` as the list of scopes the application is allowed to request, as described under [Scopes](./scopes.md).
Applications that self-registered without a `scope`, and applications created through the web UI or the management API without one, have no scope list and are not affected; they continue to receive full access.

An affected application fails in the following ways:

- A request for a scope name this deployment does not offer fails with `invalid_scope`.
- If none of the registered names are offered, every authorization fails, even one that leaves `scope` out.
- Authorization codes issued before the upgrade fail at the token endpoint with `invalid_grant` until they expire.

For the full error details, refer to ["invalid_scope" returned to your callback](./troubleshooting.md#invalid_scope-returned-to-your-callback) and ["invalid_grant" for a scope the deployment cannot mint](./troubleshooting.md#invalid_grant-for-a-scope-the-deployment-cannot-mint).

To fix an affected application, the party that holds its `registration_access_token` updates the registration with `PUT /oauth2/clients/{client_id}`, so that `scope` lists only names from `scopes_supported` in `GET /.well-known/oauth-authorization-server`.
If that token is lost, register the application again.
A Coder administrator can also fix it from the management API by [updating the application](../../../reference/api/enterprise.md#update-oauth2-application) with a `scope` that lists supported names, or with an empty `scope` to remove the allowlist.
The **Allowed scopes** field on the application page in the web UI makes the same change.

## Learn more

- Review [OAuth2 provider](./index.md) for enabling the provider and creating an application
- Review [Integration patterns](./integration-patterns.md) for client authentication methods and flows
- Check [Troubleshooting](./troubleshooting.md) if you hit an error
- Refer to [Security Best Practices](../../security/index.md) for deployment security guidance
