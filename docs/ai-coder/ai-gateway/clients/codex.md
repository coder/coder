# Codex CLI

> [!NOTE]
> AI Gateway requires the [AI Governance Add-On](../../ai-governance.md).
> As of Coder v2.32, deployments without the add-on will not be able to
> access AI Gateway.

Codex CLI can be configured to use AI Gateway by setting up a custom model provider.

## Centralized API Key

To configure Codex CLI to use AI Gateway, set the following configuration options in your Codex configuration file (e.g., `~/.codex/config.toml`):

```toml
model_provider = "ai_gateway"

[model_providers.ai_gateway]
name = "AI Gateway"
base_url = "<your-deployment-url>/api/v2/ai-gateway/openai/v1"
env_key = "OPENAI_API_KEY"
wire_api = "responses"
```

To authenticate with AI Gateway, get your **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** and set it in your environment:

```sh
export OPENAI_API_KEY="<your-coder-api-token>"
```

Run Codex as usual. It will automatically use the `ai_gateway` model provider from your configuration.

## BYOK (Personal API Key)

Add the following to your Codex configuration file (e.g., `~/.codex/config.toml`):

```toml
model_provider = "ai_gateway"

[model_providers.ai_gateway]
name = "AI Gateway"
base_url = "<your-deployment-url>/api/v2/ai-gateway/openai/v1"
wire_api = "responses"
requires_openai_auth = true
env_http_headers = { "X-Coder-AI-Governance-Token" = "CODER_API_TOKEN" }
```

Set both environment variables:

```sh
# Your personal OpenAI API key, forwarded to OpenAI.
export OPENAI_API_KEY="<your-openai-api-key>"

# Your Coder API token, used for authentication with AI Gateway.
export CODER_API_TOKEN="<your-coder-api-token>"
```

## BYOK (ChatGPT Subscription)

> [!IMPORTANT]
> This flow requires a [ChatGPT provider](../providers.md#chatgpt) on
> the deployment. Without it, Codex requests fail with
> `404 route not supported: POST /chatgpt/v1/responses`.

Add the following to your Codex configuration file (e.g., `~/.codex/config.toml`):

```toml
model_provider = "ai_gateway"

[model_providers.ai_gateway]
name = "AI Gateway"
base_url = "<your-deployment-url>/api/v2/ai-gateway/chatgpt/v1"
wire_api = "responses"
requires_openai_auth = true
env_http_headers = { "X-Coder-AI-Governance-Token" = "CODER_API_TOKEN" }
```

> [!NOTE]
> The `base_url` uses `/ai-gateway/chatgpt/v1` instead of `/ai-gateway/openai/v1` to route requests through the ChatGPT provider.

Set your Coder API token and ensure `OPENAI_API_KEY` is not set:

```sh
# Your Coder API token, used for authentication with AI Gateway.
export CODER_API_TOKEN="<your-coder-api-token>"

# Ensure no OpenAI API key is set so Codex uses ChatGPT login instead.
unset OPENAI_API_KEY
```

When you run Codex, it will prompt you to log in with your ChatGPT account.

## Pre-configuring in Templates

If configuring within a Coder workspace, you can use the
[Codex CLI](https://registry.coder.com/modules/coder-labs/codex) module.

For the centralized API key flow, set `enable_ai_gateway`:

```tf
module "codex" {
  source            = "registry.coder.com/coder-labs/codex/coder"
  version           = "~> 5.0"
  agent_id          = coder_agent.main.id
  workdir           = "/path/to/project"  # Set to your project directory
  enable_ai_gateway = true
}
```

For the ChatGPT subscription flow, pass the provider configuration
through `base_config_toml` and inject the Coder API token with a
`coder_env` resource. Users authenticate by running `codex login` with
their ChatGPT account:

```tf
resource "coder_env" "coder_api_token" {
  agent_id = coder_agent.main.id
  name     = "CODER_API_TOKEN"
  value    = data.coder_workspace_owner.me.session_token
}

module "codex" {
  source   = "registry.coder.com/coder-labs/codex/coder"
  version  = "~> 5.0"
  agent_id = coder_agent.main.id
  workdir  = "/path/to/project" # Set to your project directory

  base_config_toml = <<-TOML
    model_provider = "ai_gateway"

    [model_providers.ai_gateway]
    name = "AI Gateway"
    base_url = "${data.coder_workspace.me.access_url}/api/v2/ai-gateway/chatgpt/v1"
    wire_api = "responses"
    requires_openai_auth = true
    env_http_headers = { "X-Coder-AI-Governance-Token" = "CODER_API_TOKEN" }
  TOML
}
```

Do not set `OPENAI_API_KEY` in the workspace when using the ChatGPT
subscription flow, or Codex authenticates with the API key instead of
the ChatGPT login.

## Troubleshooting

### Codex falls back from WebSockets to HTTPS transport

Recent Codex CLI versions default to the WebSocket runtime for the
Responses API. AI Gateway does not support WebSocket transport, so each
request attempts a WebSocket connection and retries up to 5 times before
falling back to HTTPS. When this happens you will see:

```txt
Falling back from WebSockets to HTTPS transport.
```

The requests still succeed over HTTPS, but every turn waits through the
five failed WebSocket attempts first.

To stop Codex from attempting WebSockets, set `supports_websockets = false`
in your AI Gateway provider block in `~/.codex/config.toml`:

```toml
model_provider = "ai_gateway"

[model_providers.ai_gateway]
name = "AI Gateway"
base_url = "<your-deployment-url>/api/v2/ai-gateway/openai/v1"
wire_api = "responses"
supports_websockets = false
```

This forces the HTTPS transport directly and removes the fallback delay.

**References:** [Codex CLI Configuration](https://developers.openai.com/codex/config-reference)
