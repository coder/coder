<!-- DO NOT EDIT | GENERATED CONTENT -->
# Configuration reference

Coder server is configured primarily through environment variables. This page
lists every option so you can search by environment variable name, CLI flag, or
YAML key. For first-time setup guidance and worked examples, see
[Configure Control Plane Access](./index.md).

Each option can be set through one or more of the methods below. An option lists
only the methods that apply to it.

- An environment variable (recommended for production deployments running as a
  system service, container, or Helm chart).
- A CLI flag passed to `coder server` (useful for one-off invocations
  and local development).
- A key in a YAML configuration file passed with `--config`.

For a full description of each option's accepted values and behavior, follow the
flag link into the [`coder server` CLI reference](../../reference/cli/server.md).

Deprecated options are listed at the end of each section.

## General

### Allow workspace renames

Allow users to rename their workspaces. WARNING: Renaming a workspace can cause Terraform resources that depend on the workspace name to be destroyed and recreated, potentially causing data loss. Only enable this if your templates do not use workspace names in resource identifiers, or if you understand the risks.

- Environment variable: `CODER_ALLOW_WORKSPACE_RENAMES`
- CLI flag: [`--allow-workspace-renames`](../../reference/cli/server.md#--allow-workspace-renames)
- YAML key: `allowWorkspaceRenames`
- Default value: `false`

### Cache directory

The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd. This directory is NOT safe to be configured as a shared directory across coderd/provisionerd replicas.

- Environment variable: `CODER_CACHE_DIRECTORY`
- CLI flag: [`--cache-dir`](../../reference/cli/server.md#--cache-dir)
- YAML key: `cacheDir`
- Default value: `~/.cache/coder`

### Default OAuth refresh lifetime

The default lifetime duration for OAuth2 refresh tokens. This controls how long refresh tokens remain valid after issuance or rotation.

- Environment variable: `CODER_DEFAULT_OAUTH_REFRESH_LIFETIME`
- CLI flag: [`--default-oauth-refresh-lifetime`](../../reference/cli/server.md#--default-oauth-refresh-lifetime)
- YAML key: `defaultOAuthRefreshLifetime`
- Default value: `720h0m0s`

### Default token lifetime

The default lifetime duration for API tokens. This value is used when creating a token without specifying a duration, such as when authenticating the CLI or an IDE plugin.

- Environment variable: `CODER_DEFAULT_TOKEN_LIFETIME`
- CLI flag: [`--default-token-lifetime`](../../reference/cli/server.md#--default-token-lifetime)
- YAML key: `defaultTokenLifetime`
- Default value: `168h0m0s`

### Disable chat sharing

Disable chat sharing. Chat ACL checking is disabled and only owners can access their chats.

- Environment variable: `CODER_DISABLE_CHAT_SHARING`
- CLI flag: [`--disable-chat-sharing`](../../reference/cli/server.md#--disable-chat-sharing)
- YAML key: `disableChatSharing`

### Disable owner workspace access

Remove the permission for the 'owner' role to have workspace execution on all workspaces. This prevents the 'owner' from ssh, apps, and terminal access based on the 'owner' role. They still have their user permissions to access their own workspaces.

- Environment variable: `CODER_DISABLE_OWNER_WORKSPACE_ACCESS`
- CLI flag: [`--disable-owner-workspace-access`](../../reference/cli/server.md#--disable-owner-workspace-access)
- YAML key: `disableOwnerWorkspaceAccess`

### Disable path apps

Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.

- Environment variable: `CODER_DISABLE_PATH_APPS`
- CLI flag: [`--disable-path-apps`](../../reference/cli/server.md#--disable-path-apps)
- YAML key: `disablePathApps`

### Disable workspace sharing

Disable workspace sharing. Workspace ACL checking is disabled and only owners can have ssh, apps and terminal access to workspaces. Access based on the 'owner' role is also allowed unless disabled via --disable-owner-workspace-access.

- Environment variable: `CODER_DISABLE_WORKSPACE_SHARING`
- CLI flag: [`--disable-workspace-sharing`](../../reference/cli/server.md#--disable-workspace-sharing)
- YAML key: `disableWorkspaceSharing`

### Enable swagger endpoint

Expose the swagger endpoint via /swagger.

- Environment variable: `CODER_SWAGGER_ENABLE`
- CLI flag: [`--swagger-enable`](../../reference/cli/server.md#--swagger-enable)
- YAML key: `enableSwagger`

### Experiments

Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.

- Environment variable: `CODER_EXPERIMENTS`
- CLI flag: [`--experiments`](../../reference/cli/server.md#--experiments)
- YAML key: `experiments`

### External auth GitHub default provider enable

Enable the default GitHub external auth provider managed by Coder.

- Environment variable: `CODER_EXTERNAL_AUTH_GITHUB_DEFAULT_PROVIDER_ENABLE`
- CLI flag: [`--external-auth-github-default-provider-enable`](../../reference/cli/server.md#--external-auth-github-default-provider-enable)
- YAML key: `externalAuthGithubDefaultProviderEnable`
- Default value: `true`

### External token encryption keys

Encrypt OIDC and Git authentication tokens with AES-256-GCM in the database. The value must be a comma-separated list of base64-encoded keys. Each key, when base64-decoded, must be exactly 32 bytes in length. The first key will be used to encrypt new values. Subsequent keys will be used as a fallback when decrypting. During normal operation it is recommended to only set one key unless you are in the process of rotating keys with the `coder server dbcrypt rotate` command.

- Environment variable: `CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS`
- CLI flag: [`--external-token-encryption-keys`](../../reference/cli/server.md#--external-token-encryption-keys)

### Postgres auth

Type of auth to use when connecting to postgres. For AWS RDS, using IAM authentication (awsiamrds) is recommended.

- Environment variable: `CODER_PG_AUTH`
- CLI flag: [`--postgres-auth`](../../reference/cli/server.md#--postgres-auth)
- YAML key: `pgAuth`
- Default value: `password`

### Postgres connection max idle

Maximum number of idle connections to the database. Set to "auto" (the default) to use max open / 3. Value must be greater or equal to 0; 0 means explicitly no idle connections.

- Environment variable: `CODER_PG_CONN_MAX_IDLE`
- CLI flag: [`--postgres-conn-max-idle`](../../reference/cli/server.md#--postgres-conn-max-idle)
- YAML key: `pgConnMaxIdle`
- Default value: `auto`

### Postgres connection max open

Maximum number of open connections to the database. Defaults to 10.

- Environment variable: `CODER_PG_CONN_MAX_OPEN`
- CLI flag: [`--postgres-conn-max-open`](../../reference/cli/server.md#--postgres-conn-max-open)
- YAML key: `pgConnMaxOpen`
- Default value: `10`

### Postgres connection URL

URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url". Note that any special characters in the URL must be URL-encoded.

- Environment variable: `CODER_PG_CONNECTION_URL`
- CLI flag: [`--postgres-url`](../../reference/cli/server.md#--postgres-url)

### SCIM API key

Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.

- Environment variable: `CODER_SCIM_AUTH_HEADER`
- CLI flag: [`--scim-auth-header`](../../reference/cli/server.md#--scim-auth-header)

### SCIM use legacy

Use the legacy SCIM implementation instead of the SCIM 2.0 handler. This is provided for backward compatibility for existing users.

- Environment variable: `CODER_SCIM_USE_LEGACY`
- CLI flag: [`--scim-use-legacy`](../../reference/cli/server.md#--scim-use-legacy)
- YAML key: `scimUseLegacy`
- Default value: `true`

### SSH keygen algorithm

The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".

- Environment variable: `CODER_SSH_KEYGEN_ALGORITHM`
- CLI flag: [`--ssh-keygen-algorithm`](../../reference/cli/server.md#--ssh-keygen-algorithm)
- YAML key: `sshKeygenAlgorithm`
- Default value: `ed25519`

### Support links

Support links to display in the top right drop down menu.

- Environment variable: `CODER_SUPPORT_LINKS`
- CLI flag: [`--support-links`](../../reference/cli/server.md#--support-links)
- YAML key: `supportLinks`

### Terms of service URL

A URL to an external Terms of Service that must be accepted by users when logging in.

- Environment variable: `CODER_TERMS_OF_SERVICE_URL`
- CLI flag: [`--terms-of-service-url`](../../reference/cli/server.md#--terms-of-service-url)
- YAML key: `termsOfServiceURL`

### Update check

Periodically check for new releases of Coder and inform the owner. The check is performed once per day.

- Environment variable: `CODER_UPDATE_CHECK`
- CLI flag: [`--update-check`](../../reference/cli/server.md#--update-check)
- YAML key: `updateCheck`
- Default value: `false`

## AI Gateway

### AI budget period

Determines when accumulated AI spend resets to zero, aligned to UTC calendar boundaries. Only "month" is currently supported.

- Environment variable: `CODER_AI_BUDGET_PERIOD`
- CLI flag: [`--ai-budget-period`](../../reference/cli/server.md#--ai-budget-period)
- YAML key: `ai_gateway.budget_period`
- Default value: `month`

### AI budget policy

Determines the effective group when a user belongs to multiple groups with AI budgets. "highest" selects the group with the largest spend limit, and is currently the only supported value.

- Environment variable: `CODER_AI_BUDGET_POLICY`
- CLI flag: [`--ai-budget-policy`](../../reference/cli/server.md#--ai-budget-policy)
- YAML key: `ai_gateway.budget_policy`
- Default value: `highest`

### API dump directory

Base directory for dumping AI Gateway request/response pairs to disk for debugging. When set, each provider writes under a subdirectory named after the provider. Sensitive headers are redacted. Leave empty to disable.

- Environment variable: `CODER_AI_GATEWAY_DUMP_DIR`
- CLI flag: [`--ai-gateway-dump-dir`](../../reference/cli/server.md#--ai-gateway-dump-dir)
- YAML key: `ai_gateway.api_dump_dir`

### Allow BYOK

Allow users to provide their own LLM API keys or subscriptions. When disabled, only centralized key authentication is permitted.

- Environment variable: `CODER_AI_GATEWAY_ALLOW_BYOK`
- CLI flag: [`--ai-gateway-allow-byok`](../../reference/cli/server.md#--ai-gateway-allow-byok)
- YAML key: `ai_gateway.allow_byok`
- Default value: `true`

### Circuit breaker enabled

Enable the circuit breaker to protect against cascading failures from upstream AI provider overload (503, 529).

- Environment variable: `CODER_AI_GATEWAY_CIRCUIT_BREAKER_ENABLED`
- CLI flag: [`--ai-gateway-circuit-breaker-enabled`](../../reference/cli/server.md#--ai-gateway-circuit-breaker-enabled)
- YAML key: `ai_gateway.circuit_breaker_enabled`
- Default value: `false`

### Data retention duration

Length of time to retain data such as interceptions and all related records (token, prompt, tool use).

- Environment variable: `CODER_AI_GATEWAY_RETENTION`
- CLI flag: [`--ai-gateway-retention`](../../reference/cli/server.md#--ai-gateway-retention)
- YAML key: `ai_gateway.retention`
- Default value: `60d`

### Enabled

Whether to start an in-memory AI Gateway instance.

- Environment variable: `CODER_AI_GATEWAY_ENABLED`
- CLI flag: [`--ai-gateway-enabled`](../../reference/cli/server.md#--ai-gateway-enabled)
- YAML key: `ai_gateway.enabled`
- Default value: `true`

### Max concurrency

Maximum number of concurrent AI Gateway requests per replica. Set to 0 to disable (unlimited).

- Environment variable: `CODER_AI_GATEWAY_MAX_CONCURRENCY`
- CLI flag: [`--ai-gateway-max-concurrency`](../../reference/cli/server.md#--ai-gateway-max-concurrency)
- YAML key: `ai_gateway.max_concurrency`
- Default value: `0`

### Rate limit

Maximum number of AI Gateway requests per second per replica. Set to 0 to disable (unlimited).

- Environment variable: `CODER_AI_GATEWAY_RATE_LIMIT`
- CLI flag: [`--ai-gateway-rate-limit`](../../reference/cli/server.md#--ai-gateway-rate-limit)
- YAML key: `ai_gateway.rate_limit`
- Default value: `0`

### Send actor headers

Once enabled, extra headers will be added to upstream requests to identify the user (actor) making requests to AI Gateway. This is only needed if you are using a proxy between AI Gateway and an upstream AI provider. This will send X-Ai-Bridge-Actor-Id (the ID of the user making the request) and X-Ai-Bridge-Actor-Metadata-Username (their username).

- Environment variable: `CODER_AI_GATEWAY_SEND_ACTOR_HEADERS`
- CLI flag: [`--ai-gateway-send-actor-headers`](../../reference/cli/server.md#--ai-gateway-send-actor-headers)
- YAML key: `ai_gateway.send_actor_headers`
- Default value: `false`

### Structured logging

Emit structured logs for AI Gateway interception records. Use this for exporting these records to external SIEM or observability systems.

- Environment variable: `CODER_AI_GATEWAY_STRUCTURED_LOGGING`
- CLI flag: [`--ai-gateway-structured-logging`](../../reference/cli/server.md#--ai-gateway-structured-logging)
- YAML key: `ai_gateway.structured_logging`
- Default value: `false`

### Anthropic base URL

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The base URL of the Anthropic API.

- Environment variable: `CODER_AI_GATEWAY_ANTHROPIC_BASE_URL`
- CLI flag: [`--ai-gateway-anthropic-base-url`](../../reference/cli/server.md#--ai-gateway-anthropic-base-url)
- YAML key: `ai_gateway.anthropic_base_url`
- Default value: `https://api.anthropic.com/`

### Anthropic key

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The key to authenticate against the Anthropic API.

- Environment variable: `CODER_AI_GATEWAY_ANTHROPIC_KEY`
- CLI flag: [`--ai-gateway-anthropic-key`](../../reference/cli/server.md#--ai-gateway-anthropic-key)

### Bedrock access key

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The access key to authenticate against the AWS Bedrock API.

- Environment variable: `CODER_AI_GATEWAY_BEDROCK_ACCESS_KEY`
- CLI flag: [`--ai-gateway-bedrock-access-key`](../../reference/cli/server.md#--ai-gateway-bedrock-access-key)

### Bedrock access key secret

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The access key secret to use with the access key to authenticate against the AWS Bedrock API.

- Environment variable: `CODER_AI_GATEWAY_BEDROCK_ACCESS_KEY_SECRET`
- CLI flag: [`--ai-gateway-bedrock-access-key-secret`](../../reference/cli/server.md#--ai-gateway-bedrock-access-key-secret)

### Bedrock base URL

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The base URL to use for the AWS Bedrock API. Use this setting to specify an exact URL to use. Takes precedence over CODER_AI_GATEWAY_BEDROCK_REGION.

- Environment variable: `CODER_AI_GATEWAY_BEDROCK_BASE_URL`
- CLI flag: [`--ai-gateway-bedrock-base-url`](../../reference/cli/server.md#--ai-gateway-bedrock-base-url)
- YAML key: `ai_gateway.bedrock_base_url`

### Bedrock model

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The model to use when making requests to the AWS Bedrock API.

- Environment variable: `CODER_AI_GATEWAY_BEDROCK_MODEL`
- CLI flag: [`--ai-gateway-bedrock-model`](../../reference/cli/server.md#--ai-gateway-bedrock-model)
- YAML key: `ai_gateway.bedrock_model`
- Default value: `global.anthropic.claude-sonnet-4-5-20250929-v1:0`

### Bedrock region

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The AWS Bedrock API region to use. Constructs a base URL to use for the AWS Bedrock API in the form of 'https://bedrock-runtime.<region>.amazonaws.com'.

- Environment variable: `CODER_AI_GATEWAY_BEDROCK_REGION`
- CLI flag: [`--ai-gateway-bedrock-region`](../../reference/cli/server.md#--ai-gateway-bedrock-region)
- YAML key: `ai_gateway.bedrock_region`

### Bedrock small fast model

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The small fast model to use when making requests to the AWS Bedrock API. Claude Code uses Haiku-class models to perform background tasks. See https://docs.claude.com/en/docs/claude-code/settings#environment-variables.

- Environment variable: `CODER_AI_GATEWAY_BEDROCK_SMALL_FAST_MODEL`
- CLI flag: [`--ai-gateway-bedrock-small-fastmodel`](../../reference/cli/server.md#--ai-gateway-bedrock-small-fastmodel)
- YAML key: `ai_gateway.bedrock_small_fast_model`
- Default value: `global.anthropic.claude-haiku-4-5-20251001-v1:0`

### OpenAI base URL

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The base URL of the OpenAI API.

- Environment variable: `CODER_AI_GATEWAY_OPENAI_BASE_URL`
- CLI flag: [`--ai-gateway-openai-base-url`](../../reference/cli/server.md#--ai-gateway-openai-base-url)
- YAML key: `ai_gateway.openai_base_url`
- Default value: `https://api.openai.com/v1/`

### OpenAI key

**Deprecated**: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The key to authenticate against the OpenAI API.

- Environment variable: `CODER_AI_GATEWAY_OPENAI_KEY`
- CLI flag: [`--ai-gateway-openai-key`](../../reference/cli/server.md#--ai-gateway-openai-key)

## AI Gateway Proxy

### API dump directory

Directory for dumping MITM request/response pairs to disk for debugging. When set, each proxied request produces .req.txt and .resp.txt files organized by provider. Sensitive headers are redacted. Leave empty to disable.

- Environment variable: `CODER_AI_GATEWAY_PROXY_DUMP_DIR`
- CLI flag: [`--ai-gateway-proxy-dump-dir`](../../reference/cli/server.md#--ai-gateway-proxy-dump-dir)
- YAML key: `ai_gateway_proxy.api_dump_dir`

### Allowed private CIDRs

Comma-separated list of CIDR ranges that are permitted even though they fall within blocked private/reserved IP ranges. By default all private ranges are blocked to prevent SSRF attacks. Use this to allow access to specific internal networks.

- Environment variable: `CODER_AI_GATEWAY_PROXY_ALLOWED_PRIVATE_CIDRS`
- CLI flag: [`--ai-gateway-proxy-allowed-private-cidrs`](../../reference/cli/server.md#--ai-gateway-proxy-allowed-private-cidrs)
- YAML key: `ai_gateway_proxy.allowed_private_cidrs`

### Enabled

Enable the AI Gateway MITM Proxy for intercepting and decrypting AI provider requests.

- Environment variable: `CODER_AI_GATEWAY_PROXY_ENABLED`
- CLI flag: [`--ai-gateway-proxy-enabled`](../../reference/cli/server.md#--ai-gateway-proxy-enabled)
- YAML key: `ai_gateway_proxy.enabled`
- Default value: `false`

### Listen address

The address the AI Gateway Proxy will listen on.

- Environment variable: `CODER_AI_GATEWAY_PROXY_LISTEN_ADDR`
- CLI flag: [`--ai-gateway-proxy-listen-addr`](../../reference/cli/server.md#--ai-gateway-proxy-listen-addr)
- YAML key: `ai_gateway_proxy.listen_addr`
- Default value: `:8888`

### MITM CA certificate file

Path to the CA certificate file used to intercept (MITM) HTTPS traffic from AI clients. This CA must be trusted by AI clients for the proxy to decrypt their requests.

- Environment variable: `CODER_AI_GATEWAY_PROXY_CERT_FILE`
- CLI flag: [`--ai-gateway-proxy-cert-file`](../../reference/cli/server.md#--ai-gateway-proxy-cert-file)
- YAML key: `ai_gateway_proxy.cert_file`

### MITM CA key file

Path to the CA private key file used to intercept (MITM) HTTPS traffic from AI clients.

- Environment variable: `CODER_AI_GATEWAY_PROXY_KEY_FILE`
- CLI flag: [`--ai-gateway-proxy-key-file`](../../reference/cli/server.md#--ai-gateway-proxy-key-file)
- YAML key: `ai_gateway_proxy.key_file`

### TLS certificate file

Path to the TLS certificate file for the AI Gateway Proxy listener. Must be set together with AI Gateway Proxy TLS Key File.

- Environment variable: `CODER_AI_GATEWAY_PROXY_TLS_CERT_FILE`
- CLI flag: [`--ai-gateway-proxy-tls-cert-file`](../../reference/cli/server.md#--ai-gateway-proxy-tls-cert-file)
- YAML key: `ai_gateway_proxy.tls_cert_file`

### TLS key file

Path to the TLS private key file for the AI Gateway Proxy listener. Must be set together with AI Gateway Proxy TLS Certificate File.

- Environment variable: `CODER_AI_GATEWAY_PROXY_TLS_KEY_FILE`
- CLI flag: [`--ai-gateway-proxy-tls-key-file`](../../reference/cli/server.md#--ai-gateway-proxy-tls-key-file)
- YAML key: `ai_gateway_proxy.tls_key_file`

### Upstream proxy

URL of an upstream HTTP proxy to chain tunneled (non-allowlisted) requests through. Format: http://[user:pass@]host:port or https://[user:pass@]host:port.

- Environment variable: `CODER_AI_GATEWAY_PROXY_UPSTREAM`
- CLI flag: [`--ai-gateway-proxy-upstream`](../../reference/cli/server.md#--ai-gateway-proxy-upstream)
- YAML key: `ai_gateway_proxy.upstream_proxy`

### Upstream proxy CA

Path to a PEM-encoded CA certificate to trust for the upstream proxy's TLS connection. Only needed for HTTPS upstream proxies with certificates not trusted by the system. If not provided, the system certificate pool is used.

- Environment variable: `CODER_AI_GATEWAY_PROXY_UPSTREAM_CA`
- CLI flag: [`--ai-gateway-proxy-upstream-ca`](../../reference/cli/server.md#--ai-gateway-proxy-upstream-ca)
- YAML key: `ai_gateway_proxy.upstream_proxy_ca`

## Chat

Configure the background chat processing daemon.

### Debug logging enabled

Force chat debug logging on for every chat, bypassing the runtime admin and user opt-in settings.

- Environment variable: `CODER_CHAT_DEBUG_LOGGING_ENABLED`
- CLI flag: [`--chat-debug-logging-enabled`](../../reference/cli/server.md#--chat-debug-logging-enabled)
- YAML key: `chat.debugLoggingEnabled`
- Default value: `false`

## Client

These options change the behavior of how clients interact with the Coder. Clients include the Coder CLI, Coder Desktop, IDE extensions, and the web UI.

### CLI upgrade message

The upgrade message to display to users when a client/server mismatch is detected. By default it instructs users to update using 'curl -L https://coder.com/install.sh | sh'.

- Environment variable: `CODER_CLI_UPGRADE_MESSAGE`
- CLI flag: [`--cli-upgrade-message`](../../reference/cli/server.md#--cli-upgrade-message)
- YAML key: `client.cliUpgradeMessage`

### Hide AI tasks

Hide AI tasks from the dashboard.

- Environment variable: `CODER_HIDE_AI_TASKS`
- CLI flag: [`--hide-ai-tasks`](../../reference/cli/server.md#--hide-ai-tasks)
- YAML key: `client.hideAITasks`
- Default value: `false`

### SSH config options

These SSH config options will override the default SSH config options. Provide options in "key=value" or "key value" format separated by commas. Using this incorrectly can break SSH to your deployment, use cautiously. The following options are not allowed: Host, Match, Include, ProxyCommand, ProxyJump, LocalCommand, PermitLocalCommand, RemoteCommand, KnownHostsCommand, PKCS11Provider, SecurityKeyProvider, SmartcardDevice, XAuthLocation. Option values must not contain newline, carriage return, or NUL characters.

- Environment variable: `CODER_SSH_CONFIG_OPTIONS`
- CLI flag: [`--ssh-config-options`](../../reference/cli/server.md#--ssh-config-options)
- YAML key: `client.sshConfigOptions`

### Web terminal renderer

The renderer to use when opening a web terminal. Valid values are 'canvas', 'webgl', or 'dom'.

- Environment variable: `CODER_WEB_TERMINAL_RENDERER`
- CLI flag: [`--web-terminal-renderer`](../../reference/cli/server.md#--web-terminal-renderer)
- YAML key: `client.webTerminalRenderer`
- Default value: `canvas`

### Workspace hostname suffix

Workspace hostnames use this suffix in SSH config and Coder Connect on Coder Desktop. By default it is coder, resulting in names like myworkspace.coder. The suffix must not start with a dot, and must not contain spaces, newlines, or glob characters (* and ?).

- Environment variable: `CODER_WORKSPACE_HOSTNAME_SUFFIX`
- CLI flag: [`--workspace-hostname-suffix`](../../reference/cli/server.md#--workspace-hostname-suffix)
- YAML key: `client.workspaceHostnameSuffix`
- Default value: `coder`

## Config

Use a YAML configuration file when your server launch become unwieldy.

### Path

Specify a YAML file to load configuration from.

- Environment variable: `CODER_CONFIG_PATH`
- CLI flag: [`--config`](../../reference/cli/server.md#-c---config)

### Write config

Write out the current server config as YAML to stdout.

- CLI flag: [`--write-config`](../../reference/cli/server.md#--write-config)

## Email

Configure how emails are sent.

### Force TLS

Force a TLS connection to the configured SMTP smarthost.

- Environment variable: `CODER_EMAIL_FORCE_TLS`
- CLI flag: [`--email-force-tls`](../../reference/cli/server.md#--email-force-tls)
- YAML key: `email.forceTLS`
- Default value: `false`

### From address

The sender's address to use.

- Environment variable: `CODER_EMAIL_FROM`
- CLI flag: [`--email-from`](../../reference/cli/server.md#--email-from)
- YAML key: `email.from`

### Hello

The hostname identifying the SMTP server.

- Environment variable: `CODER_EMAIL_HELLO`
- CLI flag: [`--email-hello`](../../reference/cli/server.md#--email-hello)
- YAML key: `email.hello`
- Default value: `localhost`

### Smarthost

The intermediary SMTP host through which emails are sent.

- Environment variable: `CODER_EMAIL_SMARTHOST`
- CLI flag: [`--email-smarthost`](../../reference/cli/server.md#--email-smarthost)
- YAML key: `email.smarthost`

### Email authentication

Configure SMTP authentication options.

#### Identity

Identity to use with PLAIN authentication.

- Environment variable: `CODER_EMAIL_AUTH_IDENTITY`
- CLI flag: [`--email-auth-identity`](../../reference/cli/server.md#--email-auth-identity)
- YAML key: `email.emailAuth.identity`

#### Password

Password to use with PLAIN/LOGIN authentication.

- Environment variable: `CODER_EMAIL_AUTH_PASSWORD`
- CLI flag: [`--email-auth-password`](../../reference/cli/server.md#--email-auth-password)

#### Password file

File from which to load password for use with PLAIN/LOGIN authentication.

- Environment variable: `CODER_EMAIL_AUTH_PASSWORD_FILE`
- CLI flag: [`--email-auth-password-file`](../../reference/cli/server.md#--email-auth-password-file)
- YAML key: `email.emailAuth.passwordFile`

#### Username

Username to use with PLAIN/LOGIN authentication.

- Environment variable: `CODER_EMAIL_AUTH_USERNAME`
- CLI flag: [`--email-auth-username`](../../reference/cli/server.md#--email-auth-username)
- YAML key: `email.emailAuth.username`

### Email TLS

Configure TLS for your SMTP server target.

#### Certificate authority file

CA certificate file to use.

- Environment variable: `CODER_EMAIL_TLS_CACERTFILE`
- CLI flag: [`--email-tls-ca-cert-file`](../../reference/cli/server.md#--email-tls-ca-cert-file)
- YAML key: `email.emailTLS.caCertFile`

#### Certificate file

Certificate file to use.

- Environment variable: `CODER_EMAIL_TLS_CERTFILE`
- CLI flag: [`--email-tls-cert-file`](../../reference/cli/server.md#--email-tls-cert-file)
- YAML key: `email.emailTLS.certFile`

#### Certificate key file

Certificate key file to use.

- Environment variable: `CODER_EMAIL_TLS_CERTKEYFILE`
- CLI flag: [`--email-tls-cert-key-file`](../../reference/cli/server.md#--email-tls-cert-key-file)
- YAML key: `email.emailTLS.certKeyFile`

#### Server name

Server name to verify against the target certificate.

- Environment variable: `CODER_EMAIL_TLS_SERVERNAME`
- CLI flag: [`--email-tls-server-name`](../../reference/cli/server.md#--email-tls-server-name)
- YAML key: `email.emailTLS.serverName`

#### Skip certificate verification (insecure)

Skip verification of the target server's certificate (insecure).

- Environment variable: `CODER_EMAIL_TLS_SKIPVERIFY`
- CLI flag: [`--email-tls-skip-verify`](../../reference/cli/server.md#--email-tls-skip-verify)
- YAML key: `email.emailTLS.insecureSkipVerify`

#### StartTLS

Enable STARTTLS to upgrade insecure SMTP connections using TLS.

- Environment variable: `CODER_EMAIL_TLS_STARTTLS`
- CLI flag: [`--email-tls-starttls`](../../reference/cli/server.md#--email-tls-starttls)
- YAML key: `email.emailTLS.startTLS`

## Introspection

Configure logging, tracing, stat collection, and metrics exporting.

### Health check

#### Refresh

Refresh interval for healthchecks.

- Environment variable: `CODER_HEALTH_CHECK_REFRESH`
- CLI flag: [`--health-check-refresh`](../../reference/cli/server.md#--health-check-refresh)
- YAML key: `introspection.healthcheck.refresh`
- Default value: `10m0s`

#### Threshold: database

The threshold for the database health check. If the median latency of the database exceeds this threshold over 5 attempts, the database is considered unhealthy. The default value is 15ms.

- Environment variable: `CODER_HEALTH_CHECK_THRESHOLD_DATABASE`
- CLI flag: [`--health-check-threshold-database`](../../reference/cli/server.md#--health-check-threshold-database)
- YAML key: `introspection.healthcheck.thresholdDatabase`
- Default value: `15ms`

### Logging

#### Enable Terraform debug mode

Allow administrators to enable Terraform debug output.

- Environment variable: `CODER_ENABLE_TERRAFORM_DEBUG_MODE`
- CLI flag: [`--enable-terraform-debug-mode`](../../reference/cli/server.md#--enable-terraform-debug-mode)
- YAML key: `introspection.logging.enableTerraformDebugMode`
- Default value: `false`

#### Human log location

Output human-readable logs to a given file.

- Environment variable: `CODER_LOGGING_HUMAN`
- CLI flag: [`--log-human`](../../reference/cli/server.md#--log-human)
- YAML key: `introspection.logging.humanPath`
- Default value: `/dev/stderr`

#### JSON log location

Output JSON logs to a given file.

- Environment variable: `CODER_LOGGING_JSON`
- CLI flag: [`--log-json`](../../reference/cli/server.md#--log-json)
- YAML key: `introspection.logging.jsonPath`

#### Log filter

Filter debug logs by matching against a given regex. Use .* to match all debug logs.

- Environment variable: `CODER_LOG_FILTER`
- CLI flag: [`--log-filter`](../../reference/cli/server.md#-l---log-filter)
- YAML key: `introspection.logging.filter`

#### Stackdriver log location

Output Stackdriver compatible logs to a given file.

- Environment variable: `CODER_LOGGING_STACKDRIVER`
- CLI flag: [`--log-stackdriver`](../../reference/cli/server.md#--log-stackdriver)
- YAML key: `introspection.logging.stackdriverPath`

### Prometheus

#### Address

The bind address to serve prometheus metrics.

- Environment variable: `CODER_PROMETHEUS_ADDRESS`
- CLI flag: [`--prometheus-address`](../../reference/cli/server.md#--prometheus-address)
- YAML key: `introspection.prometheus.address`
- Default value: `127.0.0.1:2112`

#### Aggregate agent stats by

When collecting agent stats, aggregate metrics by a given set of comma-separated labels to reduce cardinality. Accepted values are agent_name, template_name, username, workspace_name.

- Environment variable: `CODER_PROMETHEUS_AGGREGATE_AGENT_STATS_BY`
- CLI flag: [`--prometheus-aggregate-agent-stats-by`](../../reference/cli/server.md#--prometheus-aggregate-agent-stats-by)
- YAML key: `introspection.prometheus.aggregate_agent_stats_by`
- Default value: `agent_name,template_name,username,workspace_name`

#### Collect agent stats

Collect agent stats (may increase charges for metrics storage).

- Environment variable: `CODER_PROMETHEUS_COLLECT_AGENT_STATS`
- CLI flag: [`--prometheus-collect-agent-stats`](../../reference/cli/server.md#--prometheus-collect-agent-stats)
- YAML key: `introspection.prometheus.collect_agent_stats`

#### Collect database metrics

Collect database query metrics (may increase charges for metrics storage). If set to false, a reduced set of database metrics are still collected.

- Environment variable: `CODER_PROMETHEUS_COLLECT_DB_METRICS`
- CLI flag: [`--prometheus-collect-db-metrics`](../../reference/cli/server.md#--prometheus-collect-db-metrics)
- YAML key: `introspection.prometheus.collect_db_metrics`
- Default value: `false`

#### Enable

Serve prometheus metrics on the address defined by prometheus address.

- Environment variable: `CODER_PROMETHEUS_ENABLE`
- CLI flag: [`--prometheus-enable`](../../reference/cli/server.md#--prometheus-enable)
- YAML key: `introspection.prometheus.enable`

### Stats collection

#### Usage stats

##### Enable

Enable the collection of application and workspace usage along with the associated API endpoints and the template insights page. Disabling this will also disable traffic and connection insights in the deployment stats shown to admins in the bottom bar of the Coder UI, and will prevent Prometheus collection of these values.

- Environment variable: `CODER_STATS_COLLECTION_USAGE_STATS_ENABLE`
- CLI flag: [`--stats-collection-usage-stats-enable`](../../reference/cli/server.md#--stats-collection-usage-stats-enable)
- YAML key: `introspection.statsCollection.usageStats.enable`
- Default value: `true`

### Tracing

#### Capture logs in traces

Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs.

- Environment variable: `CODER_TRACE_LOGS`
- CLI flag: [`--trace-logs`](../../reference/cli/server.md#--trace-logs)
- YAML key: `introspection.tracing.captureLogs`

#### Trace enable

Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.

- Environment variable: `CODER_TRACE_ENABLE`
- CLI flag: [`--trace`](../../reference/cli/server.md#--trace)
- YAML key: `introspection.tracing.enable`

#### Trace Honeycomb API key

Enables trace exporting to Honeycomb.io using the provided API Key.

- Environment variable: `CODER_TRACE_HONEYCOMB_API_KEY`
- CLI flag: [`--trace-honeycomb-api-key`](../../reference/cli/server.md#--trace-honeycomb-api-key)

### pprof

#### Address

The bind address to serve pprof.

- Environment variable: `CODER_PPROF_ADDRESS`
- CLI flag: [`--pprof-address`](../../reference/cli/server.md#--pprof-address)
- YAML key: `introspection.pprof.address`
- Default value: `127.0.0.1:6060`

#### Enable

Serve pprof metrics on the address defined by pprof address.

- Environment variable: `CODER_PPROF_ENABLE`
- CLI flag: [`--pprof-enable`](../../reference/cli/server.md#--pprof-enable)
- YAML key: `introspection.pprof.enable`

## Networking

### Access URL

The URL that users will use to access the Coder deployment.

- Environment variable: `CODER_ACCESS_URL`
- CLI flag: [`--access-url`](../../reference/cli/server.md#--access-url)
- YAML key: `networking.accessURL`

### Browser only

Whether Coder only allows connections to workspaces via the browser.

- Environment variable: `CODER_BROWSER_ONLY`
- CLI flag: [`--browser-only`](../../reference/cli/server.md#--browser-only)
- YAML key: `networking.browserOnly`

### Docs URL

Specifies the custom docs URL.

- Environment variable: `CODER_DOCS_URL`
- CLI flag: [`--docs-url`](../../reference/cli/server.md#--docs-url)
- YAML key: `networking.docsURL`
- Default value: `https://coder.com/docs`

### Proxy trusted headers

Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For.

- Environment variable: `CODER_PROXY_TRUSTED_HEADERS`
- CLI flag: [`--proxy-trusted-headers`](../../reference/cli/server.md#--proxy-trusted-headers)
- YAML key: `networking.proxyTrustedHeaders`

### Proxy trusted origins

Origin addresses to respect "proxy-trusted-headers" and X-Forwarded-Host for subdomain app routing. e.g. 192.168.1.0/24.

- Environment variable: `CODER_PROXY_TRUSTED_ORIGINS`
- CLI flag: [`--proxy-trusted-origins`](../../reference/cli/server.md#--proxy-trusted-origins)
- YAML key: `networking.proxyTrustedOrigins`

### Redirect to access URL

Specifies whether to redirect requests that do not match the access URL host.

- Environment variable: `CODER_REDIRECT_TO_ACCESS_URL`
- CLI flag: [`--redirect-to-access-url`](../../reference/cli/server.md#--redirect-to-access-url)
- YAML key: `networking.redirectToAccessURL`

### SameSite auth cookie

Controls the 'SameSite' property is set on browser session cookies.

- Environment variable: `CODER_SAMESITE_AUTH_COOKIE`
- CLI flag: [`--samesite-auth-cookie`](../../reference/cli/server.md#--samesite-auth-cookie)
- YAML key: `networking.sameSiteAuthCookie`
- Default value: `lax`

### Secure auth cookie

Controls if the 'Secure' property is set on browser session cookies.

- Environment variable: `CODER_SECURE_AUTH_COOKIE`
- CLI flag: [`--secure-auth-cookie`](../../reference/cli/server.md#--secure-auth-cookie)
- YAML key: `networking.secureAuthCookie`
- Default value: `(computed at runtime)`

### Wildcard access URL

Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".

- Environment variable: `CODER_WILDCARD_ACCESS_URL`
- CLI flag: [`--wildcard-access-url`](../../reference/cli/server.md#--wildcard-access-url)
- YAML key: `networking.wildcardAccessURL`

### __Host prefix cookies

Recommended to be enabled. Enables `__Host-` prefix for cookies to guarantee they are only set by the right domain. This change is disruptive to any workspaces built before release 2.31, requiring a workspace restart.

- Environment variable: `CODER_HOST_PREFIX_COOKIE`
- CLI flag: [`--host-prefix-cookie`](../../reference/cli/server.md#--host-prefix-cookie)
- YAML key: `networking.hostPrefixCookie`
- Default value: `false`

### Cluster

Configure network clustering. Coder Servers in the primary region form a cluster by communicating directly.

#### Host

Hostname or (more commonly) IP to reach this replica for clustering.

- Environment variable: `CODER_CLUSTER_HOST`
- CLI flag: [`--cluster-host`](../../reference/cli/server.md#--cluster-host)
- YAML key: `networking.cluster.clusterHost`

### DERP

Most Coder deployments never have to think about DERP because all connections between workspaces and users are peer-to-peer. However, when Coder cannot establish a peer to peer connection, Coder uses a distributed relay network backed by Tailscale and WireGuard.

#### Block direct connections

Block peer-to-peer (aka. direct) workspace connections. All workspace connections from the CLI will be proxied through Coder (or custom configured DERP servers) and will never be peer-to-peer when enabled. Workspaces may still reach out to STUN servers to get their address until they are restarted after this change has been made, but new connections will still be proxied regardless.

- Environment variable: `CODER_BLOCK_DIRECT`
- CLI flag: [`--block-direct-connections`](../../reference/cli/server.md#--block-direct-connections)
- YAML key: `networking.derp.blockDirect`

#### Config path

Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/.

- Environment variable: `CODER_DERP_CONFIG_PATH`
- CLI flag: [`--derp-config-path`](../../reference/cli/server.md#--derp-config-path)
- YAML key: `networking.derp.configPath`

#### Config URL

URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/.

- Environment variable: `CODER_DERP_CONFIG_URL`
- CLI flag: [`--derp-config-url`](../../reference/cli/server.md#--derp-config-url)
- YAML key: `networking.derp.url`

#### Force WebSockets

Force clients and agents to always use WebSocket to connect to DERP relay servers. By default, DERP uses `Upgrade: derp`, which may cause issues with some reverse proxies. Clients may automatically fallback to WebSocket if they detect an issue with `Upgrade: derp`, but this does not work in all situations.

- Environment variable: `CODER_DERP_FORCE_WEBSOCKETS`
- CLI flag: [`--derp-force-websockets`](../../reference/cli/server.md#--derp-force-websockets)
- YAML key: `networking.derp.forceWebSockets`

#### Server enable

Whether to enable or disable the embedded DERP relay server.

- Environment variable: `CODER_DERP_SERVER_ENABLE`
- CLI flag: [`--derp-server-enable`](../../reference/cli/server.md#--derp-server-enable)
- YAML key: `networking.derp.enable`
- Default value: `true`

#### Server region name

Region name that for the embedded DERP server.

- Environment variable: `CODER_DERP_SERVER_REGION_NAME`
- CLI flag: [`--derp-server-region-name`](../../reference/cli/server.md#--derp-server-region-name)
- YAML key: `networking.derp.regionName`
- Default value: `Coder Embedded Relay`

#### Server relay URL

An HTTP URL that is accessible by other replicas to relay DERP traffic. Required for high availability.

- Environment variable: `CODER_DERP_SERVER_RELAY_URL`
- CLI flag: [`--derp-server-relay-url`](../../reference/cli/server.md#--derp-server-relay-url)
- YAML key: `networking.derp.relayURL`

#### Server STUN addresses

Addresses for STUN servers to establish P2P connections. It's recommended to have at least two STUN servers to give users the best chance of connecting P2P to workspaces. Each STUN server will get it's own DERP region, with region IDs starting at `--derp-server-region-id + 1`. Use special value 'disable' to turn off STUN completely.

- Environment variable: `CODER_DERP_SERVER_STUN_ADDRESSES`
- CLI flag: [`--derp-server-stun-addresses`](../../reference/cli/server.md#--derp-server-stun-addresses)
- YAML key: `networking.derp.stunAddresses`
- Default value: `stun.l.google.com:19302,stun1.l.google.com:19302,stun2.l.google.com:19302,stun3.l.google.com:19302,stun4.l.google.com:19302`

### HTTP

#### Additional CSP policy

Coder configures a Content Security Policy (CSP) to protect against XSS attacks. This setting allows you to add additional CSP directives, which can open the attack surface of the deployment. Format matches the CSP directive format, e.g. --additional-csp-policy="script-src https://example.com".

- Environment variable: `CODER_ADDITIONAL_CSP_POLICY`
- CLI flag: [`--additional-csp-policy`](../../reference/cli/server.md#--additional-csp-policy)
- YAML key: `networking.http.additionalCSPPolicy`

#### Disable password authentication

Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.

- Environment variable: `CODER_DISABLE_PASSWORD_AUTH`
- CLI flag: [`--disable-password-auth`](../../reference/cli/server.md#--disable-password-auth)
- YAML key: `networking.http.disablePasswordAuth`

#### Disable session expiry refresh

Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.

- Environment variable: `CODER_DISABLE_SESSION_EXPIRY_REFRESH`
- CLI flag: [`--disable-session-expiry-refresh`](../../reference/cli/server.md#--disable-session-expiry-refresh)
- YAML key: `networking.http.disableSessionExpiryRefresh`

#### Address

HTTP bind address of the server. Unset to disable the HTTP endpoint.

- Environment variable: `CODER_HTTP_ADDRESS`
- CLI flag: [`--http-address`](../../reference/cli/server.md#--http-address)
- YAML key: `networking.http.httpAddress`
- Default value: `127.0.0.1:3000`

#### Max token lifetime

The maximum lifetime duration users can specify when creating an API token.

- Environment variable: `CODER_MAX_TOKEN_LIFETIME`
- CLI flag: [`--max-token-lifetime`](../../reference/cli/server.md#--max-token-lifetime)
- YAML key: `networking.http.maxTokenLifetime`
- Default value: `876600h0m0s`

#### Maximum admin token lifetime

The maximum lifetime duration administrators can specify when creating an API token.

- Environment variable: `CODER_MAX_ADMIN_TOKEN_LIFETIME`
- CLI flag: [`--max-admin-token-lifetime`](../../reference/cli/server.md#--max-admin-token-lifetime)
- YAML key: `networking.http.maxAdminTokenLifetime`
- Default value: `168h0m0s`

#### Proxy health check interval

The interval in which coderd should be checking the status of workspace proxies.

- Environment variable: `CODER_PROXY_HEALTH_INTERVAL`
- CLI flag: [`--proxy-health-interval`](../../reference/cli/server.md#--proxy-health-interval)
- YAML key: `networking.http.proxyHealthInterval`
- Default value: `1m0s`

#### Session duration

The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.

- Environment variable: `CODER_SESSION_DURATION`
- CLI flag: [`--session-duration`](../../reference/cli/server.md#--session-duration)
- YAML key: `networking.http.sessionDuration`
- Default value: `24h0m0s`

### TLS

Configure TLS / HTTPS for your Coder deployment. If you're running Coder behind a TLS-terminating reverse proxy or are accessing Coder over a secure link, you can safely ignore these settings.

#### Strict-Transport-Security

Controls if the 'Strict-Transport-Security' header is set on all static file responses. This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of the header.

- Environment variable: `CODER_STRICT_TRANSPORT_SECURITY`
- CLI flag: [`--strict-transport-security`](../../reference/cli/server.md#--strict-transport-security)
- YAML key: `networking.tls.strictTransportSecurity`
- Default value: `0`

#### Strict-Transport-Security options

Two optional fields can be set in the Strict-Transport-Security header; 'includeSubDomains' and 'preload'. The 'strict-transport-security' flag must be set to a non-zero value for these options to be used.

- Environment variable: `CODER_STRICT_TRANSPORT_SECURITY_OPTIONS`
- CLI flag: [`--strict-transport-security-options`](../../reference/cli/server.md#--strict-transport-security-options)
- YAML key: `networking.tls.strictTransportSecurityOptions`

#### Address

HTTPS bind address of the server.

- Environment variable: `CODER_TLS_ADDRESS`
- CLI flag: [`--tls-address`](../../reference/cli/server.md#--tls-address)
- YAML key: `networking.tls.address`
- Default value: `127.0.0.1:3443`

#### Allow insecure ciphers

By default, only ciphers marked as 'secure' are allowed to be used. See https://github.com/golang/go/blob/master/src/crypto/tls/cipher_suites.go#L82-L95.

- Environment variable: `CODER_TLS_ALLOW_INSECURE_CIPHERS`
- CLI flag: [`--tls-allow-insecure-ciphers`](../../reference/cli/server.md#--tls-allow-insecure-ciphers)
- YAML key: `networking.tls.tlsAllowInsecureCiphers`
- Default value: `false`

#### Certificate files

Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.

- Environment variable: `CODER_TLS_CERT_FILE`
- CLI flag: [`--tls-cert-file`](../../reference/cli/server.md#--tls-cert-file)
- YAML key: `networking.tls.certFiles`

#### Ciphers

Specify specific TLS ciphers that allowed to be used. See https://github.com/golang/go/blob/master/src/crypto/tls/cipher_suites.go#L53-L75.

- Environment variable: `CODER_TLS_CIPHERS`
- CLI flag: [`--tls-ciphers`](../../reference/cli/server.md#--tls-ciphers)
- YAML key: `networking.tls.tlsCiphers`

#### Client auth

Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".

- Environment variable: `CODER_TLS_CLIENT_AUTH`
- CLI flag: [`--tls-client-auth`](../../reference/cli/server.md#--tls-client-auth)
- YAML key: `networking.tls.clientAuth`
- Default value: `none`

#### Client CA files

PEM-encoded Certificate Authority file used for checking the authenticity of client.

- Environment variable: `CODER_TLS_CLIENT_CA_FILE`
- CLI flag: [`--tls-client-ca-file`](../../reference/cli/server.md#--tls-client-ca-file)
- YAML key: `networking.tls.clientCAFile`

#### Client cert file

Path to certificate for client TLS authentication. It requires a PEM-encoded file.

- Environment variable: `CODER_TLS_CLIENT_CERT_FILE`
- CLI flag: [`--tls-client-cert-file`](../../reference/cli/server.md#--tls-client-cert-file)
- YAML key: `networking.tls.clientCertFile`

#### Client key file

Path to key for client TLS authentication. It requires a PEM-encoded file.

- Environment variable: `CODER_TLS_CLIENT_KEY_FILE`
- CLI flag: [`--tls-client-key-file`](../../reference/cli/server.md#--tls-client-key-file)
- YAML key: `networking.tls.clientKeyFile`

#### Enable

Whether TLS will be enabled.

- Environment variable: `CODER_TLS_ENABLE`
- CLI flag: [`--tls-enable`](../../reference/cli/server.md#--tls-enable)
- YAML key: `networking.tls.enable`

#### Key files

Paths to the private keys for each of the certificates. It requires a PEM-encoded file.

- Environment variable: `CODER_TLS_KEY_FILE`
- CLI flag: [`--tls-key-file`](../../reference/cli/server.md#--tls-key-file)
- YAML key: `networking.tls.keyFiles`

#### Minimum version

Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13".

- Environment variable: `CODER_TLS_MIN_VERSION`
- CLI flag: [`--tls-min-version`](../../reference/cli/server.md#--tls-min-version)
- YAML key: `networking.tls.minVersion`
- Default value: `tls12`

## Notifications

Configure how notifications are processed and delivered.

### Dispatch timeout

How long to wait while a notification is being sent before giving up.

- Environment variable: `CODER_NOTIFICATIONS_DISPATCH_TIMEOUT`
- CLI flag: [`--notifications-dispatch-timeout`](../../reference/cli/server.md#--notifications-dispatch-timeout)
- YAML key: `notifications.dispatchTimeout`
- Default value: `1m0s`

### Max send attempts

The upper limit of attempts to send a notification.

- Environment variable: `CODER_NOTIFICATIONS_MAX_SEND_ATTEMPTS`
- CLI flag: [`--notifications-max-send-attempts`](../../reference/cli/server.md#--notifications-max-send-attempts)
- YAML key: `notifications.maxSendAttempts`
- Default value: `5`

### Method

Which delivery method to use (available options: 'smtp', 'webhook').

- Environment variable: `CODER_NOTIFICATIONS_METHOD`
- CLI flag: [`--notifications-method`](../../reference/cli/server.md#--notifications-method)
- YAML key: `notifications.method`
- Default value: `smtp`

### Email

Configure how email notifications are sent.

#### Force TLS

**Deprecated.** Force a TLS connection to the configured SMTP smarthost.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_FORCE_TLS`
- CLI flag: [`--notifications-email-force-tls`](../../reference/cli/server.md#--notifications-email-force-tls)
- YAML key: `notifications.email.forceTLS`

#### From address

**Deprecated.** The sender's address to use.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_FROM`
- CLI flag: [`--notifications-email-from`](../../reference/cli/server.md#--notifications-email-from)
- YAML key: `notifications.email.from`

#### Hello

**Deprecated.** The hostname identifying the SMTP server.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_HELLO`
- CLI flag: [`--notifications-email-hello`](../../reference/cli/server.md#--notifications-email-hello)
- YAML key: `notifications.email.hello`

#### Smarthost

**Deprecated.** The intermediary SMTP host through which emails are sent.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_SMARTHOST`
- CLI flag: [`--notifications-email-smarthost`](../../reference/cli/server.md#--notifications-email-smarthost)
- YAML key: `notifications.email.smarthost`

#### Email authentication

Configure SMTP authentication options.

##### Identity

**Deprecated.** Identity to use with PLAIN authentication.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_AUTH_IDENTITY`
- CLI flag: [`--notifications-email-auth-identity`](../../reference/cli/server.md#--notifications-email-auth-identity)
- YAML key: `notifications.email.emailAuth.identity`

##### Password

**Deprecated.** Password to use with PLAIN/LOGIN authentication.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_AUTH_PASSWORD`
- CLI flag: [`--notifications-email-auth-password`](../../reference/cli/server.md#--notifications-email-auth-password)

##### Password file

**Deprecated.** File from which to load password for use with PLAIN/LOGIN authentication.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_AUTH_PASSWORD_FILE`
- CLI flag: [`--notifications-email-auth-password-file`](../../reference/cli/server.md#--notifications-email-auth-password-file)
- YAML key: `notifications.email.emailAuth.passwordFile`

##### Username

**Deprecated.** Username to use with PLAIN/LOGIN authentication.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_AUTH_USERNAME`
- CLI flag: [`--notifications-email-auth-username`](../../reference/cli/server.md#--notifications-email-auth-username)
- YAML key: `notifications.email.emailAuth.username`

#### Email TLS

Configure TLS for your SMTP server target.

##### Certificate authority file

**Deprecated.** CA certificate file to use.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_TLS_CACERTFILE`
- CLI flag: [`--notifications-email-tls-ca-cert-file`](../../reference/cli/server.md#--notifications-email-tls-ca-cert-file)
- YAML key: `notifications.email.emailTLS.caCertFile`

##### Certificate file

**Deprecated.** Certificate file to use.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_TLS_CERTFILE`
- CLI flag: [`--notifications-email-tls-cert-file`](../../reference/cli/server.md#--notifications-email-tls-cert-file)
- YAML key: `notifications.email.emailTLS.certFile`

##### Certificate key file

**Deprecated.** Certificate key file to use.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_TLS_CERTKEYFILE`
- CLI flag: [`--notifications-email-tls-cert-key-file`](../../reference/cli/server.md#--notifications-email-tls-cert-key-file)
- YAML key: `notifications.email.emailTLS.certKeyFile`

##### Server name

**Deprecated.** Server name to verify against the target certificate.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_TLS_SERVERNAME`
- CLI flag: [`--notifications-email-tls-server-name`](../../reference/cli/server.md#--notifications-email-tls-server-name)
- YAML key: `notifications.email.emailTLS.serverName`

##### Skip certificate verification (insecure)

**Deprecated.** Skip verification of the target server's certificate (insecure).

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_TLS_SKIPVERIFY`
- CLI flag: [`--notifications-email-tls-skip-verify`](../../reference/cli/server.md#--notifications-email-tls-skip-verify)
- YAML key: `notifications.email.emailTLS.insecureSkipVerify`

##### StartTLS

**Deprecated.** Enable STARTTLS to upgrade insecure SMTP connections using TLS.

- Environment variable: `CODER_NOTIFICATIONS_EMAIL_TLS_STARTTLS`
- CLI flag: [`--notifications-email-tls-starttls`](../../reference/cli/server.md#--notifications-email-tls-starttls)
- YAML key: `notifications.email.emailTLS.startTLS`

### Inbox

#### Enabled

Enable Coder Inbox.

- Environment variable: `CODER_NOTIFICATIONS_INBOX_ENABLED`
- CLI flag: [`--notifications-inbox-enabled`](../../reference/cli/server.md#--notifications-inbox-enabled)
- YAML key: `notifications.inbox.enabled`
- Default value: `true`

### Webhook

#### Endpoint

The endpoint to which to send webhooks.

- Environment variable: `CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT`
- CLI flag: [`--notifications-webhook-endpoint`](../../reference/cli/server.md#--notifications-webhook-endpoint)
- YAML key: `notifications.webhook.endpoint`

## OAuth2

Configure login and user-provisioning with GitHub via oAuth2.

### GitHub

#### Allow everyone

Allow all logins, setting this option means allowed orgs and teams must be empty.

- Environment variable: `CODER_OAUTH2_GITHUB_ALLOW_EVERYONE`
- CLI flag: [`--oauth2-github-allow-everyone`](../../reference/cli/server.md#--oauth2-github-allow-everyone)
- YAML key: `oauth2.github.allowEveryone`

#### Allow signups

Whether new users can sign up with GitHub.

- Environment variable: `CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS`
- CLI flag: [`--oauth2-github-allow-signups`](../../reference/cli/server.md#--oauth2-github-allow-signups)
- YAML key: `oauth2.github.allowSignups`

#### Allowed orgs

Organizations the user must be a member of to Login with GitHub.

- Environment variable: `CODER_OAUTH2_GITHUB_ALLOWED_ORGS`
- CLI flag: [`--oauth2-github-allowed-orgs`](../../reference/cli/server.md#--oauth2-github-allowed-orgs)
- YAML key: `oauth2.github.allowedOrgs`

#### Allowed teams

Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.

- Environment variable: `CODER_OAUTH2_GITHUB_ALLOWED_TEAMS`
- CLI flag: [`--oauth2-github-allowed-teams`](../../reference/cli/server.md#--oauth2-github-allowed-teams)
- YAML key: `oauth2.github.allowedTeams`

#### Client ID

Client ID for Login with GitHub.

- Environment variable: `CODER_OAUTH2_GITHUB_CLIENT_ID`
- CLI flag: [`--oauth2-github-client-id`](../../reference/cli/server.md#--oauth2-github-client-id)
- YAML key: `oauth2.github.clientID`

#### Client secret

Client secret for Login with GitHub.

- Environment variable: `CODER_OAUTH2_GITHUB_CLIENT_SECRET`
- CLI flag: [`--oauth2-github-client-secret`](../../reference/cli/server.md#--oauth2-github-client-secret)

#### Default provider enable

Enable the default GitHub OAuth2 provider managed by Coder.

- Environment variable: `CODER_OAUTH2_GITHUB_DEFAULT_PROVIDER_ENABLE`
- CLI flag: [`--oauth2-github-default-provider-enable`](../../reference/cli/server.md#--oauth2-github-default-provider-enable)
- YAML key: `oauth2.github.defaultProviderEnable`
- Default value: `true`

#### Device flow

Enable device flow for Login with GitHub.

- Environment variable: `CODER_OAUTH2_GITHUB_DEVICE_FLOW`
- CLI flag: [`--oauth2-github-device-flow`](../../reference/cli/server.md#--oauth2-github-device-flow)
- YAML key: `oauth2.github.deviceFlow`
- Default value: `false`

#### Enterprise base URL

Base URL of a GitHub Enterprise deployment to use for Login with GitHub.

- Environment variable: `CODER_OAUTH2_GITHUB_ENTERPRISE_BASE_URL`
- CLI flag: [`--oauth2-github-enterprise-base-url`](../../reference/cli/server.md#--oauth2-github-enterprise-base-url)
- YAML key: `oauth2.github.enterpriseBaseURL`

## OIDC

### Enable OIDC group auto create

Automatically creates missing groups from a user's groups claim.

- Environment variable: `CODER_OIDC_GROUP_AUTO_CREATE`
- CLI flag: [`--oidc-group-auto-create`](../../reference/cli/server.md#--oidc-group-auto-create)
- YAML key: `oidc.enableGroupAutoCreate`
- Default value: `false`

### Allow signups

Whether new users can sign up with OIDC.

- Environment variable: `CODER_OIDC_ALLOW_SIGNUPS`
- CLI flag: [`--oidc-allow-signups`](../../reference/cli/server.md#--oidc-allow-signups)
- YAML key: `oidc.allowSignups`
- Default value: `true`

### Allowed groups

If provided any group name not in the list will not be allowed to authenticate. This allows for restricting access to a specific set of groups. This filter is applied after the group mapping and before the regex filter.

- Environment variable: `CODER_OIDC_ALLOWED_GROUPS`
- CLI flag: [`--oidc-allowed-groups`](../../reference/cli/server.md#--oidc-allowed-groups)
- YAML key: `oidc.groupAllowed`

### Auth URL parameters

OIDC auth URL parameters to pass to the upstream provider.

- Environment variable: `CODER_OIDC_AUTH_URL_PARAMS`
- CLI flag: [`--oidc-auth-url-params`](../../reference/cli/server.md#--oidc-auth-url-params)
- YAML key: `oidc.authURLParams`
- Default value: `{"access_type": "offline"}`

### Client cert file

Pem encoded certificate file to use for oauth2 PKI/JWT authorization. The public certificate that accompanies oidc-client-key-file. A standard x509 certificate is expected.

- Environment variable: `CODER_OIDC_CLIENT_CERT_FILE`
- CLI flag: [`--oidc-client-cert-file`](../../reference/cli/server.md#--oidc-client-cert-file)
- YAML key: `oidc.oidcClientCertFile`

### Client ID

Client ID to use for Login with OIDC.

- Environment variable: `CODER_OIDC_CLIENT_ID`
- CLI flag: [`--oidc-client-id`](../../reference/cli/server.md#--oidc-client-id)
- YAML key: `oidc.clientID`

### Client key file

Pem encoded RSA private key to use for oauth2 PKI/JWT authorization. This can be used instead of oidc-client-secret if your IDP supports it.

- Environment variable: `CODER_OIDC_CLIENT_KEY_FILE`
- CLI flag: [`--oidc-client-key-file`](../../reference/cli/server.md#--oidc-client-key-file)
- YAML key: `oidc.oidcClientKeyFile`

### Client secret

Client secret to use for Login with OIDC.

- Environment variable: `CODER_OIDC_CLIENT_SECRET`
- CLI flag: [`--oidc-client-secret`](../../reference/cli/server.md#--oidc-client-secret)

### Email domain

Email domains that clients logging in with OIDC must match.

- Environment variable: `CODER_OIDC_EMAIL_DOMAIN`
- CLI flag: [`--oidc-email-domain`](../../reference/cli/server.md#--oidc-email-domain)
- YAML key: `oidc.emailDomain`

### Email field

OIDC claim field to use as the email.

- Environment variable: `CODER_OIDC_EMAIL_FIELD`
- CLI flag: [`--oidc-email-field`](../../reference/cli/server.md#--oidc-email-field)
- YAML key: `oidc.emailField`
- Default value: `email`

### Group field

This field must be set if using the group sync feature and the scope name is not 'groups'. Set to the claim to be used for groups.

- Environment variable: `CODER_OIDC_GROUP_FIELD`
- CLI flag: [`--oidc-group-field`](../../reference/cli/server.md#--oidc-group-field)
- YAML key: `oidc.groupField`

### Group mapping

A map of OIDC group IDs and the group in Coder it should map to. This is useful for when OIDC providers only return group IDs.

- Environment variable: `CODER_OIDC_GROUP_MAPPING`
- CLI flag: [`--oidc-group-mapping`](../../reference/cli/server.md#--oidc-group-mapping)
- YAML key: `oidc.groupMapping`
- Default value: `{}`

### Ignore email verified

Ignore the email_verified claim from the upstream provider.

- Environment variable: `CODER_OIDC_IGNORE_EMAIL_VERIFIED`
- CLI flag: [`--oidc-ignore-email-verified`](../../reference/cli/server.md#--oidc-ignore-email-verified)
- YAML key: `oidc.ignoreEmailVerified`

### Ignore UserInfo

Ignore the userinfo endpoint and only use the ID token for user information.

- Environment variable: `CODER_OIDC_IGNORE_USERINFO`
- CLI flag: [`--oidc-ignore-userinfo`](../../reference/cli/server.md#--oidc-ignore-userinfo)
- YAML key: `oidc.ignoreUserInfo`
- Default value: `false`

### Issuer URL

Issuer URL to use for Login with OIDC.

- Environment variable: `CODER_OIDC_ISSUER_URL`
- CLI flag: [`--oidc-issuer-url`](../../reference/cli/server.md#--oidc-issuer-url)
- YAML key: `oidc.issuerURL`

### Name field

OIDC claim field to use as the name.

- Environment variable: `CODER_OIDC_NAME_FIELD`
- CLI flag: [`--oidc-name-field`](../../reference/cli/server.md#--oidc-name-field)
- YAML key: `oidc.nameField`
- Default value: `name`

### Regex group filter

If provided any group name not matching the regex is ignored. This allows for filtering out groups that are not needed. This filter is applied after the group mapping.

- Environment variable: `CODER_OIDC_GROUP_REGEX_FILTER`
- CLI flag: [`--oidc-group-regex-filter`](../../reference/cli/server.md#--oidc-group-regex-filter)
- YAML key: `oidc.groupRegexFilter`
- Default value: `.*`

### Scopes

Scopes to grant when authenticating with OIDC.

- Environment variable: `CODER_OIDC_SCOPES`
- CLI flag: [`--oidc-scopes`](../../reference/cli/server.md#--oidc-scopes)
- YAML key: `oidc.scopes`
- Default value: `openid,profile,email`

### User role default

If user role sync is enabled, these roles are always included for all authenticated users. The 'member' role is always assigned.

- Environment variable: `CODER_OIDC_USER_ROLE_DEFAULT`
- CLI flag: [`--oidc-user-role-default`](../../reference/cli/server.md#--oidc-user-role-default)
- YAML key: `oidc.userRoleDefault`

### User role field

This field must be set if using the user roles sync feature. Set this to the name of the claim used to store the user's role. The roles should be sent as an array of strings.

- Environment variable: `CODER_OIDC_USER_ROLE_FIELD`
- CLI flag: [`--oidc-user-role-field`](../../reference/cli/server.md#--oidc-user-role-field)
- YAML key: `oidc.userRoleField`

### User role mapping

A map of the OIDC passed in user roles and the groups in Coder it should map to. This is useful if the group names do not match. If mapped to the empty string, the role will ignored.

- Environment variable: `CODER_OIDC_USER_ROLE_MAPPING`
- CLI flag: [`--oidc-user-role-mapping`](../../reference/cli/server.md#--oidc-user-role-mapping)
- YAML key: `oidc.userRoleMapping`
- Default value: `{}`

### Username field

OIDC claim field to use as the username.

- Environment variable: `CODER_OIDC_USERNAME_FIELD`
- CLI flag: [`--oidc-username-field`](../../reference/cli/server.md#--oidc-username-field)
- YAML key: `oidc.usernameField`
- Default value: `preferred_username`

### OpenID connect sign in text

The text to show on the OpenID Connect sign in button.

- Environment variable: `CODER_OIDC_SIGN_IN_TEXT`
- CLI flag: [`--oidc-sign-in-text`](../../reference/cli/server.md#--oidc-sign-in-text)
- YAML key: `oidc.signInText`
- Default value: `OpenID Connect`

### OpenID connect icon URL

URL pointing to the icon to use on the OpenID Connect login button.

- Environment variable: `CODER_OIDC_ICON_URL`
- CLI flag: [`--oidc-icon-url`](../../reference/cli/server.md#--oidc-icon-url)
- YAML key: `oidc.iconURL`

### Signups disabled text

The custom text to show on the error page informing about disabled OIDC signups. Markdown format is supported.

- Environment variable: `CODER_OIDC_SIGNUPS_DISABLED_TEXT`
- CLI flag: [`--oidc-signups-disabled-text`](../../reference/cli/server.md#--oidc-signups-disabled-text)
- YAML key: `oidc.signupsDisabledText`

### Skip OIDC issuer checks (not recommended)

OIDC issuer urls must match in the request, the id_token 'iss' claim, and in the well-known configuration. This flag disables that requirement, and can lead to an insecure OIDC configuration. It is not recommended to use this flag.

- Environment variable: `CODER_DANGEROUS_OIDC_SKIP_ISSUER_CHECKS`
- CLI flag: [`--dangerous-oidc-skip-issuer-checks`](../../reference/cli/server.md#--dangerous-oidc-skip-issuer-checks)
- YAML key: `oidc.dangerousSkipIssuerChecks`

## Provisioning

Tune the behavior of the provisioner, which is responsible for creating, updating, and deleting workspace resources.

### Force cancel interval

Time to force cancel provisioning tasks that are stuck.

- Environment variable: `CODER_PROVISIONER_FORCE_CANCEL_INTERVAL`
- CLI flag: [`--provisioner-force-cancel-interval`](../../reference/cli/server.md#--provisioner-force-cancel-interval)
- YAML key: `provisioning.forceCancelInterval`
- Default value: `10m0s`

### Provisioner daemon pre-shared key (PSK)

Pre-shared key to authenticate external provisioner daemons to Coder server.

- Environment variable: `CODER_PROVISIONER_DAEMON_PSK`
- CLI flag: [`--provisioner-daemon-psk`](../../reference/cli/server.md#--provisioner-daemon-psk)

### Provisioner daemons

Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.

- Environment variable: `CODER_PROVISIONER_DAEMONS`
- CLI flag: [`--provisioner-daemons`](../../reference/cli/server.md#--provisioner-daemons)
- YAML key: `provisioning.daemons`
- Default value: `3`

### Poll interval

**Deprecated** and ignored.

- Environment variable: `CODER_PROVISIONER_DAEMON_POLL_INTERVAL`
- CLI flag: [`--provisioner-daemon-poll-interval`](../../reference/cli/server.md#--provisioner-daemon-poll-interval)
- YAML key: `provisioning.daemonPollInterval`
- Default value: `1s`

### Poll jitter

**Deprecated** and ignored.

- Environment variable: `CODER_PROVISIONER_DAEMON_POLL_JITTER`
- CLI flag: [`--provisioner-daemon-poll-jitter`](../../reference/cli/server.md#--provisioner-daemon-poll-jitter)
- YAML key: `provisioning.daemonPollJitter`
- Default value: `100ms`

## Retention

Configure data retention policies for various database tables. Retention policies automatically purge old data to reduce database size and improve performance. Setting a retention duration to 0 disables automatic purging for that data type.

### API keys retention

How long expired API keys are retained before being deleted. Keeping expired keys allows the backend to return a more helpful error when a user tries to use an expired key. Set to 0 to disable automatic deletion of expired keys.

- Environment variable: `CODER_API_KEYS_RETENTION`
- CLI flag: [`--api-keys-retention`](../../reference/cli/server.md#--api-keys-retention)
- YAML key: `retention.api_keys`
- Default value: `7d`

### Audit logs retention

How long audit log entries are retained. Set to 0 to disable (keep indefinitely). We advise keeping audit logs for at least a year, and in accordance with your compliance requirements.

- Environment variable: `CODER_AUDIT_LOGS_RETENTION`
- CLI flag: [`--audit-logs-retention`](../../reference/cli/server.md#--audit-logs-retention)
- YAML key: `retention.audit_logs`
- Default value: `0`

### Boundary log retention

How long boundary audit log entries are retained. Boundary logs record HTTP requests processed by a Boundary confinement proxy. Set to 0 to disable automatic deletion (keep indefinitely). Adjust to match your organization's regulatory requirements.

- Environment variable: `CODER_BOUNDARY_LOG_RETENTION`
- CLI flag: [`--boundary-log-retention`](../../reference/cli/server.md#--boundary-log-retention)
- YAML key: `retention.boundary_logs`
- Default value: `0`

### Connection logs retention

How long connection log entries are retained. Set to 0 to disable (keep indefinitely).

- Environment variable: `CODER_CONNECTION_LOGS_RETENTION`
- CLI flag: [`--connection-logs-retention`](../../reference/cli/server.md#--connection-logs-retention)
- YAML key: `retention.connection_logs`
- Default value: `0`

### Workspace agent logs retention

How long workspace agent logs are retained. Logs from non-latest builds are deleted if the agent hasn't connected within this period. Logs from the latest build are always retained. Set to 0 to disable automatic deletion.

- Environment variable: `CODER_WORKSPACE_AGENT_LOGS_RETENTION`
- CLI flag: [`--workspace-agent-logs-retention`](../../reference/cli/server.md#--workspace-agent-logs-retention)
- YAML key: `retention.workspace_agent_logs`
- Default value: `7d`

## Telemetry

Telemetry is critical to our ability to improve Coder. We strip all personal information before sending data to our servers. Please only disable telemetry when required by your organization's security policy.

### Enable

Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.

- Environment variable: `CODER_TELEMETRY_ENABLE`
- CLI flag: [`--telemetry`](../../reference/cli/server.md#--telemetry)
- YAML key: `telemetry.enable`
- Default value: `true`

## Template Builder

### Disable Template Builder

Disable the template builder feature for guided template creation. When disabled, all /api/v2/templatebuilder/* endpoints return 404.

- Environment variable: `CODER_DISABLE_TEMPLATE_BUILDER`
- CLI flag: [`--disable-template-builder`](../../reference/cli/server.md#--disable-template-builder)
- YAML key: `templateBuilder.disabled`

### Registry URL

The base URL of the module registry used by the template builder for module source paths.

- Environment variable: `CODER_TEMPLATE_BUILDER_REGISTRY_URL`
- CLI flag: [`--template-builder-registry-url`](../../reference/cli/server.md#--template-builder-registry-url)
- YAML key: `templateBuilder.registryURL`
- Default value: `registry.coder.com`

## User quiet hours schedule

Allow users to set quiet hours schedules each day for workspaces to avoid workspaces stopping during the day due to template scheduling.

### Allow custom quiet hours

Allow users to set their own quiet hours schedule for workspaces to stop in (depending on template autostop requirement settings). If false, users can't change their quiet hours schedule and the site default is always used.

- Environment variable: `CODER_ALLOW_CUSTOM_QUIET_HOURS`
- CLI flag: [`--allow-custom-quiet-hours`](../../reference/cli/server.md#--allow-custom-quiet-hours)
- YAML key: `userQuietHoursSchedule.allowCustomQuietHours`
- Default value: `true`

### Default quiet hours schedule

The default daily cron schedule applied to users that haven't set a custom quiet hours schedule themselves. The quiet hours schedule determines when workspaces will be force stopped due to the template's autostop requirement, and will round the max deadline up to be within the user's quiet hours window (or default). The format is the same as the standard cron format, but the day-of-month, month and day-of-week must be *. Only one hour and minute can be specified (ranges or comma separated values are not supported).

- Environment variable: `CODER_QUIET_HOURS_DEFAULT_SCHEDULE`
- CLI flag: [`--default-quiet-hours-schedule`](../../reference/cli/server.md#--default-quiet-hours-schedule)
- YAML key: `userQuietHoursSchedule.defaultQuietHoursSchedule`
- Default value: `CRON_TZ=UTC 0 0 * * *`

## Workspace prebuilds

Configure how workspace prebuilds behave.

### Reconciliation interval

How often to reconcile workspace prebuilds state.

- Environment variable: `CODER_WORKSPACE_PREBUILDS_RECONCILIATION_INTERVAL`
- CLI flag: [`--workspace-prebuilds-reconciliation-interval`](../../reference/cli/server.md#--workspace-prebuilds-reconciliation-interval)
- YAML key: `workspace_prebuilds.reconciliation_interval`
- Default value: `1m0s`

## ⚠️ Dangerous

### Allow path app sharing

Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.

- Environment variable: `CODER_DANGEROUS_ALLOW_PATH_APP_SHARING`
- CLI flag: [`--dangerous-allow-path-app-sharing`](../../reference/cli/server.md#--dangerous-allow-path-app-sharing)

### Allow site owners to access path apps

Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.

- Environment variable: `CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS`
- CLI flag: [`--dangerous-allow-path-app-site-owner-access`](../../reference/cli/server.md#--dangerous-allow-path-app-site-owner-access)
