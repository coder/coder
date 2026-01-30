# Claude Code

[Claude Code](https://code.claude.com/docs/en/overview) is fully supported by AI Bridge and works out of the box with Anthropic endpoints.

## Configuration

Claude Code can be configured using environment variables.

* **Base URL**: `ANTHROPIC_BASE_URL` should point to `https://coder.example.com/api/v2/aibridge/anthropic`
* **API Key**: `ANTHROPIC_API_KEY` should be your [Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself).

### Pre-configuring in Templates

Template admins can pre-configure Claude Code for a seamless experience. Admins can automatically inject the user's Coder session token and the AI Bridge base URL into the workspace environment.

```hcl
data "coder_workspace_owner" "me" {}

data "coder_workspace" "me" {}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir
  env = {
    ANTHROPIC_BASE_URL : "${data.coder_workspace.me.access_url}/api/v2/aibridge/anthropic",
    ANTHROPIC_AUTH_TOKEN : data.coder_workspace_owner.me.session_token
  }
  ... # other agent configuration
}
```

### Coder Tasks

Agents like Claude Code can be configured to route through AI Bridge in any template by pre-configuring the agent with the Coder session token. [Coder Tasks](../../tasks.md) is particularly useful for this pattern, providing a framework for agents to complete background development operations autonomously.

```hcl
data "coder_workspace_owner" "me" {}

data "coder_workspace" "me" {}

data "coder_task" "me" {}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir
  env = {
    ANTHROPIC_BASE_URL : "${data.coder_workspace.me.access_url}/api/v2/aibridge/anthropic",
    ANTHROPIC_AUTH_TOKEN : data.coder_workspace_owner.me.session_token
  }
  ... # other agent configuration
}

# See https://registry.coder.com/modules/coder/claude-code for more information
module "claude-code" {
  count               = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = ">= 4.0.0"
  agent_id            = coder_agent.dev.id
  workdir             = "/home/coder/project"
  claude_api_key      = data.coder_workspace_owner.me.session_token # Use the Coder session token to authenticate with AI Bridge
  ai_prompt           = data.coder_task.me.prompt
  ... # other claude-code configuration
}

# The coder_ai_task resource associates the task to the app.
resource "coder_ai_task" "task" {
  count  = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  app_id = module.claude-code[0].task_app_id
}
```

## VS Code Extension

The Claude Code VS Code extension is also supported.

1. If pre-configured in the workspace environment variables (as shown above), it typically respects them.
2. You may need to sign in once; afterwards, it respects the workspace environment variables.

**References:** [Claude Code Settings](https://docs.claude.com/en/docs/claude-code/settings#environment-variables)
