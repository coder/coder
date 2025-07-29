# Coder Tasks (Beta)

Coder Tasks is an interface for running & managing coding agents such as Claude Code and Aider, powered by Coder workspaces.

![Tasks UI](../images/guides/ai-agents/tasks-ui.png)

Coder Tasks is best for cases where the IDE is secondary, such as prototyping or running long-running background jobs. However, tasks run inside full workspaces so developers can [connect via an IDE](../user-guides/workspace-access) to take a task to completion.

> [!NOTE]
> Coder Tasks is free and open source. If you are a Coder Premium customer or want to run hundreds of tasks in the background, [contact us](https://coder.com/contact) for roadmap information and volume pricing.

## Supported Agents (and Models)

Any terminal-based agent that supports Model Context Protocol (MCP) can be integrated with Coder Tasks, including your own custom agents.

Out of the box, agents like Claude Code and Goose are supported with built-in modules that can be added to a template. [See all modules compatible with Tasks in the Registry](https://registry.coder.com/modules?search=tag%3Atasks).

Enterprise LLM Providers such as AWS Bedrock, GCP Vertex and proxies such as LiteLLM can be used as well in order to keep intellectual property private. Self-hosted models such as llama4 can also be configured with specific agents, such as Aider and Goose.

## Architecture

Each task runs inside its own Coder workspace for isolation purposes. Agents like Claude Code also run in the workspace, and can be pre-installed via a module in the Coder Template. Agents then communicate with your LLM provider, so no GPUs are directly required in your workspaces for inference.

![High-Level Architecture](../images/guides/ai-agents/architecture-high-level.png)

Coder's [built-in modules for agents](https://registry.coder.com/modules?search=tag%3Atasks) will pre-install the agent alongside [AgentAPI](https://github.com/coder/agentapi). AgentAPI is an open source project developed by Coder which improves status reporting and the Chat UI, regardless of which agent you use.

## Getting Started with Tasks

### Option 1&rpar; Import and Modify Our Example Template

Our example template is the best way to experiment with Tasks with a [real world demo app](https://github.com/gothinkster/realworld). The application is running in the background and you can experiment with coding agents.

![Tasks UI with realworld app](../images/guides/ai-agents/realworld-ui.png)

Try prompts such as:

- "rewrite the backend in go"
- "document the project structure"
- "change the primary color theme to purple"

To import the template and begin configuring it, follow the [documentation in the Coder Registry](https://registry.coder.com/templates/coder-labs/tasks-docker)

> [!NOTE]
> The Tasks tab will appear automatically after you add a Tasks-compatible template and refresh the page.

### Option 2&rpar; Create or Duplicate Your Own Template

A template becomes a Task template if it defines a `coder_ai_task` resource and a `coder_parameter` named `"AI Prompt"`. Coder analyzes template files during template version import to determine if these requirements are met.

```hcl
data "coder_parameter" "ai_prompt" {
    name = "AI Prompt"
    type = "string"
}

# Multiple coder_ai_tasks can be defined in a template
resource "coder_ai_task" "claude-code" {
    # At most one coder ai task can be instantiated during a workspace build.
    # Coder fails the build if it would instantiate more than 1.
    count = data.coder_parameter.ai_prompt.value != "" ? 1 : 0

    sidebar_app {
        # which app to display in the sidebar on the task page
        id = coder_app.claude-code.id
    }
}
```

> [!NOTE]
> This definition is not final and may change while Tasks is in beta. After any changes, we guarantee backwards compatibility for one minor Coder version. After that, you may need to update your template to continue using it with Tasks.

Because Tasks run unpredictable AI agents, often for background tasks, we recommend creating a separate template for Coder Tasks with limited permissions. You can always duplicate your existing template, then apply separate network policies/firewalls/permissions to the template. From there, follow the docs for one of our [built-in modules for agents](https://registry.coder.com/modules?search=tag%3Atasks) in order to add it to your template, configure your LLM provider.

Alternatively, follow our guide for [custom agents](./custom-agents.md).

## Customizing the Task UI

The Task UI displays all workspace apps declared in a Task template. You can customize the app shown in the sidebar using the `sidebar_app.id` field on the `coder_ai_task` resource.

If a workspace app has the special `"preview"` slug, a navbar will appear above it. This is intended for templates that let users preview a web app they’re working on.

We plan to introduce more customization options in future releases.

## Opting out of Tasks

If you tried Tasks and decided you don't want to use it, you can hide the Tasks tab by starting `coder server` with the `CODER_HIDE_AI_TASKS=true` environment variable or the `--hide-ai-tasks` flag.

## Next Steps

<children></children>
