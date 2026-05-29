# Coder Agents through AI Gateway

[Coder Agents](../../agents/index.md) routes model requests through AI Gateway by
using the AI provider configuration stored in Coder. This gives Agents traffic
AI Gateway observability and governance without requiring administrators to point
Agents providers back at Coder's public AI Gateway routes.

## Prerequisites

- AI Gateway is [enabled](../setup.md#activation) on your Coder deployment.
- At least one Coder Agents [provider](../../agents/models.md#providers) is
  configured with upstream credentials.
- The provider has at least one enabled [model](../../agents/models.md#add-a-model).

## URL concepts

There are two different URLs involved in AI Gateway routing:

| URL type                         | Used by                                                                                       | Example                                               |
|----------------------------------|-----------------------------------------------------------------------------------------------|-------------------------------------------------------|
| **Provider Endpoint/Base URL**   | The shared Coder provider configuration. Set this to the upstream provider or proxy endpoint. | `https://api.openai.com/v1/`                          |
| **Public AI Gateway client URL** | External clients that call AI Gateway over HTTP.                                              | `https://coder.example.com/api/v2/aibridge/openai/v1` |

For the default Coder Agents path, configure the provider **Endpoint** or
**Base URL** as the upstream provider or proxy URL. Do not set it to
`https://<coder-host>/api/v2/aibridge/...`.

External clients, such as IDE extensions and command line tools running outside
Coder Agents, still use the public AI Gateway client URL. See the other
[client setup guides](./index.md) for those tools.

## Configure provider endpoints

Coder Agents and AI Gateway share provider configuration. Update provider
settings from the Coder dashboard:

1. Open the Coder dashboard and navigate to the **Agents** page.
1. Open **Settings** > **Manage Agents** and select the **Providers** tab.
1. Select the provider you want to configure.
1. Set **Endpoint** or **Base URL** to the upstream provider or proxy endpoint.
1. Enter the provider credential that upstream endpoint expects.
1. Click **Save**.

For OpenAI-shaped providers, AI Gateway appends request suffixes such as
`/chat/completions`, `/responses`, and `/models` to the configured provider
Endpoint/Base URL. Include the upstream provider's OpenAI-compatible prefix in
that value.

| Provider type                       | Example Endpoint/Base URL                                  |
|-------------------------------------|------------------------------------------------------------|
| OpenAI                              | `https://api.openai.com/v1/`                               |
| Azure OpenAI                        | `https://<resource-name>.openai.azure.com/openai/v1`       |
| Google Gemini OpenAI-compatible API | `https://generativelanguage.googleapis.com/v1beta/openai/` |
| OpenRouter                          | `https://openrouter.ai/api/v1`                             |
| Vercel AI Gateway                   | `https://ai-gateway.vercel.sh/v1`                          |
| Generic OpenAI-compatible proxy     | `https://provider.example.com/v1`                          |

These examples show the expected shape. Confirm the exact endpoint in your
provider or proxy documentation. A URL that passes syntax validation can still
fail if the upstream does not support the APIs Coder sends.

Anthropic-shaped providers, such as Anthropic and Bedrock, use their own route
shape and do not use the OpenAI `/v1/chat/completions` or `/v1/responses`
suffixes.

## Migrate old AI Gateway Base URLs

Older guidance instructed administrators to set an Agents provider Base URL to a
Coder AI Gateway route such as
`https://coder.example.com/api/v2/aibridge/openai/v1`. That is no longer the
correct default configuration for Coder Agents.

To migrate a provider that still points at `/api/v2/aibridge/...`:

1. Replace the provider Endpoint/Base URL with the upstream provider or proxy
   endpoint.
1. Verify that OpenAI-compatible providers include the upstream compatibility
   prefix, such as `/v1`, `/openai/v1`, or `/v1beta/openai`.
1. Confirm the upstream supports the request paths Coder may call, especially
   `/models`, `/chat/completions`, and `/responses` for OpenAI-shaped
   providers.
1. Save the provider.
1. Start a new chat and send a short prompt to test the model.

## Credential selection and BYOK

Coder Agents routed through AI Gateway does not exactly follow the direct
provider key policy matrix in [Models](../../agents/models.md#key-policy). For
this path, BYOK behavior is governed by the global AI Gateway BYOK setting and
whether the user has a saved provider key.

| Situation                                                                                       | Upstream credential behavior                                                                     |
|-------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------|
| Centralized provider key is configured and the user has no saved key                            | AI Gateway uses the centralized provider credential.                                             |
| Global AI Gateway BYOK is disabled                                                              | AI Gateway uses centralized provider credentials. Saved user keys are ignored for this route.    |
| Global AI Gateway BYOK is enabled and the user has a saved OpenAI-compatible key                | The user key is delegated through AI Gateway and used for the upstream OpenAI-shaped request.    |
| Global AI Gateway BYOK is enabled and the user has a saved Anthropic or Bedrock key             | The user key is delegated through AI Gateway and used for the upstream Anthropic-shaped request. |
| Required provider metadata, provider row, active delegated key, or transport support is missing | The Agents model request fails before Coder sends an upstream provider request.                  |

This is a current limitation of the AI Gateway-routed Agents path. The key
policy matrix in [Models](../../agents/models.md#key-policy) describes a
separate direct-provider credential policy and should not be read as the AI
Gateway-routed behavior.

## Identity and audit

You do not need to configure Coder API tokens in provider settings for Agents to
use AI Gateway. Agents requests are routed inside the Coder control plane and are
marked with the Coder Agents source for AI Gateway sessions.

To verify routing:

1. Start a new chat from the **Agents** page and send a short prompt.
1. Open the AI Gateway sessions UI at
   `https://coder.example.com/aibridge/sessions`.
1. Check that the recent session shows the Coder Agents client and includes the
   prompt, token usage, and tool usage records.

## Troubleshooting

- **Requests fail with upstream 404 errors.** Check the provider Endpoint/Base
  URL. For OpenAI-compatible providers, the endpoint usually needs a provider
  path prefix such as `/v1`, `/openai/v1`, or `/v1beta/openai`.
- **Requests fail before reaching the upstream provider.** Confirm the provider
  is enabled, has a configured model, has usable credentials, and uses a
  provider type that AI Gateway can route.
- **Provider does not appear in the Agents model selector.** Add at least one
  [model](../../agents/models.md#add-a-model) to the provider. Providers
  without an enabled model are hidden from developers.
- **External clients work, but Coder Agents fails.** Do not copy an external
  client URL from `/api/v2/aibridge/...` into the shared provider
  Endpoint/Base URL. Use the upstream provider or proxy URL instead.
- **A user key is not used.** Confirm global AI Gateway BYOK is enabled and the
  user has saved a key for the selected provider.
- **"Chat interrupted" error when resuming a conversation.** This occurs when
  the API key that was used to start a chat turn is no longer available. Common
  causes include upgrading from a version before `api_key_id` tracking was
  introduced or deleting an API key while a chat is active. The error is
  self-healing: send your message again and the new message will use your
  current API key. If the error persists after resending, report it.

## Related documentation

- [Coder Agents: Models and providers](../../agents/models.md) for provider,
  model, and direct key policy settings.
- [AI Gateway authentication](../auth.md) for centralized credentials and BYOK.
- [AI Gateway setup](../setup.md) for enabling AI Gateway and configuring
  upstream provider endpoints.
- [AI Gateway supported APIs](../reference.md#supported-apis) for route coverage.
- [Auditing AI sessions](../audit.md) for how AI Gateway groups Coder Agents
  traffic into sessions.
