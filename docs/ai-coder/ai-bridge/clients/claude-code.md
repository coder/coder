# Claude Code

## Configuration

Claude Code can be configured using environment variables.

* **Base URL**: `ANTHROPIC_BASE_URL` should point to `https://coder.example.com/api/v2/aibridge/anthropic`
* **API Key**: `ANTHROPIC_API_KEY` should be your [Coder session token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself).

### Pre-configuring in Templates

Template admins can pre-configure Claude Code for a seamless experience. Admins can automatically inject the user's Coder session token and the AI Bridge base URL into the workspace environment.

```hcl
module "claude-code" {
  source          = "registry.coder.com/coder/claude-code/coder"
  version         = "4.7.3"
  agent_id        = coder_agent.main.id
  workdir         = "/path/to/project"  # Set to your project directory
  enable_aibridge = true
}
```

### Coder Tasks

[Coder Tasks](../../tasks.md) provides a framework for agents to complete background development operations autonomously. Claude Code can be configured in your Tasks automatically:

```hcl
resource "coder_ai_task" "task" {
  count  = data.coder_workspace.me.start_count
  app_id = module.claude-code.task_app_id
}

data "coder_task" "me" {}

module "claude-code" {
  source         = "registry.coder.com/coder/claude-code/coder"
  version        = "4.7.3"
  agent_id       = coder_agent.main.id
  workdir        = "/path/to/project"  # Set to your project directory
  ai_prompt      = data.coder_task.me.prompt

  # Route through AI Bridge (Premium feature)
  enable_aibridge = true
}
```

## VS Code Extension

The Claude Code VS Code extension is also supported.

1. If pre-configured in the workspace environment variables (as shown above), it typically respects them.
2. You may need to sign in once; afterwards, it respects the workspace environment variables.

**References:** [Claude Code Settings](https://docs.claude.com/en/docs/claude-code/settings#environment-variables)
