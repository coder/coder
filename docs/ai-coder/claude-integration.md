# Integrating Claude with Coder

> [!NOTE]
>
> This functionality is in beta and is evolving rapidly.
>
> When using any AI tool for development, exercise a level of caution appropriate to your use case and environment.
> Always review AI-generated content before using it in critical systems.
>
> Join our [Discord channel](https://discord.gg/coder) or
> [contact us](https://coder.com/contact) to get help or share feedback.

## Overview

This guide shows you how to set up [Anthropic's Claude](https://www.anthropic.com/claude) in your Coder workspaces using Claude Code.
Claude Code is an AI-powered coding agent built on Claude that helps with development tasks, documentation, and more.

If you're new to AI coding agents in Coder, check out our [introduction to AI agents](./agents.md) first.

## Prerequisites

Before you begin, make sure you have:

- A Coder deployment with v2.21.0 or later (use the [quickstart guide](../tutorials/quickstart.md) to get started quickly)
- An [API key from Anthropic](https://console.anthropic.com/keys) or access through AWS Bedrock/GCP Vertex AI

## Quick setup: Claude Code template module

The easiest way to get started with Claude in Coder is to add the
[pre-built module](https://registry.coder.com/modules/claude-code) to a template.

1. Create a new Coder template or modify an existing one
1. Add the Claude Code module to your template's `main.tf` file:

   ```hcl
   # Add Claude Code module for integration
   module "claude-code" {
     source  = "registry.coder.com/modules/claude-code/coder"
     version = "1.2.1"

     # Required - connects to your workspace agent
     agent_id = coder_agent.main.id
     
     # Enable dashboard integration
     experiment_use_screen   = true
     experiment_report_tasks = true
   }
   
   # Set environment variables for Claude configuration
   resource "coder_env" "claude_api_key" {
     agent_id = coder_agent.main.id
     name     = "CLAUDE_API_KEY"
     value    = var.anthropic_api_key
   }
   ```

1. After that section, add a template parameter for your API key:

   ```hcl
   variable "anthropic_api_key" {
     type        = string
     description = "Anthropic API key for Claude Code"
     sensitive   = true  # This hides the value in logs and UI
     default     = ""
   }
   ```

1. With the `coder_env` resource above, your API key is already properly configured. No additional code is needed.

1. Push your template:

   ```bash
   coder templates push my-claude-template
   ```

1. Create a workspace with this template and provide your API key when prompted.

## Authentication options

<div class="tabs">

Claude Code supports multiple authentication methods:

## Anthropic API key

We recommend this method for getting started.

Get your API key from the [Anthropic Console](https://console.anthropic.com/keys) and use it as shown in the quick setup above.

## AWS Bedrock

For enterprise users.

If you're using Claude through AWS Bedrock:

```hcl
module "claude-code" {
  # ... other settings
  use_aws_bedrock    = true
  aws_region         = "us-east-1"  # Your AWS region
  aws_access_key_id  = var.aws_access_key_id
  aws_secret_key     = var.aws_secret_key
}
```

## Google Vertex AI

For enterprise users.

If you're using Claude through Google Vertex AI:

```hcl
module "claude-code" {
  # ... other settings
  use_vertex_ai       = true
  gcp_project_id      = var.gcp_project_id
  gcp_region          = "us-central1"  # Your GCP region
  gcp_service_account = var.gcp_service_account
}
```

</div>

## Customize your Claude setup

You can customize Claude's behavior with additional environment variables:

```hcl
# Set environment variables to customize Claude behavior

# Authentication
resource "coder_env" "claude_api_key" {
  agent_id = coder_agent.main.id
  name     = "CLAUDE_API_KEY"
  value    = var.anthropic_api_key
}

# Choose a specific Claude model (use Sonnet for best performance)
resource "coder_env" "claude_model" {
  agent_id = coder_agent.main.id
  name     = "CLAUDE_MODEL"
  value    = "claude-3-sonnet-20240229"
}

# Custom instructions
resource "coder_env" "claude_system_prompt" {
  agent_id = coder_agent.main.id
  name     = "CODER_MCP_CLAUDE_SYSTEM_PROMPT"
  value    = "You are a Python expert focused on writing clean, efficient code."
}

# Add special capabilities through MCP
resource "coder_env" "claude_mcp_instructions" {
  agent_id = coder_agent.main.id
  name     = "CODER_MCP_INSTRUCTIONS"
  value    = "Use playwright-mcp and desktop-commander tools when available"
}

# Claude Code module with dashboard integration
module "claude-code" {
  # ... basic settings from above
  source = "registry.coder.com/modules/claude-code/coder"
  agent_id = coder_agent.main.id
  experiment_use_screen = true
  experiment_report_tasks = true
}
```

For the full list of configuration options, consult the [module documentation](https://registry.coder.com/modules/claude-code).
For advanced configuration using environment variables, use the table in [Environment Variables Reference](#environment-variables-reference).

## Using Claude in your workspace

Once you've created a workspace with the Claude module, you can start using it right away!

After you connect to your workspace (via SSH, VS Code, or the web terminal), you can run your first Claude command in
the terminal:

```bash
claude "Hello! What can you help me with today?"
```

Claude responds in your terminal.
You'll also notice that this task appears in the Coder dashboard under your workspace.

### Everyday coding tasks

Claude is most helpful for these common tasks:

Generating code:

- Write a simple function:

  ```bash
  claude "Write a function to sort an array in JavaScript"
  ```

- Create a complete component:

  ```bash
  claude "Create a React component that displays a list of user profiles"
  ```

Working with files:

- Ask Claude to analyze a specific file:

  ```bash
  claude "Explain what this code does" app.js
  ```

Improve existing code:

```bash
claude "Add error handling to this function" user-service.js
```

Understanding your codebase:

1. Navigate to your repository:

   ```bash
   cd /path/to/repo
   ```

1. Get a high-level overview:

   ```bash
   claude "Help me understand this codebase"
   ```

1. Ask about specific parts:

   ```bash
   claude "Explain how authentication works in this app"
   ```

### Advanced workflows

As you get comfortable with Claude, try having it work directly with GitHub issues:

1. Make sure you have the GitHub CLI:

   ```bash
   which gh || sudo apt-get update && sudo apt-get install -y gh
   ```

1. Authenticate (if using external auth with Coder):

   ```bash
   eval "$(coder external-auth url-application github)"
   ```

1. Work on an issue directly:

   ```bash
   ISSUE_DESCRIPTION=$(gh issue view 123 --json body -q .body)
   claude "Implement this feature: $ISSUE_DESCRIPTION"
   ```

See our [issue tracker integration guide](./issue-tracker.md) for more workflows.

## Using Claude in VS Code

<div class="tabs">

To use Claude directly in your IDE, add the Claude VS Code extension to your workspace:

### VS Code

If you use VS Code on your local machine with the [Remote SSH extension](../user-guides/workspace-access/vscode.md):

1. Install the [Claude extension](https://marketplace.visualstudio.com/items?itemName=anthropic.claude-vscode) in your local VS Code.
1. Configure it with your Anthropic API key.
1. Connect to your Coder workspace via Remote SSH.
1. Use Claude directly within VS Code while working with your remote files.

### code-server

1. Add this to the script section in your template:

   ```bash
   CLAUDE_VSIX_URL="https://open-vsx.org/api/anthropic/claude-vscode/latest/file/anthropic.claude-vscode-latest.vsix"
   curl -L $CLAUDE_VSIX_URL -o /tmp/claude-vscode.vsix
   code-server --install-extension /tmp/claude-vscode.vsix
   ```

1. Create a settings file with your API key:

   ```bash
   mkdir -p ~/.config/Code/User/ && \
   cat > ~/.config/Code/User/settings.json <<EOF
   {
     "claude.apiKey": "${var.anthropic_api_key}",
     "claude.apiProvider": "anthropic"
   }
   EOF
   ```

1. After your workspace starts, open code-server and:
   - Look for the Claude icon in the sidebar
   - Click it to open the Claude panel
   - Start chatting with Claude about your code

</div>

## Using Claude Desktop with Coder (Advanced)

For power users, you can connect [Claude Desktop](https://claude.ai/download) to your Coder workspace:

1. Install Claude Desktop on your local machine
1. Use Coder to configure MCP integration:

   ```bash
   # Run this on your local machine (not in the workspace)
   coder exp mcp configure claude-desktop
   ```

1. In Claude Desktop, you can now:
   - Connect to your Coder workspaces
   - View your workspace files
   - Run commands in your workspace
   - Monitor agent activities

Learn more about [MCP integration](./best-practices.md#adding-tools-via-mcp) in our best practices guide.

## Troubleshooting

Having issues with Claude in your workspace? Here are some common solutions:

### Authentication issues

If Claude reports authentication errors:

1. Double-check your API key is correct
1. Verify the API key has been passed to your workspace
1. Try running `claude config show` to see your current configuration

### Claude seems slow or crashes

If Claude is running out of memory or seems sluggish:

1. Increase your workspace's memory allocation
1. For large requests, try breaking them into smaller, more focused prompts
1. If using VS Code extension, try the CLI version instead for better performance

### Enable debug logging

1. Check your Claude version:

   ```bash
   claude --version
   ```

1. Enable debug logging:

   ```bash
   export CLAUDE_CODE_DEBUG=1
   claude "Test prompt"
   ```

## Security best practices

When using Claude with Coder, keep these security tips in mind:

- Always store API keys as [sensitive template variables](../admin/templates/extending-templates/parameters.md#sensitive-parameters).
- Use [RBAC](../admin/users/groups-roles.md) to control which users can access AI features.
- Regularly review Claude's activity in your Coder dashboard.
- Consider [workspace boundaries](./securing.md) to limit what Claude can access.

## Environment Variables Reference

The following environment variables can be used to configure and fine-tune Claude's behavior in your Coder workspace.
These are particularly useful for troubleshooting and advanced use cases.

| Variable                         | Description                             | Default                    | Required | Example                      |
|----------------------------------|-----------------------------------------|----------------------------|----------|------------------------------|
| `CLAUDE_API_KEY`                 | Anthropic API key for authentication    | None                       | Yes      | `sk-ant-...`                 |
| `CLAUDE_MODEL`                   | Claude model to use                     | claude-3-5-sonnet-20240620 | No       | claude-3-7-sonnet-20240229   |
| `CLAUDE_CODE_DEBUG`              | Enable verbose debug logging            | 0                          | No       | 1                            |
| `CLAUDE_TIMEOUT_SECONDS`         | Maximum time for a request (seconds)    | 300                        | No       | 600                          |
| `CODER_AGENT_TOKEN`              | Token for Coder Agent authentication    | None                       | No       | `coder...`                   |
| `CODER_AGENT_TOKEN_FILE`         | Path to file containing the agent token | None                       | No       | `/path/to/token`             |
| `CODER_MCP_APP_STATUS_SLUG`      | Identifier for status reporting         | None                       | No       | claude                       |
| `CODER_MCP_CLAUDE_SYSTEM_PROMPT` | Override system prompt                  | Default prompt             | No       | "You are a Python expert..." |
| `CODER_MCP_CLAUDE_CODER_PROMPT`  | Override coder prompt                   | Default prompt             | No       | "You are a Go specialist..." |
| `CODER_MCP_INSTRUCTIONS`         | Custom instructions for MCP server      | None                       | No       | "Only use approved tools"    |

### Example usage

```bash
# Basic configuration
export CLAUDE_API_KEY="sk-ant-your-api-key"
export CLAUDE_MODEL="claude-3-7-sonnet-20240229"

# Performance tuning
export CLAUDE_TIMEOUT_SECONDS=600

# Advanced debug options
export CLAUDE_CODE_DEBUG=1

# Run Claude with custom settings
claude "Write a unit test for this function"
```

## What's next

- [Connect Claude to your issue tracker](./issue-tracker.md) to help with tickets
- [Try other AI coding agents](./agents.md) in Coder
- [Add AI tools via MCP](./best-practices.md) to enhance Claude's capabilities
- Learn about [running AI agents without an IDE](./headless.md)
