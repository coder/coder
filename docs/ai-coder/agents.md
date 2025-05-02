# AI Coding Agents

> [!NOTE]
>
> This page is not exhaustive and the landscape is evolving rapidly.
>
> Please [open an issue](https://github.com/coder/coder/issues/new) or submit a
> pull request if you'd like to see your favorite agent added or updated.

Coding agents are rapidly emerging to help developers tackle repetitive tasks,
explore codebases, and generate solutions with increasing effectiveness.

You can run these agents in Coder workspaces to leverage the power of cloud resources
and deep integration with your existing development workflows.

## Why Run AI Coding Agents in Coder?

Coder provides unique advantages for running AI coding agents:

- **Consistent environments**: Agents work in the same standardized environments as your developers.
- **Resource optimization**: Leverage powerful cloud resources without taxing local machines.
- **Security and isolation**: Keep sensitive code, API keys, and secrets in controlled environments.
- **Seamless collaboration**: Multiple developers can observe and interact with agent activity.
- **Deep integration**: Status reporting and task management directly in the Coder UI.
- **Scalability**: Run multiple agents across multiple projects simultaneously.
- **Persistent sessions**: Agents can continue working even when developers disconnect.

## Types of Coding Agents

AI coding agents generally fall into two categories, both fully supported in Coder:

### Headless Agents

Headless agents can run without an IDE open, making them ideal for:

- **Background automation**: Execute repetitive tasks without supervision.
- **Resource-efficient development**: Work on projects without keeping an IDE running.
- **CI/CD integration**: Generate code, tests, or documentation as part of automated workflows.
- **Multi-project management**: Monitor and contribute to multiple repositories simultaneously.

Additionally, with Coder, headless agents benefit from:

- Status reporting directly to the Coder dashboard.
- Workspace lifecycle management (auto-stop).
- Resource monitoring and limits to prevent runaway processes.
- API-driven management for enterprise automation.

| Agent         | Supported models                                        | Coder integration         | Notes                                                                                         |
|---------------|---------------------------------------------------------|---------------------------|-----------------------------------------------------------------------------------------------|
| Claude Code ⭐ | Anthropic Models Only (+ AWS Bedrock and GCP Vertex AI) | First class integration ✅ | Enhanced security through workspace isolation, resource optimization, task status in Coder UI |
| Goose         | Most popular AI models + gateways                       | First class integration ✅ | Simplified setup with Terraform module, environment consistency                               |
| Aider         | Most popular AI models + gateways                       | In progress ⏳             | Coming soon with workspace resource optimization                                              |
| OpenHands     | Most popular AI models + gateways                       | In progress ⏳ ⏳           | Coming soon                                                                                   |

[Claude Code](https://github.com/anthropics/claude-code) is our recommended
coding agent due to its strong performance on complex programming tasks.
See our [detailed Claude integration guide](./claude-integration.md) for comprehensive
setup and usage instructions.

> [!INFO]
> Any agent can run in a Coder workspace via our [MCP integration](./headless.md),
> even if we don't have a specific module for it yet.

### In-IDE agents

In-IDE agents run within development environments like VS Code, Cursor, or Windsurf.

These are ideal for exploring new codebases, complex problem solving, pair programming,
or rubber-ducking.

| Agent                       | Supported Models                  | Coder integration                                            | Coder key advantages                                           |
|-----------------------------|-----------------------------------|--------------------------------------------------------------|----------------------------------------------------------------|
| Cursor (Agent Mode)         | Most popular AI models + gateways | ✅ [Cursor Module](https://registry.coder.com/modules/cursor) | Pre-configured environment, containerized dependencies         |
| Windsurf (Agents and Flows) | Most popular AI models + gateways | ✅ via Remote SSH                                             | Consistent setup across team, powerful cloud compute           |
| Cline                       | Most popular AI models + gateways | ✅ via VS Code Extension                                      | Enterprise-friendly API key management, consistent environment |

## Agent status reports in the Coder dashboard

Claude Code and Goose can report their status directly to the Coder dashboard:

- Task progress appears in the workspace overview.
- Completion status is visible without opening the terminal.
- Error states are highlighted.

## Get started

Ready to deploy AI coding agents in your Coder deployment?

1. [Create a Coder template for agents](./create-template.md).
1. Configure your chosen agent with appropriate API keys and permissions.
1. Start monitoring agent activity in the Coder dashboard.

## Next Steps

- [Create a Coder template for agents](./create-template.md)
- [Integrate with your issue tracker](./issue-tracker.md)
- [Learn about MCP and adding AI tools](./best-practices.md)
