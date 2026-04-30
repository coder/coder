# Coder Agents

[Coder Agents](../../agents/index.md) is a chat interface and API for delegating
development work to coding agents that run inside the Coder control plane. When AI Gateway is enabled on the same deployment, Coder Agents
traffic can be routed through it for full audit and governance coverage.

## Prerequisites

- AI Gateway is [enabled](../setup.md#activation) on your Coder deployment.
- At least one [provider](../setup.md#configure-providers) is configured in
  AI Gateway with a valid upstream key.
- You are an administrator with permission to configure Coder Agents
  [providers](../../agents/models.md#providers).

> [!NOTE]
> AI Gateway and Coder Agents use independent provider configurations. Adding
> a provider to AI Gateway does not enable it in Coder Agents, and vice versa.
> Configure each separately.

## Configuration

Point each Agents provider's **Base URL** at your local AI Gateway endpoint
and set the **API key** to a credential AI Gateway accepts. Because both
services run in the same `coderd` process, the AI Gateway endpoint is just
your deployment URL plus `/api/v2/aibridge/<provider>`.

<div class="tabs">

### Anthropic

1. Open the Coder dashboard and navigate to the **Agents** page.
1. Click **Admin**, then select the **Providers** tab.
1. Click **Anthropic**.
1. Set the **Base URL** to
   `https://coder.example.com/api/v2/aibridge/anthropic`.
1. Set the **API Key** to a value AI Gateway accepts. By default, this is
   the upstream Anthropic key configured on AI Gateway. See
   [Authentication](#authentication) for the exact value to use.
1. Click **Save**.

### OpenAI

1. Open the Coder dashboard and navigate to the **Agents** page.
1. Click **Admin**, then select the **Providers** tab.
1. Click **OpenAI**.
1. Set the **Base URL** to
   `https://coder.example.com/api/v2/aibridge/openai/v1`.
1. Set the **API Key** to a value AI Gateway accepts. See
   [Authentication](#authentication) for the exact value to use.
1. Click **Save**.

### OpenAI Compatible

Use **OpenAI Compatible** when you want to route Agents traffic through a
named AI Gateway provider instance, such as a
[multi-instance Anthropic configuration](../setup.md#multiple-instances-of-the-same-provider)
or a ChatGPT provider.

1. Open the Coder dashboard and navigate to the **Agents** page.
1. Click **Admin**, then select the **Providers** tab.
1. Click **OpenAI Compatible**.
1. Set the **Base URL** to
   `https://coder.example.com/api/v2/aibridge/<instance-name>/v1`.
1. Set the **API Key** to a value AI Gateway accepts.
1. Click **Save**.

</div>

Replace `coder.example.com` with your Coder deployment URL.

After saving, [add or update a model](../../agents/models.md#add-a-model) on
each provider so developers can select it from the chat. Models from a
provider only appear in the model selector once the provider has valid
credentials.

## Authentication

AI Gateway only accepts Coder-issued tokens for client authentication. The
provider API keys you configured for AI Gateway (for example,
`CODER_AIBRIDGE_OPENAI_KEY`) are used by AI Gateway internally to call the
upstream provider; they are not what Agents sends.

Coder Agents stores the **API Key** field as the bearer credential it
forwards to AI Gateway on each request. You can use either of the following:

- A long-lived [API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)
  for a [service account](../../../admin/users/headless-auth.md#create-a-service-account)
  that has access to AI Gateway. This keeps the credential separate from any
  individual user.
- A long-lived API token for an admin user with AI Gateway access.

Because the Agents provider is a deployment-wide configuration, every chat
that uses this provider sends the same token to AI Gateway. AI Gateway then
attributes the request to the chat's owner using the
[Coder identity headers](#identity-and-correlation-headers) described below,
not to the user the token belongs to.

> [!NOTE]
> Coder Agents does not support BYOK (Bring Your Own Key) when routing through
> AI Gateway today. The Agents [User API keys](../../agents/models.md#user-api-keys-byok)
> feature is independent of AI Gateway and is intended for direct provider
> calls.

## Identity and correlation headers

When Coder Agents calls a provider, it attaches Coder identity headers to
every outgoing request so AI Gateway can group interceptions into the
correct [session](../audit.md#concepts):

| Header                 | Value                                                                   |
|------------------------|-------------------------------------------------------------------------|
| `User-Agent`           | `coder-agents/<version> (<os>/<arch>)`                                  |
| `X-Coder-Owner-Id`     | Coder user ID of the chat owner.                                        |
| `X-Coder-Chat-Id`      | Top-level chat ID. For sub-chats, this is the parent chat ID.           |
| `X-Coder-Subchat-Id`   | Sub-chat ID. Only present when the request originates from a sub-agent. |
| `X-Coder-Workspace-Id` | Workspace ID, when the chat is pinned to a workspace.                   |

You don't need to configure these headers; they are set automatically. AI
Gateway uses `X-Coder-Chat-Id` as the session key, which means every
interception in a chat (and its sub-agents) appears under a single session
in the [audit UI](../audit.md#sessions-list).

## Pre-configuring in templates

You don't need to configure anything inside workspaces for Agents to use AI
Gateway. The agent loop runs in the control plane, so the Agents provider's
Base URL is the only place AI Gateway needs to be wired up.

If you also want IDE-based clients running inside Agents-provisioned
workspaces (such as Claude Code or Codex CLI) to route through AI Gateway,
configure them on the workspace template. See the
[Configuring In-Workspace Tools](./index.md#configuring-in-workspace-tools)
section for the general pattern, plus the per-client pages such as
[Claude Code](./claude-code.md#pre-configuring-in-templates).

## Verifying the integration

After saving the provider, start a new chat from the Agents page and send a
short prompt. Then:

1. Open the AI Gateway sessions UI at
   `https://coder.example.com/aibridge/sessions`.
1. The most recent session should show **Coder Agents** as the client and
   the chat owner as the initiator.
1. Click into the session to see the chat's interceptions, token usage, and
   any tool invocations.

If the session does not appear, check that the Agents provider's Base URL
points at your deployment's `/api/v2/aibridge/...` path and that the API key
is a valid Coder token.

## Troubleshooting

- **`401 Unauthorized` from the chat.** The API key on the Agents provider
  is not a valid Coder token, has been revoked, or belongs to a user that
  cannot reach AI Gateway. Generate a new long-lived token and update the
  provider.
- **Sessions in audit show a generic client instead of Coder Agents.** This
  usually means the request bypassed AI Gateway. Confirm the provider's
  Base URL starts with your deployment's `/api/v2/aibridge/` path and not
  the upstream provider URL.
- **Provider does not appear in the Agents model selector.** Add at least
  one [model](../../agents/models.md#add-a-model) to the provider after
  saving the Base URL. Providers without an enabled model are hidden from
  developers.

## Related documentation

- [Coder Agents: Models and providers](../../agents/models.md) for the full
  reference on configuring providers in Agents.
- [Coder Agents: Using an LLM proxy](../../agents/models.md#using-an-llm-proxy)
  for the short version of this same configuration.
- [AI Gateway setup](../setup.md) for enabling AI Gateway and configuring
  upstream provider credentials.
- [Auditing AI sessions](../audit.md) for how AI Gateway groups Coder Agents
  traffic into sessions.
