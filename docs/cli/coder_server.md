## coder server

Start a Coder server

```
coder server [flags]
```

### Options

```
      --access-url string                            External URL to access your deployment. This must be accessible by all provisioned workspaces.
                                                     [38;2;88;88;88mConsumes $CODER_ACCESS_URL[0m
      --api-rate-limit int                           Maximum number of requests per minute allowed to the API per user, or per IP address for unauthenticated users. Negative values mean no rate limit. Some API endpoints have separate strict rate limits regardless of this value to prevent denial-of-service or brute force attacks.
                                                     [38;2;88;88;88mConsumes $CODER_API_RATE_LIMIT[0m (default 512)
      --cache-dir string                             The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.
                                                     [38;2;88;88;88mConsumes $CODER_CACHE_DIRECTORY[0m (default "/home/coder/.cache/coder")
      --dangerous-allow-path-app-sharing             Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.
                                                     [38;2;88;88;88mConsumes $CODER_DANGEROUS_ALLOW_PATH_APP_SHARING[0m
      --dangerous-allow-path-app-site-owner-access   Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.
                                                     [38;2;88;88;88mConsumes $CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS[0m
      --dangerous-disable-rate-limits                Disables all rate limits. This is not recommended in production.
                                                     [38;2;88;88;88mConsumes $CODER_RATE_LIMIT_DISABLE_ALL[0m
      --derp-config-path string                      Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/
                                                     [38;2;88;88;88mConsumes $CODER_DERP_CONFIG_PATH[0m
      --derp-config-url string                       URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/
                                                     [38;2;88;88;88mConsumes $CODER_DERP_CONFIG_URL[0m
      --derp-server-enable                           Whether to enable or disable the embedded DERP relay server.
                                                     [38;2;88;88;88mConsumes $CODER_DERP_SERVER_ENABLE[0m (default true)
      --derp-server-region-code string               Region code to use for the embedded DERP server.
                                                     [38;2;88;88;88mConsumes $CODER_DERP_SERVER_REGION_CODE[0m (default "coder")
      --derp-server-region-id int                    Region ID to use for the embedded DERP server.
                                                     [38;2;88;88;88mConsumes $CODER_DERP_SERVER_REGION_ID[0m (default 999)
      --derp-server-region-name string               Region name that for the embedded DERP server.
                                                     [38;2;88;88;88mConsumes $CODER_DERP_SERVER_REGION_NAME[0m (default "Coder Embedded Relay")
      --derp-server-stun-addresses strings           Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.
                                                     [38;2;88;88;88mConsumes $CODER_DERP_SERVER_STUN_ADDRESSES[0m (default [stun.l.google.com:19302])
      --disable-path-apps                            Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.
                                                     [38;2;88;88;88mConsumes $CODER_DISABLE_PATH_APPS[0m
      --experiments strings                          Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.
                                                     [38;2;88;88;88mConsumes $CODER_EXPERIMENTS[0m
  -h, --help                                         help for server
      --http-address string                          HTTP bind address of the server. Unset to disable the HTTP endpoint.
                                                     [38;2;88;88;88mConsumes $CODER_HTTP_ADDRESS[0m (default "127.0.0.1:3000")
      --log-human string                             Output human-readable logs to a given file.
                                                     [38;2;88;88;88mConsumes $CODER_LOGGING_HUMAN[0m (default "/dev/stderr")
      --log-json string                              Output JSON logs to a given file.
                                                     [38;2;88;88;88mConsumes $CODER_LOGGING_JSON[0m
      --log-stackdriver string                       Output Stackdriver compatible logs to a given file.
                                                     [38;2;88;88;88mConsumes $CODER_LOGGING_STACKDRIVER[0m
      --max-token-lifetime duration                  The maximum lifetime duration for any user creating a token.
                                                     [38;2;88;88;88mConsumes $CODER_MAX_TOKEN_LIFETIME[0m (default 720h0m0s)
      --oauth2-github-allow-everyone                 Allow all logins, setting this option means allowed orgs and teams must be empty.
                                                     [38;2;88;88;88mConsumes $CODER_OAUTH2_GITHUB_ALLOW_EVERYONE[0m
      --oauth2-github-allow-signups                  Whether new users can sign up with GitHub.
                                                     [38;2;88;88;88mConsumes $CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS[0m
      --oauth2-github-allowed-orgs strings           Organizations the user must be a member of to Login with GitHub.
                                                     [38;2;88;88;88mConsumes $CODER_OAUTH2_GITHUB_ALLOWED_ORGS[0m
      --oauth2-github-allowed-teams strings          Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.
                                                     [38;2;88;88;88mConsumes $CODER_OAUTH2_GITHUB_ALLOWED_TEAMS[0m
      --oauth2-github-client-id string               Client ID for Login with GitHub.
                                                     [38;2;88;88;88mConsumes $CODER_OAUTH2_GITHUB_CLIENT_ID[0m
      --oauth2-github-client-secret string           Client secret for Login with GitHub.
                                                     [38;2;88;88;88mConsumes $CODER_OAUTH2_GITHUB_CLIENT_SECRET[0m
      --oauth2-github-enterprise-base-url string     Base URL of a GitHub Enterprise deployment to use for Login with GitHub.
                                                     [38;2;88;88;88mConsumes $CODER_OAUTH2_GITHUB_ENTERPRISE_BASE_URL[0m
      --oidc-allow-signups                           Whether new users can sign up with OIDC.
                                                     [38;2;88;88;88mConsumes $CODER_OIDC_ALLOW_SIGNUPS[0m (default true)
      --oidc-client-id string                        Client ID to use for Login with OIDC.
                                                     [38;2;88;88;88mConsumes $CODER_OIDC_CLIENT_ID[0m
      --oidc-client-secret string                    Client secret to use for Login with OIDC.
                                                     [38;2;88;88;88mConsumes $CODER_OIDC_CLIENT_SECRET[0m
      --oidc-email-domain strings                    Email domains that clients logging in with OIDC must match.
                                                     [38;2;88;88;88mConsumes $CODER_OIDC_EMAIL_DOMAIN[0m
      --oidc-ignore-email-verified                   Ignore the email_verified claim from the upstream provider.
                                                     [38;2;88;88;88mConsumes $CODER_OIDC_IGNORE_EMAIL_VERIFIED[0m
      --oidc-issuer-url string                       Issuer URL to use for Login with OIDC.
                                                     [38;2;88;88;88mConsumes $CODER_OIDC_ISSUER_URL[0m
      --oidc-scopes strings                          Scopes to grant when authenticating with OIDC.
                                                     [38;2;88;88;88mConsumes $CODER_OIDC_SCOPES[0m (default [openid,profile,email])
      --oidc-username-field string                   OIDC claim field to use as the username.
                                                     [38;2;88;88;88mConsumes $CODER_OIDC_USERNAME_FIELD[0m (default "preferred_username")
      --postgres-url string                          URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".
                                                     [38;2;88;88;88mConsumes $CODER_PG_CONNECTION_URL[0m
      --pprof-address string                         The bind address to serve pprof.
                                                     [38;2;88;88;88mConsumes $CODER_PPROF_ADDRESS[0m (default "127.0.0.1:6060")
      --pprof-enable                                 Serve pprof metrics on the address defined by pprof address.
                                                     [38;2;88;88;88mConsumes $CODER_PPROF_ENABLE[0m
      --prometheus-address string                    The bind address to serve prometheus metrics.
                                                     [38;2;88;88;88mConsumes $CODER_PROMETHEUS_ADDRESS[0m (default "127.0.0.1:2112")
      --prometheus-enable                            Serve prometheus metrics on the address defined by prometheus address.
                                                     [38;2;88;88;88mConsumes $CODER_PROMETHEUS_ENABLE[0m
      --provisioner-daemon-poll-interval duration    Time to wait before polling for a new job.
                                                     [38;2;88;88;88mConsumes $CODER_PROVISIONER_DAEMON_POLL_INTERVAL[0m (default 1s)
      --provisioner-daemon-poll-jitter duration      Random jitter added to the poll interval.
                                                     [38;2;88;88;88mConsumes $CODER_PROVISIONER_DAEMON_POLL_JITTER[0m (default 100ms)
      --provisioner-daemons int                      Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.
                                                     [38;2;88;88;88mConsumes $CODER_PROVISIONER_DAEMONS[0m (default 3)
      --provisioner-force-cancel-interval duration   Time to force cancel provisioning tasks that are stuck.
                                                     [38;2;88;88;88mConsumes $CODER_PROVISIONER_FORCE_CANCEL_INTERVAL[0m (default 10m0s)
      --proxy-trusted-headers strings                Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For
                                                     [38;2;88;88;88mConsumes $CODER_PROXY_TRUSTED_HEADERS[0m
      --proxy-trusted-origins strings                Origin addresses to respect "proxy-trusted-headers". e.g. 192.168.1.0/24
                                                     [38;2;88;88;88mConsumes $CODER_PROXY_TRUSTED_ORIGINS[0m
      --secure-auth-cookie                           Controls if the 'Secure' property is set on browser session cookies.
                                                     [38;2;88;88;88mConsumes $CODER_SECURE_AUTH_COOKIE[0m
      --ssh-keygen-algorithm string                  The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".
                                                     [38;2;88;88;88mConsumes $CODER_SSH_KEYGEN_ALGORITHM[0m (default "ed25519")
      --swagger-enable                               Expose the swagger endpoint via /swagger.
                                                     [38;2;88;88;88mConsumes $CODER_SWAGGER_ENABLE[0m
      --telemetry                                    Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.
                                                     [38;2;88;88;88mConsumes $CODER_TELEMETRY_ENABLE[0m (default true)
      --telemetry-trace                              Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.
                                                     [38;2;88;88;88mConsumes $CODER_TELEMETRY_TRACE[0m (default true)
      --tls-address string                           HTTPS bind address of the server.
                                                     [38;2;88;88;88mConsumes $CODER_TLS_ADDRESS[0m (default "127.0.0.1:3443")
      --tls-cert-file strings                        Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.
                                                     [38;2;88;88;88mConsumes $CODER_TLS_CERT_FILE[0m
      --tls-client-auth string                       Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".
                                                     [38;2;88;88;88mConsumes $CODER_TLS_CLIENT_AUTH[0m (default "none")
      --tls-client-ca-file string                    PEM-encoded Certificate Authority file used for checking the authenticity of client
                                                     [38;2;88;88;88mConsumes $CODER_TLS_CLIENT_CA_FILE[0m
      --tls-client-cert-file string                  Path to certificate for client TLS authentication. It requires a PEM-encoded file.
                                                     [38;2;88;88;88mConsumes $CODER_TLS_CLIENT_CERT_FILE[0m
      --tls-client-key-file string                   Path to key for client TLS authentication. It requires a PEM-encoded file.
                                                     [38;2;88;88;88mConsumes $CODER_TLS_CLIENT_KEY_FILE[0m
      --tls-enable                                   Whether TLS will be enabled.
                                                     [38;2;88;88;88mConsumes $CODER_TLS_ENABLE[0m
      --tls-key-file strings                         Paths to the private keys for each of the certificates. It requires a PEM-encoded file.
                                                     [38;2;88;88;88mConsumes $CODER_TLS_KEY_FILE[0m
      --tls-min-version string                       Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"
                                                     [38;2;88;88;88mConsumes $CODER_TLS_MIN_VERSION[0m (default "tls12")
      --tls-redirect-http-to-https                   Whether HTTP requests will be redirected to the access URL (if it's a https URL and TLS is enabled). Requests to local IP addresses are never redirected regardless of this setting.
                                                     [38;2;88;88;88mConsumes $CODER_TLS_REDIRECT_HTTP[0m (default true)
      --trace                                        Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md
                                                     [38;2;88;88;88mConsumes $CODER_TRACE_ENABLE[0m
      --trace-honeycomb-api-key string               Enables trace exporting to Honeycomb.io using the provided API Key.
                                                     [38;2;88;88;88mConsumes $CODER_TRACE_HONEYCOMB_API_KEY[0m
      --trace-logs                                   Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs. If the verbose flag was supplied, debug-level logs will be included.
                                                     [38;2;88;88;88mConsumes $CODER_TRACE_CAPTURE_LOGS[0m
      --update-check                                 Periodically check for new releases of Coder and inform the owner. The check is performed once per day.
                                                     [38;2;88;88;88mConsumes $CODER_UPDATE_CHECK[0m
      --wildcard-access-url string                   Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".
                                                     [38;2;88;88;88mConsumes $CODER_WILDCARD_ACCESS_URL[0m
```

### Options inherited from parent commands

```
      --global-config coder   Path to the global coder config directory.
                              [38;2;88;88;88mConsumes $CODER_CONFIG_DIR[0m (default "/home/coder/.config/coderv2")
      --header stringArray    HTTP headers added to all requests. Provide as "Key=Value".
                              [38;2;88;88;88mConsumes $CODER_HEADER[0m
      --no-feature-warning    Suppress warnings about unlicensed features.
                              [38;2;88;88;88mConsumes $CODER_NO_FEATURE_WARNING[0m
      --no-version-warning    Suppress warning when client and server versions do not match.
                              [38;2;88;88;88mConsumes $CODER_NO_VERSION_WARNING[0m
      --token string          Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.
                              [38;2;88;88;88mConsumes $CODER_SESSION_TOKEN[0m
      --url string            URL to a deployment.
                              [38;2;88;88;88mConsumes $CODER_URL[0m
  -v, --verbose               Enable verbose output.
                              [38;2;88;88;88mConsumes $CODER_VERBOSE[0m
```

### SEE ALSO

- [coder](coder.md) -
- [coder server postgres-builtin-serve](coder_server_postgres-builtin-serve.md) - Run the built-in PostgreSQL deployment.
- [coder server postgres-builtin-url](coder_server_postgres-builtin-url.md) - Output the connection URL for the built-in PostgreSQL deployment.

###### Auto generated by spf13/cobra on 26-Jan-2023
