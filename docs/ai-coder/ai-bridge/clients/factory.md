# Factory


## Configuration

1. Open `~/.factory/settings.json` (create it if it does not exist).
2. Add a `customModels` entry for each provider you want to use with AI Bridge.
3. Replace `coder.example.com` with your Coder deployment URL.
4. Use a **[Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** for `apiKey`.

```json
{
  "customModels": [
    {
      "model": "claude-4-5-opus",
      "displayName": "Claude (Coder AI Bridge)",
      "baseUrl": "https://coder.example.com/api/v2/aibridge/anthropic",
      "apiKey": "<your-coder-session-token>",
      "provider": "anthropic",
      "maxOutputTokens": 8192
    },
    {
      "model": "gpt-5.2-codex",
      "displayName": "GPT (Coder AI Bridge)",
      "baseUrl": "https://coder.example.com/api/v2/aibridge/openai/v1",
      "apiKey": "<your-coder-session-token>",
      "provider": "openai",
      "maxOutputTokens": 16384
    }
  ]
}
```

**References:** [Factory BYOK OpenAI & Anthropic](https://docs.factory.ai/cli/byok/openai-anthropic)
