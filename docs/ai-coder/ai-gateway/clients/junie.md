# Junie

> [!NOTE]
> AI Gateway is part of [AI Governance](../../ai-governance.md), which is
> included with a Premium license.

[Junie CLI](https://junie.jetbrains.com/docs/junie-cli.html) supports AI Gateway
through [custom model profiles](https://www.jetbrains.com/help/junie/custom-llm-models.html).

| Provider  | API type          | Endpoint                 |
|-----------|-------------------|--------------------------|
| OpenAI    | `OpenAIResponses` | `/openai/v1/responses`   |
| Anthropic | `Anthropic`       | `/anthropic/v1/messages` |

For Junie inside JetBrains IDEs, refer to [JetBrains IDEs](./jetbrains.md).

## Prerequisites

* **Junie CLI**: Installed with `curl -fsSL https://junie.jetbrains.com/install.sh | bash`.
* **Authentication**: Your **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.

## Centralized API Key

Junie discovers custom model profiles from JSON files in two locations:

| Scope   | Location                 |
|---------|--------------------------|
| User    | `~/.junie/models/*.json` |
| Project | `.junie/models/*.json`   |

The file name without the `.json` extension is the profile ID.

Create `~/.junie/models/ai-gateway.json` for the OpenAI endpoint:

```json
{
  "id": "gpt-5.5",
  "baseUrl": "https://coder.example.com/api/v2/ai-gateway/openai/v1/responses",
  "apiType": "OpenAIResponses",
  "apiKey": "${CODER_API_TOKEN}",
  "fasterModel": { "id": "gpt-5.4-mini" }
}
```

Or for the Anthropic endpoint:

```json
{
  "id": "claude-sonnet-5",
  "baseUrl": "https://coder.example.com/api/v2/ai-gateway/anthropic/v1/messages",
  "apiType": "Anthropic",
  "apiKey": "${CODER_API_TOKEN}",
  "fasterModel": { "id": "claude-haiku-4-5-20251001" }
}
```

*Replace `coder.example.com` with your Coder deployment URL.*

> [!NOTE]
> `baseUrl` is the complete endpoint URL. Junie does not append a path to it, so
> include `/v1/responses` for `OpenAIResponses` and `/v1/messages` for
> `Anthropic`.

Junie resolves `${VAR}` references in `apiKey` and `extraHeaders` from the
environment, so the token never has to be written to disk:

```sh
export CODER_API_TOKEN=$(coder login token)
```

Run Junie with the profile:

```sh
junie --model custom:ai-gateway
```

Custom profiles also appear in the model selection list of the interactive TUI
and in the `/model` slash command.

## BYOK (Personal API Key)

Junie supports custom headers, so BYOK works by sending your personal provider
key as the API key and the Coder API token as the governance token:

```json
{
  "id": "gpt-5.5",
  "baseUrl": "https://coder.example.com/api/v2/ai-gateway/openai/v1/responses",
  "apiType": "OpenAIResponses",
  "apiKey": "${OPENAI_API_KEY}",
  "extraHeaders": { "X-Coder-AI-Governance-Token": "${CODER_API_TOKEN}" }
}
```

Set both environment variables:

```sh
# Your personal provider API key, forwarded to the upstream provider.
export OPENAI_API_KEY="<your-openai-api-key>"

# Your Coder API token, used for authentication with AI Gateway.
export CODER_API_TOKEN="<your-coder-api-token>"
```

## Pre-configuring in Templates

Commit the profile to your repository at `.junie/models/ai-gateway.json` and
inject the token from the template, so users get a working configuration without
manual setup:

```tf
data "coder_workspace_owner" "me" {}

data "coder_workspace" "me" {}

resource "coder_env" "coder_api_token" {
  agent_id = coder_agent.main.id
  name     = "CODER_API_TOKEN"
  value    = data.coder_workspace_owner.me.session_token
}
```

Reference the deployment URL in the profile with
`${data.coder_workspace.me.access_url}/api/v2/ai-gateway/openai/v1/responses`
if you generate the file from the template instead of committing it.

**References:** [Junie custom LLMs](https://www.jetbrains.com/help/junie/custom-llm-models.html)
