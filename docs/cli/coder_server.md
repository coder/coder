## coder server

Start a Coder server

```
coder server [flags]
```

### Options

```
      --access-url string                                 External URL to access your deployment. This must be accessible by all provisioned workspaces.
                                                          Consumes $CODER_ACCESS_URL
      --api-rate-limit int                                Maximum number of requests per minute allowed to the API per user, or per IP address for unauthenticated users. Negative values mean no rate limit. Some API endpoints have separate strict rate limits regardless of this value to prevent denial-of-service or brute force attacks.
                                                          Consumes $CODER_API_RATE_LIMIT (default 512)
      --cache-dir string                                  The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.
                                                          Consumes $CODER_CACHE_DIRECTORY (default "~/.cache/coder")
      --dangerous-allow-path-app-sharing                  Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.
                                                          Consumes $CODER_DANGEROUS_ALLOW_PATH_APP_SHARING
      --dangerous-allow-path-app-site-owner-access        Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.
                                                          Consumes $CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS
      --dangerous-disable-rate-limits                     Disables all rate limits. This is not recommended in production.
                                                          Consumes $CODER_RATE_LIMIT_DISABLE_ALL
      --derp-config-path string                           Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/
                                                          Consumes $CODER_DERP_CONFIG_PATH
      --derp-config-url string                            URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/
                                                          Consumes $CODER_DERP_CONFIG_URL
      --derp-server-enable                                Whether to enable or disable the embedded DERP relay server.
                                                          Consumes $CODER_DERP_SERVER_ENABLE (default true)
      --derp-server-region-code string                    Region code to use for the embedded DERP server.
                                                          Consumes $CODER_DERP_SERVER_REGION_CODE (default "coder")
      --derp-server-region-id int                         Region ID to use for the embedded DERP server.
                                                          Consumes $CODER_DERP_SERVER_REGION_ID (default 999)
      --derp-server-region-name string                    Region name that for the embedded DERP server.
                                                          Consumes $CODER_DERP_SERVER_REGION_NAME (default "Coder Embedded Relay")
      --derp-server-stun-addresses strings                Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.
                                                          Consumes $CODER_DERP_SERVER_STUN_ADDRESSES (default [stun.l.google.com:19302])
      --disable-password-auth coder server create-admin   Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the coder server create-admin command to create a new admin user directly in the database.
                                                          Consumes $CODER_DISABLE_PASSWORD_AUTH
      --disable-path-apps                                 Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.
                                                          Consumes $CODER_DISABLE_PATH_APPS
      --disable-session-expiry-refresh                    Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.
                                                          Consumes $CODER_DISABLE_SESSION_EXPIRY_REFRESH
      --experiments strings                               Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.
                                                          Consumes $CODER_EXPERIMENTS
  -h, --help                                              help for server
      --http-address string                               HTTP bind address of the server. Unset to disable the HTTP endpoint.
                                                          Consumes $CODER_HTTP_ADDRESS (default "127.0.0.1:3000")
      --log-human string                                  Output human-readable logs to a given file.
                                                          Consumes $CODER_LOGGING_HUMAN (default "/dev/stderr")
      --log-json string                                   Output JSON logs to a given file.
                                                          Consumes $CODER_LOGGING_JSON
      --log-stackdriver string                            Output Stackdriver compatible logs to a given file.
                                                          Consumes $CODER_LOGGING_STACKDRIVER
      --max-token-lifetime duration                       The maximum lifetime duration users can specify when creating an API token.
                                                          Consumes $CODER_MAX_TOKEN_LIFETIME (default 720h0m0s)
      --oauth2-github-allow-everyone                      Allow all logins, setting this option means allowed orgs and teams must be empty.
                                                          Consumes $CODER_OAUTH2_GITHUB_ALLOW_EVERYONE
      --oauth2-github-allow-signups                       Whether new users can sign up with GitHub.
                                                          Consumes $CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS
      --oauth2-github-allowed-orgs strings                Organizations the user must be a member of to Login with GitHub.
                                                          Consumes $CODER_OAUTH2_GITHUB_ALLOWED_ORGS
      --oauth2-github-allowed-teams strings               Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.
                                                          Consumes $CODER_OAUTH2_GITHUB_ALLOWED_TEAMS
      --oauth2-github-client-id string                    Client ID for Login with GitHub.
                                                          Consumes $CODER_OAUTH2_GITHUB_CLIENT_ID
      --oauth2-github-client-secret string                Client secret for Login with GitHub.
                                                          Consumes $CODER_OAUTH2_GITHUB_CLIENT_SECRET
      --oauth2-github-enterprise-base-url string          Base URL of a GitHub Enterprise deployment to use for Login with GitHub.
                                                          Consumes $CODER_OAUTH2_GITHUB_ENTERPRISE_BASE_URL
      --oidc-allow-signups                                Whether new users can sign up with OIDC.
                                                          Consumes $CODER_OIDC_ALLOW_SIGNUPS (default true)
      --oidc-client-id string                             Client ID to use for Login with OIDC.
                                                          Consumes $CODER_OIDC_CLIENT_ID
      --oidc-client-secret string                         Client secret to use for Login with OIDC.
                                                          Consumes $CODER_OIDC_CLIENT_SECRET
      --oidc-email-domain strings                         Email domains that clients logging in with OIDC must match.
                                                          Consumes $CODER_OIDC_EMAIL_DOMAIN
      --oidc-icon-url string                              URL pointing to the icon to use on the OepnID Connect login button
                                                          Consumes $CODER_OIDC_ICON_URL
      --oidc-ignore-email-verified                        Ignore the email_verified claim from the upstream provider.
                                                          Consumes $CODER_OIDC_IGNORE_EMAIL_VERIFIED
      --oidc-issuer-url string                            Issuer URL to use for Login with OIDC.
                                                          Consumes $CODER_OIDC_ISSUER_URL
      --oidc-scopes strings                               Scopes to grant when authenticating with OIDC.
                                                          Consumes $CODER_OIDC_SCOPES (default [openid,profile,email])
      --oidc-sign-in-text string                          The text to show on the OpenID Connect sign in button
                                                          Consumes $CODER_OIDC_SIGN_IN_TEXT (default "OpenID Connect")
      --oidc-username-field string                        OIDC claim field to use as the username.
                                                          Consumes $CODER_OIDC_USERNAME_FIELD (default "preferred_username")
      --postgres-url string                               URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".
                                                          Consumes $CODER_PG_CONNECTION_URL
      --pprof-address string                              The bind address to serve pprof.
                                                          Consumes $CODER_PPROF_ADDRESS (default "127.0.0.1:6060")
      --pprof-enable                                      Serve pprof metrics on the address defined by pprof address.
                                                          Consumes $CODER_PPROF_ENABLE
      --prometheus-address string                         The bind address to serve prometheus metrics.
                                                          Consumes $CODER_PROMETHEUS_ADDRESS (default "127.0.0.1:2112")
      --prometheus-enable                                 Serve prometheus metrics on the address defined by prometheus address.
                                                          Consumes $CODER_PROMETHEUS_ENABLE
      --provisioner-daemon-poll-interval duration         Time to wait before polling for a new job.
                                                          Consumes $CODER_PROVISIONER_DAEMON_POLL_INTERVAL (default 1s)
      --provisioner-daemon-poll-jitter duration           Random jitter added to the poll interval.
                                                          Consumes $CODER_PROVISIONER_DAEMON_POLL_JITTER (default 100ms)
      --provisioner-daemons int                           Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.
                                                          Consumes $CODER_PROVISIONER_DAEMONS (default 3)
      --provisioner-force-cancel-interval duration        Time to force cancel provisioning tasks that are stuck.
                                                          Consumes $CODER_PROVISIONER_FORCE_CANCEL_INTERVAL (default 10m0s)
      --proxy-trusted-headers strings                     Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For
                                                          Consumes $CODER_PROXY_TRUSTED_HEADERS
      --proxy-trusted-origins strings                     Origin addresses to respect "proxy-trusted-headers". e.g. 192.168.1.0/24
                                                          Consumes $CODER_PROXY_TRUSTED_ORIGINS
      --redirect-to-access-url                            Specifies whether to redirect requests that do not match the access URL host.
                                                          Consumes $CODER_REDIRECT_TO_ACCESS_URL
      --secure-auth-cookie                                Controls if the 'Secure' property is set on browser session cookies.
                                                          Consumes $CODER_SECURE_AUTH_COOKIE
      --session-duration duration                         The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.
                                                          Consumes $CODER_MAX_SESSION_EXPIRY (default 24h0m0s)
      --ssh-keygen-algorithm string                       The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".
                                                          Consumes $CODER_SSH_KEYGEN_ALGORITHM (default "ed25519")
      --strict-transport-security int                     Controls if the 'Strict-Transport-Security' header is set on all static file responses. This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of the header.
                                                          Consumes $CODER_STRICT_TRANSPORT_SECURITY
      --strict-transport-security-options strings         Two optional fields can be set in the Strict-Transport-Security header; 'includeSubDomains' and 'preload'. The 'strict-transport-security' flag must be set to a non-zero value for these options to be used.
                                                          Consumes $CODER_STRICT_TRANSPORT_SECURITY_OPTIONS
      --swagger-enable                                    Expose the swagger endpoint via /swagger.
                                                          Consumes $CODER_SWAGGER_ENABLE
      --telemetry                                         Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.
                                                          Consumes $CODER_TELEMETRY_ENABLE (default true)
      --telemetry-trace                                   Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.
                                                          Consumes $CODER_TELEMETRY_TRACE (default true)
      --tls-address string                                HTTPS bind address of the server.
                                                          Consumes $CODER_TLS_ADDRESS (default "127.0.0.1:3443")
      --tls-cert-file strings                             Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.
                                                          Consumes $CODER_TLS_CERT_FILE
      --tls-client-auth string                            Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".
                                                          Consumes $CODER_TLS_CLIENT_AUTH (default "none")
      --tls-client-ca-file string                         PEM-encoded Certificate Authority file used for checking the authenticity of client
                                                          Consumes $CODER_TLS_CLIENT_CA_FILE
      --tls-client-cert-file string                       Path to certificate for client TLS authentication. It requires a PEM-encoded file.
                                                          Consumes $CODER_TLS_CLIENT_CERT_FILE
      --tls-client-key-file string                        Path to key for client TLS authentication. It requires a PEM-encoded file.
                                                          Consumes $CODER_TLS_CLIENT_KEY_FILE
      --tls-enable                                        Whether TLS will be enabled.
                                                          Consumes $CODER_TLS_ENABLE
      --tls-key-file strings                              Paths to the private keys for each of the certificates. It requires a PEM-encoded file.
                                                          Consumes $CODER_TLS_KEY_FILE
      --tls-min-version string                            Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"
                                                          Consumes $CODER_TLS_MIN_VERSION (default "tls12")
      --trace                                             Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md
                                                          Consumes $CODER_TRACE_ENABLE
      --trace-honeycomb-api-key string                    Enables trace exporting to Honeycomb.io using the provided API Key.
                                                          Consumes $CODER_TRACE_HONEYCOMB_API_KEY
      --trace-logs                                        Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs. If the verbose flag was supplied, debug-level logs will be included.
                                                          Consumes $CODER_TRACE_CAPTURE_LOGS
      --update-check                                      Periodically check for new releases of Coder and inform the owner. The check is performed once per day.
                                                          Consumes $CODER_UPDATE_CHECK
      --wildcard-access-url string                        Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".
                                                          Consumes $CODER_WILDCARD_ACCESS_URL
```

### Options inherited from parent commands

```
      --global-config coder   Path to the global coder config directory.
                              Consumes $CODER_CONFIG_DIR (default "~/.config/coderv2")
      --header stringArray    HTTP headers added to all requests. Provide as "Key=Value".
                              Consumes $CODER_HEADER
      --no-feature-warning    Suppress warnings about unlicensed features.
                              Consumes $CODER_NO_FEATURE_WARNING
      --no-version-warning    Suppress warning when client and server versions do not match.
                              Consumes $CODER_NO_VERSION_WARNING
      --token string          Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.
                              Consumes $CODER_SESSION_TOKEN
      --url string            URL to a deployment.
                              Consumes $CODER_URL
  -v, --verbose               Enable verbose output.
                              Consumes $CODER_VERBOSE
```

### SEE ALSO

- [coder](coder.md) -
- [coder server create-admin-user](coder_server_create-admin-user.md) - Create a new admin user with the given username, email and password and adds it to every organization.
- [coder server postgres-builtin-serve](coder_server_postgres-builtin-serve.md) - Run the built-in PostgreSQL deployment.
- [coder server postgres-builtin-url](coder_server_postgres-builtin-url.md) - Output the connection URL for the built-in PostgreSQL deployment.
