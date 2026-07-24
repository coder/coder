
> [!NOTE]
> AI Gateway requires the [AI Governance Add-On](../../ai-governance.md).
> As of Coder v2.32, deployments without the add-on will not be able to
> access AI Gateway.

VS Code's native chat can be configured to use AI Gateway via the **Custom Endpoint** language model provider (VS Code 1.122+, Stable). GitHub sign-in is not required, so this works in air-gapped or restricted environments.

## Setup

Requires VS Code 1.122+ and the [GitHub Copilot Chat extension](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot-chat).

For each provider below, the setup steps are:

1. Open the Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P` on Mac) and run **Chat: Manage Language Models**.
1. Select **Add** → **Custom Endpoint**.
1. Enter a **group name**, **display name**, your **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** as the API key, and the **API type** shown below.
1. To add or edit models, select the gear icon next to the provider in the Language Models view to open `chatLanguageModels.json`.

> [!IMPORTANT]
> Enter your API token through the UI. VS Code stores it securely and inserts a reference like `${input:chat.lm.secret.XXXXX}` into the JSON. Do not paste your token directly into the JSON file.

_Replace `coder.example.com` with your Coder deployment URL. Model IDs must match what is configured in your AI Gateway._

### OpenAI-compatible models

Set **API type** to `responses`.

```json
{
    "name": "Coder (OpenAI)",
    "vendor": "customendpoint",
    "apiKey": "${input:chat.lm.secret.XXXXX}",
    "apiType": "responses",
    "models": [
        {
            "id": "gpt-5.5",
            "name": "GPT 5.5",
            "url": "https://coder.example.com/api/v2/ai-gateway/openai",
            "toolCalling": true,
            "vision": true,
            "thinking": true,
            "streaming": true,
            "maxInputTokens": 272000,
            "maxOutputTokens": 128000
        }
    ]
}
```

### Anthropic models

Set **API type** to `messages`.

```json
{
    "name": "Coder (Anthropic)",
    "vendor": "customendpoint",
    "apiKey": "${input:chat.lm.secret.XXXXX}",
    "apiType": "messages",
    "models": [
        {
            "id": "claude-sonnet-4.6",
            "name": "Claude Sonnet 4.6",
            "url": "https://coder.example.com/api/v2/ai-gateway/anthropic",
            "toolCalling": true,
            "vision": true,
            "thinking": true,
            "streaming": true,
            "maxInputTokens": 1000000,
            "maxOutputTokens": 64000
        }
    ]
}
```

**References:** [VS Code - Bring your own language model](https://code.visualstudio.com/docs/copilot/customization/language-models)
