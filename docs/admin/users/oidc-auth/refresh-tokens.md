# Configure OIDC refresh tokens

OIDC refresh tokens allow your Coder deployment to maintain user sessions beyond the initial access token expiration.
Without properly configured refresh tokens, users will be automatically logged out when their access token expires.
This is typically after one hour, but varies by provider, and can disrupt the user's workflow.

> [!IMPORTANT]
> Misconfigured refresh tokens can lead to frequent user authentication prompts.
>
> After the admin enables refresh tokens, all existing users must log out and back in again to obtain a refresh token.

<div class="tabs">

<!-- markdownlint-disable MD001 -->

### Azure AD

Go to the Azure Portal > **Azure Active Directory** > **App registrations** > Your Coder app and make the following changes:

1. In the **Authentication** tab:

   - **Platform configuration** > Web
   - Ensure **Allow public client flows** is `No` (Coder is confidential)
   - **Implicit grant / hybrid flows** can stay unchecked

1. In the **API permissions** tab:

   - Add the built-in permission `offline_access` under **Microsoft Graph** > **Delegated permissions**
   - Keep `openid`, `profile`, and `email`

1. In the **Certificates & secrets** tab:

   - Verify a Client secret (or certificate) is valid.
     Coder uses it to redeem refresh tokens.

1. In your [Coder configuration](../../../reference/cli/server.md#--oidc-auth-url-params), request the same scopes:

   ```env
   CODER_OIDC_SCOPES=openid,profile,email,offline_access
   ```

1. Restart Coder and have users log out and back again for the changes to take effect.

   Alternatively, you can force a sign-out for all users with the
   [sign-out request process](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc#send-a-sign-out-request).

1. Azure issues rolling refresh tokens with a default absolute expiration of 90 days and inactivity expiration of 24 hours.

   You can adjust these settings under **Authentication methods** > **Token lifetime** (or use Conditional-Access policies in Entra ID).

You don't need to configure the 'Expose an API' section for refresh tokens to work.

Learn more in the [Microsoft Entra documentation](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc#enable-id-tokens).

### Google

To ensure Coder receives a refresh token when users authenticate with Google directly, set the `prompt` to `consent`
in the auth URL parameters (`CODER_OIDC_AUTH_URL_PARAMS`).
Without this, users will be logged out when their access token expires.

In your [Coder configuration](../../../reference/cli/server.md#--oidc-auth-url-params):

```env
CODER_OIDC_SCOPES=openid,profile,email
CODER_OIDC_AUTH_URL_PARAMS='{"access_type": "offline", "prompt": "consent"}'
```

### Keycloak

The `access_type` parameter has two possible values: `online` and `offline`.
By default, the value is set to `offline`.

This means that when a user authenticates using OIDC, the application requests offline access to the user's resources,
including the ability to refresh access tokens without requiring the user to reauthenticate.

Add the `offline_access` scope to enable refresh tokens in your
[Coder configuration](../../../reference/cli/server.md#--oidc-auth-url-params):

```env
CODER_OIDC_SCOPES=openid,profile,email,offline_access
CODER_OIDC_AUTH_URL_PARAMS='{"access_type":"offline"}'
```

### PingFederate

1. In PingFederate go to **Applications** > **OAuth Clients** > Your Coder client.

1. On the **Client** tab:

   - **Grant Types**: Enable `refresh_token`
   - **Allowed Scopes**: Add `offline_access` and keep `openid`, `profile`, and `email`

1. Optionally, in **Token Settings**

   - **Refresh Token Lifetime**: set a value that matches your security policy. Ping's default is 30 days.
   - **Idle Timeout**: ensure it's more than or equal to the lifetime of the access token so that refreshes don't fail prematurely.

1. Save your changes in PingFederate.

1. In your [Coder configuration](../../../reference/cli/server.md#--oidc-scopes), add the `offline_access` scope:

   ```env
   CODER_OIDC_SCOPES=openid,profile,email,offline_access
   ```

1. Restart your Coder deployment to apply these changes.

Users must log out and log in once to store their new refresh tokens.
After that, sessions should last until the Ping Federate refresh token expires.

Learn more in the [PingFederate documentation](https://docs.pingidentity.com/pingfederate/12.2/administrators_reference_guide/pf_configuring_oauth_clients.html).

</div>

## Confirm refresh token configuration

To verify refresh tokens are working correctly:

1. Check that your OIDC configuration includes the required refresh token parameters:

     - `offline_access` scope for most providers
     - `"access_type": "offline"` for Google

1. Verify provider-specific token configuration:

   <div class="tabs">

   ### Azure AD

   Use [jwt.ms](https://jwt.ms) to inspect the `id_token` and ensure the `rt_hash` claim is present.
   This shows that a refresh token was issued.

   ### Google

   If users are still being logged out periodically, check your client configuration in Google Cloud Console.

   ### Keycloak

   Review Keycloak sessions for the presence of refresh tokens.

   ### Ping Federate

   - Verify the client sent `offline_access` in the `grantedScopes` portion of the ID token.
   - Confirm `refresh_token` appears in the `grant_types` list returned by `/pf-admin-api/v1/oauth/clients/{id}`.

   </div>

1. Verify users can stay logged in beyond the identity provider's access token expiration period (typically 1 hour).

1. Monitor Coder logs for `failed to renew OIDC token: token has expired` messages.
   There should not be any.

If all verification steps pass successfully, your refresh token configuration is working properly.

## Troubleshooting OIDC Refresh Tokens

### Users are logged out too frequently

**Symptoms**:

- Users experience session timeouts and must re-authenticate.
- Session timeouts typically occur after the access token expiration period (varies by provider, commonly 1 hour).

**Causes**:

- Missing required refresh token configuration:
  - `offline_access` scope for most providers
  - `"access_type": "offline"` for Google
- Provider not correctly configured to issue refresh tokens.
- User has not logged in since refresh token configuration was added.

**Solution**:

- For most providers, add `offline_access` to your `CODER_OIDC_SCOPES` configuration.
  - `"access_type": "offline"` for Google
- Configure your identity provider according to the provider-specific instructions above.
- Have users log out and log in again to obtain refresh tokens.
  Look for entries containing `failed to renew OIDC token` which might indicate specific provider issues.

### Refresh tokens don't work after configuration change

**Symptoms**:

- Session timeouts continue despite refresh token configuration and users re-authenticating.
- Some users experience frequent logouts.

**Cause**:

- Existing user sessions don't have refresh tokens stored.
- Configuration may be incomplete.

**Solution**:

- Users must log out and log in again to get refresh tokens stored in the database.
- Verify you've correctly configured your provider as described in the configuration steps above.
- Check Coder logs for specific error messages related to token refresh.

Users might get logged out again before the new configuration takes effect completely.
