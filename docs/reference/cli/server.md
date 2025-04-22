<!-- DO NOT EDIT | GENERATED CONTENT -->
# server

Start a Coder server

## Usage

```console
coder server [flags]
```

## Subcommands

| Name                                                                      | Purpose                                                                                                |
|---------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------|
| [<code>create-admin-user</code>](./server_create-admin-user.md)           | Create a new admin user with the given username, email and password and adds it to every organization. |
| [<code>postgres-builtin-url</code>](./server_postgres-builtin-url.md)     | Output the connection URL for the built-in PostgreSQL deployment.                                      |
| [<code>postgres-builtin-serve</code>](./server_postgres-builtin-serve.md) | Run the built-in PostgreSQL deployment.                                                                |
| [<code>dbcrypt</code>](./server_dbcrypt.md)                               | Manage database encryption.                                                                            |

## Options

### --access-url

|             |                                   |
|-------------|-----------------------------------|
| Type        | <code>url</code>                  |
| Environment | <code>$CODER_ACCESS_URL</code>    |
| YAML        | <code>networking.accessURL</code> |

The URL that users will use to access the Coder deployment.

### --wildcard-access-url

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_WILDCARD_ACCESS_URL</code>   |
| YAML        | <code>networking.wildcardAccessURL</code> |

Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".

### --docs-url

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>url</code>                    |
| Environment | <code>$CODER_DOCS_URL</code>        |
| YAML        | <code>networking.docsURL</code>     |
| Default     | <code>https://coder.com/docs</code> |

Specifies the custom docs URL.

### --redirect-to-access-url

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>bool</code>                           |
| Environment | <code>$CODER_REDIRECT_TO_ACCESS_URL</code>  |
| YAML        | <code>networking.redirectToAccessURL</code> |

Specifies whether to redirect requests that do not match the access URL host.

### --http-address

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_HTTP_ADDRESS</code>         |
| YAML        | <code>networking.http.httpAddress</code> |
| Default     | <code>127.0.0.1:3000</code>              |

HTTP bind address of the server. Unset to disable the HTTP endpoint.

### --tls-address

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>host:port</code>              |
| Environment | <code>$CODER_TLS_ADDRESS</code>     |
| YAML        | <code>networking.tls.address</code> |
| Default     | <code>127.0.0.1:3443</code>         |

HTTPS bind address of the server.

### --tls-enable

|             |                                    |
|-------------|------------------------------------|
| Type        | <code>bool</code>                  |
| Environment | <code>$CODER_TLS_ENABLE</code>     |
| YAML        | <code>networking.tls.enable</code> |

Whether TLS will be enabled.

### --tls-cert-file

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>string-array</code>             |
| Environment | <code>$CODER_TLS_CERT_FILE</code>     |
| YAML        | <code>networking.tls.certFiles</code> |

Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.

### --tls-client-ca-file

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_TLS_CLIENT_CA_FILE</code>   |
| YAML        | <code>networking.tls.clientCAFile</code> |

PEM-encoded Certificate Authority file used for checking the authenticity of client.

### --tls-client-auth

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_TLS_CLIENT_AUTH</code>    |
| YAML        | <code>networking.tls.clientAuth</code> |
| Default     | <code>none</code>                      |

Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".

### --tls-key-file

|             |                                      |
|-------------|--------------------------------------|
| Type        | <code>string-array</code>            |
| Environment | <code>$CODER_TLS_KEY_FILE</code>     |
| YAML        | <code>networking.tls.keyFiles</code> |

Paths to the private keys for each of the certificates. It requires a PEM-encoded file.

### --tls-min-version

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_TLS_MIN_VERSION</code>    |
| YAML        | <code>networking.tls.minVersion</code> |
| Default     | <code>tls12</code>                     |

Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13".

### --tls-client-cert-file

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>string</code>                        |
| Environment | <code>$CODER_TLS_CLIENT_CERT_FILE</code>   |
| YAML        | <code>networking.tls.clientCertFile</code> |

Path to certificate for client TLS authentication. It requires a PEM-encoded file.

### --tls-client-key-file

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_TLS_CLIENT_KEY_FILE</code>   |
| YAML        | <code>networking.tls.clientKeyFile</code> |

Path to key for client TLS authentication. It requires a PEM-encoded file.

### --tls-ciphers

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>string-array</code>              |
| Environment | <code>$CODER_TLS_CIPHERS</code>        |
| YAML        | <code>networking.tls.tlsCiphers</code> |

Specify specific TLS ciphers that allowed to be used. See https://github.com/golang/go/blob/master/src/crypto/tls/cipher_suites.go#L53-L75.

### --tls-allow-insecure-ciphers

|             |                                                     |
|-------------|-----------------------------------------------------|
| Type        | <code>bool</code>                                   |
| Environment | <code>$CODER_TLS_ALLOW_INSECURE_CIPHERS</code>      |
| YAML        | <code>networking.tls.tlsAllowInsecureCiphers</code> |
| Default     | <code>false</code>                                  |

By default, only ciphers marked as 'secure' are allowed to be used. See https://github.com/golang/go/blob/master/src/crypto/tls/cipher_suites.go#L82-L95.

### --derp-server-enable

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_DERP_SERVER_ENABLE</code> |
| YAML        | <code>networking.derp.enable</code>    |
| Default     | <code>true</code>                      |

Whether to enable or disable the embedded DERP relay server.

### --derp-server-region-name

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_DERP_SERVER_REGION_NAME</code> |
| YAML        | <code>networking.derp.regionName</code>     |
| Default     | <code>Coder Embedded Relay</code>           |

Region name that for the embedded DERP server.

### --derp-server-stun-addresses

|             |                                                                                                                                          |
|-------------|------------------------------------------------------------------------------------------------------------------------------------------|
| Type        | <code>string-array</code>                                                                                                                |
| Environment | <code>$CODER_DERP_SERVER_STUN_ADDRESSES</code>                                                                                           |
| YAML        | <code>networking.derp.stunAddresses</code>                                                                                               |
| Default     | <code>stun.l.google.com:19302,stun1.l.google.com:19302,stun2.l.google.com:19302,stun3.l.google.com:19302,stun4.l.google.com:19302</code> |

Addresses for STUN servers to establish P2P connections. It's recommended to have at least two STUN servers to give users the best chance of connecting P2P to workspaces. Each STUN server will get it's own DERP region, with region IDs starting at `--derp-server-region-id + 1`. Use special value 'disable' to turn off STUN completely.

### --derp-server-relay-url

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>url</code>                          |
| Environment | <code>$CODER_DERP_SERVER_RELAY_URL</code> |
| YAML        | <code>networking.derp.relayURL</code>     |

An HTTP URL that is accessible by other replicas to relay DERP traffic. Required for high availability.

### --block-direct-connections

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>bool</code>                        |
| Environment | <code>$CODER_BLOCK_DIRECT</code>         |
| YAML        | <code>networking.derp.blockDirect</code> |

Block peer-to-peer (aka. direct) workspace connections. All workspace connections from the CLI will be proxied through Coder (or custom configured DERP servers) and will never be peer-to-peer when enabled. Workspaces may still reach out to STUN servers to get their address until they are restarted after this change has been made, but new connections will still be proxied regardless.

### --derp-force-websockets

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>bool</code>                            |
| Environment | <code>$CODER_DERP_FORCE_WEBSOCKETS</code>    |
| YAML        | <code>networking.derp.forceWebSockets</code> |

Force clients and agents to always use WebSocket to connect to DERP relay servers. By default, DERP uses `Upgrade: derp`, which may cause issues with some reverse proxies. Clients may automatically fallback to WebSocket if they detect an issue with `Upgrade: derp`, but this does not work in all situations.

### --derp-config-url

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_DERP_CONFIG_URL</code> |
| YAML        | <code>networking.derp.url</code>    |

URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/.

### --derp-config-path

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_DERP_CONFIG_PATH</code>    |
| YAML        | <code>networking.derp.configPath</code> |

Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/.

### --prometheus-enable

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>bool</code>                            |
| Environment | <code>$CODER_PROMETHEUS_ENABLE</code>        |
| YAML        | <code>introspection.prometheus.enable</code> |

Serve prometheus metrics on the address defined by prometheus address.

### --prometheus-address

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>host:port</code>                        |
| Environment | <code>$CODER_PROMETHEUS_ADDRESS</code>        |
| YAML        | <code>introspection.prometheus.address</code> |
| Default     | <code>127.0.0.1:2112</code>                   |

The bind address to serve prometheus metrics.

### --prometheus-collect-agent-stats

|             |                                                           |
|-------------|-----------------------------------------------------------|
| Type        | <code>bool</code>                                         |
| Environment | <code>$CODER_PROMETHEUS_COLLECT_AGENT_STATS</code>        |
| YAML        | <code>introspection.prometheus.collect_agent_stats</code> |

Collect agent stats (may increase charges for metrics storage).

### --prometheus-aggregate-agent-stats-by

|             |                                                                |
|-------------|----------------------------------------------------------------|
| Type        | <code>string-array</code>                                      |
| Environment | <code>$CODER_PROMETHEUS_AGGREGATE_AGENT_STATS_BY</code>        |
| YAML        | <code>introspection.prometheus.aggregate_agent_stats_by</code> |
| Default     | <code>agent_name,template_name,username,workspace_name</code>  |

When collecting agent stats, aggregate metrics by a given set of comma-separated labels to reduce cardinality. Accepted values are agent_name, template_name, username, workspace_name.

### --prometheus-collect-db-metrics

|             |                                                          |
|-------------|----------------------------------------------------------|
| Type        | <code>bool</code>                                        |
| Environment | <code>$CODER_PROMETHEUS_COLLECT_DB_METRICS</code>        |
| YAML        | <code>introspection.prometheus.collect_db_metrics</code> |
| Default     | <code>false</code>                                       |

Collect database query metrics (may increase charges for metrics storage). If set to false, a reduced set of database metrics are still collected.

### --pprof-enable

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>bool</code>                       |
| Environment | <code>$CODER_PPROF_ENABLE</code>        |
| YAML        | <code>introspection.pprof.enable</code> |

Serve pprof metrics on the address defined by pprof address.

### --pprof-address

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>host:port</code>                   |
| Environment | <code>$CODER_PPROF_ADDRESS</code>        |
| YAML        | <code>introspection.pprof.address</code> |
| Default     | <code>127.0.0.1:6060</code>              |

The bind address to serve pprof.

### --oauth2-github-client-id

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_OAUTH2_GITHUB_CLIENT_ID</code> |
| YAML        | <code>oauth2.github.clientID</code>         |

Client ID for Login with GitHub.

### --oauth2-github-client-secret

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>string</code>                             |
| Environment | <code>$CODER_OAUTH2_GITHUB_CLIENT_SECRET</code> |

Client secret for Login with GitHub.

### --oauth2-github-device-flow

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>bool</code>                             |
| Environment | <code>$CODER_OAUTH2_GITHUB_DEVICE_FLOW</code> |
| YAML        | <code>oauth2.github.deviceFlow</code>         |
| Default     | <code>false</code>                            |

Enable device flow for Login with GitHub.

### --oauth2-github-default-provider-enable

|             |                                                           |
|-------------|-----------------------------------------------------------|
| Type        | <code>bool</code>                                         |
| Environment | <code>$CODER_OAUTH2_GITHUB_DEFAULT_PROVIDER_ENABLE</code> |
| YAML        | <code>oauth2.github.defaultProviderEnable</code>          |
| Default     | <code>true</code>                                         |

Enable the default GitHub OAuth2 provider managed by Coder.

### --oauth2-github-allowed-orgs

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>string-array</code>                      |
| Environment | <code>$CODER_OAUTH2_GITHUB_ALLOWED_ORGS</code> |
| YAML        | <code>oauth2.github.allowedOrgs</code>         |

Organizations the user must be a member of to Login with GitHub.

### --oauth2-github-allowed-teams

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>string-array</code>                       |
| Environment | <code>$CODER_OAUTH2_GITHUB_ALLOWED_TEAMS</code> |
| YAML        | <code>oauth2.github.allowedTeams</code>         |

Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.

### --oauth2-github-allow-signups

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>bool</code>                               |
| Environment | <code>$CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS</code> |
| YAML        | <code>oauth2.github.allowSignups</code>         |

Whether new users can sign up with GitHub.

### --oauth2-github-allow-everyone

|             |                                                  |
|-------------|--------------------------------------------------|
| Type        | <code>bool</code>                                |
| Environment | <code>$CODER_OAUTH2_GITHUB_ALLOW_EVERYONE</code> |
| YAML        | <code>oauth2.github.allowEveryone</code>         |

Allow all logins, setting this option means allowed orgs and teams must be empty.

### --oauth2-github-enterprise-base-url

|             |                                                       |
|-------------|-------------------------------------------------------|
| Type        | <code>string</code>                                   |
| Environment | <code>$CODER_OAUTH2_GITHUB_ENTERPRISE_BASE_URL</code> |
| YAML        | <code>oauth2.github.enterpriseBaseURL</code>          |

Base URL of a GitHub Enterprise deployment to use for Login with GitHub.

### --oidc-allow-signups

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_OIDC_ALLOW_SIGNUPS</code> |
| YAML        | <code>oidc.allowSignups</code>         |
| Default     | <code>true</code>                      |

Whether new users can sign up with OIDC.

### --oidc-client-id

|             |                                    |
|-------------|------------------------------------|
| Type        | <code>string</code>                |
| Environment | <code>$CODER_OIDC_CLIENT_ID</code> |
| YAML        | <code>oidc.clientID</code>         |

Client ID to use for Login with OIDC.

### --oidc-client-secret

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_OIDC_CLIENT_SECRET</code> |

Client secret to use for Login with OIDC.

### --oidc-client-key-file

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_OIDC_CLIENT_KEY_FILE</code> |
| YAML        | <code>oidc.oidcClientKeyFile</code>      |

Pem encoded RSA private key to use for oauth2 PKI/JWT authorization. This can be used instead of oidc-client-secret if your IDP supports it.

### --oidc-client-cert-file

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_OIDC_CLIENT_CERT_FILE</code> |
| YAML        | <code>oidc.oidcClientCertFile</code>      |

Pem encoded certificate file to use for oauth2 PKI/JWT authorization. The public certificate that accompanies oidc-client-key-file. A standard x509 certificate is expected.

### --oidc-email-domain

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>string-array</code>             |
| Environment | <code>$CODER_OIDC_EMAIL_DOMAIN</code> |
| YAML        | <code>oidc.emailDomain</code>         |

Email domains that clients logging in with OIDC must match.

### --oidc-issuer-url

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_OIDC_ISSUER_URL</code> |
| YAML        | <code>oidc.issuerURL</code>         |

Issuer URL to use for Login with OIDC.

### --oidc-scopes

|             |                                   |
|-------------|-----------------------------------|
| Type        | <code>string-array</code>         |
| Environment | <code>$CODER_OIDC_SCOPES</code>   |
| YAML        | <code>oidc.scopes</code>          |
| Default     | <code>openid,profile,email</code> |

Scopes to grant when authenticating with OIDC.

### --oidc-ignore-email-verified

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_OIDC_IGNORE_EMAIL_VERIFIED</code> |
| YAML        | <code>oidc.ignoreEmailVerified</code>          |

Ignore the email_verified claim from the upstream provider.

### --oidc-username-field

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_OIDC_USERNAME_FIELD</code> |
| YAML        | <code>oidc.usernameField</code>         |
| Default     | <code>preferred_username</code>         |

OIDC claim field to use as the username.

### --oidc-name-field

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_OIDC_NAME_FIELD</code> |
| YAML        | <code>oidc.nameField</code>         |
| Default     | <code>name</code>                   |

OIDC claim field to use as the name.

### --oidc-email-field

|             |                                      |
|-------------|--------------------------------------|
| Type        | <code>string</code>                  |
| Environment | <code>$CODER_OIDC_EMAIL_FIELD</code> |
| YAML        | <code>oidc.emailField</code>         |
| Default     | <code>email</code>                   |

OIDC claim field to use as the email.

### --oidc-auth-url-params

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>struct[map[string]string]</code>   |
| Environment | <code>$CODER_OIDC_AUTH_URL_PARAMS</code> |
| YAML        | <code>oidc.authURLParams</code>          |
| Default     | <code>{"access_type": "offline"}</code>  |

OIDC auth URL parameters to pass to the upstream provider.

### --oidc-ignore-userinfo

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>bool</code>                        |
| Environment | <code>$CODER_OIDC_IGNORE_USERINFO</code> |
| YAML        | <code>oidc.ignoreUserInfo</code>         |
| Default     | <code>false</code>                       |

Ignore the userinfo endpoint and only use the ID token for user information.

### --oidc-group-field

|             |                                      |
|-------------|--------------------------------------|
| Type        | <code>string</code>                  |
| Environment | <code>$CODER_OIDC_GROUP_FIELD</code> |
| YAML        | <code>oidc.groupField</code>         |

This field must be set if using the group sync feature and the scope name is not 'groups'. Set to the claim to be used for groups.

### --oidc-group-mapping

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>struct[map[string]string]</code> |
| Environment | <code>$CODER_OIDC_GROUP_MAPPING</code> |
| YAML        | <code>oidc.groupMapping</code>         |
| Default     | <code>{}</code>                        |

A map of OIDC group IDs and the group in Coder it should map to. This is useful for when OIDC providers only return group IDs.

### --oidc-group-auto-create

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>bool</code>                          |
| Environment | <code>$CODER_OIDC_GROUP_AUTO_CREATE</code> |
| YAML        | <code>oidc.enableGroupAutoCreate</code>    |
| Default     | <code>false</code>                         |

Automatically creates missing groups from a user's groups claim.

### --oidc-group-regex-filter

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>regexp</code>                         |
| Environment | <code>$CODER_OIDC_GROUP_REGEX_FILTER</code> |
| YAML        | <code>oidc.groupRegexFilter</code>          |
| Default     | <code>.*</code>                             |

If provided any group name not matching the regex is ignored. This allows for filtering out groups that are not needed. This filter is applied after the group mapping.

### --oidc-allowed-groups

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string-array</code>               |
| Environment | <code>$CODER_OIDC_ALLOWED_GROUPS</code> |
| YAML        | <code>oidc.groupAllowed</code>          |

If provided any group name not in the list will not be allowed to authenticate. This allows for restricting access to a specific set of groups. This filter is applied after the group mapping and before the regex filter.

### --oidc-user-role-field

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_OIDC_USER_ROLE_FIELD</code> |
| YAML        | <code>oidc.userRoleField</code>          |

This field must be set if using the user roles sync feature. Set this to the name of the claim used to store the user's role. The roles should be sent as an array of strings.

### --oidc-user-role-mapping

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>struct[map[string][]string]</code>   |
| Environment | <code>$CODER_OIDC_USER_ROLE_MAPPING</code> |
| YAML        | <code>oidc.userRoleMapping</code>          |
| Default     | <code>{}</code>                            |

A map of the OIDC passed in user roles and the groups in Coder it should map to. This is useful if the group names do not match. If mapped to the empty string, the role will ignored.

### --oidc-user-role-default

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>string-array</code>                  |
| Environment | <code>$CODER_OIDC_USER_ROLE_DEFAULT</code> |
| YAML        | <code>oidc.userRoleDefault</code>          |

If user role sync is enabled, these roles are always included for all authenticated users. The 'member' role is always assigned.

### --oidc-sign-in-text

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_OIDC_SIGN_IN_TEXT</code> |
| YAML        | <code>oidc.signInText</code>          |
| Default     | <code>OpenID Connect</code>           |

The text to show on the OpenID Connect sign in button.

### --oidc-icon-url

|             |                                   |
|-------------|-----------------------------------|
| Type        | <code>url</code>                  |
| Environment | <code>$CODER_OIDC_ICON_URL</code> |
| YAML        | <code>oidc.iconURL</code>         |

URL pointing to the icon to use on the OpenID Connect login button.

### --oidc-signups-disabled-text

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>string</code>                            |
| Environment | <code>$CODER_OIDC_SIGNUPS_DISABLED_TEXT</code> |
| YAML        | <code>oidc.signupsDisabledText</code>          |

The custom text to show on the error page informing about disabled OIDC signups. Markdown format is supported.

### --dangerous-oidc-skip-issuer-checks

|             |                                                       |
|-------------|-------------------------------------------------------|
| Type        | <code>bool</code>                                     |
| Environment | <code>$CODER_DANGEROUS_OIDC_SKIP_ISSUER_CHECKS</code> |
| YAML        | <code>oidc.dangerousSkipIssuerChecks</code>           |

OIDC issuer urls must match in the request, the id_token 'iss' claim, and in the well-known configuration. This flag disables that requirement, and can lead to an insecure OIDC configuration. It is not recommended to use this flag.

### --telemetry

|             |                                      |
|-------------|--------------------------------------|
| Type        | <code>bool</code>                    |
| Environment | <code>$CODER_TELEMETRY_ENABLE</code> |
| YAML        | <code>telemetry.enable</code>        |
| Default     | <code>true</code>                    |

Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.

### --trace

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>bool</code>                         |
| Environment | <code>$CODER_TRACE_ENABLE</code>          |
| YAML        | <code>introspection.tracing.enable</code> |

Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.

### --trace-honeycomb-api-key

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_TRACE_HONEYCOMB_API_KEY</code> |

Enables trace exporting to Honeycomb.io using the provided API Key.

### --trace-logs

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_TRACE_LOGS</code>                 |
| YAML        | <code>introspection.tracing.captureLogs</code> |

Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs.

### --provisioner-daemons

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>int</code>                        |
| Environment | <code>$CODER_PROVISIONER_DAEMONS</code> |
| YAML        | <code>provisioning.daemons</code>       |
| Default     | <code>3</code>                          |

Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.

### --provisioner-daemon-poll-interval

|             |                                                      |
|-------------|------------------------------------------------------|
| Type        | <code>duration</code>                                |
| Environment | <code>$CODER_PROVISIONER_DAEMON_POLL_INTERVAL</code> |
| YAML        | <code>provisioning.daemonPollInterval</code>         |
| Default     | <code>1s</code>                                      |

Deprecated and ignored.

### --provisioner-daemon-poll-jitter

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>duration</code>                              |
| Environment | <code>$CODER_PROVISIONER_DAEMON_POLL_JITTER</code> |
| YAML        | <code>provisioning.daemonPollJitter</code>         |
| Default     | <code>100ms</code>                                 |

Deprecated and ignored.

### --provisioner-force-cancel-interval

|             |                                                       |
|-------------|-------------------------------------------------------|
| Type        | <code>duration</code>                                 |
| Environment | <code>$CODER_PROVISIONER_FORCE_CANCEL_INTERVAL</code> |
| YAML        | <code>provisioning.forceCancelInterval</code>         |
| Default     | <code>10m0s</code>                                    |

Time to force cancel provisioning tasks that are stuck.

### --provisioner-daemon-psk

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>string</code>                        |
| Environment | <code>$CODER_PROVISIONER_DAEMON_PSK</code> |

Pre-shared key to authenticate external provisioner daemons to Coder server.

### -l, --log-filter

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>string-array</code>                 |
| Environment | <code>$CODER_LOG_FILTER</code>            |
| YAML        | <code>introspection.logging.filter</code> |

Filter debug logs by matching against a given regex. Use .* to match all debug logs.

### --log-human

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>string</code>                          |
| Environment | <code>$CODER_LOGGING_HUMAN</code>            |
| YAML        | <code>introspection.logging.humanPath</code> |
| Default     | <code>/dev/stderr</code>                     |

Output human-readable logs to a given file.

### --log-json

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_LOGGING_JSON</code>            |
| YAML        | <code>introspection.logging.jsonPath</code> |

Output JSON logs to a given file.

### --log-stackdriver

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>string</code>                                |
| Environment | <code>$CODER_LOGGING_STACKDRIVER</code>            |
| YAML        | <code>introspection.logging.stackdriverPath</code> |

Output Stackdriver compatible logs to a given file.

### --enable-terraform-debug-mode

|             |                                                             |
|-------------|-------------------------------------------------------------|
| Type        | <code>bool</code>                                           |
| Environment | <code>$CODER_ENABLE_TERRAFORM_DEBUG_MODE</code>             |
| YAML        | <code>introspection.logging.enableTerraformDebugMode</code> |
| Default     | <code>false</code>                                          |

Allow administrators to enable Terraform debug output.

### --additional-csp-policy

|             |                                                  |
|-------------|--------------------------------------------------|
| Type        | <code>string-array</code>                        |
| Environment | <code>$CODER_ADDITIONAL_CSP_POLICY</code>        |
| YAML        | <code>networking.http.additionalCSPPolicy</code> |

Coder configures a Content Security Policy (CSP) to protect against XSS attacks. This setting allows you to add additional CSP directives, which can open the attack surface of the deployment. Format matches the CSP directive format, e.g. --additional-csp-policy="script-src https://example.com".

### --dangerous-allow-path-app-sharing

|             |                                                      |
|-------------|------------------------------------------------------|
| Type        | <code>bool</code>                                    |
| Environment | <code>$CODER_DANGEROUS_ALLOW_PATH_APP_SHARING</code> |

Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.

### --dangerous-allow-path-app-site-owner-access

|             |                                                                |
|-------------|----------------------------------------------------------------|
| Type        | <code>bool</code>                                              |
| Environment | <code>$CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS</code> |

Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.

### --experiments

|             |                                 |
|-------------|---------------------------------|
| Type        | <code>string-array</code>       |
| Environment | <code>$CODER_EXPERIMENTS</code> |
| YAML        | <code>experiments</code>        |

Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.

### --update-check

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>bool</code>                |
| Environment | <code>$CODER_UPDATE_CHECK</code> |
| YAML        | <code>updateCheck</code>         |
| Default     | <code>false</code>               |

Periodically check for new releases of Coder and inform the owner. The check is performed once per day.

### --max-token-lifetime

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>duration</code>                         |
| Environment | <code>$CODER_MAX_TOKEN_LIFETIME</code>        |
| YAML        | <code>networking.http.maxTokenLifetime</code> |
| Default     | <code>876600h0m0s</code>                      |

The maximum lifetime duration users can specify when creating an API token.

### --default-token-lifetime

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>duration</code>                      |
| Environment | <code>$CODER_DEFAULT_TOKEN_LIFETIME</code> |
| YAML        | <code>defaultTokenLifetime</code>          |
| Default     | <code>168h0m0s</code>                      |

The default lifetime duration for API tokens. This value is used when creating a token without specifying a duration, such as when authenticating the CLI or an IDE plugin.

### --swagger-enable

|             |                                    |
|-------------|------------------------------------|
| Type        | <code>bool</code>                  |
| Environment | <code>$CODER_SWAGGER_ENABLE</code> |
| YAML        | <code>enableSwagger</code>         |

Expose the swagger endpoint via /swagger.

### --proxy-trusted-headers

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string-array</code>                   |
| Environment | <code>$CODER_PROXY_TRUSTED_HEADERS</code>   |
| YAML        | <code>networking.proxyTrustedHeaders</code> |

Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For.

### --proxy-trusted-origins

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string-array</code>                   |
| Environment | <code>$CODER_PROXY_TRUSTED_ORIGINS</code>   |
| YAML        | <code>networking.proxyTrustedOrigins</code> |

Origin addresses to respect "proxy-trusted-headers". e.g. 192.168.1.0/24.

### --cache-dir

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_CACHE_DIRECTORY</code> |
| YAML        | <code>cacheDir</code>               |
| Default     | <code>~/.cache/coder</code>         |

The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd. This directory is NOT safe to be configured as a shared directory across coderd/provisionerd replicas.

### --postgres-url

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_PG_CONNECTION_URL</code> |

URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url". Note that any special characters in the URL must be URL-encoded.

### --postgres-auth

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>password\|awsiamrds</code> |
| Environment | <code>$CODER_PG_AUTH</code>      |
| YAML        | <code>pgAuth</code>              |
| Default     | <code>password</code>            |

Type of auth to use when connecting to postgres. For AWS RDS, using IAM authentication (awsiamrds) is recommended.

### --secure-auth-cookie

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>bool</code>                        |
| Environment | <code>$CODER_SECURE_AUTH_COOKIE</code>   |
| YAML        | <code>networking.secureAuthCookie</code> |

Controls if the 'Secure' property is set on browser session cookies.

### --samesite-auth-cookie

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>lax\|none</code>                     |
| Environment | <code>$CODER_SAMESITE_AUTH_COOKIE</code>   |
| YAML        | <code>networking.sameSiteAuthCookie</code> |
| Default     | <code>lax</code>                           |

Controls the 'SameSite' property is set on browser session cookies.

### --terms-of-service-url

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_TERMS_OF_SERVICE_URL</code> |
| YAML        | <code>termsOfServiceURL</code>           |

A URL to an external Terms of Service that must be accepted by users when logging in.

### --strict-transport-security

|             |                                                     |
|-------------|-----------------------------------------------------|
| Type        | <code>int</code>                                    |
| Environment | <code>$CODER_STRICT_TRANSPORT_SECURITY</code>       |
| YAML        | <code>networking.tls.strictTransportSecurity</code> |
| Default     | <code>0</code>                                      |

Controls if the 'Strict-Transport-Security' header is set on all static file responses. This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of the header.

### --strict-transport-security-options

|             |                                                            |
|-------------|------------------------------------------------------------|
| Type        | <code>string-array</code>                                  |
| Environment | <code>$CODER_STRICT_TRANSPORT_SECURITY_OPTIONS</code>      |
| YAML        | <code>networking.tls.strictTransportSecurityOptions</code> |

Two optional fields can be set in the Strict-Transport-Security header; 'includeSubDomains' and 'preload'. The 'strict-transport-security' flag must be set to a non-zero value for these options to be used.

### --ssh-keygen-algorithm

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_SSH_KEYGEN_ALGORITHM</code> |
| YAML        | <code>sshKeygenAlgorithm</code>          |
| Default     | <code>ed25519</code>                     |

The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".

### --browser-only

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>bool</code>                   |
| Environment | <code>$CODER_BROWSER_ONLY</code>    |
| YAML        | <code>networking.browserOnly</code> |

Whether Coder only allows connections to workspaces via the browser.

### --scim-auth-header

|             |                                      |
|-------------|--------------------------------------|
| Type        | <code>string</code>                  |
| Environment | <code>$CODER_SCIM_AUTH_HEADER</code> |

Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.

### --external-token-encryption-keys

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>string-array</code>                          |
| Environment | <code>$CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS</code> |

Encrypt OIDC and Git authentication tokens with AES-256-GCM in the database. The value must be a comma-separated list of base64-encoded keys. Each key, when base64-decoded, must be exactly 32 bytes in length. The first key will be used to encrypt new values. Subsequent keys will be used as a fallback when decrypting. During normal operation it is recommended to only set one key unless you are in the process of rotating keys with the `coder server dbcrypt rotate` command.

### --disable-path-apps

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>bool</code>                     |
| Environment | <code>$CODER_DISABLE_PATH_APPS</code> |
| YAML        | <code>disablePathApps</code>          |

Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.

### --disable-owner-workspace-access

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>bool</code>                                  |
| Environment | <code>$CODER_DISABLE_OWNER_WORKSPACE_ACCESS</code> |
| YAML        | <code>disableOwnerWorkspaceAccess</code>           |

Remove the permission for the 'owner' role to have workspace execution on all workspaces. This prevents the 'owner' from ssh, apps, and terminal access based on the 'owner' role. They still have their user permissions to access their own workspaces.

### --session-duration

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>duration</code>                        |
| Environment | <code>$CODER_SESSION_DURATION</code>         |
| YAML        | <code>networking.http.sessionDuration</code> |
| Default     | <code>24h0m0s</code>                         |

The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.

### --disable-session-expiry-refresh

|             |                                                          |
|-------------|----------------------------------------------------------|
| Type        | <code>bool</code>                                        |
| Environment | <code>$CODER_DISABLE_SESSION_EXPIRY_REFRESH</code>       |
| YAML        | <code>networking.http.disableSessionExpiryRefresh</code> |

Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.

### --disable-password-auth

|             |                                                  |
|-------------|--------------------------------------------------|
| Type        | <code>bool</code>                                |
| Environment | <code>$CODER_DISABLE_PASSWORD_AUTH</code>        |
| YAML        | <code>networking.http.disablePasswordAuth</code> |

Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.

### -c, --config

|             |                                 |
|-------------|---------------------------------|
| Type        | <code>yaml-config-path</code>   |
| Environment | <code>$CODER_CONFIG_PATH</code> |

Specify a YAML file to load configuration from.

### --ssh-hostname-prefix

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_SSH_HOSTNAME_PREFIX</code> |
| YAML        | <code>client.sshHostnamePrefix</code>   |
| Default     | <code>coder.</code>                     |

The SSH deployment prefix is used in the Host of the ssh config.

### --workspace-hostname-suffix

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>string</code>                           |
| Environment | <code>$CODER_WORKSPACE_HOSTNAME_SUFFIX</code> |
| YAML        | <code>client.workspaceHostnameSuffix</code>   |
| Default     | <code>coder</code>                            |

Workspace hostnames use this suffix in SSH config and Coder Connect on Coder Desktop. By default it is coder, resulting in names like myworkspace.coder.

### --ssh-config-options

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>string-array</code>              |
| Environment | <code>$CODER_SSH_CONFIG_OPTIONS</code> |
| YAML        | <code>client.sshConfigOptions</code>   |

These SSH config options will override the default SSH config options. Provide options in "key=value" or "key value" format separated by commas.Using this incorrectly can break SSH to your deployment, use cautiously.

### --cli-upgrade-message

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_CLI_UPGRADE_MESSAGE</code> |
| YAML        | <code>client.cliUpgradeMessage</code>   |

The upgrade message to display to users when a client/server mismatch is detected. By default it instructs users to update using 'curl -L https://coder.com/install.sh | sh'.

### --write-config

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

<br/>Write out the current server config as YAML to stdout.

### --support-links

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>struct[[]codersdk.LinkConfig]</code> |
| Environment | <code>$CODER_SUPPORT_LINKS</code>          |
| YAML        | <code>supportLinks</code>                  |

Support links to display in the top right drop down menu.

### --proxy-health-interval

|             |                                                  |
|-------------|--------------------------------------------------|
| Type        | <code>duration</code>                            |
| Environment | <code>$CODER_PROXY_HEALTH_INTERVAL</code>        |
| YAML        | <code>networking.http.proxyHealthInterval</code> |
| Default     | <code>1m0s</code>                                |

The interval in which coderd should be checking the status of workspace proxies.

### --default-quiet-hours-schedule

|             |                                                               |
|-------------|---------------------------------------------------------------|
| Type        | <code>string</code>                                           |
| Environment | <code>$CODER_QUIET_HOURS_DEFAULT_SCHEDULE</code>              |
| YAML        | <code>userQuietHoursSchedule.defaultQuietHoursSchedule</code> |
| Default     | <code>CRON_TZ=UTC 0 0 ** *</code>                             |

The default daily cron schedule applied to users that haven't set a custom quiet hours schedule themselves. The quiet hours schedule determines when workspaces will be force stopped due to the template's autostop requirement, and will round the max deadline up to be within the user's quiet hours window (or default). The format is the same as the standard cron format, but the day-of-month, month and day-of-week must be *. Only one hour and minute can be specified (ranges or comma separated values are not supported).

### --allow-custom-quiet-hours

|             |                                                           |
|-------------|-----------------------------------------------------------|
| Type        | <code>bool</code>                                         |
| Environment | <code>$CODER_ALLOW_CUSTOM_QUIET_HOURS</code>              |
| YAML        | <code>userQuietHoursSchedule.allowCustomQuietHours</code> |
| Default     | <code>true</code>                                         |

Allow users to set their own quiet hours schedule for workspaces to stop in (depending on template autostop requirement settings). If false, users can't change their quiet hours schedule and the site default is always used.

### --web-terminal-renderer

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_WEB_TERMINAL_RENDERER</code> |
| YAML        | <code>client.webTerminalRenderer</code>   |
| Default     | <code>canvas</code>                       |

The renderer to use when opening a web terminal. Valid values are 'canvas', 'webgl', or 'dom'.

### --allow-workspace-renames

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>bool</code>                           |
| Environment | <code>$CODER_ALLOW_WORKSPACE_RENAMES</code> |
| YAML        | <code>allowWorkspaceRenames</code>          |
| Default     | <code>false</code>                          |

DEPRECATED: Allow users to rename their workspaces. Use only for temporary compatibility reasons, this will be removed in a future release.

### --health-check-refresh

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>duration</code>                          |
| Environment | <code>$CODER_HEALTH_CHECK_REFRESH</code>       |
| YAML        | <code>introspection.healthcheck.refresh</code> |
| Default     | <code>10m0s</code>                             |

Refresh interval for healthchecks.

### --health-check-threshold-database

|             |                                                          |
|-------------|----------------------------------------------------------|
| Type        | <code>duration</code>                                    |
| Environment | <code>$CODER_HEALTH_CHECK_THRESHOLD_DATABASE</code>      |
| YAML        | <code>introspection.healthcheck.thresholdDatabase</code> |
| Default     | <code>15ms</code>                                        |

The threshold for the database health check. If the median latency of the database exceeds this threshold over 5 attempts, the database is considered unhealthy. The default value is 15ms.

### --email-from

|             |                                |
|-------------|--------------------------------|
| Type        | <code>string</code>            |
| Environment | <code>$CODER_EMAIL_FROM</code> |
| YAML        | <code>email.from</code>        |

The sender's address to use.

### --email-smarthost

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_EMAIL_SMARTHOST</code> |
| YAML        | <code>email.smarthost</code>        |

The intermediary SMTP host through which emails are sent.

### --email-hello

|             |                                 |
|-------------|---------------------------------|
| Type        | <code>string</code>             |
| Environment | <code>$CODER_EMAIL_HELLO</code> |
| YAML        | <code>email.hello</code>        |
| Default     | <code>localhost</code>          |

The hostname identifying the SMTP server.

### --email-force-tls

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>bool</code>                   |
| Environment | <code>$CODER_EMAIL_FORCE_TLS</code> |
| YAML        | <code>email.forceTLS</code>         |
| Default     | <code>false</code>                  |

Force a TLS connection to the configured SMTP smarthost.

### --email-auth-identity

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_EMAIL_AUTH_IDENTITY</code> |
| YAML        | <code>email.emailAuth.identity</code>   |

Identity to use with PLAIN authentication.

### --email-auth-username

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_EMAIL_AUTH_USERNAME</code> |
| YAML        | <code>email.emailAuth.username</code>   |

Username to use with PLAIN/LOGIN authentication.

### --email-auth-password

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_EMAIL_AUTH_PASSWORD</code> |

Password to use with PLAIN/LOGIN authentication.

### --email-auth-password-file

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>string</code>                          |
| Environment | <code>$CODER_EMAIL_AUTH_PASSWORD_FILE</code> |
| YAML        | <code>email.emailAuth.passwordFile</code>    |

File from which to load password for use with PLAIN/LOGIN authentication.

### --email-tls-starttls

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_EMAIL_TLS_STARTTLS</code> |
| YAML        | <code>email.emailTLS.startTLS</code>   |

Enable STARTTLS to upgrade insecure SMTP connections using TLS.

### --email-tls-server-name

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_EMAIL_TLS_SERVERNAME</code> |
| YAML        | <code>email.emailTLS.serverName</code>   |

Server name to verify against the target certificate.

### --email-tls-skip-verify

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_EMAIL_TLS_SKIPVERIFY</code>       |
| YAML        | <code>email.emailTLS.insecureSkipVerify</code> |

Skip verification of the target server's certificate (insecure).

### --email-tls-ca-cert-file

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_EMAIL_TLS_CACERTFILE</code> |
| YAML        | <code>email.emailTLS.caCertFile</code>   |

CA certificate file to use.

### --email-tls-cert-file

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_EMAIL_TLS_CERTFILE</code> |
| YAML        | <code>email.emailTLS.certFile</code>   |

Certificate file to use.

### --email-tls-cert-key-file

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_EMAIL_TLS_CERTKEYFILE</code> |
| YAML        | <code>email.emailTLS.certKeyFile</code>   |

Certificate key file to use.

### --notifications-method

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_NOTIFICATIONS_METHOD</code> |
| YAML        | <code>notifications.method</code>        |
| Default     | <code>smtp</code>                        |

Which delivery method to use (available options: 'smtp', 'webhook').

### --notifications-dispatch-timeout

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>duration</code>                              |
| Environment | <code>$CODER_NOTIFICATIONS_DISPATCH_TIMEOUT</code> |
| YAML        | <code>notifications.dispatchTimeout</code>         |
| Default     | <code>1m0s</code>                                  |

How long to wait while a notification is being sent before giving up.

### --notifications-email-from

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>string</code>                          |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_FROM</code> |
| YAML        | <code>notifications.email.from</code>        |

The sender's address to use.

### --notifications-email-smarthost

|             |                                                   |
|-------------|---------------------------------------------------|
| Type        | <code>string</code>                               |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_SMARTHOST</code> |
| YAML        | <code>notifications.email.smarthost</code>        |

The intermediary SMTP host through which emails are sent.

### --notifications-email-hello

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>string</code>                           |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_HELLO</code> |
| YAML        | <code>notifications.email.hello</code>        |

The hostname identifying the SMTP server.

### --notifications-email-force-tls

|             |                                                   |
|-------------|---------------------------------------------------|
| Type        | <code>bool</code>                                 |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_FORCE_TLS</code> |
| YAML        | <code>notifications.email.forceTLS</code>         |

Force a TLS connection to the configured SMTP smarthost.

### --notifications-email-auth-identity

|             |                                                       |
|-------------|-------------------------------------------------------|
| Type        | <code>string</code>                                   |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_AUTH_IDENTITY</code> |
| YAML        | <code>notifications.email.emailAuth.identity</code>   |

Identity to use with PLAIN authentication.

### --notifications-email-auth-username

|             |                                                       |
|-------------|-------------------------------------------------------|
| Type        | <code>string</code>                                   |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_AUTH_USERNAME</code> |
| YAML        | <code>notifications.email.emailAuth.username</code>   |

Username to use with PLAIN/LOGIN authentication.

### --notifications-email-auth-password

|             |                                                       |
|-------------|-------------------------------------------------------|
| Type        | <code>string</code>                                   |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_AUTH_PASSWORD</code> |

Password to use with PLAIN/LOGIN authentication.

### --notifications-email-auth-password-file

|             |                                                            |
|-------------|------------------------------------------------------------|
| Type        | <code>string</code>                                        |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_AUTH_PASSWORD_FILE</code> |
| YAML        | <code>notifications.email.emailAuth.passwordFile</code>    |

File from which to load password for use with PLAIN/LOGIN authentication.

### --notifications-email-tls-starttls

|             |                                                      |
|-------------|------------------------------------------------------|
| Type        | <code>bool</code>                                    |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_TLS_STARTTLS</code> |
| YAML        | <code>notifications.email.emailTLS.startTLS</code>   |

Enable STARTTLS to upgrade insecure SMTP connections using TLS.

### --notifications-email-tls-server-name

|             |                                                        |
|-------------|--------------------------------------------------------|
| Type        | <code>string</code>                                    |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_TLS_SERVERNAME</code> |
| YAML        | <code>notifications.email.emailTLS.serverName</code>   |

Server name to verify against the target certificate.

### --notifications-email-tls-skip-verify

|             |                                                              |
|-------------|--------------------------------------------------------------|
| Type        | <code>bool</code>                                            |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_TLS_SKIPVERIFY</code>       |
| YAML        | <code>notifications.email.emailTLS.insecureSkipVerify</code> |

Skip verification of the target server's certificate (insecure).

### --notifications-email-tls-ca-cert-file

|             |                                                        |
|-------------|--------------------------------------------------------|
| Type        | <code>string</code>                                    |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_TLS_CACERTFILE</code> |
| YAML        | <code>notifications.email.emailTLS.caCertFile</code>   |

CA certificate file to use.

### --notifications-email-tls-cert-file

|             |                                                      |
|-------------|------------------------------------------------------|
| Type        | <code>string</code>                                  |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_TLS_CERTFILE</code> |
| YAML        | <code>notifications.email.emailTLS.certFile</code>   |

Certificate file to use.

### --notifications-email-tls-cert-key-file

|             |                                                         |
|-------------|---------------------------------------------------------|
| Type        | <code>string</code>                                     |
| Environment | <code>$CODER_NOTIFICATIONS_EMAIL_TLS_CERTKEYFILE</code> |
| YAML        | <code>notifications.email.emailTLS.certKeyFile</code>   |

Certificate key file to use.

### --notifications-webhook-endpoint

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>url</code>                                   |
| Environment | <code>$CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT</code> |
| YAML        | <code>notifications.webhook.endpoint</code>        |

The endpoint to which to send webhooks.

### --notifications-inbox-enabled

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>bool</code>                               |
| Environment | <code>$CODER_NOTIFICATIONS_INBOX_ENABLED</code> |
| YAML        | <code>notifications.inbox.enabled</code>        |
| Default     | <code>true</code>                               |

Enable Coder Inbox.

### --notifications-max-send-attempts

|             |                                                     |
|-------------|-----------------------------------------------------|
| Type        | <code>int</code>                                    |
| Environment | <code>$CODER_NOTIFICATIONS_MAX_SEND_ATTEMPTS</code> |
| YAML        | <code>notifications.maxSendAttempts</code>          |
| Default     | <code>5</code>                                      |

The upper limit of attempts to send a notification.
