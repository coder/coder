# Coder Agents

[Coder Agents](../../agents/index.md) is a chat interface and API for delegating
development work to coding agents that run inside the Coder control plane. When
AI Gateway is enabled on the same deployment, Coder Agents traffic can be
routed through it for full audit and governance coverage.

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
and set the **API Key** to a credential AI Gateway accepts. Because both
services run in the same `coderd` process, the AI Gateway endpoint is just
your deployment URL plus `/api/v2/aibridge/<provider>`.

<div class="tabs">

### Anthropic

1. Open the Coder dashboard and navigate to the **Agents** page.
1. Click **Admin**, then select the **Providers** tab.
1. Click **Anthropic**.
1. Set the **Base URL** to
   `https://coder.example.com/api/v2/aibridge/anthropic`.
1. Set the **API Key** to a Coder API token. See
   [Authentication](#authentication) for which token to use.
1. Click **Save**.

To target a [named Anthropic instance](../setup.md#multiple-instances-of-the-same-provider)
in AI Gateway (for example, `anthropic-corp`), replace `anthropic` in the
Base URL with the instance name:
`https://coder.example.com/api/v2/aibridge/anthropic-corp`.

### OpenAI

1. Open the Coder dashboard and navigate to the **Agents** page.
1. Click **Admin**, then select the **Providers** tab.
1. Click **OpenAI**.
1. Set the **Base URL** to
   `https://coder.example.com/api/v2/aibridge/openai/v1`.
1. Set the **API Key** to a Coder API token. See
   [Authentication](#authentication) for which token to use.
1. Click **Save**.

To target a [named OpenAI instance](../setup.md#multiple-instances-of-the-same-provider)
in AI Gateway (for example, an Azure OpenAI deployment named `azure-openai`),
replace `openai` in the Base URL with the instance name:
`https://coder.example.com/api/v2/aibridge/azure-openai/v1`.

### OpenAI Compatible

Use **OpenAI Compatible** for an OpenAI-typed AI Gateway named instance
(for example, an Azure OpenAI or ChatGPT provider) when you want to
opt out of the OpenAI-specific options the Agents OpenAI provider
applies by default, such as reasoning effort or parallel tool calls.

1. Open the Coder dashboard and navigate to the **Agents** page.
1. Click **Admin**, then select the **Providers** tab.
1. Click **OpenAI Compatible**.
1. Set the **Base URL** to
   `https://coder.example.com/api/v2/aibridge/<instance-name>/v1`.
   The instance must be of type `openai` in AI Gateway.
1. Set the **API Key** to a Coder API token.
1. Click **Save**.

> [!NOTE]
> OpenAI Compatible speaks the OpenAI wire protocol, so it cannot target
> Anthropic-typed AI Gateway instances. To route a named Anthropic instance
> through AI Gateway, configure the Agents **Anthropic** provider as shown
> above.

</div>

Replace `coder.example.com` with your Coder deployment URL.

After saving, [add or update a model](../../agents/models.md#add-a-model) on
each provider so developers can select it from the chat. Models from a
provider only appear in the model selector once the provider has valid
credentials.

## Authentication

AI Gateway only accepts Coder-issued tokens for client authentication. The
upstream provider keys you configured for AI Gateway (for example,
`CODER_AIBRIDGE_OPENAI_KEY`) are used by AI Gateway internally to call the
upstream provider; they are not what Coder Agents sends.

Coder Agents stores the **API Key** field on each provider as the bearer
credential it forwards to AI Gateway on every request from any chat that
uses that provider. AI Gateway resolves the bearer token to a Coder user
and uses **that user** as the initiator on every interception.

Because the Agents provider config is deployment-wide, every chat that
uses this provider is logged in AI Gateway under the identity of whoever
owns the API token configured here. Per-chat attribution to the developer
who started a chat is **not** preserved when routing Agents traffic
through AI Gateway today. See
[Known limitations](#known-limitations) below.

For that reason, **use a long-lived API token for a dedicated
[service account](../../../admin/users/headless-auth.md#create-a-service-account)**
that is intended to represent Agents traffic in audit. Avoid using an
admin's personal token: every chat would otherwise appear to have been
initiated by that admin.

> [!NOTE]
> Coder Agents does not support Bring Your Own Key when routing through
> AI Gateway today. The Agents
> [User API keys](../../agents/models.md#user-api-keys-byok) feature is
> independent of AI Gateway and applies to direct provider calls only.

## Identity and correlation headers

When Coder Agents calls a provider, it attaches identity headers to every
outgoing request. Today AI Gateway uses two of them:

| Header            | Used by AI Gateway today                                                                                                 |
|-------------------|--------------------------------------------------------------------------------------------------------------------------|
| `User-Agent`      | Detects Coder Agents traffic and labels sessions with the `Coder Agents` client name.                                    |
| `X-Coder-Chat-Id` | Acts as the AI Gateway session key, so every interception in a chat (and its sub-agents) appears under a single session. |

Coder Agents also sends `X-Coder-Owner-Id`, `X-Coder-Subchat-Id`, and
`X-Coder-Workspace-Id`. These are emitted for forward compatibility but
are not consumed by AI Gateway today.

You don't need to configure these headers; they are set automatically.

## Pre-configuring in templates

You don't need to configure anything inside workspaces for Coder Agents
itself to use AI Gateway. The agent loop runs in the control plane, so
the Agents provider's Base URL is the only place AI Gateway needs to be
wired up.

If you also want IDE-based clients running inside Agents-provisioned
workspaces (such as Claude Code or Codex CLI) to route through AI
Gateway, configure them on the workspace template. See the
[Configuring In-Workspace Tools](./index.md#configuring-in-workspace-tools)
section for the general pattern, plus the per-client pages such as
[Claude Code](./claude-code.md#pre-configuring-in-templates).

## Verifying the integration

After saving the provider, start a new chat from the Agents page and send
a short prompt. Then:

1. Open the AI Gateway sessions UI at
   `https://coder.example.com/aibridge/sessions`.
1. The most recent session should show **Coder Agents** as the client and
   the user that owns the API token configured on the Agents provider as
   the initiator.
1. Click into the session to see the chat's interceptions, token usage,
   and any tool invocations.

If the session does not appear, check that the Agents provider's Base URL
points at your deployment's `/api/v2/aibridge/...` path and that the API
key is a valid Coder token.

## Troubleshooting

- **`401 Unauthorized` from the chat.** The API key on the Agents provider
  is not a valid Coder token, has been revoked, or belongs to a user that
  cannot reach AI Gateway. Generate a new long-lived token and update the
  provider.
- **Sessions in audit show a generic client instead of Coder Agents.**
  This usually means the request bypassed AI Gateway. Confirm the
  provider's Base URL starts with your deployment's `/api/v2/aibridge/`
  path and not the upstream provider URL.
- **Provider does not appear in the Agents model selector.** Add at least
  one [model](../../agents/models.md#add-a-model) to the provider after
  saving the Base URL. Providers without an enabled model are hidden from
  developers.

## Known limitations

- **Per-developer attribution is not preserved.** AI Gateway attributes
  every interception to the user that owns the bearer token configured
  on the Agents provider, regardless of which developer started the
  chat. The chat owner ID is sent by Coder Agents in `X-Coder-Owner-Id`
  but is not consumed by AI Gateway today. Use a dedicated service
  account for the Agents provider's API token so audit data is
  attributed to a single, non-human identity.
- **Bring Your Own Key (BYOK) is not supported through AI Gateway.**
  Personal LLM credentials configured under
  [User API keys](../../agents/models.md#user-api-keys-byok) are sent
  directly to the provider; AI Gateway is not involved when BYOK is
  active.

## Related documentation

- [Coder Agents: Models and providers](../../agents/models.md) for the
  full reference on configuring providers in Agents.
- [Coder Agents: Using an LLM proxy](../../agents/models.md#using-an-llm-proxy)
  for the short version of this same configuration.
- [AI Gateway setup](../setup.md) for enabling AI Gateway and
  configuring upstream provider credentials.
- [Auditing AI sessions](../audit.md) for how AI Gateway groups Coder
  Agents traffic into sessions.
