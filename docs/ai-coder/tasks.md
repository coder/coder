# Coder Tasks (Beta)

## What is a Task? 

Tasks is Coder's platform for managing coding agents and other AI-enabled tools. With Coder Tasks, you can

- Connect an AI Agent like Claude Code or OpenAI's Codex to your IDE to assist in day-to-day development and building
- Kick off AI-enabled workflows such as upgrading a vulnerable package and automatically opening a GitHub Pull Requests with the patch
- Facilitate an automated agent to detect a failure in your CI/CD pipeline, spin up a Coder Workspace, apply a fix, and prepare a PR _without_ manual input

![Tasks UI](../images/guides/ai-agents/tasks-ui.png)Coder Tasks Dashboard view to see all available tasks.

Coder Tasks allows you and your organization to build and automate workflows to fully leverage AI. Tasks operate through Coder Workspaces, so developers can [connect via an IDE](../user-guides/workspace-access) to jump in and guide development and fully interact with the agent.

## Why Use Tasks?

- The problems they solve (consistency, reproducibility, reducing manual setup)
- How they fit into the Coder mental model
- Relation to developer productivity and environment setup

Coder Tasks make both developer-driven _and_ autonomous agentic workflows first-class citizens within your organization. Without Tasks, teams will fall back to ad-hoc scripts, one-off commands, or manual checklists to perform simpler operations that LLMs can easily automate. These work arounds can help a single engineer, but don't scale or provide consistency across an organization that is attempting to use AI as a true force multiplier. 

Tasks exist to solve these types of problems:

- **Consistency:** Capture a known, safe, & secure workflow once that can then be run anywhere
- **Reproducability:** Every Task runs from a Coder Workspace, so results are reliable
- **Productivity:** Eliminate manual processes from developer processes enabling them to focus on less defined and harder-to-do issues
- **Scalability:** Once a workflow is captured in a Task, it can be reused by other teams within your organization scaling with you as you grow
- **Flexibility:** Support both developer *AND* autonomous agentic workflows

## How to Make a Task Template
- Define what a “task template” is
- How a template differs from a single-use task
- Lifecycle: draft → reusable template

As a quick refresher, a template is a Terraofrm `main.tf` file that defines the contents of the workspace and the resources it requires to run.  


## Task Template Design Principles
- Reusability  
- Composability (can be chained or combined)  
- Transparency (clear inputs/outputs)  
- Portability (works across environments)  

## How Tasks Fit Into Coder
- Place in the overall system (workspaces, environments, AI Coder)  
- How tasks interact with other features (e.g., provisioning, configuration)

## Best Practices for Authoring Tasks
- Keep them modular and focused
- Use clear naming and documentation
- Handle errors and edge cases gracefully

## Looking Ahead
- How tasks and templates may evolve
- Vision for tasks as part of the broader developer workflow





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

A template becomes a Task template if it defines a `coder_ai_task` resource and a `coder_parameter` named `"AI Prompt"`. Coder analyzes template files during template version import to determine if these requirements are met. Try adding this terraform block to an existing template where you'll add our Claude Code module. Note: the `coder_ai_task` resource is defined within the [Claude Code Module](https://registry.coder.com/modules/coder/claude-code?tab=readme), so it's not defined within this block.

```hcl
data "coder_parameter" "ai_prompt" {
    name = "AI Prompt"
    type = "string"
}

data "coder_parameter" "setup_script" {
  name         = "setup_script"
  display_name = "Setup Script"
  type         = "string"
  form_type    = "textarea"
  description  = "Script to run before running the agent"
  mutable      = false
  default      = ""
}

# The Claude Code module does the automatic task reporting
# Other agent modules: https://registry.coder.com/modules?search=agent
# Or use a custom agent:  
module "claude-code" {
  count               = data.coder_workspace.me.start_count
  source              = "registry.coder.com/coder/claude-code/coder"
  version             = "2.2.0"
  agent_id            = coder_agent.main.id
  folder              = "/home/coder/projects"
  install_claude_code = true
  claude_code_version = "latest"
  order               = 999

  # experiment_post_install_script = data.coder_parameter.setup_script.value

  # This enables Coder Tasks
  experiment_report_tasks = true
}

variable "anthropic_api_key" {
  type        = string
  description = "Generate one at: https://console.anthropic.com/settings/keys"
  sensitive   = true
}

resource "coder_env" "anthropic_api_key" {
  agent_id = coder_agent.main.id
  name     = "CODER_MCP_CLAUDE_API_KEY"
  value    = var.anthropic_api_key
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

## Automatically name your tasks

Coder can automatically generate a name your tasks if you set the `ANTHROPIC_API_KEY` environment variable on the Coder server. Otherwise, tasks will be given randomly generated names.

## Opting out of Tasks

If you tried Tasks and decided you don't want to use it, you can hide the Tasks tab by starting `coder server` with the `CODER_HIDE_AI_TASKS=true` environment variable or the `--hide-ai-tasks` flag.

## Next Steps

<children></children>
