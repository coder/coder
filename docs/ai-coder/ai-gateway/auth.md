# Authentication

> [!NOTE]
> AI Gateway requires the [AI Governance Add-On](../ai-governance.md).

AI Gateway uses different credentials for connections from clients to the Gateway, standalone Gateway replicas to the Coder control plane, and the Gateway to upstream providers:

- AI clients use a Coder API token to authenticate as a user.
- Standalone Gateway replicas use AI Gateway keys to connect to `coderd`.
- AI Gateway uses provider credentials configured by an administrator to authenticate to upstream AI services.
- In Bring Your Own Key (BYOK) mode, a user also supplies a personal provider credential or subscription token.

These credentials are not interchangeable.
A Gateway key does not authenticate an AI client, and a Coder API token does not authenticate a standalone replica.

## Authenticate AI clients

AI Gateway authenticates clients with the same Coder API token
that a user uses for the rest of the Coder API.
No separate AI Gateway login is required for client traffic.
For token creation, expiration, and revocation, refer to [Sessions and API tokens](../../admin/users/sessions-tokens.md).

Authenticating with a Coder token avoids distributing centralized provider API keys,
such as OpenAI or Anthropic keys, to individual users.
AI Gateway handles upstream credentials centrally and
forwards each request to the configured provider on the user's behalf.

The exact environment variable or setting name differs between tools.
Refer to the list of [supported clients](./clients/index.md) and
your tool's documentation for details.

### Create a Coder API token

You can generate a token from the Coder dashboard or the CLI.

From the dashboard, go to **Account settings** > **Tokens** and create a new token.
For long-lived tokens, refer to [Sessions and API tokens](../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself).
For headless or service-account use, refer to [Headless authentication](../../admin/users/headless-auth.md).

From the CLI, print your current session token with [`coder login token`](../../reference/cli/login_token.md):

```sh
coder login token
```

Or create a new long-lived token with a name and lifetime:

```sh
coder tokens create --lifetime 30d -n my-ai-token
```

Use short lifetimes for automation and CI to limit the blast radius if a token leaks.

### Retrieve your session token

If you're logged in with the Coder CLI, retrieve your current session token
with [`coder login token`](../../reference/cli/login_token.md):

```sh
export ANTHROPIC_API_KEY=$(coder login token)
export ANTHROPIC_BASE_URL="https://coder.example.com/api/v2/ai-gateway/anthropic"
```

### AI Gateway Proxy authentication

For tools that don't support a configurable base URL,
[AI Gateway Proxy](./ai-gateway-proxy/index.md) intercepts traffic and forwards it to AI Gateway.
The Coder token is supplied in the proxy URL:

```sh
export HTTPS_PROXY="https://coder:$(coder login token)@<proxy-host>:8888"
```

The client machine also needs to trust the proxy's CA certificate.
For full setup, refer to [AI Gateway Proxy setup](./ai-gateway-proxy/setup.md).

## Authenticate standalone Gateway replicas

AI Gateway keys are scoped to the Coder deployment.
A [standalone AI Gateway](./standalone.md) uses one of these keys to connect to `coderd`.
The built-in Owner role can create, list, and delete these keys.
A site-level custom role can also manage them when it has the corresponding `ai_gateway_key` permissions.

Create a key with a descriptive name:

```console
coder ai-gateway keys create standalone-production
```

The command displays the plaintext key once.
Save it immediately in your secret manager because Coder cannot retrieve it later.
Coder stores only a visible prefix and a SHA-256 hash of the secret.

Configure the standalone process with exactly 1 of the following options:

- `CODER_AI_GATEWAY_KEY` or `--key` supplies the key directly.
- `CODER_AI_GATEWAY_KEY_FILE` or `--key-file` reads the key from a file.

A user login and `CODER_SESSION_TOKEN` are not used by `coder ai-gateway start`.
The same Gateway key can authenticate multiple replicas. Separate keys make it easier to rotate or revoke each deployment independently.

List keys and their most recent usage heartbeat:

```console
coder ai-gateway keys list
```

A replica records a heartbeat when it connects and updates the timestamp every 60 seconds while the session remains active.

For command options, refer to the generated CLI reference for [creating](../../reference/cli/ai-gateway_keys_create.md), [listing](../../reference/cli/ai-gateway_keys_list.md), and [deleting](../../reference/cli/ai-gateway_keys_delete.md) Gateway keys.

### Rotate a Gateway key

Rotate a key without interrupting traffic:

1. Create a new Gateway key.
1. Update the Secret, environment variable, or key file used by every replica.
1. Restart or roll out the standalone deployment so every replica uses the new key.
1. Verify readiness and confirm that the new key has a recent heartbeat.
1. Delete the old key.

Delete a key by name or ID:

```console
coder ai-gateway keys delete standalone-production
```

Deleting a key rejects new connections immediately.
An existing session closes after a later heartbeat detects the deletion. Gateway may retry connecting with the same key, that attempt will be rejected.
Stop or update every replica before deleting its key for an orderly rotation.

## Authenticate to upstream providers

Administrators configure provider credentials in the Coder dashboard or with the [AI Providers API](../../reference/api/aiproviders.md).
Standalone replicas fetch this provider configuration from `coderd`, so do not copy centralized provider credentials into each standalone deployment.

For provider setup and credential failover, refer to [Provider configuration](./providers.md).

## Bring Your Own Key (BYOK)

In addition to centralized key management, AI Gateway supports Bring Your Own Key (BYOK) mode.
Users can provide their own LLM API keys or provider subscriptions,
such as Claude Pro or Max and ChatGPT Plus or Pro,
while AI Gateway continues to provide observability and governance.

![BYOK authentication flow](../../images/aibridge/clients/byok_auth_flow.png)

In BYOK mode, users need two credentials:

- A Coder API token to authenticate with AI Gateway.
- Their own LLM credential, such as a personal API key or subscription token,
  which AI Gateway forwards to the upstream provider.

BYOK and centralized modes can be used together.
When a user provides their own credential, AI Gateway forwards it directly.
When no user credential is present, AI Gateway uses the administrator-configured provider key.
This approach offers centralized keys as a default
while allowing individual users to bring their own key.

> [!NOTE]
> When a BYOK credential is present, [key failover](./providers.md#key-failover)
> is skipped.

Coder Agents requests routed through AI Gateway are in-process control plane
requests, not external client requests that send their own AI Gateway bearer token.
Coder Agents use the same global BYOK setting.
When BYOK is enabled, users can save personal API keys for any enabled AI provider
from the Agents settings page.
Refer to [Agents credential selection](../agents/models.md#credential-selection)
for the Agents-specific behavior.

Visit individual [client pages](./clients/index.md) for configuration details.

### Enable or disable BYOK

BYOK is enabled by default.
Administrators can disable it for the embedded Gateway with `--ai-gateway-allow-byok=false` or `CODER_AI_GATEWAY_ALLOW_BYOK=false`:

```sh
coder server --ai-gateway-allow-byok=false
```

For a standalone Gateway, set the option on each replica:

```sh
CODER_AI_GATEWAY_ALLOW_BYOK=false coder ai-gateway start
```

Keep this setting consistent across replicas.
When disabled, BYOK requests are rejected with a `403 Forbidden` response, and only centralized provider credentials are permitted.
