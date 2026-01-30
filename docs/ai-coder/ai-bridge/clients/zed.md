# Zed

Zed IDE supports AI Bridge via its `language_models` configuration in `settings.json`.

## Configuration

To configure Zed to use AI Bridge, you need to edit your `settings.json` file. You can access this by pressing `Cmd/Ctrl + ,` or opening the command palette and searching for "Open Settings".

<!-- TODO: Add screenshot of Zed settings.json or assistant panel configuration -->

You can configure both Anthropic and OpenAI providers to point to AI Bridge.

<div class="tabs">

### OpenAI Provider

To use OpenAI-compatible models (e.g., `gpt-4o`):

```json
{
  "language_models": {
    "openai": {
      "api_url": "https://coder.example.com/api/v2/aibridge/openai/v1"
    }
  }
}
```

### Anthropic Provider

To use Anthropic models (e.g., `claude-3-5-sonnet`):

```json
{
  "language_models": {
    "anthropic": {
      "api_url": "https://coder.example.com/api/v2/aibridge/anthropic"
    }
  }
}
```

</div>

*Replace `coder.example.com` with your Coder deployment URL.*

## Authentication

Zed requires an API key for these providers. For AI Bridge, this key is your **[Coder Session Token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.

You can set this in two ways:

1. **Zed UI**:
    * Open the Assistant Panel (right sidebar).
    * Click "Configuration" or the settings icon.
    * Select your provider ("Anthropic" or "OpenAI").
    * Paste your Coder Session Token when prompted for the API Key.

2. **Environment Variables**:
    * Set `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` to your Coder session token in the environment where you launch Zed.

**References:** [Configuring Zed - Language Models](https://zed.dev/docs/configuring-zed#language-models)
