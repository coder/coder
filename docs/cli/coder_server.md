<!-- DO NOT EDIT | GENERATED CONTENT -->
# coder server

Start a Coder server
## Usage
```console
coder server [flags]
```

## Subcommands
| Name |   Purpose |
| ---- |   ----- |
| <code>create-admin-user</code> | Create a new admin user with the given username, email and password and adds it to every organization. |
| <code>postgres-builtin-serve</code> | Run the built-in PostgreSQL deployment. |
| <code>postgres-builtin-url</code> | Output the connection URL for the built-in PostgreSQL deployment. |

## Local Flags
| Name |  Default | Usage |
| ---- |  ------- | ----- |
| --access-url |  | <code>External URL to access your deployment. This must be accessible by all provisioned workspaces.<br/>Consumes $CODER_ACCESS_URL</code>|
| --address, -a |  | <code>Bind address of the server.<br/>Consumes $CODER_ADDRESS</code>|
| --agent-fallback-troubleshooting-url | https://coder.com/docs/coder-oss/latest/templates#troubleshooting-templates | <code>URL to use for agent troubleshooting when not set in the template<br/>Consumes $CODER_AGENT_FALLBACK_TROUBLESHOOTING_URL</code>|
| --agent-stats-refresh-interval | 10m0s | <code>How frequently agent stats are recorded<br/>Consumes $CODER_AGENT_STAT_REFRESH_INTERVAL</code>|
| --api-rate-limit | 512 | <code>Maximum number of requests per minute allowed to the API per user, or per IP address for unauthenticated users. Negative values mean no rate limit. Some API endpoints have separate strict rate limits regardless of this value to prevent denial-of-service or brute force attacks.<br/>Consumes $CODER_API_RATE_LIMIT</code>|
| --autobuild-poll-interval | 1m0s | <code>Interval to poll for scheduled workspace builds.<br/>Consumes $CODER_AUTOBUILD_POLL_INTERVAL</code>|
| --cache-dir | ~/.cache/coder | <code>The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.<br/>Consumes $CODER_CACHE_DIRECTORY</code>|
| --dangerous-allow-path-app-sharing | false | <code>Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.<br/>Consumes $CODER_DANGEROUS_ALLOW_PATH_APP_SHARING</code>|
| --dangerous-allow-path-app-site-owner-access | false | <code>Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.<br/>Consumes $CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS</code>|
| --dangerous-disable-rate-limits | false | <code>Disables all rate limits. This is not recommended in production.<br/>Consumes $CODER_RATE_LIMIT_DISABLE_ALL</code>|
| --derp-config-path |  | <code>Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/<br/>Consumes $CODER_DERP_CONFIG_PATH</code>|
| --derp-config-url |  | <code>URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/<br/>Consumes $CODER_DERP_CONFIG_URL</code>|
| --derp-server-enable | true | <code>Whether to enable or disable the embedded DERP relay server.<br/>Consumes $CODER_DERP_SERVER_ENABLE</code>|
| --derp-server-region-code | coder | <code>Region code to use for the embedded DERP server.<br/>Consumes $CODER_DERP_SERVER_REGION_CODE</code>|
| --derp-server-region-id | 999 | <code>Region ID to use for the embedded DERP server.<br/>Consumes $CODER_DERP_SERVER_REGION_ID</code>|
| --derp-server-region-name | Coder Embedded Relay | <code>Region name that for the embedded DERP server.<br/>Consumes $CODER_DERP_SERVER_REGION_NAME</code>|
| --derp-server-stun-addresses | [stun.l.google.com:19302] | <code>Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.<br/>Consumes $CODER_DERP_SERVER_STUN_ADDRESSES</code>|
| --disable-password-auth | false | <code>Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.<br/>Consumes $CODER_DISABLE_PASSWORD_AUTH</code>|
| --disable-path-apps | false | <code>Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.<br/>Consumes $CODER_DISABLE_PATH_APPS</code>|
| --disable-session-expiry-refresh | false | <code>Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.<br/>Consumes $CODER_DISABLE_SESSION_EXPIRY_REFRESH</code>|
| --experimental | false | <code>Enable experimental features. Experimental features are not ready for production.<br/>Consumes $CODER_EXPERIMENTAL</code>|
| --experiments | [] | <code>Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.<br/>Consumes $CODER_EXPERIMENTS</code>|
| --http-address | 127.0.0.1:3000 | <code>HTTP bind address of the server. Unset to disable the HTTP endpoint.<br/>Consumes $CODER_HTTP_ADDRESS</code>|
| --in-memory | false | <code>Controls whether data will be stored in an in-memory database.<br/>Consumes $CODER_IN_MEMORY_DATABASE</code>|
| --log-human | /dev/stderr | <code>Output human-readable logs to a given file.<br/>Consumes $CODER_LOGGING_HUMAN</code>|
| --log-json |  | <code>Output JSON logs to a given file.<br/>Consumes $CODER_LOGGING_JSON</code>|
| --log-stackdriver |  | <code>Output Stackdriver compatible logs to a given file.<br/>Consumes $CODER_LOGGING_STACKDRIVER</code>|
| --max-token-lifetime | 720h0m0s | <code>The maximum lifetime duration users can specify when creating an API token.<br/>Consumes $CODER_MAX_TOKEN_LIFETIME</code>|
| --metrics-cache-refresh-interval | 1h0m0s | <code>How frequently metrics are refreshed<br/>Consumes $CODER_METRICS_CACHE_REFRESH_INTERVAL</code>|
| --oauth2-github-allow-everyone | false | <code>Allow all logins, setting this option means allowed orgs and teams must be empty.<br/>Consumes $CODER_OAUTH2_GITHUB_ALLOW_EVERYONE</code>|
| --oauth2-github-allow-signups | false | <code>Whether new users can sign up with GitHub.<br/>Consumes $CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS</code>|
| --oauth2-github-allowed-orgs | [] | <code>Organizations the user must be a member of to Login with GitHub.<br/>Consumes $CODER_OAUTH2_GITHUB_ALLOWED_ORGS</code>|
| --oauth2-github-allowed-teams | [] | <code>Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.<br/>Consumes $CODER_OAUTH2_GITHUB_ALLOWED_TEAMS</code>|
| --oauth2-github-client-id |  | <code>Client ID for Login with GitHub.<br/>Consumes $CODER_OAUTH2_GITHUB_CLIENT_ID</code>|
| --oauth2-github-client-secret |  | <code>Client secret for Login with GitHub.<br/>Consumes $CODER_OAUTH2_GITHUB_CLIENT_SECRET</code>|
| --oauth2-github-enterprise-base-url |  | <code>Base URL of a GitHub Enterprise deployment to use for Login with GitHub.<br/>Consumes $CODER_OAUTH2_GITHUB_ENTERPRISE_BASE_URL</code>|
| --oidc-allow-signups | true | <code>Whether new users can sign up with OIDC.<br/>Consumes $CODER_OIDC_ALLOW_SIGNUPS</code>|
| --oidc-client-id |  | <code>Client ID to use for Login with OIDC.<br/>Consumes $CODER_OIDC_CLIENT_ID</code>|
| --oidc-client-secret |  | <code>Client secret to use for Login with OIDC.<br/>Consumes $CODER_OIDC_CLIENT_SECRET</code>|
| --oidc-email-domain | [] | <code>Email domains that clients logging in with OIDC must match.<br/>Consumes $CODER_OIDC_EMAIL_DOMAIN</code>|
| --oidc-icon-url |  | <code>URL pointing to the icon to use on the OepnID Connect login button<br/>Consumes $CODER_OIDC_ICON_URL</code>|
| --oidc-ignore-email-verified | false | <code>Ignore the email_verified claim from the upstream provider.<br/>Consumes $CODER_OIDC_IGNORE_EMAIL_VERIFIED</code>|
| --oidc-issuer-url |  | <code>Issuer URL to use for Login with OIDC.<br/>Consumes $CODER_OIDC_ISSUER_URL</code>|
| --oidc-scopes | [openid,profile,email] | <code>Scopes to grant when authenticating with OIDC.<br/>Consumes $CODER_OIDC_SCOPES</code>|
| --oidc-sign-in-text | OpenID Connect | <code>The text to show on the OpenID Connect sign in button<br/>Consumes $CODER_OIDC_SIGN_IN_TEXT</code>|
| --oidc-username-field | preferred_username | <code>OIDC claim field to use as the username.<br/>Consumes $CODER_OIDC_USERNAME_FIELD</code>|
| --postgres-url |  | <code>URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".<br/>Consumes $CODER_PG_CONNECTION_URL</code>|
| --pprof-address | 127.0.0.1:6060 | <code>The bind address to serve pprof.<br/>Consumes $CODER_PPROF_ADDRESS</code>|
| --pprof-enable | false | <code>Serve pprof metrics on the address defined by pprof address.<br/>Consumes $CODER_PPROF_ENABLE</code>|
| --prometheus-address | 127.0.0.1:2112 | <code>The bind address to serve prometheus metrics.<br/>Consumes $CODER_PROMETHEUS_ADDRESS</code>|
| --prometheus-enable | false | <code>Serve prometheus metrics on the address defined by prometheus address.<br/>Consumes $CODER_PROMETHEUS_ENABLE</code>|
| --provisioner-daemon-poll-interval | 1s | <code>Time to wait before polling for a new job.<br/>Consumes $CODER_PROVISIONER_DAEMON_POLL_INTERVAL</code>|
| --provisioner-daemon-poll-jitter | 100ms | <code>Random jitter added to the poll interval.<br/>Consumes $CODER_PROVISIONER_DAEMON_POLL_JITTER</code>|
| --provisioner-daemons | 3 | <code>Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.<br/>Consumes $CODER_PROVISIONER_DAEMONS</code>|
| --provisioner-force-cancel-interval | 10m0s | <code>Time to force cancel provisioning tasks that are stuck.<br/>Consumes $CODER_PROVISIONER_FORCE_CANCEL_INTERVAL</code>|
| --proxy-trusted-headers | [] | <code>Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For<br/>Consumes $CODER_PROXY_TRUSTED_HEADERS</code>|
| --proxy-trusted-origins | [] | <code>Origin addresses to respect "proxy-trusted-headers". e.g. 192.168.1.0/24<br/>Consumes $CODER_PROXY_TRUSTED_ORIGINS</code>|
| --redirect-to-access-url | false | <code>Specifies whether to redirect requests that do not match the access URL host.<br/>Consumes $CODER_REDIRECT_TO_ACCESS_URL</code>|
| --secure-auth-cookie | false | <code>Controls if the 'Secure' property is set on browser session cookies.<br/>Consumes $CODER_SECURE_AUTH_COOKIE</code>|
| --session-duration | 24h0m0s | <code>The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.<br/>Consumes $CODER_MAX_SESSION_EXPIRY</code>|
| --ssh-keygen-algorithm | ed25519 | <code>The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".<br/>Consumes $CODER_SSH_KEYGEN_ALGORITHM</code>|
| --strict-transport-security | 0 | <code>Controls if the 'Strict-Transport-Security' header is set on all static file responses. This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of the header.<br/>Consumes $CODER_STRICT_TRANSPORT_SECURITY</code>|
| --strict-transport-security-options | [] | <code>Two optional fields can be set in the Strict-Transport-Security header; 'includeSubDomains' and 'preload'. The 'strict-transport-security' flag must be set to a non-zero value for these options to be used.<br/>Consumes $CODER_STRICT_TRANSPORT_SECURITY_OPTIONS</code>|
| --swagger-enable | false | <code>Expose the swagger endpoint via /swagger.<br/>Consumes $CODER_SWAGGER_ENABLE</code>|
| --telemetry | true | <code>Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.<br/>Consumes $CODER_TELEMETRY_ENABLE</code>|
| --telemetry-trace | true | <code>Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.<br/>Consumes $CODER_TELEMETRY_TRACE</code>|
| --telemetry-url | https://telemetry.coder.com | <code>URL to send telemetry.<br/>Consumes $CODER_TELEMETRY_URL</code>|
| --tls-address | 127.0.0.1:3443 | <code>HTTPS bind address of the server.<br/>Consumes $CODER_TLS_ADDRESS</code>|
| --tls-cert-file | [] | <code>Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.<br/>Consumes $CODER_TLS_CERT_FILE</code>|
| --tls-client-auth | none | <code>Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".<br/>Consumes $CODER_TLS_CLIENT_AUTH</code>|
| --tls-client-ca-file |  | <code>PEM-encoded Certificate Authority file used for checking the authenticity of client<br/>Consumes $CODER_TLS_CLIENT_CA_FILE</code>|
| --tls-client-cert-file |  | <code>Path to certificate for client TLS authentication. It requires a PEM-encoded file.<br/>Consumes $CODER_TLS_CLIENT_CERT_FILE</code>|
| --tls-client-key-file |  | <code>Path to key for client TLS authentication. It requires a PEM-encoded file.<br/>Consumes $CODER_TLS_CLIENT_KEY_FILE</code>|
| --tls-enable | false | <code>Whether TLS will be enabled.<br/>Consumes $CODER_TLS_ENABLE</code>|
| --tls-key-file | [] | <code>Paths to the private keys for each of the certificates. It requires a PEM-encoded file.<br/>Consumes $CODER_TLS_KEY_FILE</code>|
| --tls-min-version | tls12 | <code>Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"<br/>Consumes $CODER_TLS_MIN_VERSION</code>|
| --tls-redirect-http-to-https | true | <code>Whether HTTP requests will be redirected to the access URL (if it's a https URL and TLS is enabled). Requests to local IP addresses are never redirected regardless of this setting.<br/>Consumes $CODER_TLS_REDIRECT_HTTP</code>|
| --trace | false | <code>Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md<br/>Consumes $CODER_TRACE_ENABLE</code>|
| --trace-honeycomb-api-key |  | <code>Enables trace exporting to Honeycomb.io using the provided API Key.<br/>Consumes $CODER_TRACE_HONEYCOMB_API_KEY</code>|
| --trace-logs | false | <code>Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs. If the verbose flag was supplied, debug-level logs will be included.<br/>Consumes $CODER_TRACE_CAPTURE_LOGS</code>|
| --update-check | false | <code>Periodically check for new releases of Coder and inform the owner. The check is performed once per day.<br/>Consumes $CODER_UPDATE_CHECK</code>|
| --wildcard-access-url |  | <code>Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".<br/>Consumes $CODER_WILDCARD_ACCESS_URL</code>|