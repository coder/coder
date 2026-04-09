# OpenCode

OpenCode supports both OpenAI and Anthropic models and can be configured to use AI Gateway by setting custom base URLs for each provider.

## Centralized API Key

You can configure OpenCode to connect to AI Gateway by setting the following configuration options in your OpenCode configuration file (e.g., `~/.config/opencode/opencode.json`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "anthropic": {
      "options": {
        "baseURL": "https://coder.example.com/api/v2/aibridge/anthropic/v1"
      }
    },
    "openai": {
      "options": {
        "baseURL": "https://coder.example.com/api/v2/aibridge/openai/v1"
      }
    }
  }
}
```

To authenticate with AI Gateway, get your **[Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** and replace `<your-coder-session-token>` in `~/.local/share/opencode/auth.json`

```json
{
  "anthropic": {
    "type": "api",
    "key": "<your-coder-session-token>"
  },
  "openai": {
    "type": "api",
    "key": "<your-coder-session-token>"
  }
}
```

## BYOK (Personal API Key)

Set the following in `~/.config/opencode/opencode.json`, including the `X-Coder-AI-Governance-Token` header with your Coder session token:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "anthropic": {
      "options": {
        "baseURL": "https://coder.example.com/api/v2/aibridge/anthropic/v1",
        "headers": {
          "X-Coder-AI-Governance-Token": "<your-coder-session-token>"
        }
      }
    },
    "openai": {
      "options": {
        "baseURL": "https://coder.example.com/api/v2/aibridge/openai/v1",
        "headers": {
          "X-Coder-AI-Governance-Token": "<your-coder-session-token>"
        }
      }
    }
  }
}
```

Set your personal API keys in `~/.local/share/opencode/auth.json`:

```json
{
  "anthropic": {
    "type": "api",
    "key": "<your-anthropic-api-key>"
  },
  "openai": {
    "type": "api",
    "key": "<your-openai-api-key>"
  }
}
```

**References:** [OpenCode Documentation](https://opencode.ai/docs/providers/#config)
