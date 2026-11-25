---
title: JetBrains
---

> [!NOTE]
> AI Gateway is part of [AI Governance](../../index.md), which is included with a Premium license.

JetBrains IDEs 2026.3 and later include [JetBrains Air](https://www.jetbrains.com/help/idea/agents-in-ide.html), an agent workspace provided by the bundled Air plugin.
When you connect a JetBrains IDE to a Coder workspace, Air runs coding agents inside the workspace and [reuses their existing configuration](https://www.jetbrains.com/help/air-ides/air-configure-authorization.html).
If the workspace is configured for AI Gateway, Claude Agent and Codex sessions route requests through AI Gateway without any configuration in the IDE.

## Prerequisites

- **JetBrains IDE**: 2026.3 or later, connected to a Coder workspace with [JetBrains Toolbox](../../../workspaces/workspace-access/jetbrains/toolbox.md) or [JetBrains Gateway](../../../workspaces/workspace-access/jetbrains/gateway.md), or running locally.
- **Air plugin**: Bundled and enabled by default.
- **AI Gateway configuration**: Set up as described in [Configure the template](#configure-the-template) or [Use a local IDE](#use-a-local-ide).

You don't need a JetBrains AI subscription.

## How Air uses the workspace configuration

| Agent        | How Air runs it                                   | Settings read from the workspace                        |
|--------------|---------------------------------------------------|---------------------------------------------------------|
| Claude Agent | Installs its own copy from the ACP agent registry | `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN`         |
| Codex        | Runs the Codex CLI installed in the workspace     | The AI Gateway model provider in `~/.codex/config.toml` |

## Configure the template

To configure both agents for every workspace, set `enable_ai_gateway = true` on the [Claude Code module](./claude-code.md#pre-configuring-in-templates) and the [Codex module](./codex.md#pre-configuring-in-templates).
For Claude Agent, you can instead set the environment variables on the workspace agent, as shown in [Configure in-workspace tools](./index.md#configure-in-workspace-tools).

## Use a local IDE

If you run the IDE locally instead of in a Coder workspace, Air uses the local configuration of each agent.
Configure [Claude Code](./claude-code.md#centralized-api-key) and [Codex](./codex.md#centralized-api-key) for AI Gateway with a [long-lived Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself).

## Start a session

1. Open the **Agent Sessions** tool window.
1. Next to **New Session**, select a Claude Agent or Codex preset.
1. Send a prompt.

**References:**
[Agents in IntelliJ IDEA](https://www.jetbrains.com/help/idea/agents-in-ide.html),
[Configure agent authorization](https://www.jetbrains.com/help/air-ides/air-configure-authorization.html)
