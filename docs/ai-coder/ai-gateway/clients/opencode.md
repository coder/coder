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

You can use OpenCode with custom OpenAI model IDs through a [Bedrock Mantle provider](../providers.md#mantle) configured by your Coder administrator.
AI Gateway signs requests with AWS credentials centrally, so you only need a [Coder API token](../auth.md#authenticate-ai-clients) in OpenCode.
Use a custom OpenAI provider in OpenCode for this connection, rather than its native Amazon Bedrock integration.

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
       "coder-mantle-openai": {
         "npm": "@ai-sdk/openai",
         "name": "Coder Mantle OpenAI",
         "options": {
           "baseURL": "https://coder.example.com/api/v2/ai-gateway/bedrock-mantle-us-west-2/v1",
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
     "model": "coder-mantle-openai/astra",
     "small_model": "coder-mantle-openai/astra"
   }
   ```

   Replace `coder.example.com` with your AI Gateway host and `bedrock-mantle-us-west-2` with the provider name configured in Coder.
   Keep the `/v1` suffix.
   Replace `openai.gpt-6-astra` with the exact Mantle model ID available in your AWS account and region.

   The `astra` key is a local model alias in OpenCode.
   The `id` field is the model ID sent upstream, including the `openai.` prefix.
   You can add more entries under `models` to use model IDs outside OpenCode's built-in catalog.

   The optional `model` and `small_model` fields select the default model and the model for lightweight tasks, such as title generation.
   This example uses Astra for both, so both types of request use the same gateway provider.
   To use a different small model, add its entry under `models` and update `small_model` to `coder-mantle-openai/<model-alias>`.

3. Start OpenCode from the same shell:

   ```sh
   opencode
   ```

   OpenCode uses the configured Astra model through AI Gateway.

This example uses `@ai-sdk/openai` for the Responses API (`/v1/responses`).
For a model that uses Chat Completions (`/v1/chat/completions`), set `npm` to `@ai-sdk/openai-compatible` instead.
Keep the same gateway base URL and use a model that supports that API.
For more configuration options, refer to [OpenCode custom providers](https://opencode.ai/docs/providers/#custom-provider).

**References:** [OpenCode Documentation](https://opencode.ai/docs/providers/#config)
