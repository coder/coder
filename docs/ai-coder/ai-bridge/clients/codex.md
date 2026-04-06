# Codex CLI

Codex CLI can be configured to use AI Bridge by setting up a custom model provider.

## Centralized API Key

To configure Codex CLI to use AI Bridge, set the following configuration options in your Codex configuration file (e.g., `~/.codex/config.toml`):

```toml
model_provider = "aibridge"

[model_providers.aibridge]
name = "AI Bridge"
base_url = "<your-deployment-url>/api/v2/aibridge/openai/v1"
env_key = "OPENAI_API_KEY"
wire_api = "responses"
```

To authenticate with AI Bridge, get your **[Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** and set it in your environment:

```bash
export OPENAI_API_KEY="<your-coder-session-token>"
```

Run Codex as usual. It will automatically use the `aibridge` model provider from your configuration.

## BYOK (Personal API Key)

Add the following to your Codex configuration file (e.g., `~/.codex/config.toml`):

```toml
model_provider = "aibridge"

[model_providers.aibridge]
name = "AI Bridge"
base_url = "<your-deployment-url>/api/v2/aibridge/openai/v1"
wire_api = "responses"
requires_openai_auth = true
env_http_headers = { "X-Coder-AI-Governance-Token" = "CODER_SESSION_TOKEN" }
```

Set both environment variables:

```bash
# Your personal OpenAI API key, forwarded to OpenAI.
export OPENAI_API_KEY="<your-openai-api-key>"

# Your Coder session token, used for authentication with AI Bridge.
export CODER_SESSION_TOKEN="<your-coder-session-token>"
```

## BYOK (ChatGPT Subscription)

Add the following to your Codex configuration file (e.g., `~/.codex/config.toml`):

```toml
model_provider = "aibridge"

[model_providers.aibridge]
name = "AI Bridge"
base_url = "<your-deployment-url>/api/v2/aibridge/chatgpt/v1"
wire_api = "responses"
requires_openai_auth = true
env_http_headers = { "X-Coder-AI-Governance-Token" = "CODER_SESSION_TOKEN" }
```

> [!NOTE]
> The `base_url` uses `/aibridge/chatgpt/v1` instead of `/aibridge/openai/v1` to route requests through the ChatGPT provider.

Set your Coder session token and ensure `OPENAI_API_KEY` is not set:

```bash
# Your Coder session token, used for authentication with AI Bridge.
export CODER_SESSION_TOKEN="<your-coder-session-token>"

# Ensure no OpenAI API key is set so Codex uses ChatGPT login instead.
unset OPENAI_API_KEY
```

When you run Codex, it will prompt you to log in with your ChatGPT account.

## Pre-configuring in Templates

If configuring within a Coder workspace, you can use the
[Codex CLI](https://registry.coder.com/modules/coder-labs/codex) module:

```tf
module "codex" {
  source          = "registry.coder.com/coder-labs/codex/coder"
  version         = "~> 4.1"
  agent_id        = coder_agent.main.id
  workdir         = "/path/to/project"  # Set to your project directory
  enable_aibridge = true
}
```

**References:** [Codex CLI Configuration](https://developers.openai.com/codex/config-advanced)
