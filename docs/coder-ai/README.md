# Use AI Coding Agents in Coder Workspaces

> [!NOTE]
>
> This functionality is in early access and still evolving.
> For now, we recommend testing it in a demo or staging environment,
> rather than deploying to production.
>
> Join our [Discord channel](https://discord.gg/coder) or
> [contact us](https://coder.com/contact) to get help or share feedback.

AI Coding Agents such as [Claude Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview), [Goose](https://block.github.io/goose/), and [Aider](https://github.com/paul-gauthier/aider) are becoming increasingly popular for:

- Protyping web applications or landing pages
- Researching / onboarding to a codebase
- Assisting with lightweight refactors
- Writing tests and documentation
- Small, well-defined chores

With Coder, you can self-host AI agents in isolated development environments with proper context and tooling around your existing developer workflows. Whether you are a regulated enterprise or an individual developer, running AI agents at scale with Coder is much more productive and secure than running them locally.

![AI Agents in Coder](../images/guides/ai-agents/landing.png)

## Prerequisites

Coder is free and open source for developers, with a [premium plan](https://coder.com/pricing) for enterprises. You can self-host a Coder deployment in your own cloud provider.

- A [Coder deployment](../install/index.md) with v2.21.0 or later
- A Coder [template](../admin/templates/index.md) for your project(s).
- Access to at least one ML model (e.g. Anthropic Claude, Google Gemini, OpenAI)
  - Cloud Model Providers (AWS Bedrock, GCP Vertex AI, Azure OpenAI) are supported with some agents
  - Self-hosted models (e.g. llama3) and AI proxies (OpenRouter) are supported with some agents

## Table of Contents

<children></children>
