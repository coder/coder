# Codex CLI

The OpenAI Codex CLI is supported with specific version requirements.

## Configuration

To configure Codex CLI to use AI Bridge, set the following configuration options in your Codex configuration file (e.g., `~/.codex/config.yaml`):

```yaml
[model_providers.aibridge]
name = "AI Bridge"
base_url = "${data.coder_workspace.me.access_url}/api/v2/aibridge/openai/v1"
env_key = "CODER_AIBRIDGE_SESSION_TOKEN"
wire_api = "responses"

[profiles.aibridge]
model_provider = "aibridge"
model = "${var.codex_model}"
model_reasoning_effort = "${var.model_reasoning_effort}"
```

If configuring within a Coder workspace, you can also use the [Codex CLI](https://registry.coder.com/modules/coder-labs/codex) module and set the following variables:

```tf
module "codex" {
  source          = "registry.coder.com/coder-labs/codex/coder"
  version         = "~> 4.1"
  agent_id        = coder_agent.main.id
  workdir         = "/home/coder/project"
  enable_aibridge = true
}
```
**References:** [Codex CLI Configuration](https://developers.openai.com/codex/config-advanced)
