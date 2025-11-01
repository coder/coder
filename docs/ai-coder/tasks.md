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

### Option 2&rpar; Create or Duplicate Your Own Template

A template becomes a Task template if it defines a `coder_ai_task` resource. Coder analyzes template files during template version import to determine if these requirements are met. Try adding this terraform block to an existing template where you'll add our Claude Code module. Note: the `coder_ai_task` resource is defined within the [Claude Code Module](https://registry.coder.com/modules/coder/claude-code?tab=readme), so it's not defined within this block.

```hcl
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
      version = ">= 2.12"
    }
  }
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
  source   = "registry.coder.com/coder/claude-code/coder"
  version  = "4.0.0"
  agent_id = coder_agent.example.id
  workdir  = "/home/coder/project"

  claude_api_key = var.anthropic_api_key
  # OR
  # claude_code_oauth_token = var.anthropic_oauth_token

  claude_code_version = "1.0.82" # Pin to a specific version
  agentapi_version    = "v0.6.1"

  ai_prompt = coder_task.task.prompt
  model     = "sonnet"

  # Optional: run your pre-flight script
  # pre_install_script = data.coder_parameter.setup_script.value

  permission_mode = "plan"

  mcp = <<-EOF
  {
    "mcpServers": {
      "my-custom-tool": {
        "command": "my-tool-server",
        "args": ["--port", "8080"]
      }
    }
  }
  EOF
}

resource "coder_ai_task" "task" {
  app_id = module.claude-code.task_app_id
}

# Rename to `anthropic_oauth_token` if using the Oauth Token
variable "anthropic_api_key" {
  type        = string
  description = "Generate one at: https://console.anthropic.com/settings/keys"
  sensitive   = true
}
```

Because Tasks run unpredictable AI agents, often for background tasks, we recommend creating a separate template for Coder Tasks with limited permissions. You can always duplicate your existing template, then apply separate network policies/firewalls/permissions to the template. From there, follow the docs for one of our [built-in modules for agents](https://registry.coder.com/modules?search=tag%3Atasks) in order to add it to your template, configure your LLM provider.

Alternatively, follow our guide for [custom agents](./custom-agents.md).

## Migrating Task Templates for Coder version 2.28.0

Prior to Coder version 2.28.0, the definition of a Coder task was different to the above. It required the following to be defined in the template:

1. A Coder parameter specifically named `"AI Prompt"`,
2. A `coder_workspace_app` that runs the `coder/agentapi` binary,
3. A `coder_ai_task` resource in the template that sets `sidebar_app.id`. This was generally defined in Coder modules specific to AI Tasks.

Note that 2 and 3 were generally handled by the `coder/agentapi` Terraform module.

The pre-2.28.0 definition will be supported until the release of 2.29.0. You will need to update your Tasks-enabled templates to continue using Tasks after this release.

You can view an [example here](https://github.com/coder/coder/pull/20426). Alternatively, follow the steps below:

1. Update the Coder Terraform provider to at least version 2.12.0:

```diff
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
-      version = "x.y.z"
+      version = ">= 2.12"
    }
  }
}
```

1. Define a `coder_ai_task` resource in your template:

```diff
+resource "coder_ai_task" "task" {}
```

1. Update the version of the respective AI agent module (e.g. `claude-code`) to at least 4.0.0 and provide the prompt from `coder_ai_task.prompt` instead of the "AI Prompt" parameter:

```diff
module "claude-code" {
  source              = "registry.coder.com/coder/claude-code/coder"
-  version             = "4.0.0"
+  version             = "4.0.0"
    ...
-  ai_prompt           = data.coder_parameter.ai_prompt.value
+  ai_prompt           = coder_ai_task.task.prompt
}
```

1. Add the `coder_ai_task` resource and set `app_id` to the `task_app_id` output of the module:

```diff
resource "coder_ai_task" "task" {
+ app_id = module.claude-code.task_app_id
}
```

## Customizing the Task UI

The Task UI displays all workspace apps declared in a Task template. You can customize the app shown in the sidebar using the `sidebar_app.id` field on the `coder_ai_task` resource.

If a workspace app has the special `"preview"` slug, a navbar will appear above it. This is intended for templates that let users preview a web app they’re working on.

We plan to introduce more customization options in future releases.

## Automatically name your tasks

Coder can automatically generate a name your tasks if you set the `ANTHROPIC_API_KEY` environment variable on the Coder server. Otherwise, tasks will be given randomly generated names.

## Opting out of Tasks

If you tried Tasks and decided you don't want to use it, you can hide the Tasks tab by starting `coder server` with the `CODER_HIDE_AI_TASKS=true` environment variable or the `--hide-ai-tasks` flag.

## Command Line Interface

See [Tasks CLI](./cli.md).

## Next Steps

<children></children>
