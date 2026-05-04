# Run AI Coding Agents in Coder

Learn how to run & manage coding agents with Coder, both alongside existing
workspaces and for background task execution.

## Agents in the IDE

Coder [integrates with IDEs](../user-guides/workspace-access/index.md) such as
Cursor, Windsurf, and Zed that include built-in coding agents to work alongside
developers. Additionally, template admins can
[pre-install extensions](https://registry.coder.com/modules/coder/vscode-web)
for agents such as GitHub Copilot and Roo Code.

These agents work well inside existing Coder workspaces as they can simply be
enabled via an extension or are built-into the editor.

## Coder Agents

In cases where the IDE is secondary, such as prototyping, research, or
long-running background jobs, [Coder Agents](./agents/index.md) is the
recommended way to delegate development work to coding agents in your Coder
deployment.

Coder Agents is a native AI coding agent built into Coder. The agent loop runs
in the Coder control plane on your infrastructure rather than inside the
workspace, so workspaces can be completely network isolated. Developers
interact with agents through the web UI, the CLI (`coder agents`), or the
REST API.

![Coder Agents chat interface with git diff sidebar](../images/agents-hero-image.png)

[Learn more about Coder Agents](./agents/index.md) for architecture details,
supported LLM providers, and how to get started.

## Secure Your Workflows with Agent Firewall

AI agents can be powerful teammates, but must be treated as untrusted and
unpredictable interns as opposed to tools. Without the right controls, they can
go rogue.

[Agent Firewall](./agent-firewall/index.md) is a new tool that offers
process-level safeguards that detect and prevent destructive actions. Unlike
traditional mitigation methods like firewalls, service meshes, and RBAC systems,
Agent Firewall is an agent-aware, centralized control point that can either be
embedded in the same secure Coder Workspaces that enterprises already trust, or
used through an open source CLI.

To learn more about features, implementation details, and how to get started,
check out the [Agent Firewall documentation](./agent-firewall/index.md).
