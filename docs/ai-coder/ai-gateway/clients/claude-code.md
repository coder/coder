---
title: Claude Code
---

> [!NOTE]
> AI Gateway is part of [AI Governance](../../ai-governance.md), which is included with a Premium license.

Claude Code can be configured using environment variables.
All modes require a **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** for authentication with AI Gateway.

## Centralized API Key

```sh
# AI Gateway base URL.
export ANTHROPIC_BASE_URL="<your-deployment-url>/api/v2/ai-gateway/anthropic"

# Your Coder API token, used for authentication with AI Gateway.
export ANTHROPIC_AUTH_TOKEN="<your-coder-api-token>"
```

<details>
<summary>Persistent config (<code>.claude/settings.json</code>)</summary>

Merge the following into `~/.claude/settings.json`, preserving your existing settings:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "<your-deployment-url>/api/v2/ai-gateway/anthropic"
  },
  "apiKeyHelper": "printenv ANTHROPIC_AUTH_TOKEN"
}
```

The helper reads your Coder API token from `ANTHROPIC_AUTH_TOKEN` when Claude Code needs it.
Keep this variable in the environment of the terminal or IDE that starts Claude Code.
The token stays out of the settings file.

If you authenticate through the [Coder CLI](../../../reference/cli/login/index.md), you can use the saved login token instead:

```json
{
  "apiKeyHelper": "coder login token"
}
```

This alternative requires a saved CLI login for the same deployment as `ANTHROPIC_BASE_URL`.
Unset `ANTHROPIC_AUTH_TOKEN` so Claude Code uses the saved token.

</details>

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

When you run Claude Code, it will prompt you to log in with your Anthropic account.

## Pre-configuring in Templates

Template admins can pre-configure Claude Code for a seamless experience.
Admins can automatically inject the user's Coder session token and the AI Gateway base URL into the workspace environment.

```tf
module "claude-code" {
  source            = "registry.coder.com/coder/claude-code/coder"
  version           = "~> 5.5.1"
  agent_id          = coder_agent.main.id
  workdir           = "/path/to/project" # Set to your project directory
  enable_ai_gateway = true
  managed_settings = {
    apiKeyHelper = "printenv ANTHROPIC_AUTH_TOKEN"
  }
}
```

Visit the [Claude Code module on the Coder Registry](https://registry.coder.com/modules/coder/claude-code) for the full list of inputs and outputs.

## VS Code Extension

The Claude Code VS Code extension is also supported.

1. If pre-configured in the workspace environment variables (as shown above), it typically respects them.
2. You may need to sign in once; afterwards, it respects the workspace environment variables.

**References:**
[Claude Code Settings](https://docs.claude.com/en/docs/claude-code/settings#environment-variables)
