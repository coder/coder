# Keycloak Authentication with Coder

The access_type parameter has two possible values: "online" and "offline." By default, the value is set to "offline". This means that when a user authenticates using OIDC, the application requests offline access to the user's resources, including the ability to refresh access tokens without requiring the user to reauthenticate.

To enable the `offline_access` scope, which allows for the refresh token functionality, you need to add it to the list of requested scopes during the authentication flow. Including the `offline_access` scope in the requested scopes ensures that the user is granted the necessary permissions to obtain refresh tokens.

By combining the `{"access_type":"offline"}` parameter in the OIDC Auth URL with the `offline_access` scope, you can achieve the desired behavior of obtaining refresh tokens for offline access to the user's resources.
