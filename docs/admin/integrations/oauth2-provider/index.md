---
title: OAuth2 provider
---

> [!NOTE]
> The OAuth2 provider is generally available and off by default.
> Set `CODER_OAUTH2_PROVIDER_ENABLE=true` to turn it on.
> The `oauth2` experiment has been removed.

Coder can act as an OAuth2 authorization server, allowing third-party applications to authenticate users through Coder and access the Coder API on their behalf.
This enables integrations where external applications can leverage Coder's authentication and user management.

## Requirements

- Admin privileges in Coder
- `CODER_OAUTH2_PROVIDER_ENABLE=true` set on the control plane
- HTTPS recommended for production deployments

## Enable OAuth2 provider

The provider is off by default.
While it is off, the OAuth2 endpoints and discovery documents return 404 and the **OAuth2 Applications** page is hidden.
Turn it on with the CLI flag:

```sh
coder server --oauth2-provider-enable
```

Or set the environment variable:

```dotenv
CODER_OAUTH2_PROVIDER_ENABLE=true
```

Or set it in the YAML configuration file:

```yaml
oauth2:
  provider:
    enable: true
```

For Kubernetes deployments that use the Helm chart, add the environment variable to `coder.env` in your values file:

```yaml
coder:
  env:
    - name: CODER_OAUTH2_PROVIDER_ENABLE
      value: "true"
```

Existing applications, secrets, and user authorizations are kept while the provider is off and work again when you turn it on.
Turning the provider off does not invalidate access tokens it already issued.
Those tokens keep authenticating to the regular Coder API while the OAuth2 refresh and revocation endpoints return 404.
Treat the setting as a way to stop new authorizations rather than as a way to revoke access, and delete the application before you disable the provider if you need to cut off every user's existing tokens too.

## Create OAuth2 applications

<a id="creating-oauth2-applications"></a>

### Create an application in the web UI

<a id="method-1-web-ui"></a>

1. Navigate to **Deployment Settings** > **OAuth2 Applications**.
2. On the **Applications** tab, select **Add application**.
3. Fill in the application details:
   - **Name**: Your application name
   - **Default callback**: `https://yourapp.example.com/callback` (web) or `myapp://callback` (native/desktop). Select **Add redirect URI** for additional callback URLs.
   - **Icon**: Optional icon URL
   - **Allowed scopes**: Optional. Refer to [Scopes](./scopes.md).
4. Select **Create application**.

Coder creates the application and takes you to its details page, which prompts you to generate a client secret.

### Create an application with the API

<a id="method-2-management-api"></a>

Create an application using the Coder API:

```sh
curl -X POST \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Application",
    "redirect_uris": [
      "https://myapp.example.com/callback",
      "http://localhost:8080/callback"
    ],
    "icon": "https://myapp.example.com/icon.png"
  }' \
  "$CODER_URL/api/v2/oauth2-provider/apps"
```

`callback_url` is still accepted and still returned, but it is deprecated: it is equal to the first entry in `redirect_uris`. New scripts should send and read `redirect_uris` instead.

Update an application with `PUT`. Fetch it first and edit the fields you want to change, then send the result back:

```sh
curl -X PUT \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Application",
    "redirect_uris": ["https://myapp.example.com/callback"],
    "icon": "https://myapp.example.com/icon.png"
  }' \
  "$CODER_URL/api/v2/oauth2-provider/apps/$APP_ID"
```

`name` is required on every `PUT`, and `icon` is cleared if you leave it out.
`redirect_uris` replaces the stored list when present and keeps it when omitted.
`scope` is kept when omitted; refer to [Scopes](./scopes.md) for how to change it.

Add an optional `scope` field to restrict which scopes the application's clients may request.
Refer to [Scopes](./scopes.md) for how the allowlist is applied and how to change it later.

Generate a client secret:

```sh
curl -X POST \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/oauth2-provider/apps/$APP_ID/secrets"
```

The response includes `client_secret_full`, the plaintext secret. Save it now: later reads of this application return only a truncated version.

Every client, whichever method you used to create its application, must complete PKCE (Proof Key for Code Exchange) to exchange a code for a token; refer to [PKCE Flow](./integration-patterns.md#pkce-flow-required) before you start integrating.

## Dynamic Client Registration

Dynamic Client Registration ([RFC 7591](https://datatracker.ietf.org/doc/html/rfc7591)) lets a client register itself against `/oauth2/register` instead of an admin creating the application manually. It's **disabled by default**; an owner must turn it on before any client can self-register.

Change the setting in the web UI:

1. Navigate to **Deployment Settings** > **OAuth2 Applications**.
2. Select the **Settings** tab.
3. Select **Enable** or **Disable** next to **Dynamic Client Registration**.

Enabling asks you to confirm first.
Disabling does not.
The tab is linkable directly at `https://$CODER_ACCESS_URL/deployment/oauth2-provider/apps?tab=settings`.

Viewing the tab requires permission to view deployment configuration, and changing the setting requires permission to edit it.
Without edit permission the button is present but inactive, and the page says why.

Check or change the setting with the CLI:

```sh
coder oauth2-provider dcr enable
coder oauth2-provider dcr disable
```

Or with the management API:

```sh
curl -X PUT \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dynamic_client_registration_enabled": true}' \
  "$CODER_URL/api/v2/oauth2-provider/settings"
```

```sh
curl -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/oauth2-provider/settings"
```

Disabling only blocks *new* self-registrations. Applications that already
registered while it was enabled keep authorizing and exchanging tokens
normally; disabling does not revoke or otherwise affect them.

An application may list several `redirect_uris`, whether it registered itself or an admin created it.
A request may present any of them, and the code it receives can only be exchanged with that same URI.
The first entry is the primary callback: it is what the web UI shows for the application, and what a request that omits `redirect_uri` is sent to.
An admin can edit the list through the management API, and a self-registered client can update its own list with its registration access token.
The admin `PUT` also validates the stored name, so a self-registered client whose name has leading or trailing whitespace can only be updated with its registration access token.

<a id="next-steps"></a>

## Learn more

<a id="integration-patterns"></a>

- [Integration patterns](./integration-patterns.md): client authentication methods, the PKCE flow, and discovery endpoints
  - <a id="client-authentication-methods"></a>[Client authentication methods](./integration-patterns.md#client-authentication-methods)
  - <a id="standard-oauth2-flow"></a><a id="pkce-flow-required"></a>[PKCE flow](./integration-patterns.md#pkce-flow-required)
  - <a id="discovery-endpoints"></a><a id="standards-compliance"></a>[Discovery endpoints](./integration-patterns.md#discovery-endpoints)

<a id="scopes"></a>

- [Scopes](./scopes.md): how access is bounded

<a id="token-management"></a>

- [Token management](./token-management.md): refresh, revocation, and deletion
  - <a id="refresh-tokens"></a>[Refresh tokens](./token-management.md#refresh-tokens)
  - <a id="revoke-a-token"></a>[Revoke a token](./token-management.md#revoke-a-token)
  - <a id="revoke-access"></a>[Revoke your authorization](./token-management.md#revoke-your-authorization-for-an-application)
  - <a id="delete-an-application"></a>[Delete an application](./token-management.md#delete-an-application)

<a id="callback-url-schemes"></a>

- [Callback URL schemes](./callback-url-schemes.md): accepted redirect URIs

<a id="common-issues"></a><a id="oauth2-endpoints-return-404"></a><a id="invalid-redirect_uri"></a><a id="invalid-callback-url-on-the-consent-page"></a><a id="invalid_scope-returned-to-your-callback"></a><a id="invalid_grant-for-a-scope-the-deployment-cannot-mint"></a><a id="invalid_scope-for-a-refresh-that-names-a-scope"></a><a id="invalid_client-for-a-refresh-or-a-revocation"></a><a id="invalid_request-for-client_secret-in-the-query-string"></a><a id="unsupported_response_type-returned-to-your-callback"></a><a id="invalid_request-for-code_challenge_method"></a><a id="invalid_request-for-a-rejected-parameter"></a><a id="invalid_request-from-post-oauth2tokens-for-a-repeated-parameter"></a><a id="invalid_target-for-a-rejected-resource"></a><a id="pkce-verification-failed"></a><a id="public-clients-may-not-use-the-mailtotelsms-scheme"></a>

- [Troubleshooting](./troubleshooting.md): look up an error by its exact code or message

- [Security and limitations](./security.md): security considerations, current limitations, and upgrade notes
  - <a id="security-considerations"></a>[Security considerations](./security.md#security-considerations)
  - <a id="limitations"></a>[Limitations](./security.md#limitations)

- Check [External Authentication](../../external-auth/index.md) for configuring Coder as an OAuth2 client
- Review the [API Reference](../../../reference/api/index.md) for complete endpoint documentation

## Feedback

Report issues and feedback through [GitHub Issues](https://github.com/coder/coder/issues) with the `oauth2` label.
