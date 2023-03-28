<!-- DO NOT EDIT | GENERATED CONTENT -->

# server

Start a Coder server

## Usage

```console
coder server [flags]
```

## Subcommands

| Name                                                                   | Purpose                                                                                                |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| [<code>create-admin-user</code>](./server_create-admin-user)           | Create a new admin user with the given username, email and password and adds it to every organization. |
| [<code>postgres-builtin-serve</code>](./server_postgres-builtin-serve) | Run the built-in PostgreSQL deployment.                                                                |
| [<code>postgres-builtin-url</code>](./server_postgres-builtin-url)     | Output the connection URL for the built-in PostgreSQL deployment.                                      |

## Options

### --access-url

|             |                                |
| ----------- | ------------------------------ |
| Type        | <code>url</code>               |
| Environment | <code>$CODER_ACCESS_URL</code> |

The URL that users will use to access the Coder deployment.

### --audit-logging

|             |                                   |
| ----------- | --------------------------------- |
| Type        | <code>bool</code>                 |
| Environment | <code>$CODER_AUDIT_LOGGING</code> |
| Default     | <code>true</code>                 |

Specifies whether audit logging is enabled.

### --browser-only

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>bool</code>                |
| Environment | <code>$CODER_BROWSER_ONLY</code> |

Whether Coder only allows connections to workspaces via the browser.

### --cache-dir

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_CACHE_DIRECTORY</code> |
| Default     | <code>~/.cache/coder</code>         |

The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.

### --dangerous-allow-path-app-sharing

|             |                                                      |
| ----------- | ---------------------------------------------------- |
| Type        | <code>bool</code>                                    |
| Environment | <code>$CODER_DANGEROUS_ALLOW_PATH_APP_SHARING</code> |

Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.

### --dangerous-allow-path-app-site-owner-access

|             |                                                                |
| ----------- | -------------------------------------------------------------- |
| Type        | <code>bool</code>                                              |
| Environment | <code>$CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS</code> |

Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.

### --derp-config-path

|             |                                      |
| ----------- | ------------------------------------ |
| Type        | <code>string</code>                  |
| Environment | <code>$CODER_DERP_CONFIG_PATH</code> |

Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/.

### --derp-config-url

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_DERP_CONFIG_URL</code> |

URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/.

### --derp-server-enable

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_DERP_SERVER_ENABLE</code> |
| Default     | <code>true</code>                      |

Whether to enable or disable the embedded DERP relay server.

### --derp-server-region-code

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_DERP_SERVER_REGION_CODE</code> |
| Default     | <code>coder</code>                          |

Region code to use for the embedded DERP server.

### --derp-server-region-id

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>int</code>                          |
| Environment | <code>$CODER_DERP_SERVER_REGION_ID</code> |
| Default     | <code>999</code>                          |

Region ID to use for the embedded DERP server.

### --derp-server-region-name

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_DERP_SERVER_REGION_NAME</code> |
| Default     | <code>Coder Embedded Relay</code>           |

Region name that for the embedded DERP server.

### --derp-server-relay-url

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>url</code>                          |
| Environment | <code>$CODER_DERP_SERVER_RELAY_URL</code> |

An HTTP URL that is accessible by other replicas to relay DERP traffic. Required for high availability.

### --derp-server-stun-addresses

|             |                                                |
| ----------- | ---------------------------------------------- |
| Type        | <code>string-array</code>                      |
| Environment | <code>$CODER_DERP_SERVER_STUN_ADDRESSES</code> |
| Default     | <code>stun.l.google.com:19302</code>           |

Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.

### --disable-password-auth

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>bool</code>                         |
| Environment | <code>$CODER_DISABLE_PASSWORD_AUTH</code> |

Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.

### --disable-path-apps

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>bool</code>                     |
| Environment | <code>$CODER_DISABLE_PATH_APPS</code> |

Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.

### --disable-session-expiry-refresh

|             |                                                    |
| ----------- | -------------------------------------------------- |
| Type        | <code>bool</code>                                  |
| Environment | <code>$CODER_DISABLE_SESSION_EXPIRY_REFRESH</code> |

Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.

### --experiments

|             |                                 |
| ----------- | ------------------------------- |
| Type        | <code>string-array</code>       |
| Environment | <code>$CODER_EXPERIMENTS</code> |

Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '\*' to opt-in to all available experiments.

### --http-address

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>string</code>              |
| Environment | <code>$CODER_HTTP_ADDRESS</code> |
| Default     | <code>127.0.0.1:3000</code>      |

HTTP bind address of the server. Unset to disable the HTTP endpoint.

### --log-human

|             |                                   |
| ----------- | --------------------------------- |
| Type        | <code>string</code>               |
| Environment | <code>$CODER_LOGGING_HUMAN</code> |
| Default     | <code>/dev/stderr</code>          |

Output human-readable logs to a given file.

### --log-json

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>string</code>              |
| Environment | <code>$CODER_LOGGING_JSON</code> |

Output JSON logs to a given file.

### --log-stackdriver

|             |                                         |
| ----------- | --------------------------------------- |
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_LOGGING_STACKDRIVER</code> |

Output Stackdriver compatible logs to a given file.

### --max-token-lifetime

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>duration</code>                  |
| Environment | <code>$CODER_MAX_TOKEN_LIFETIME</code> |
| Default     | <code>876600h0m0s</code>               |

The maximum lifetime duration users can specify when creating an API token.

### --oauth2-github-allow-everyone

|             |                                                  |
| ----------- | ------------------------------------------------ |
| Type        | <code>bool</code>                                |
| Environment | <code>$CODER_OAUTH2_GITHUB_ALLOW_EVERYONE</code> |

Allow all logins, setting this option means allowed orgs and teams must be empty.

### --oauth2-github-allow-signups

|             |                                                 |
| ----------- | ----------------------------------------------- |
| Type        | <code>bool</code>                               |
| Environment | <code>$CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS</code> |

Whether new users can sign up with GitHub.

### --oauth2-github-allowed-orgs

|             |                                                |
| ----------- | ---------------------------------------------- |
| Type        | <code>string-array</code>                      |
| Environment | <code>$CODER_OAUTH2_GITHUB_ALLOWED_ORGS</code> |

Organizations the user must be a member of to Login with GitHub.

### --oauth2-github-allowed-teams

|             |                                                 |
| ----------- | ----------------------------------------------- |
| Type        | <code>string-array</code>                       |
| Environment | <code>$CODER_OAUTH2_GITHUB_ALLOWED_TEAMS</code> |

Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.

### --oauth2-github-client-id

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_OAUTH2_GITHUB_CLIENT_ID</code> |

Client ID for Login with GitHub.

### --oauth2-github-client-secret

|             |                                                 |
| ----------- | ----------------------------------------------- |
| Type        | <code>string</code>                             |
| Environment | <code>$CODER_OAUTH2_GITHUB_CLIENT_SECRET</code> |

Client secret for Login with GitHub.

### --oauth2-github-enterprise-base-url

|             |                                                       |
| ----------- | ----------------------------------------------------- |
| Type        | <code>string</code>                                   |
| Environment | <code>$CODER_OAUTH2_GITHUB_ENTERPRISE_BASE_URL</code> |

Base URL of a GitHub Enterprise deployment to use for Login with GitHub.

### --oidc-allow-signups

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_OIDC_ALLOW_SIGNUPS</code> |
| Default     | <code>true</code>                      |

Whether new users can sign up with OIDC.

### --oidc-client-id

|             |                                    |
| ----------- | ---------------------------------- |
| Type        | <code>string</code>                |
| Environment | <code>$CODER_OIDC_CLIENT_ID</code> |

Client ID to use for Login with OIDC.

### --oidc-client-secret

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_OIDC_CLIENT_SECRET</code> |

Client secret to use for Login with OIDC.

### --oidc-email-domain

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string-array</code>             |
| Environment | <code>$CODER_OIDC_EMAIL_DOMAIN</code> |

Email domains that clients logging in with OIDC must match.

### --oidc-group-field

|             |                                      |
| ----------- | ------------------------------------ |
| Type        | <code>string</code>                  |
| Environment | <code>$CODER_OIDC_GROUP_FIELD</code> |

Change the OIDC default 'groups' claim field. By default, will be 'groups' if present in the oidc scopes argument.

### --oidc-group-mapping

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>struct[map[string]string]</code> |
| Environment | <code>$OIDC_GROUP_MAPPING</code>       |
| Default     | <code>{}</code>                        |

A map of OIDC group IDs and the group in Coder it should map to. This is useful for when OIDC providers only return group IDs.

### --oidc-icon-url

|             |                                   |
| ----------- | --------------------------------- |
| Type        | <code>url</code>                  |
| Environment | <code>$CODER_OIDC_ICON_URL</code> |

URL pointing to the icon to use on the OepnID Connect login button.

### --oidc-ignore-email-verified

|             |                                                |
| ----------- | ---------------------------------------------- |
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_OIDC_IGNORE_EMAIL_VERIFIED</code> |

Ignore the email_verified claim from the upstream provider.

### --oidc-issuer-url

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_OIDC_ISSUER_URL</code> |

Issuer URL to use for Login with OIDC.

### --oidc-scopes

|             |                                   |
| ----------- | --------------------------------- |
| Type        | <code>string-array</code>         |
| Environment | <code>$CODER_OIDC_SCOPES</code>   |
| Default     | <code>openid,profile,email</code> |

Scopes to grant when authenticating with OIDC.

### --oidc-sign-in-text

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_OIDC_SIGN_IN_TEXT</code> |
| Default     | <code>OpenID Connect</code>           |

The text to show on the OpenID Connect sign in button.

### --oidc-username-field

|             |                                         |
| ----------- | --------------------------------------- |
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_OIDC_USERNAME_FIELD</code> |
| Default     | <code>preferred_username</code>         |

OIDC claim field to use as the username.

### --postgres-url

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_PG_CONNECTION_URL</code> |

URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".

### --pprof-address

|             |                                   |
| ----------- | --------------------------------- |
| Type        | <code>host:port</code>            |
| Environment | <code>$CODER_PPROF_ADDRESS</code> |
| Default     | <code>127.0.0.1:6060</code>       |

The bind address to serve pprof.

### --pprof-enable

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>bool</code>                |
| Environment | <code>$CODER_PPROF_ENABLE</code> |

Serve pprof metrics on the address defined by pprof address.

### --prometheus-address

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>host:port</code>                 |
| Environment | <code>$CODER_PROMETHEUS_ADDRESS</code> |
| Default     | <code>127.0.0.1:2112</code>            |

The bind address to serve prometheus metrics.

### --prometheus-enable

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>bool</code>                     |
| Environment | <code>$CODER_PROMETHEUS_ENABLE</code> |

Serve prometheus metrics on the address defined by prometheus address.

### --provisioner-daemon-poll-interval

|             |                                                      |
| ----------- | ---------------------------------------------------- |
| Type        | <code>duration</code>                                |
| Environment | <code>$CODER_PROVISIONER_DAEMON_POLL_INTERVAL</code> |
| Default     | <code>1s</code>                                      |

Time to wait before polling for a new job.

### --provisioner-daemon-poll-jitter

|             |                                                    |
| ----------- | -------------------------------------------------- |
| Type        | <code>duration</code>                              |
| Environment | <code>$CODER_PROVISIONER_DAEMON_POLL_JITTER</code> |
| Default     | <code>100ms</code>                                 |

Random jitter added to the poll interval.

### --provisioner-daemons

|             |                                         |
| ----------- | --------------------------------------- |
| Type        | <code>int</code>                        |
| Environment | <code>$CODER_PROVISIONER_DAEMONS</code> |
| Default     | <code>3</code>                          |

Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.

### --provisioner-force-cancel-interval

|             |                                                       |
| ----------- | ----------------------------------------------------- |
| Type        | <code>duration</code>                                 |
| Environment | <code>$CODER_PROVISIONER_FORCE_CANCEL_INTERVAL</code> |
| Default     | <code>10m0s</code>                                    |

Time to force cancel provisioning tasks that are stuck.

### --proxy-trusted-headers

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>string-array</code>                 |
| Environment | <code>$CODER_PROXY_TRUSTED_HEADERS</code> |

Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For.

### --proxy-trusted-origins

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>string-array</code>                 |
| Environment | <code>$CODER_PROXY_TRUSTED_ORIGINS</code> |

Origin addresses to respect "proxy-trusted-headers". e.g. 192.168.1.0/24.

### --redirect-to-access-url

|             |                                            |
| ----------- | ------------------------------------------ |
| Type        | <code>bool</code>                          |
| Environment | <code>$CODER_REDIRECT_TO_ACCESS_URL</code> |

Specifies whether to redirect requests that do not match the access URL host.

### --scim-auth-header

|             |                                      |
| ----------- | ------------------------------------ |
| Type        | <code>string</code>                  |
| Environment | <code>$CODER_SCIM_AUTH_HEADER</code> |

Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.

### --secure-auth-cookie

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_SECURE_AUTH_COOKIE</code> |

Controls if the 'Secure' property is set on browser session cookies.

### --session-duration

|             |                                      |
| ----------- | ------------------------------------ |
| Type        | <code>duration</code>                |
| Environment | <code>$CODER_SESSION_DURATION</code> |
| Default     | <code>24h0m0s</code>                 |

The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.

### --ssh-config-options

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>string-array</code>              |
| Environment | <code>$CODER_SSH_CONFIG_OPTIONS</code> |

These SSH config options will override the default SSH config options. Provide options in "key=value" or "key value" format separated by commas.Using this incorrectly can break SSH to your deployment, use cautiously.

### --ssh-hostname-prefix

|             |                                         |
| ----------- | --------------------------------------- |
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_SSH_HOSTNAME_PREFIX</code> |
| Default     | <code>coder.</code>                     |

The SSH deployment prefix is used in the Host of the ssh config.

### --ssh-keygen-algorithm

|             |                                          |
| ----------- | ---------------------------------------- |
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_SSH_KEYGEN_ALGORITHM</code> |
| Default     | <code>ed25519</code>                     |

The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".

### --strict-transport-security

|             |                                               |
| ----------- | --------------------------------------------- |
| Type        | <code>int</code>                              |
| Environment | <code>$CODER_STRICT_TRANSPORT_SECURITY</code> |
| Default     | <code>0</code>                                |

Controls if the 'Strict-Transport-Security' header is set on all static file responses. This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of the header.

### --strict-transport-security-options

|             |                                                       |
| ----------- | ----------------------------------------------------- |
| Type        | <code>string-array</code>                             |
| Environment | <code>$CODER_STRICT_TRANSPORT_SECURITY_OPTIONS</code> |

Two optional fields can be set in the Strict-Transport-Security header; 'includeSubDomains' and 'preload'. The 'strict-transport-security' flag must be set to a non-zero value for these options to be used.

### --swagger-enable

|             |                                    |
| ----------- | ---------------------------------- |
| Type        | <code>bool</code>                  |
| Environment | <code>$CODER_SWAGGER_ENABLE</code> |

Expose the swagger endpoint via /swagger.

### --telemetry

|             |                                      |
| ----------- | ------------------------------------ |
| Type        | <code>bool</code>                    |
| Environment | <code>$CODER_TELEMETRY_ENABLE</code> |
| Default     | <code>true</code>                    |

Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.

### --telemetry-trace

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>bool</code>                   |
| Environment | <code>$CODER_TELEMETRY_TRACE</code> |
| Default     | <code>true</code>                   |

Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.

### --tls-address

|             |                                 |
| ----------- | ------------------------------- |
| Type        | <code>host:port</code>          |
| Environment | <code>$CODER_TLS_ADDRESS</code> |
| Default     | <code>127.0.0.1:3443</code>     |

HTTPS bind address of the server.

### --tls-cert-file

|             |                                   |
| ----------- | --------------------------------- |
| Type        | <code>string-array</code>         |
| Environment | <code>$CODER_TLS_CERT_FILE</code> |

Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.

### --tls-client-auth

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_TLS_CLIENT_AUTH</code> |
| Default     | <code>none</code>                   |

Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".

### --tls-client-ca-file

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_TLS_CLIENT_CA_FILE</code> |

PEM-encoded Certificate Authority file used for checking the authenticity of client.

### --tls-client-cert-file

|             |                                          |
| ----------- | ---------------------------------------- |
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_TLS_CLIENT_CERT_FILE</code> |

Path to certificate for client TLS authentication. It requires a PEM-encoded file.

### --tls-client-key-file

|             |                                         |
| ----------- | --------------------------------------- |
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_TLS_CLIENT_KEY_FILE</code> |

Path to key for client TLS authentication. It requires a PEM-encoded file.

### --tls-enable

|             |                                |
| ----------- | ------------------------------ |
| Type        | <code>bool</code>              |
| Environment | <code>$CODER_TLS_ENABLE</code> |

Whether TLS will be enabled.

### --tls-key-file

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>string-array</code>        |
| Environment | <code>$CODER_TLS_KEY_FILE</code> |

Paths to the private keys for each of the certificates. It requires a PEM-encoded file.

### --tls-min-version

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_TLS_MIN_VERSION</code> |
| Default     | <code>tls12</code>                  |

Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13".

### --trace

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>bool</code>                |
| Environment | <code>$CODER_TRACE_ENABLE</code> |

Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.

### --trace-honeycomb-api-key

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_TRACE_HONEYCOMB_API_KEY</code> |

Enables trace exporting to Honeycomb.io using the provided API Key.

### --trace-logs

|             |                                |
| ----------- | ------------------------------ |
| Type        | <code>bool</code>              |
| Environment | <code>$CODER_TRACE_LOGS</code> |

Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs. If the verbose flag was supplied, debug-level logs will be included.

### --update-check

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>bool</code>                |
| Environment | <code>$CODER_UPDATE_CHECK</code> |
| Default     | <code>false</code>               |

Periodically check for new releases of Coder and inform the owner. The check is performed once per day.

### -v, --verbose

|             |                             |
| ----------- | --------------------------- |
| Type        | <code>bool</code>           |
| Environment | <code>$CODER_VERBOSE</code> |

Output debug-level logs.

### --wildcard-access-url

|             |                                         |
| ----------- | --------------------------------------- |
| Type        | <code>url</code>                        |
| Environment | <code>$CODER_WILDCARD_ACCESS_URL</code> |

Specifies the wildcard hostname to use for workspace applications in the form "\*.example.com".
