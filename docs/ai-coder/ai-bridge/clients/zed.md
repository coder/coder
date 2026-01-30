# Zed

[Zed](https://zed.dev) IDE supports AI Bridge via its `language_models` configuration in `settings.json`.

## Configuration

To configure Zed to use AI Bridge, you need to edit your `settings.json` file. You can access this by pressing `Cmd/Ctrl + ,` or opening the command palette and searching for "Open Settings".

<!-- TODO: Add screenshot of Zed settings.json or assistant panel configuration -->

You can configure both Anthropic and OpenAI providers to point to AI Bridge.

```json
{
    "agent": {
        "default_model": {
            "provider": "anthropic",
            "model": "claude-sonnet-4-5-latest",
        },
    },
  "language_models": {
    "anthropic": {
      "api_url": "https://coder.example.com/api/v2/aibridge/anthropic",
    },
    "openai": {
      "api_url": "https://coder.example.com/api/v2/aibridge/openai/v1",
    },
  },
}
```

*Replace `coder.example.com` with your Coder deployment URL.*

## Authentication

Zed requires an API key for these providers. For AI Bridge, this key is your **[Coder Session Token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.

You can set this in two ways:

<div class="tabs">

### Zed UI

1. Open the Assistant Panel (right sidebar).
1. Click "Configuration" or the settings icon.
1. Select your provider ("Anthropic" or "OpenAI").
1. Paste your Coder Session Token for the API Key.

### Environment Variables

1. Set `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` to your Coder session token in the environment where you launch Zed.

</div>

**References:** [Configuring Zed - Language Models](https://zed.dev/docs/reference/all-settings#language-models)
