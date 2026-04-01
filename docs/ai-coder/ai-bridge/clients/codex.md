# Codex CLI

Codex CLI can be configured to use AI Bridge by setting up a custom model provider.

## Configuration

To configure Codex CLI to use AI Bridge, set the following configuration options in your Codex configuration file (e.g., `~/.codex/config.toml`):

```toml
model_provider = "aibridge"

[model_providers.aibridge]
name = "AI Bridge"
base_url = "<your-deployment-url>/api/v2/aibridge/openai/v1"
env_key = "OPENAI_API_KEY"
wire_api = "responses"
```

Run Codex as usual. It will automatically use the `aibridge` model provider from your configuration:

If configuring within a Coder workspace, you can also use the [Codex CLI](https://registry.coder.com/modules/coder-labs/codex) module and set the following variables:

```tf
module "codex" {
  source          = "registry.coder.com/coder-labs/codex/coder"
  version         = "~> 4.1"
  agent_id        = coder_agent.main.id
  workdir         = "/path/to/project"  # Set to your project directory
  enable_aibridge = true
}
```

## Authentication

To authenticate with AI Bridge, get your **[Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** and set it in your environment:

```bash
export OPENAI_API_KEY="<your-coder-session-token>"
```

**References:** [Codex CLI Configuration](https://developers.openai.com/codex/config-advanced)
