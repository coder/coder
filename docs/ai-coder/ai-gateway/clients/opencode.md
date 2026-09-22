---
title: OpenCode
---

> [!NOTE]
> AI Gateway is part of [AI Governance](../../ai-governance.md), which is
> included with a Premium license.

OpenCode supports both OpenAI and Anthropic models and can be configured to use AI Gateway by setting custom base URLs for each provider.

## Centralized API Key

You can configure OpenCode to connect to AI Gateway by setting the following configuration options in your OpenCode configuration file (e.g., `~/.config/opencode/opencode.json`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "anthropic": {
      "options": {
        "baseURL": "https://coder.example.com/api/v2/ai-gateway/anthropic/v1"
      }
    },
    "openai": {
      "options": {
        "baseURL": "https://coder.example.com/api/v2/ai-gateway/openai/v1"
      }
    }
  }
}
```

To authenticate with AI Gateway, get your **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** and replace `<your-coder-api-token>` in `~/.local/share/opencode/auth.json`

```json
{
  "anthropic": {
    "type": "api",
    "key": "<your-coder-api-token>"
  },
  "openai": {
    "type": "api",
    "key": "<your-coder-api-token>"
  }
}
```

## BYOK (Personal API Key)

Set the following in `~/.config/opencode/opencode.json`, including the `X-Coder-AI-Governance-Token` header with your Coder API token:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "anthropic": {
      "options": {
        "baseURL": "https://coder.example.com/api/v2/ai-gateway/anthropic/v1",
        "headers": {
          "X-Coder-AI-Governance-Token": "<your-coder-api-token>"
        }
      }
    },
    "openai": {
      "options": {
        "baseURL": "https://coder.example.com/api/v2/ai-gateway/openai/v1",
        "headers": {
          "X-Coder-AI-Governance-Token": "<your-coder-api-token>"
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

## Custom models through Bedrock Mantle

You can use OpenCode with custom model IDs through a [Bedrock Mantle provider](../providers.md#mantle) configured by your Coder administrator.
AI Gateway signs requests with AWS credentials centrally, so you only need a [Coder API token](../auth.md#authenticate-ai-clients) in OpenCode.
Use a custom provider in OpenCode that matches your model's API, rather than its native Amazon Bedrock integration.
The following example uses Astra with the Responses API.
For other models, choose the SDK package that matches the API:

| Model API          | OpenCode `npm` package      |
|--------------------|-----------------------------|
| Responses          | `@ai-sdk/openai`            |
| Chat Completions   | `@ai-sdk/openai-compatible` |
| Anthropic Messages | `@ai-sdk/anthropic`         |

1. In a shell where you're logged in with the Coder CLI, set your token:

   ```sh
   export CODER_TOKEN="$(coder login token)"
   ```

   This command sets `CODER_TOKEN` without printing the token.

2. Merge the following configuration into `~/.config/opencode/opencode.json`:

   ```json
   {
     "$schema": "https://opencode.ai/config.json",
     "provider": {
       "coder-mantle": {
         "npm": "@ai-sdk/openai",
         "name": "Coder Bedrock Mantle",
         "options": {
           "baseURL": "https://coder.example.com/api/v2/ai-gateway/bedrock-provider-name/v1",
           "apiKey": "{env:CODER_TOKEN}"
         },
         "models": {
           "astra": {
             "id": "openai.gpt-6-astra",
             "name": "Astra",
             "tool_call": true
           }
         }
       }
     },
     "model": "coder-mantle/astra",
     "small_model": "coder-mantle/astra"
   }
   ```

   Replace `coder.example.com` with your AI Gateway host and `bedrock-provider-name` with the provider name configured in Coder.
   Keep the `/v1` suffix.
   Replace `openai.gpt-6-astra` with the exact Mantle model ID available in your AWS account and region.
   Set `npm` to the SDK package for that model's API.
   Set `tool_call` to `true` only if the model supports tools.

   The `astra` key is a local model alias in OpenCode.
   The `id` field is the model ID sent upstream.
   Preserve any vendor prefix in the model ID, such as `openai.` or `anthropic.`.
   You can add more entries under `models` to use model IDs outside OpenCode's built-in catalog.

   The optional `model` and `small_model` fields select the default model and the model for lightweight tasks, such as title generation.
   This example uses Astra for both, so both types of request use the same gateway provider.
   To use a different small model, add its entry under `models` and update `small_model` to `coder-mantle/<model-alias>`.

3. Start OpenCode from the same shell:

   ```sh
   opencode
   ```

   OpenCode uses the configured Astra model through AI Gateway.

All three SDK packages use the same gateway base URL ending in `/v1`.
The selected SDK sends requests to `/v1/responses`, `/v1/chat/completions`, or `/v1/messages`.
Use a model that supports the selected API.
For more configuration options, refer to [OpenCode custom providers](https://opencode.ai/docs/providers/#custom-provider).

**References:** [OpenCode Documentation](https://opencode.ai/docs/providers/#config)
