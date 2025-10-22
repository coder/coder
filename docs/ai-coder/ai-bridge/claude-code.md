# Claude Code with AI Bridge

[Claude Code](https://github.com/anthropics/claude-code) is Anthropic's official AI coding assistant, available as both a CLI tool and VS Code extension.

## Support Status

| Variant                           | AI Bridge Support | Notes                              |
|-----------------------------------|-------------------|------------------------------------|
| **Claude Code CLI**               | ✅ Fully Supported | Can be pre-configured in templates |
| **Claude Code VS Code Extension** | ✅ Supported       | Requires initial login flow        |

## Claude Code CLI

### Prerequisites

- Claude Code CLI installed
- Coder session token

### Installation

```sh
# Install Claude Code CLI
npm install -g @anthropic-ai/claude-code
```

### Configuration

Set environment variables:

```sh
export ANTHROPIC_BASE_URL="https://coder.example.com/api/experimental/aibridge/anthropic"
export ANTHROPIC_API_KEY="your-coder-session-token"
```

### Usage

```sh
# Run Claude Code with a prompt
claude-code "Add authentication to the user service"

# Interactive mode
claude-code
```

### Template Configuration

Pre-configure Claude Code in your Coder template:

```hcl
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    ANTHROPIC_BASE_URL = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/anthropic"
    ANTHROPIC_API_KEY  = data.coder_workspace_owner.me.session_token
  }
}
```

### For Coder Tasks

Claude Code works great with [Coder Tasks](../tasks.md):

```hcl
module "claude-code" {
  source    = "registry.coder.com/coder/claude-code/coder"
  version   = "3.1.0"
  agent_id  = coder_agent.main.id
  workdir   = "/home/coder/project"
  ai_prompt = data.coder_parameter.ai_prompt.value

  # Use AI Bridge instead of direct Anthropic API
  claude_api_key = data.coder_workspace_owner.me.session_token
}

resource "coder_env" "bridge_base_url" {
  agent_id = coder_agent.main.id
  name     = "ANTHROPIC_BASE_URL"
  value    = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/anthropic"
}
```

## Claude Code VS Code Extension

### Prerequisites

- Visual Studio Code
- Claude Code extension
- Coder session token

### Installation

1. Install from [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=Anthropic.claude-code-vscode)
1. Complete the initial authentication flow

### Configuration

After the initial login:

1. Set environment variables in your workspace:

```sh
export ANTHROPIC_BASE_URL="https://coder.example.com/api/experimental/aibridge/anthropic"
export ANTHROPIC_API_KEY="your-coder-session-token"
```

1. Restart VS Code for changes to take effect

### Template Configuration

```hcl
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    ANTHROPIC_BASE_URL = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/anthropic"
    ANTHROPIC_API_KEY  = data.coder_workspace_owner.me.session_token
  }
}
```

### Authentication Flow

The VS Code extension requires an initial login before respecting environment variables:

1. First launch: Complete Anthropic authentication
2. Subsequent sessions: Environment variables override the default API endpoint

## Troubleshooting

### CLI: Command Not Found

```sh
# Verify Claude Code is installed
which claude-code

# Install if missing
npm install -g @anthropic-ai/claude-code
```

### CLI: Authentication Errors

```sh
# Verify environment variables
echo $ANTHROPIC_BASE_URL
echo $ANTHROPIC_API_KEY

# Generate fresh token
coder tokens create
```

### VS Code: Extension Not Using AI Bridge

1. Ensure you completed the initial login flow
2. Verify environment variables are set
3. Restart VS Code completely
4. Check the extension output for errors

### Template: Module Not Found

Ensure you're using the correct module source:

```hcl
module "claude-code" {
  source  = "registry.coder.com/coder/claude-code/coder"
  version = "3.1.0"
  # ...
}
```

## Known Limitations

- VS Code extension requires initial authentication flow
- Cannot skip the login step on first use
- Environment variables only apply after authentication

## Related Documentation

- [AI Bridge Setup](./index.md#setup)
- [Coder Tasks](../tasks.md)
- [Template Configuration](./index.md#pre-configuring-in-coder-templates)
- [MCP Server Integration](./index.md#mcp)
