# Create a Coder template for agents

> [!NOTE]
>
> This functionality is in beta and is evolving rapidly.
>
> For now, we recommend testing it in a demo or staging environment,
> rather than deploying to production.
>
> Join our [Discord channel](https://discord.gg/coder) or
> [contact us](https://coder.com/contact) to get help or share feedback.

## Overview

Coder has first-class support for managing agents through Github, but can also
integrate with other issue trackers. Use our action to interact with agents
directly in issues and PRs.

## Prerequisites

- A Coder deployment with v2.21 or later
- A [template configured for AI agents](./create-template.md)

## GitHub

### GitHub Action

The [start-workspace](https://github.com/coder/start-workspace-action) GitHub
action will create a Coder workspace based on a specific phrase in a comment
(e.g. `@coder`).

![GitHub Issue](../images/guides/ai-agents/github-action.png)

When properly configured with an [AI template](./create-template.md), the agent
will begin working on the issue.

### Pull Request Support (Coming Soon)

We're working on adding support for an agent automatically creating pull
requests and responding to your comments. Check back soon or
[join our Discord](https://discord.gg/coder) to stay updated.

![GitHub Pull Request](../images/guides/ai-agents/github-pr.png)

## Integrating with Other Issue Trackers

While support for other issue trackers is under consideration, you can can use
the [REST API](../reference/api/index.md) or [CLI](../reference/cli/index.md) to integrate
with other issue trackers or CI pipelines.

In addition, an [Open in Coder](../admin/templates/open-in-coder.md) flow can
be used to generate a URL and/or markdown button in your issue tracker to
automatically create a workspace with specific parameters.

## Next Steps

- [Best practices & adding tools via MCP](./best-practices.md)
- [Supervise Agents in the UI](./coder-dashboard.md)
- [Supervise Agents in the IDE](./ide-integration.md)
- [Supervise Agents Programmatically](./headless.md)
- [Securing Agents with Boundaries](./securing.md)
