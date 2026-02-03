# OpenCode

## Configuration

You can configure OpenCode to connect to AI Bridge by setting the following configuration options in your OpenCode configuration file (e.g., `~/.config/opencode/opencode.json`):

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

## Authentication

To authenticate with AI Bridge, get your **[Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** and replace `<your-coder-session-token>` in `~/.local/share/opencode/auth.json`

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

**References:** [OpenCode Documentation](https://opencode.ai/docs/providers/#config)
