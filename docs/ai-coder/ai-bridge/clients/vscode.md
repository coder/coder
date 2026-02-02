# VS Code

[VS Code](https://code.visualstudio.com) can be configured to use AI Bridge with the GitHub Copilot Chat extension's custom language model support.

## Configuration

> [!IMPORTANT]
> You need the **Pre-release** version of the [GitHub Copilot Chat extension](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot-chat) and [VS Code Insiders](https://code.visualstudio.com/insiders/).

1. Open command palette (`Ctrl+Shift+P` or `Cmd+Shift+P` on Mac) and search for _Chat: Open Language Models (JSON)_.
1. Paste the following JSON configuration, replacing `<your-coder-session-token>` with your **[Coder Session Token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**:

```json
[
    {
        "name": "Coder",
        "vendor": "customoai",
        "apiKey": "your-coder-session-token>",
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

_Replace `coder.example.com` with your Coder deployment URL._

> [!NOTE]
> The setting names may change as the feature moves from pre-release to stable. Refer to the official documentation for the latest setting keys.

**References:** [GitHub Copilot - Bring your own language model](https://code.visualstudio.com/docs/copilot/customization/language-models#_add-an-openaicompatible-model)
