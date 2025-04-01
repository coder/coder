# Best Practices & Adding Tools via MCP

> [!NOTE]
>
> This functionality is in early access and subject to change. Do not run in
> production as it is unstable. Instead, deploy these changes into a demo or
> staging environment.
>
> Join our [Discord channel](https://discord.gg/coder) or
> [contact us](https://coder.com/contact) to get help or share feedback.

## Overview

Coder templates should be pre-equipped with the tools and dependencies needed
for development. With AI Agents, this is no exception.

## Prerequisites

- A Coder deployment with v2.21 or later
- A [template configured for AI agents](./create-template.md)

## Best Practices

- Since agents are still early, it is best to use the most capable ML models you
  have access to in order to evaluate their performance.
- Set a system prompt with the `AI_SYSTEM_PROMPT` environment in your template
- Within your repositories, write a `.cursorrules`, `CLAUDE.md` or similar file
  to guide the agent's behavior.
- To read issue descriptions or pull request comments, install the proper CLI
  (e.g. `gh`) in your image/template.
- Ensure your [template](./create-template.md) is truly pre-configured for
  development without manual intervention (e.g. repos are cloned, dependencies
  are built, secrets are added/mocked, etc.)
  > Note: [External authentication](../../admin/external-auth.md) can be helpful
  > to authenticate with third-party services such as GitHub or JFrog.
- Give your agent the proper tools via MCP to interact with your codebase and
  related services.
- Read our recommendations on [securing agents](./securing.md) to avoid
  surprises.

## Adding Tools via MCP

Model Context Protocol (MCP) is an emerging standard for adding tools to your
agents.

Follow the documentation for your [agent](./agents.md) to learn how to configure
MCP servers. See
[modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers)
to browse open source MCP servers.

### Our Favorite MCP Servers

In internal testing, we have seen significant improvements in agent performance
when these tools are added via MCP.

- [Playwright](https://github.com/microsoft/playwright-mcp): Instruct your agent
  to open a browser, and check its work by viewing output and taking
  screenshots.
- [desktop-commander](https://github.com/wonderwhy-er/DesktopCommanderMCP):
  Instruct your agent to run long-running tasks (e.g. `npm run dev`) in the
  background instead of blocking the main thread.

## Next Steps

- [Supervise Agents in the UI](./coder-dashboard.md)
- [Supervise Agents in the IDE](./ide-integration.md)
- [Supervise Agents Programatically](./headless.md)
- [Securing Agents](./securing.md)
