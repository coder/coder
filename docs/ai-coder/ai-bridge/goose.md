# Goose with AI Bridge

[Goose](https://github.com/block/goose) is an AI coding assistant developed by Block, available as both a desktop application and CLI tool.

## Support Status

- **OpenAI**: ⚠️ Partial Support
- **Anthropic**: ✅ Fully Supported

## Goose Desktop

### Prerequisites

- Goose Desktop application installed
- Coder session token

### Installation

Download Goose Desktop from the [official releases](https://github.com/block/goose/releases).

### Configuration

Set environment variables before launching Goose Desktop:

```sh
# Anthropic configuration (recommended)
export ANTHROPIC_BASE_URL="https://coder.example.com/api/experimental/aibridge/anthropic"
export ANTHROPIC_API_KEY="your-coder-session-token"

# OpenAI configuration (optional)
export OPENAI_BASE_URL="https://coder.example.com/api/experimental/aibridge/openai/v1"
export OPENAI_API_KEY="your-coder-session-token"

# Launch Goose Desktop
goose
```

### Template Configuration

Pre-configure in your Coder template:

```hcl
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    ANTHROPIC_BASE_URL = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/anthropic"
    ANTHROPIC_API_KEY  = data.coder_workspace_owner.me.session_token

    OPENAI_BASE_URL    = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/openai/v1"
    OPENAI_API_KEY     = data.coder_workspace_owner.me.session_token
  }
}
```

## Goose CLI

### Prerequisites

- Goose CLI installed
- Coder session token

### Installation

```sh
# Install via pip
pip install goose-ai

# Or via pipx (recommended for isolated installation)
pipx install goose-ai
```

### Configuration

Set environment variables:

```sh
export ANTHROPIC_BASE_URL="https://coder.example.com/api/experimental/aibridge/anthropic"
export ANTHROPIC_API_KEY="your-coder-session-token"
```

### Usage

```sh
# Run Goose CLI
goose run "refactor the authentication module"

# Interactive session
goose session
```

### Template Configuration

For Coder Tasks or agent workspaces:

```hcl
module "goose" {
  source   = "registry.coder.com/coder-labs/goose/coder"
  version  = "1.0.0"
  agent_id = coder_agent.main.id
  workdir  = "/home/coder/project"

  env = {
    ANTHROPIC_BASE_URL = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/anthropic"
    ANTHROPIC_API_KEY  = data.coder_workspace_owner.me.session_token
  }

  ai_prompt = data.coder_parameter.ai_prompt.value
}
```

## Common Configuration

### Model Selection

Goose supports multiple models. Configure which model to use:

```sh
# Use Claude (recommended with AI Bridge)
export GOOSE_MODEL="claude-3-5-sonnet-20241022"

# Or use OpenAI
export GOOSE_MODEL="gpt-4"
```

### Workspace Integration

Add Goose to your shell profile for persistent configuration:

```sh
# Add to ~/.bashrc or ~/.zshrc
export ANTHROPIC_BASE_URL="https://coder.example.com/api/experimental/aibridge/anthropic"
export ANTHROPIC_API_KEY="$(coder tokens create --lifetime 720h -q)"
export GOOSE_MODEL="claude-3-5-sonnet-20241022"
```

## Troubleshooting

### Goose Not Finding AI Bridge

1. Verify environment variables are set:

   ```sh
   echo $ANTHROPIC_BASE_URL
   echo $ANTHROPIC_API_KEY
   ```

2. Ensure Goose was launched from a shell with the variables set

3. For Desktop: Close completely and relaunch from terminal

### Authentication Errors

```sh
# Generate a fresh token
coder tokens create

# Test the token
curl -H "Coder-Session-Token: YOUR_TOKEN" \
  https://coder.example.com/api/v2/users/me
```

### Desktop App Not Using AI Bridge

The Goose Desktop application must be launched from a terminal where environment variables are set:

```sh
# Set variables
export ANTHROPIC_BASE_URL="https://coder.example.com/api/experimental/aibridge/anthropic"
export ANTHROPIC_API_KEY="your-token"

# Launch from this shell
open -a Goose  # macOS
goose          # Linux
```

### CLI: Module Not Found

```sh
# Verify installation
which goose

# Reinstall if needed
pip install --upgrade goose-ai
```

### Model Not Available

Ensure you're using a model supported by your configured provider:

```sh
# Anthropic models (via AI Bridge)
export GOOSE_MODEL="claude-3-5-sonnet-20241022"
export GOOSE_MODEL="claude-3-opus-20240229"

# OpenAI models (via AI Bridge)
export GOOSE_MODEL="gpt-4"
export GOOSE_MODEL="gpt-4-turbo"
```

## Known Limitations

### Desktop Application Environment

The desktop app inherits environment variables from the shell that launched it. Variables set after launch will not be picked up without a restart.

### OpenAI Partial Support

OpenAI support may have limitations. Anthropic models are recommended for the best experience with Goose.

## Related Documentation

- [AI Bridge Setup](./index.md#setup)
- [Client Configuration Overview](./index.md#client-configuration)
- [Coder Tasks](../tasks.md)
- [Template Examples](./index.md#pre-configuring-in-coder-templates)
