# Configure OIDC refresh tokens

<div class="tabs">

## Google

To ensure Coder receives a refresh token when users authenticate with Google
directly, set the `prompt` to `consent` in the auth URL parameters. Without
this, users will be logged out after 1 hour.

In your Coder configuration:

```shell
CODER_OIDC_AUTH_URL_PARAMS='{"access_type": "offline", "prompt": "consent"}'
```

## Keycloak

The `access_type` parameter has two possible values: `online` and `offline`.
By default, the value is set to `offline`.

This means that when a user authenticates using OIDC, the application requests
offline access to the user's resources, including the ability to refresh access
tokens without requiring the user to reauthenticate.

To enable the `offline_access` scope which allows for the refresh token
functionality, you need to add it to the list of requested scopes during the
authentication flow.
Including the `offline_access` scope in the requested scopes ensures that the
user is granted the necessary permissions to obtain refresh tokens.

By combining the `{"access_type":"offline"}` parameter in the OIDC Auth URL with
the `offline_access` scope, you can achieve the desired behavior of obtaining
refresh tokens for offline access to the user's resources.

</div>

## Troubleshooting OIDC refresh tokens

### Users Are Logged Out Every Hour

**Symptoms**: Users experience session timeouts approximately every hour and must re-authenticate
**Cause**: Missing `offline_access` scope in `CODER_OIDC_SCOPES`
**Solution**:

1. Add `offline_access` to your `CODER_OIDC_SCOPES` configuration
1. Restart your Coder deployment
1. All existing users must logout and login once to receive refresh tokens

### Refresh Tokens Not Working After Configuration Change

**Symptoms**: Hourly timeouts, even after adding `offline_access`
**Cause**: Existing user sessions don't have refresh tokens stored
**Solution**: Users must logout and login again to get refresh tokens stored in the database

### Verify Refresh Token Configuration

To confirm that refresh tokens are working correctly:

1. Check that `offline_access` is included in your `CODER_OIDC_SCOPES`
1. Verify users can stay logged in beyond Okta's access token lifetime (typically one hour)
1. Monitor Coder logs for any OIDC refresh errors during token renewal
