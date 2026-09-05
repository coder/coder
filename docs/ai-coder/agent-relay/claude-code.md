---
title: Agent Relay for Claude Code
---

> [!NOTE]
> Agent Relay for Claude Code is in [early access](../../install/releases/feature-stages.md#early-access-features)
> and is currently in closed preview with select customers.

[Agent Relay](./index.md) connects [Claude Code](https://claude.ai/code) sessions to self-hosted Coder workspaces.
Anthropic calls this self-hosted worker model bring-your-own-compute (BYOC),
and also refers to it as [self-hosted environments](https://code.claude.com/docs/en/self-hosted-environments).
Developers keep using the claude.ai/code or Claude Desktop client and workflow they already know.
The Claude Code runner's tool calls run inside a Coder workspace on infrastructure you control
instead of an Anthropic-managed environment.

Anthropic's agent orchestration and AI inference remain cloud-hosted.
Coder doesn't proxy or observe model inference.
Coder provides the workspace where the runner executes,
and logs that correlate the Claude Code session and user to that workspace.

## How it works

1. A developer starts a Claude Code session at claude.ai/code or in Claude Desktop,
   targeting a self-hosted runner pool mapped to a Coder organization and workspace template.
1. Agent Relay claims the pending work order for that session
   and asks the Coder control plane to provision a workspace from the mapped template for that user.
1. A Claude Code self-hosted runner inside the workspace registers with the corresponding Claude Code session
   and executes the agent's tool calls.
1. When the session ends,
   Agent Relay manages workspace teardown.

Each Claude Code session gets its own ephemeral workspace.

## Requirements

- A licensed Coder deployment
- An Anthropic plan with self-hosted runner (BYOC) support

## Get started

Talk to your [Coder account team](https://coder.com/contact) or email [sales@coder.com](mailto:sales@coder.com) to get access to Agent Relay for Claude Code.

## Learn more

- [Agent Relay](./index.md)
- [Architecture](../../admin/infrastructure/architecture.md)
