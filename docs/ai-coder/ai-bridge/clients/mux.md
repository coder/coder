# Mux

Mux makes it easy to run parallel coding agents, each with its own isolated workspace, from your browser or desktop; it is open source and provider-agnostic. For more background on Mux, see [Coder Research](../../../coder-research.md#mux).

Mux can be configured to route OpenAI- and Anthropic-compatible traffic through AI Bridge by setting a custom provider base URL and using a Coder-issued token for authentication.

## Prerequisites

- AI Bridge is enabled on your Coder deployment.
- A **[Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** or long-lived API key.

## Configuration

<div class="tabs">

### OpenAI

1. Open Mux settings (`Cmd+,` / `Ctrl+,`).
2. Go to **Providers** → **OpenAI**.
3. Set **API Key** to your Coder session token.
4. Set **Base URL** to `https://coder.example.com/api/v2/aibridge/openai/v1`.

### Anthropic

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

## Running Mux in a Coder workspace

If you want to run Mux inside a Coder workspace (for example, as a Coder app), you can install it with the [Mux module](https://registry.coder.com/modules/coder/mux) and pre-configure AI Bridge via environment variables on the agent:

```tf
data "coder_workspace" "me" {}

data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  # ... other agent configuration
  env = {
    OPENAI_API_KEY     = data.coder_workspace_owner.me.session_token
    OPENAI_BASE_URL    = "${data.coder_workspace.me.access_url}/api/v2/aibridge/openai/v1"
    ANTHROPIC_API_KEY  = data.coder_workspace_owner.me.session_token
    ANTHROPIC_BASE_URL = "${data.coder_workspace.me.access_url}/api/v2/aibridge/anthropic"
  }
}

module "mux" {
  source   = "registry.coder.com/coder/mux/coder"
  version  = "~> 1.0" # See the module page for the latest version.
  agent_id = coder_agent.main.id
}
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
