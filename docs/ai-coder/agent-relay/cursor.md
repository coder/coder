---
title: Agent Relay for Cursor
---

> [!NOTE]
> Agent Relay for Cursor is in [early access](../../install/releases/feature-stages.md#early-access-features)
> and is currently in closed preview with select customers.

[Agent Relay](./index.md) connects [Cursor Cloud Agents](https://cursor.com/cloud) to self-hosted Coder workspaces.
Cursor also refers to this self-hosted worker model as
[bring-your-own-machine (BYOM)](https://cursor.com/docs/cloud-agent/bring-your-own-machine).
Developers keep using the Cursor client and cloud agent workflow they
already know.
The agent's tool calls run inside a Coder workspace on infrastructure you
control instead of a Cursor-managed environment.

Cursor's agent orchestration and AI inference remain cloud-hosted.
Coder doesn't proxy or observe model inference.
Coder provides the workspace where the agent executes, and logs that
correlate the Cursor session and user to that workspace.

## How it works

1. A developer selects a Cursor worker pool mapped to a Coder organization
   and workspace template, then starts a Cursor agent session.
1. Agent Relay claims the pending Cursor request and asks the Coder control
   plane to provision a workspace from the mapped template for that user.
1. A Cursor worker process inside the workspace connects to the
   corresponding Cursor cloud session and executes the agent's tool calls.
1. When the session ends, Agent Relay manages workspace teardown.

Each Cursor agent session gets its own ephemeral workspace.

## Requirements

- A licensed Coder deployment
- A Cursor Enterprise plan

## Get started

Talk to your [Coder account team](https://coder.com/contact) or email
[sales@coder.com](mailto:sales@coder.com) to get access to Agent Relay for
Cursor.

## Learn more

- [Agent Relay](./index.md)
- [Architecture](../../admin/infrastructure/architecture.md)
