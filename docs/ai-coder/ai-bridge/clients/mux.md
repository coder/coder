# Mux

Mux can be configured to route OpenAI- and Anthropic-compatible traffic through AI Bridge by setting a custom provider base URL and using a Coder-issued token for authentication.

## Prerequisites

- AI Bridge is enabled on your Coder deployment.
- A **[Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** or long-lived API key.

## Configuration

<div class="tabs">

### OpenAI (GPT / Codex)

1. Open Mux settings (`Cmd+,` / `Ctrl+,`).
2. Go to **Providers** → **OpenAI**.
3. Set **API Key** to your Coder session token.
4. Set **Base URL** to `https://coder.example.com/api/v2/aibridge/openai/v1`.

### Anthropic (Claude)

1. Open Mux settings (`Cmd+,` / `Ctrl+,`).
2. Go to **Providers** → **Anthropic**.
3. Set **API Key** to your Coder session token.
4. Set **Base URL** to `https://coder.example.com/api/v2/aibridge/anthropic`.

</div>

_Replace `coder.example.com` with your Coder deployment URL._

## Environment variables

Mux reads provider configuration from its settings UI and also from environment variables.
Environment variables are useful in CI or when running Mux inside a Coder workspace.

> [!NOTE]
> Mux treats environment variables as a fallback when a provider is not configured in settings.
> If you have already configured a provider in the UI, clear it (or update it) for env vars to take effect.

```sh
# OpenAI-compatible traffic (GPT, Codex, etc.)
export OPENAI_API_KEY="<your-coder-session-token>"
export OPENAI_BASE_URL="https://coder.example.com/api/v2/aibridge/openai/v1"

# Anthropic-compatible traffic (Claude, etc.)
export ANTHROPIC_API_KEY="<your-coder-session-token>"
export ANTHROPIC_BASE_URL="https://coder.example.com/api/v2/aibridge/anthropic"
```

## Advanced: providers.jsonc

If you prefer a file-based config, edit `~/.mux/providers.jsonc`:

```jsonc
{
  "openai": {
    "apiKey": "<your-coder-session-token>",
    "baseUrl": "https://coder.example.com/api/v2/aibridge/openai/v1"
  },
  "anthropic": {
    "apiKey": "<your-coder-session-token>",
    "baseUrl": "https://coder.example.com/api/v2/aibridge/anthropic"
  }
}
```

**References:** [Mux provider environment variables](https://mux.coder.com/config/providers#environment-variables)
