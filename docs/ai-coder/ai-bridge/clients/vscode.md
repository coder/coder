# VS Code

[VS Code](https://code.visualstudio.com) can be configured to use AI Bridge with the GitHub Copilot Chat extension's custom language model support.

## Configuration

### Prerequisites

- You need the **Pre-release** version of the GitHub Copilot Chat extension and VS Code Insiders.

### Steps

1. Open command palette (`Ctrl+Shift+P` or `Cmd+Shift+P` on Mac) and search for _Chat: Manage Language Models_.
1. Click on _Add Models..._ --> _Open AI Compatible_.
1. Add a name for your model provider (e.g., `Coder`).
1. Add the `CODER_SESSION_TOKEN` (your [Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)) as the API key.
1. Then when prompted, add the below configuration in `chatLanguageModels.json`.
1. Add or modify the model entries as needed.

```json
[
    {
        "name": "Coder",
        "vendor": "customoai",
        "apiKey": "${input:chat.lm.secret.-40213ea}", // Replace with your secret input name added automatically when adding the API key
        "models": [
            {
                "name": "GPT 5.2",
                "url": "https://coder.example.com/api/v2/aibridge/openai/v1/chat/completions",
                "toolCalling": true,
                "vision": true,
                "thinking": true,
                "maxInputTokens": 272000,
                "maxOutputTokens": 128000,
                "id": "gpt-5.2"
            },
            {
                "name": "GPT 5.2 Codex",
                "url": "https://coder.example.com/api/v2/aibridge/openai/v1/responses",
                "toolCalling": true,
                "vision": true,
                "thinking": true,
                "maxInputTokens": 272000,
                "maxOutputTokens": 128000,
                "id": "gpt-5.2-codex"
            }
        ]
    }
]
```

> [!NOTE]
> The setting names may change as the feature moves from pre-release to stable. Refer to the official documentation for the latest setting keys.

**References:** [GitHub Copilot - Bring your own language model](https://code.visualstudio.com/docs/copilot/customization/language-models#_add-an-openaicompatible-model)
