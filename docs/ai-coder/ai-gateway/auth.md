# Authentication

> [!NOTE]
> AI Gateway requires the [AI Governance Add-On](../ai-governance.md).

AI Gateway authenticates clients with the same Coder API token
that a user already uses against the rest of the Coder API.
No separate AI Gateway login or credential is required.

Authenticating with a Coder token avoids distributing provider-specific API keys
(such as OpenAI or Anthropic keys) to individual users.
AI Gateway handles upstream credentials centrally and
forwards each request to the configured provider on the user's behalf.

> [!NOTE]
> Only Coder-issued tokens can authenticate users to AI Gateway.
> AI Gateway will use provider-specific API keys to
> [authenticate against upstream AI services](./setup.md#configure-providers).

The exact environment variable or setting naming may differ from tool to tool.
Refer to the list of [supported clients](./clients/index.md),
and consult your tool's documentation for details.

## Create a Coder API token

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

## Retrieve your session token

If you're logged in with the Coder CLI, you can retrieve your current session token
by using [`coder login token`](../../reference/cli/login_token.md):

```sh
export ANTHROPIC_API_KEY=$(coder login token)
export ANTHROPIC_BASE_URL="https://coder.example.com/api/v2/ai-gateway/anthropic"
```

Alternatively, you can [generate a long-lived API token](../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)
from the Coder dashboard.

For headless or service-account use, refer to [Headless authentication](../../admin/users/headless-auth.md).

## AI Gateway Proxy authentication

For tools that don't support a configurable base URL,
[AI Gateway Proxy](./ai-gateway-proxy/index.md) intercepts traffic and forwards it to AI Gateway.
The Coder token is supplied in the proxy URL:

```sh
export HTTPS_PROXY="https://coder:$(coder login token)@<proxy-host>:8888"
```

The client machine also needs to trust the proxy's CA certificate.
For full setup, refer to [AI Gateway Proxy setup](./ai-gateway-proxy/setup.md).

## Bring Your Own Key (BYOK)

In addition to centralized key management, AI Gateway supports **Bring Your Own Key** (BYOK) mode.
Users can provide their own LLM API keys or use provider subscriptions
(such as Claude Pro/Max or ChatGPT Plus/Pro),
while AI Gateway continues to provide observability and governance.

![BYOK authentication flow](../../images/aibridge/clients/byok_auth_flow.png)

In BYOK mode, users need two credentials:

- A **Coder API token** to authenticate with AI Gateway.
- Their **own LLM credential** (personal API key or subscription token)
  which AI Gateway forwards to the upstream provider.

BYOK and centralized modes can be used together.
When a user provides their own credential, AI Gateway forwards it directly.
When no user credential is present, AI Gateway uses the admin-configured provider key.
This approach offers centralized keys as a default,
while allowing individual users to bring their own key.

> [!NOTE]
> When a BYOK credential is present, [key failover](./providers.md#key-failover)
> is skipped.

Coder Agents requests routed through AI Gateway are in-process control plane
requests, not external client requests that send their own AI Gateway bearer
token. Coder Agents use this same global BYOK setting. When BYOK is enabled,
users can save personal API keys for any enabled AI provider from the Agents
settings page. See
[Agents credential selection](../agents/models.md#credential-selection)
for the Agents-specific behavior.

Visit individual [client pages](./clients/index.md) for configuration details.

### Enable or disable BYOK

BYOK is enabled by default.
Administrators can disable it using `--ai-gateway-allow-byok=false` or `CODER_AI_GATEWAY_ALLOW_BYOK=false`:

```sh
coder server --ai-gateway-allow-byok=false
```

When disabled, BYOK requests are rejected with a `403 Forbidden` response and only centralized key authentication is permitted.

## Rotate or revoke a token

To rotate a token without downtime:

1. Create a new token with `coder tokens create`.
2. Update the client's configuration to use the new token.
3. Delete the old token from the dashboard or with `coder tokens rm <name>`.

Deleting a token immediately revokes access.
Deleting the user that owns a token revokes every token that user holds at the same time.
