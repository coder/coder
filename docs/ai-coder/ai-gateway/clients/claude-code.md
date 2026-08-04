# Claude Code

> [!NOTE]
> AI Gateway is part of [AI Governance](../../ai-governance.md), which is
> included with a Premium license.

Claude Code can be configured using environment variables. All modes require a **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** for authentication with AI Gateway.

## Centralized API Key

```sh
# AI Gateway base URL.
export ANTHROPIC_BASE_URL="<your-deployment-url>/api/v2/ai-gateway/anthropic"

# Your Coder API token, used for authentication with AI Gateway.
export ANTHROPIC_AUTH_TOKEN="<your-coder-api-token>"
```

## BYOK (Personal API Key)

```sh
# AI Gateway base URL.
export ANTHROPIC_BASE_URL="<your-deployment-url>/api/v2/ai-gateway/anthropic"

# Your personal Anthropic API key, forwarded to Anthropic.
export ANTHROPIC_API_KEY="<your-anthropic-api-key>"

# Your Coder API token, used for authentication with AI Gateway.
export ANTHROPIC_CUSTOM_HEADERS="X-Coder-AI-Governance-Token: <your-coder-api-token>"

# Ensure no auth token is set so Claude Code uses the API key instead.
unset ANTHROPIC_AUTH_TOKEN
```

## BYOK (Claude Subscription)

```sh
# AI Gateway base URL.
export ANTHROPIC_BASE_URL="<your-deployment-url>/api/v2/ai-gateway/anthropic"

# Your Coder API token, used for authentication with AI Gateway.
export ANTHROPIC_CUSTOM_HEADERS="X-Coder-AI-Governance-Token: <your-coder-api-token>"

# Ensure no auth token is set so Claude Code uses subscription login instead.
unset ANTHROPIC_AUTH_TOKEN
```

When you run Claude Code, it will prompt you to log in with your Anthropic
account.

## Pre-configuring in Templates

Template admins can pre-configure Claude Code for a seamless experience. Admins can automatically inject the user's Coder session token and the AI Gateway base URL into the workspace environment.

```tf
module "claude-code" {
  source            = "registry.coder.com/coder/claude-code/coder"
  version           = "~> 5.2"
  agent_id          = coder_agent.main.id
  workdir           = "/path/to/project" # Set to your project directory
  enable_ai_gateway = true
}
```

Visit the [Claude Code module on the Coder Registry](https://registry.coder.com/modules/coder/claude-code) for the full list of inputs and outputs.

## VS Code Extension

The Claude Code VS Code extension is also supported.

1. If pre-configured in the workspace environment variables (as shown above), it typically respects them.
2. You may need to sign in once; afterwards, it respects the workspace environment variables.

**References:** [Claude Code Settings](https://docs.claude.com/en/docs/claude-code/settings#environment-variables)
