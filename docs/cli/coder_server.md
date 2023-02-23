<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder server

Start a Coder server

## Usage

```console
coder server [flags]
```

## Subcommands

| Name                                                                         | Purpose                                                                                                |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| [<code>create-admin-user</code>](./coder_server_create-admin-user)           | Create a new admin user with the given username, email and password and adds it to every organization. |
| [<code>postgres-builtin-serve</code>](./coder_server_postgres-builtin-serve) | Run the built-in PostgreSQL deployment.                                                                |
| [<code>postgres-builtin-url</code>](./coder_server_postgres-builtin-url)     | Output the connection URL for the built-in PostgreSQL deployment.                                      |

## Flags

### --access-url

External URL to access your deployment. This must be accessible by all provisioned workspaces.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_ACCESS_URL</code> |

### --api-rate-limit

Maximum number of requests per minute allowed to the API per user, or per IP address for unauthenticated users. Negative values mean no rate limit. Some API endpoints have separate strict rate limits regardless of this value to prevent denial-of-service or brute force attacks.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_API_RATE_LIMIT</code> |
| Default | <code>512</code> |

### --cache-dir

The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_CACHE_DIRECTORY</code> |
| Default | <code>~/.cache/coder</code> |

### --dangerous-allow-path-app-sharing

Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DANGEROUS_ALLOW_PATH_APP_SHARING</code> |
| Default | <code>false</code> |

### --dangerous-allow-path-app-site-owner-access

Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS</code> |
| Default | <code>false</code> |

### --dangerous-disable-rate-limits

Disables all rate limits. This is not recommended in production.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_RATE_LIMIT_DISABLE_ALL</code> |
| Default | <code>false</code> |

### --derp-config-path

Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DERP_CONFIG_PATH</code> |

### --derp-config-url

URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DERP_CONFIG_URL</code> |

### --derp-server-enable

Whether to enable or disable the embedded DERP relay server.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DERP_SERVER_ENABLE</code> |
| Default | <code>true</code> |

### --derp-server-region-code

Region code to use for the embedded DERP server.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DERP_SERVER_REGION_CODE</code> |
| Default | <code>coder</code> |

### --derp-server-region-id

Region ID to use for the embedded DERP server.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DERP_SERVER_REGION_ID</code> |
| Default | <code>999</code> |

### --derp-server-region-name

Region name that for the embedded DERP server.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DERP_SERVER_REGION_NAME</code> |
| Default | <code>Coder Embedded Relay</code> |

### --derp-server-stun-addresses

Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DERP_SERVER_STUN_ADDRESSES</code> |
| Default | <code>[stun.l.google.com:19302]</code> |

### --disable-password-auth

Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DISABLE_PASSWORD_AUTH</code> |
| Default | <code>false</code> |

### --disable-path-apps

Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DISABLE_PATH_APPS</code> |
| Default | <code>false</code> |

### --disable-session-expiry-refresh

Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_DISABLE_SESSION_EXPIRY_REFRESH</code> |
| Default | <code>false</code> |

### --experiments

Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '\*' to opt-in to all available experiments.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_EXPERIMENTS</code> |
| Default | <code>[]</code> |

### --http-address

HTTP bind address of the server. Unset to disable the HTTP endpoint.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_HTTP_ADDRESS</code> |
| Default | <code>127.0.0.1:3000</code> |

### --log-human

Output human-readable logs to a given file.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOGGING_HUMAN</code> |
| Default | <code>/dev/stderr</code> |

### --log-json

Output JSON logs to a given file.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOGGING_JSON</code> |

### --log-stackdriver

Output Stackdriver compatible logs to a given file.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOGGING_STACKDRIVER</code> |

### --max-token-lifetime

The maximum lifetime duration users can specify when creating an API token.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_MAX_TOKEN_LIFETIME</code> |
| Default | <code>720h0m0s</code> |

### --oauth2-github-allow-everyone

Allow all logins, setting this option means allowed orgs and teams must be empty.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OAUTH2_GITHUB_ALLOW_EVERYONE</code> |
| Default | <code>false</code> |

### --oauth2-github-allow-signups

Whether new users can sign up with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS</code> |
| Default | <code>false</code> |

### --oauth2-github-allowed-orgs

Organizations the user must be a member of to Login with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OAUTH2_GITHUB_ALLOWED_ORGS</code> |
| Default | <code>[]</code> |

### --oauth2-github-allowed-teams

Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OAUTH2_GITHUB_ALLOWED_TEAMS</code> |
| Default | <code>[]</code> |

### --oauth2-github-client-id

Client ID for Login with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OAUTH2_GITHUB_CLIENT_ID</code> |

### --oauth2-github-client-secret

Client secret for Login with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OAUTH2_GITHUB_CLIENT_SECRET</code> |

### --oauth2-github-enterprise-base-url

Base URL of a GitHub Enterprise deployment to use for Login with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OAUTH2_GITHUB_ENTERPRISE_BASE_URL</code> |

### --oidc-allow-signups

Whether new users can sign up with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_ALLOW_SIGNUPS</code> |
| Default | <code>true</code> |

### --oidc-client-id

Client ID to use for Login with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_CLIENT_ID</code> |

### --oidc-client-secret

Client secret to use for Login with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_CLIENT_SECRET</code> |

### --oidc-email-domain

Email domains that clients logging in with OIDC must match.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_EMAIL_DOMAIN</code> |
| Default | <code>[]</code> |

### --oidc-icon-url

URL pointing to the icon to use on the OepnID Connect login button
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_ICON_URL</code> |

### --oidc-ignore-email-verified

Ignore the email_verified claim from the upstream provider.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_IGNORE_EMAIL_VERIFIED</code> |
| Default | <code>false</code> |

### --oidc-issuer-url

Issuer URL to use for Login with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_ISSUER_URL</code> |

### --oidc-scopes

Scopes to grant when authenticating with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_SCOPES</code> |
| Default | <code>[openid,profile,email]</code> |

### --oidc-sign-in-text

The text to show on the OpenID Connect sign in button
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_SIGN_IN_TEXT</code> |
| Default | <code>OpenID Connect</code> |

### --oidc-username-field

OIDC claim field to use as the username.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_OIDC_USERNAME_FIELD</code> |
| Default | <code>preferred_username</code> |

### --postgres-url

URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PG_CONNECTION_URL</code> |

### --pprof-address

The bind address to serve pprof.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PPROF_ADDRESS</code> |
| Default | <code>127.0.0.1:6060</code> |

### --pprof-enable

Serve pprof metrics on the address defined by pprof address.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PPROF_ENABLE</code> |
| Default | <code>false</code> |

### --prometheus-address

The bind address to serve prometheus metrics.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PROMETHEUS_ADDRESS</code> |
| Default | <code>127.0.0.1:2112</code> |

### --prometheus-enable

Serve prometheus metrics on the address defined by prometheus address.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PROMETHEUS_ENABLE</code> |
| Default | <code>false</code> |

### --provisioner-daemon-poll-interval

Time to wait before polling for a new job.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PROVISIONER_DAEMON_POLL_INTERVAL</code> |
| Default | <code>1s</code> |

### --provisioner-daemon-poll-jitter

Random jitter added to the poll interval.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PROVISIONER_DAEMON_POLL_JITTER</code> |
| Default | <code>100ms</code> |

### --provisioner-daemons

Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PROVISIONER_DAEMONS</code> |
| Default | <code>3</code> |

### --provisioner-force-cancel-interval

Time to force cancel provisioning tasks that are stuck.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PROVISIONER_FORCE_CANCEL_INTERVAL</code> |
| Default | <code>10m0s</code> |

### --proxy-trusted-headers

Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PROXY_TRUSTED_HEADERS</code> |
| Default | <code>[]</code> |

### --proxy-trusted-origins

Origin addresses to respect "proxy-trusted-headers". e.g. 192.168.1.0/24
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PROXY_TRUSTED_ORIGINS</code> |
| Default | <code>[]</code> |

### --redirect-to-access-url

Specifies whether to redirect requests that do not match the access URL host.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_REDIRECT_TO_ACCESS_URL</code> |
| Default | <code>false</code> |

### --secure-auth-cookie

Controls if the 'Secure' property is set on browser session cookies.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SECURE_AUTH_COOKIE</code> |
| Default | <code>false</code> |

### --session-duration

The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_MAX_SESSION_EXPIRY</code> |
| Default | <code>24h0m0s</code> |

### --ssh-keygen-algorithm

The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SSH_KEYGEN_ALGORITHM</code> |
| Default | <code>ed25519</code> |

### --strict-transport-security

Controls if the 'Strict-Transport-Security' header is set on all static file responses. This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of the header.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_STRICT_TRANSPORT_SECURITY</code> |
| Default | <code>0</code> |

### --strict-transport-security-options

Two optional fields can be set in the Strict-Transport-Security header; 'includeSubDomains' and 'preload'. The 'strict-transport-security' flag must be set to a non-zero value for these options to be used.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_STRICT_TRANSPORT_SECURITY_OPTIONS</code> |
| Default | <code>[]</code> |

### --swagger-enable

Expose the swagger endpoint via /swagger.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SWAGGER_ENABLE</code> |
| Default | <code>false</code> |

### --telemetry

Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TELEMETRY_ENABLE</code> |
| Default | <code>true</code> |

### --telemetry-trace

Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TELEMETRY_TRACE</code> |
| Default | <code>true</code> |

### --tls-address

HTTPS bind address of the server.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TLS_ADDRESS</code> |
| Default | <code>127.0.0.1:3443</code> |

### --tls-cert-file

Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TLS_CERT_FILE</code> |
| Default | <code>[]</code> |

### --tls-client-auth

Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TLS_CLIENT_AUTH</code> |
| Default | <code>none</code> |

### --tls-client-ca-file

PEM-encoded Certificate Authority file used for checking the authenticity of client
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TLS_CLIENT_CA_FILE</code> |

### --tls-client-cert-file

Path to certificate for client TLS authentication. It requires a PEM-encoded file.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TLS_CLIENT_CERT_FILE</code> |

### --tls-client-key-file

Path to key for client TLS authentication. It requires a PEM-encoded file.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TLS_CLIENT_KEY_FILE</code> |

### --tls-enable

Whether TLS will be enabled.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TLS_ENABLE</code> |
| Default | <code>false</code> |

### --tls-key-file

Paths to the private keys for each of the certificates. It requires a PEM-encoded file.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TLS_KEY_FILE</code> |
| Default | <code>[]</code> |

### --tls-min-version

Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TLS_MIN_VERSION</code> |
| Default | <code>tls12</code> |

### --trace

Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TRACE_ENABLE</code> |
| Default | <code>false</code> |

### --trace-honeycomb-api-key

Enables trace exporting to Honeycomb.io using the provided API Key.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TRACE_HONEYCOMB_API_KEY</code> |

### --trace-logs

Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs. If the verbose flag was supplied, debug-level logs will be included.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TRACE_CAPTURE_LOGS</code> |
| Default | <code>false</code> |

### --update-check

Periodically check for new releases of Coder and inform the owner. The check is performed once per day.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_UPDATE_CHECK</code> |
| Default | <code>false</code> |

### --wildcard-access-url

Specifies the wildcard hostname to use for workspace applications in the form "\*.example.com".
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_WILDCARD_ACCESS_URL</code> |
