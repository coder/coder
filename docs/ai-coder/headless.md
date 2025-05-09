> [!NOTE]
>
> This functionality is in beta and is evolving rapidly.
>
> When using any AI tool for development, exercise a level of caution appropriate to your use case and environment.
> Always review AI-generated content before using it in critical systems.
>
> Join our [Discord channel](https://discord.gg/coder) or
> [contact us](https://coder.com/contact) to get help or share feedback.

## Prerequisites

- A Coder deployment with v2.21 or later
- A [template configured for AI agents](./create-template.md)

## Overview

Once you have an agent running and reporting activity to Coder, you can manage
it programmatically via the MCP server, Coder CLI, and/or REST API.

## MCP Server

Power users can configure [Claude Desktop](https://claude.ai/download), Cursor,
or other tools with MCP support to interact with Coder in order to:

- List workspaces
- Create/start/stop workspaces
- Run commands on workspaces
- Check in on agent activity

In this model, an [IDE Agent](./agents.md#in-ide-agents) could interact with a
remote Coder workspace, or Coder can be used in a remote pipeline or a larger
workflow.

The Coder CLI has options to automatically configure MCP servers for you. On
your local machine, run the following command:

```sh
coder exp mcp configure claude-desktop # Configure Claude Desktop to interact with Coder
coder exp mcp configure cursor # Configure Cursor to interact with Coder
```

> MCP is also used for various agents to report activity back to Coder. Learn more about this in [custom agents](./custom-agents.md).

## Coder CLI

Workspaces can be created, started, and stopped via the Coder CLI. See the
[CLI docs](../reference/cli/index.md) for more information.

## REST API

The Coder REST API can be used to manage workspaces and agents. See the
[API docs](../reference/api/index.md) for more information.

## Next Steps

- [Securing Agents](./securing.md)
