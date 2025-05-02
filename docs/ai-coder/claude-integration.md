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

This guide shows you how to set up [Anthropic's Claude](https://www.anthropic.com/claude) in your Coder workspaces. Claude is an AI assistant that can help you with coding tasks, documentation, and more.

If you're new to AI coding agents in Coder, check out our [introduction to AI agents](./agents.md) first.

## What you'll need

Before you begin, make sure you have:

- A Coder deployment with v2.21.0 or later (see [installation guide](../install/index.md) if needed)
- An [API key from Anthropic](https://console.anthropic.com/keys) or access through AWS Bedrock/GCP Vertex AI
- Basic familiarity with [Coder templates](../admin/templates/index.md) (or follow our [quick template guide](./create-template.md))

## Quick setup: Use our template module

The easiest way to get started with Claude in Coder is to use our [pre-built module from the registry](https://registry.coder.com/modules/claude-code).

1. Create a new Coder template or modify an existing one
2. Add the Claude Code module to your template's `main.tf` file:

```hcl
module "claude-code" {
  source  = "registry.coder.com/modules/claude-code/coder"
  version = "1.0.0"
  
  agent                   = var.agent  # This connects the module to your agent
  experiment_use_screen   = true       # Enable reporting to Coder dashboard
  experiment_report_tasks = true       # Show tasks in Coder UI
}
```

3. Add a template parameter for your API key:

```hcl
variable "anthropic_api_key" {
  type        = string
  description = "Anthropic API key for Claude Code"
  sensitive   = true  # This hides the value in logs and UI
  default     = ""
}
```

4. Pass the API key to the module:

```hcl
module "claude-code" {
  # ... existing settings from above
  anthropic_api_key = var.anthropic_api_key
}
```

5. Push your template:
   ```bash
   coder templates push my-claude-template
   ```

6. Create a workspace with this template, providing your API key when prompted.

## Authentication options

Claude Code supports multiple authentication methods:

### Option 1: Direct Anthropic API key (recommended for getting started)

Get your API key from the [Anthropic Console](https://console.anthropic.com/keys) and use it as shown in the quick setup above.

### Option 2: AWS Bedrock (for enterprise users)

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

### Option 3: Google Vertex AI (for enterprise users)

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

## Customizing your Claude setup

You can customize Claude's behavior with these additional options:

```hcl
module "claude-code" {
  # ... basic settings from above
  
  # Choose a specific Claude model
  model = "claude-3-7-sonnet-20240229"  # Most capable model
  # Or use a more economical option
  # model = "claude-3-5-sonnet-20240620"
  
  # Give Claude specific instructions
  custom_system_prompt = "You are a Python expert focused on writing clean, efficient code."
  
  # Add special capabilities through MCP
  additional_tools = ["playwright-mcp", "desktop-commander"]
  
  # Set resource limits
  timeout_seconds = 300  # Maximum time for a single request
}
```

For the full list of configuration options, see the [module documentation](https://registry.coder.com/modules/claude-code).

## Using Claude in your workspace

Once you've created a workspace with the Claude module, you can start using it right away!

### Getting started

After connecting to your workspace (via SSH, VS Code, or the web terminal), you can:

1. Run your first Claude command to test it:

   ```bash
   claude-code "Hello! What can you help me with today?"
   ```

2. You should see Claude respond in your terminal. You'll also notice that this task appears in the Coder dashboard under your workspace.

### Everyday coding tasks

Claude is most helpful for these common tasks:

#### Generating code

```bash
# Write a simple function
claude-code "Write a function to sort an array in JavaScript"

# Create a complete component
claude-code "Create a React component that displays a list of user profiles"
```

#### Working with files

```bash
# Ask Claude to analyze a specific file
claude-code "Explain what this code does" app.js

# Improve existing code
claude-code "Add error handling to this function" user-service.js
```

#### Understanding your codebase

```bash
# Navigate to your repository first
cd /path/to/repo

# Get a high-level overview
claude-code "Help me understand this codebase"

# Ask about specific parts
claude-code "Explain how authentication works in this app"
```

### Advanced workflows

As you get comfortable with Claude, try these more powerful workflows:

#### Working with GitHub issues

If your template includes GitHub integration:

```bash
# Make sure you have the GitHub CLI
which gh || sudo apt-get update && sudo apt-get install -y gh

# Authenticate (if using external auth with Coder)
eval "$(coder external-auth url-application github)"

# Work on an issue directly
ISSUE_DESCRIPTION=$(gh issue view 123 --json body -q .body)
claude-code "Implement this feature: $ISSUE_DESCRIPTION"
```

See our [issue tracker integration guide](./issue-tracker.md) for more workflows.

## Using Claude in VS Code

Want to use Claude directly in your IDE? You can add the Claude VS Code extension to your workspace:

### Adding Claude to code-server (browser-based VS Code)

1. Add this to your template's startup script:

```bash
# Add this to the script section of your template
CLAUDE_VSIX_URL="https://open-vsx.org/api/anthropic/claude-vscode/latest/file/anthropic.claude-vscode-latest.vsix"
curl -L $CLAUDE_VSIX_URL -o /tmp/claude-vscode.vsix
code-server --install-extension /tmp/claude-vscode.vsix

# Create a settings file with your API key
mkdir -p ~/.config/Code/User/
cat > ~/.config/Code/User/settings.json <<EOF
{
  "claude.apiKey": "${var.anthropic_api_key}",
  "claude.apiProvider": "anthropic" 
}
EOF
```

2. After your workspace starts, open code-server and:
   - Look for the Claude icon in the sidebar
   - Click it to open the Claude panel
   - Start chatting with Claude about your code

### Using Claude with VS Code Desktop

If you use VS Code on your local machine with the [Remote SSH extension](../user-guides/workspace-access/vscode.md):

1. Install the [Claude extension](https://marketplace.visualstudio.com/items?itemName=anthropic.claude-vscode) in your local VS Code
2. Configure it with your Anthropic API key
3. Connect to your Coder workspace via Remote SSH
4. Use Claude directly within VS Code while working with your remote files

## Using Claude Desktop with Coder (Advanced)

For power users, you can connect [Claude Desktop](https://claude.ai/download) to your Coder workspace:

1. Install Claude Desktop on your local machine
2. Use Coder to configure MCP integration:

   ```bash
   # Run this on your local machine (not in the workspace)
   coder exp mcp configure claude-desktop
   ```

3. In Claude Desktop, you can now:
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
2. Verify the API key has been passed to your workspace
3. Try running `claude-code config show` to see your current configuration

### Claude seems slow or crashes

If Claude is running out of memory or seems sluggish:

1. Increase your workspace's memory allocation
2. For large requests, try breaking them into smaller, more focused prompts
3. If using VS Code extension, try the CLI version instead for better performance

### Getting more help

Still having trouble?

```bash
# Check your Claude version
claude-code --version

# Enable debug logging for more information
export CLAUDE_CODE_DEBUG=1
claude-code "Test prompt"
```

For more detailed help, join our [Discord community](https://discord.gg/coder) or check the [Claude documentation](https://docs.anthropic.com/claude/docs).

## Security best practices

When using Claude with Coder, keep these security tips in mind:

- Always store API keys as [sensitive template variables](../admin/templates/extending-templates/parameters.md#sensitive-parameters)
- Use [RBAC](../admin/users/groups-roles.md) to control which users can access AI features
- Regularly review Claude's activity in your Coder dashboard
- Consider [workspace boundaries](./securing.md) to limit what Claude can access

## What's next

- [Connect Claude to your issue tracker](./issue-tracker.md) to help with tickets
- [Try other AI coding agents](./agents.md) in Coder
- [Add AI tools via MCP](./best-practices.md) to enhance Claude's capabilities
- Learn about [running AI agents without an IDE](./headless.md)