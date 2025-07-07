# Best Practices

This document includes a mix of cultural and technical best practices and guidelines for introducing AI agents into your organization.

## Identify Use Cases

To successfully implement AI coding agents, identify 3-5 practical use cases where AI tools can deliver real value. Additionally, find a target group of developers and projects that are the best candidates for each specific use case.

Below are common scenarios where AI coding agents provide the most impact, along with the right tools for each use case:

| Scenario                                       | Description                                                                                                               | Examples                                                                                                             | Tools                                                                                          |
|------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| **Automating actions in the IDE**              | Supplement tedious development with agents                                                                                | Small refactors, generating unit tests, writing inline documentation, code search and navigation                     | [IDE Agents](./ide-agents.md) in Workspaces                                                    |
| **Developer-led investigation and setup**      | Developers delegate research and initial implementation to AI, then take over in their preferred IDE to complete the work | Bug triage and analysis, exploring technical approaches, understanding legacy code, creating starter implementations | [Tasks](./tasks.md), to a full IDE with [Workspaces](../user-guides/workspace-access/index.md) |
| **Prototyping & Business Applications**        | User-friendly interface for engineers and non-technical users to build and prototype within new or existing codebases     | Creating dashboards, building simple web apps, data analysis workflows, proof-of-concept development                 | [Tasks](./tasks.md)                                                                            |
| **Full background jobs & long-running agents** | Agents that run independently without user interaction for extended periods of time                                       | Automated code reviews, scheduled data processing, continuous integration tasks, monitoring and alerting             | [Tasks](./tasks.md) API *(in development)*                                                     |
| **External agents and chat clients**           | External AI agents and chat clients that need access to Coder workspaces for development environments and code sandboxing | ChatGPT, Claude Desktop, custom enterprise agents running tests, performing development tasks, code analysis         | [MCP Server](./mcp-server.md)                                                                  |

## Provide Agents with Proper Context

While LLMs are trained on general knowledge, it's important to provide additional context to help agents understand your codebase and organization.

### Memory

Coding Agents like Claude Code often refer to a [memory file](https://docs.anthropic.com/en/docs/claude-code/memory) in order to gain context about your repository or organization.

Look up the docs for the specific agent you're using to learn more about how to provide context to your agents.

### Tools (Model Context Protocol)

Agents can also use tools, often via [Model Context Protocol](https://modelcontextprotocol.io/introduction) to look up information or perform actions. A common example would be fetching style guidelines from an internal wiki, or looking up the documentation for a service within your catalog.

Look up the docs for the specific agent you're using to learn more about how to provide tools to your agents.

#### Our Favorite MCP Servers

In internal testing, we have seen significant improvements in agent performance when these tools are added via MCP.

- [Playwright](https://github.com/microsoft/playwright-mcp): Instruct your agent
  to open a browser, and check its work by viewing output and taking
  screenshots.
- [desktop-commander](https://github.com/wonderwhy-er/DesktopCommanderMCP):
  Instruct your agent to run long-running tasks (e.g. `npm run dev`) in the background instead of blocking the main thread.

## Security & Permissions

LLMs and agents can be dangerous if not run with proper boundaries. Be sure not to give agents full permissions on behalf of a user, and instead use separate identities with limited scope whenever interacting autonomously.

[Learn more about securing agents with Coder Tasks](./security.md)

## Keep it Simple

Today's LLMs and AI agents are not going to refactor entire codebases with production-grade code on their own! Using coding agents can be extremely fun and productive, but it is important to keep the scope of your use cases small and simple, and grow them over time.
