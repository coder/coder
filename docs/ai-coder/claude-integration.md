# Detailed Claude Integration with Coder

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

This guide shows you how to integrate Anthropic's Claude with Coder. Claude is [recommended as our primary AI coding agent](./agents.md) due to its strong performance on complex programming tasks.

## Prerequisites

Before you begin, make sure you have:

- A Coder deployment with v2.21.0 or later
- A [template configured for AI agents](./create-template.md)
- Access to Anthropic API through one of these methods:
  - Direct Anthropic API key
  - AWS Bedrock (enterprise)
  - GCP Vertex AI (enterprise)

## Authentication configuration

Claude Code supports multiple authentication methods. Choose the option that works best for your infrastructure.

### Direct Anthropic API key

1. Add a template variable for the API key:

```hcl
variable "anthropic_api_key" {
  type        = string
  description = "Anthropic API key for Claude Code"
  sensitive   = true
  default     = ""
}
```

2. Pass it to the module:

```hcl
module "claude-code" {
  # ... other settings from the registry template
  anthropic_api_key = var.anthropic_api_key
}
```

### AWS Bedrock

For enterprise customers using AWS Bedrock:

```hcl
module "claude-code" {
  # ... other settings
  use_aws_bedrock    = true
  aws_region         = "us-east-1"  # Your AWS region
  aws_access_key_id  = var.aws_access_key_id
  aws_secret_key     = var.aws_secret_key
}
```

### Google Vertex AI

For enterprise customers using Google Vertex AI:

```hcl
module "claude-code" {
  # ... other settings
  use_vertex_ai       = true
  gcp_project_id      = var.gcp_project_id
  gcp_region          = "us-central1"  # Your GCP region
  gcp_service_account = var.gcp_service_account
}
```

## Advanced Claude Code configuration

Beyond basic setup, Claude Code supports additional configuration options:

```hcl
module "claude-code" {
  source  = "registry.coder.com/modules/claude-code/coder"
  version = "1.0.0"
  
  agent                    = var.agent
  experiment_use_screen    = true    # Required for Coder dashboard integration
  experiment_report_tasks  = true    # Required for Coder dashboard integration
  model                    = "claude-3-7-sonnet-20240229"  # Recommended model
  
  # Optional parameters
  custom_system_prompt     = "You are a helpful assistant focused on Python development."
  additional_tools         = ["playwright-mcp", "desktop-commander"]
  enable_file_upload       = true
  timeout_seconds          = 300
}
```

## Using Claude Code in your workspace

After your workspace is running with the Claude Code module, you can use it in several ways.

### Basic usage

Run simple prompts or interact with files:

```bash
# Simple prompt
claude-code "Write a function to sort an array in JavaScript"

# Interact with a file
claude-code "Add proper error handling to this file" path/to/file.js
```

### Repository exploration

Explore and understand your codebase:

```bash
# Navigate to your repository
cd /path/to/repo

# Ask Claude to explore and understand the codebase
claude-code "Help me understand this codebase"

# Get specific help with a component
claude-code "Explain how the authentication system works in this app"
```

### Task automation

Automate common development tasks:

```bash
# Generate tests for a specific module
claude-code "Write comprehensive tests for this module" src/module.js

# Review and suggest improvements
claude-code "Review this PR and suggest improvements" $(gh pr view --json body -q .body)
```

### Using with issue trackers

Claude Code works with issue tracking systems. For GitHub issues:

```bash
# Install GitHub CLI if not already in your template
apt-get update && apt-get install -y gh

# Authenticate (if using Coder's external authentication)
eval "$(coder external-auth url-application github)"

# Get an issue description and ask Claude to implement it
ISSUE_DESCRIPTION=$(gh issue view 123 --json body -q .body)
claude-code "Implement this feature: $ISSUE_DESCRIPTION"
```

## Claude with VS Code integration

For interactive assistance in VS Code, use the Claude VS Code extension.

Add this to your template's startup script:

```bash
# Install Claude extension for VS Code
code-server --install-extension anthropic.claude-vscode

# Configure Claude VS Code extension
mkdir -p ~/.config/Code/User/
cat > ~/.config/Code/User/settings.json <<EOF
{
  "claude.apiKey": "${var.anthropic_api_key}",
  "claude.apiProvider": "anthropic" // or "bedrock" or "vertexai"
}
EOF
```

## Advanced usage with MCP

[MCP (Model Context Protocol)](./best-practices.md#adding-tools-via-mcp) lets Claude access additional tools and capabilities.

Configure Claude Desktop to work with your Coder workspace:

```bash
# On your local machine
coder exp mcp configure claude-desktop
```

In Claude Desktop, you can then:
- Connect to your Coder workspace
- Run commands and access files
- View agent status and activity

## Troubleshooting

### Common issues

- **Authentication errors**: Check that your API keys are correctly configured and have proper permissions
- **Memory limitations**: If Claude Code is terminated due to OOM errors, increase workspace memory allocation
- **Tool failures**: Verify that required dependencies for MCP tools are installed in your workspace

### Debugging Claude Code

To troubleshoot Claude Code issues:

```bash
# Enable debug logging
export CLAUDE_CODE_DEBUG=1
claude-code "Your prompt"

# Check Claude Code version
claude-code --version

# Verify config
claude-code config show
```

## Security considerations

When using Claude with Coder:

- Store API keys as [sensitive template variables](../templates/parameters.md#sensitive-parameters)
- Consider using [RBAC](../admin/rbac.md) to control which users can create workspaces with AI capabilities
- Review [securing agents](./securing.md) guidelines for additional protection

## What's next

- [Integrate with your issue tracker](./issue-tracker.md)
- [Learn about MCP and adding AI tools](./best-practices.md)
- [Explore headless agent capabilities](./headless.md)