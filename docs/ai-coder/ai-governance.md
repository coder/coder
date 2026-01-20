# AI Governance Add-On (Premium)

While you can run any AI tool (such as [Cursor](https://registry.coder.com/modules/coder/cursor) or [Claude Code](https://registry.coder.com/modules/coder/claude-code)) inside Coder Workspaces, many enterprises need additional observability, management, and policy for secure and productive AI rollouts.

Coder’s AI Governance Add-On for Premium licenses includes a set of features that help organizations safely roll out AI tooling at scale:

- [AI Bridge](./ai-bridge/index.md): LLM gateway to audit AI sessions, central MCP server management, and policy enforcement
- [Boundaries](./boundary/agent-boundary.md): Process-level firewalls for agents, restricting which domains can be accessed by AI agents
- [Additional Tasks Use (via Agent Workspace Builds)](#how-coder-tasks-usage-is-measured): Additional allowance of Agent Workspace Builds for continued use of Coder Tasks.

## What this means for Coder Premium customers

As of Coder’s February 2026 release (v2.30), Coder AI Bridge and Agent Boundaries enter General Availability, and a warning will appear for deployments with these features enabled. In v2.30, this is a soft warning, but a feature version of Coder will restrict usage.

You can [contact your account team](https://coder.com/contact) for more information about pricing or a trial license to evaluate these features.

## Who this add-on is for

Coder already provides a vendor-neutral platform for development environments. The AI Governance Add-On is for teams that want to extend that platform to support AI-powered IDEs and coding agents in a controlled, observable way.

It’s a good fit if you’re:

- Rolling out AI-powered IDEs like Cursor and AI coding agents like Claude Code across teams
- Looking to centrally observe, audit, and govern AI activity in Coder Workspaces
- Managing AI workflows against sensitive or regulated codebases
- Expanding the use of Coder Tasks for AI-driven background work

If you already use other AI governance tools, such as third-party LLM gateways or vendor-managed policies, you can continue using them. Coder Workspaces can still serve as the backend for development environments and AI workflows, with or without the AI Governance Add-On.

## How Coder Tasks usage is measured

The usage metric used to measure Coder Tasks consumption is called **Agent Workspace Builds.**

An Agent Workspace Build is counted each time a workspace is started specifically for a coding agent to independently work on a Coder Task. Most of the work in this workspace is performed by the agent, not a human developer. Each Coder Task starts its own workspace, and the usage meter counts one Agent Workspace Build.

Traditional Coder Workspaces started manually by developers or scheduled to auto-start do not count as an Agent Workspace Build. These are considered daily-driver development environments where developers co-exist with their IDEs and coding assistants.

### Scenarios

| Scenario | Consumes Agent Workspace Build |
| --- | --- |
| Developer creates a Coder Task to write end-to-end tests | Yes |
| Automated pipeline creates a task via Coder Tasks CLI (with Claude Code) to review a pull request | Yes |
| Developer resumes an old Coder Task order to continue prototyping | Yes |
| Developer starts a workspace for use with VS Code and Jupyter | No |
| Developer creates a workspace for use with Cursor and Claude Code CLI | No |
| Developer creates a workspace for use with Coder AI Bridge and Boundaries | No |

In the future, additional capabilities for managing agents (beyond Coder Tasks) may also consume agent workspace builds.

### Agent Workspace Build Limits

Without proper controls and sandboxing, it is not recommended to open up Coder Tasks to a large audience in the enterprise. Coder Premium deployments have up to 1,000 Agent Workspace Builds, primarily for proof-of-concept use and basic workflows.

Our [AI Governance Add-On](./ai-governance.md) includes a shared usage pool of Agent Workspace Builds for automated workflows, along with limits that scale proportionately with user count. If you are approaching your deployment-wide limits, [contact us](https://coder.com/contact) to discuss your use case with our team.
