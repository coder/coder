# Zed

Zed IDE supports AI Bridge via its `language_models` configuration in `settings.json`.

## Configuration

To configure Zed to use AI Bridge, you need to edit your `settings.json` file. You can access this by pressing `Cmd/Ctrl + ,` or opening the command palette and searching for "Open Settings".

You can configure both Anthropic and OpenAI providers to point to AI Bridge.

```json
{
  "language_models": {
    "anthropic": {
      "api_url": "https://coder.example.com/api/v2/aibridge/anthropic"
    },
    "openai": {
      "api_url": "https://coder.example.com/api/v2/aibridge/openai/v1"
    }
  }
}
```

Replace `coder.example.com` with your Coder deployment URL.

## Authentication

Zed requires an API key for these providers. For AI Bridge, this key is your **Coder Session Token**.

You can set this in two ways:

1.  **Zed UI**:
    *   Open the Assistant Panel (right sidebar).
    *   Click "Configuration" or the settings icon.
    *   Select your provider ("Anthropic" or "Coder").
    *   Paste your Coder Session Token when prompted for the API Key.

2.  **Environment Variables**:
    *   If you are running Zed in an environment where you can set variables (or launch it from a shell), you can set `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` to your session token.

---

**References:** [Configuring Zed - Language Models](https://zed.dev/docs/configuring-zed#language-models)
