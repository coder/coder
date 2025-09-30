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

## Command Line Interface

The Coder CLI provides experimental commands for managing tasks programmatically. These are available under `coder exp task`:

```bash
USAGE:
  coder exp task

  Experimental task commands.

  Aliases: tasks

SUBCOMMANDS:
    create    Create an experimental task
    delete    Delete experimental tasks
    list      List experimental tasks
    logs      Show a task's logs
    send      Send input to a task
    status    Show the status of a task.
```

### Create task

```bash
USAGE:
  coder exp task create [flags] [input]

  Create an experimental task

  # Create a task with direct input
  $ coder exp task create "Add authentication to the user service"

  # Create a task with stdin input
  $ echo "Add authentication to the user service" | coder exp task create

  # Create a task with a specific name
  $ coder exp task create --name task1 "Add authentication to the user service"

  # Create a task from a specific template / preset
  $ coder exp task create --template backend-dev --preset "My Preset" "Add authentication to the user service"

  # Create a task for another user (requires appropriate permissions)
  $ coder exp task create --owner user@example.com "Add authentication to the user service"

OPTIONS:
  -O, --org string, $CODER_ORGANIZATION
          Select which organization (uuid or name) to use.

      --name string
          Specify the name of the task. If you do not specify one, a name will be generated for you.

      --owner string (default: me)
          Specify the owner of the task. Defaults to the current user.

      --preset string, $CODER_TASK_PRESET_NAME (default: none)
  -q, --quiet bool
          Only display the created task's ID.

      --stdin bool
          Reads from stdin for the task input.

      --template string, $CODER_TASK_TEMPLATE_NAME
      --template-version string, $CODER_TASK_TEMPLATE_VERSION
```

### Deleting Tasks

```bash
USAGE:
  coder exp task delete [flags] <task> [<task> ...]

  Delete experimental tasks

  Aliases: rm

  # Delete a single task.
  $ coder exp task delete task1

  # Delete multiple tasks.
  $ coder exp task delete task1 task2 task3

  # Delete a task without confirmation
  $ coder exp task delete task4 --yes

OPTIONS:
  -y, --yes bool
          Bypass prompts.
```

### List tasks

```bash
USAGE:
  coder exp task list [flags]

  List experimental tasks

  Aliases: ls

    # List tasks for the current user
  $ coder exp task list

  # List tasks for a specific user
  $ coder exp task list --user someone-else

  # List all tasks you can view
  $ coder exp task list --all

  # List all your running tasks
  $ coder exp task list --status running

  # As above, but only show IDs
  $ coder exp task list --status running --quiet

OPTIONS:
  -a, --all bool (default: false)
          List tasks for all users you can view.

  -c, --column [id|organization id|owner id|owner name|name|template id|template name|template display name|template icon|workspace id|workspace agent id|workspace agent lifecycle|workspace agent health|initial prompt|status|state|message|created at|updated at|state changed] (default: name,status,state,state changed,message)
          Columns to display in table output.

  -o, --output table|json (default: table)
          Output format.

  -q, --quiet bool (default: false)
          Only display task IDs.

      --status string
          Filter by task status (e.g. running, failed, etc).

      --user string
          List tasks for the specified user (username, "me").
```

### Viewing Task Logs

```bash
USAGE:
  coder exp task logs [flags] <task>

  Show a task's logs

    # Show a task's logs.
  $ coder exp task logs task1

OPTIONS:
  -c, --column [id|content|type|time] (default: type,content)
          Columns to display in table output.

  -o, --output table|json (default: table)
          Output format.
```

### Send input to a task

```bash
USAGE:
  coder exp task send [flags] <task> [<input> | --stdin]

  Send input to a task

# Send input to a task.
  $ coder exp task send task1 "Please also add unit tests"

  # Send input from stdin to a task.
  $ echo "Please also add unit tests" | coder exp task send task1 --stdin

OPTIONS:
      --stdin bool
          Reads the input from stdin.
```

### Viewing Task Status

```bash
USAGE:
  coder exp task status [flags]

  Show the status of a task.

  Aliases: stat

  # Show the status of a given task.
  $ coder exp task status task1

  # Watch the status of a given task until it completes (idle or stopped).
  $ coder exp task status task1 --watch

OPTIONS:
  -c, --column [state changed|status|healthy|state|message] (default: state changed,status,healthy,state,message)
          Columns to display in table output.

  -o, --output table|json (default: table)
          Output format.

      --watch bool (default: false)
          Watch the task status output. This will stream updates to the terminal until the underlying workspace is stopped.
```

> **Note**: The `--watch` flag will automatically exit when the task reaches a terminal state. Watch mode ends when:
>
> - The workspace is stopped
> - The workspace agent becomes unhealthy or is shutting down
> - The task completes (reaches a non-working state like completed, failed, or canceled)

### Task Identification

Tasks can be identified in CLI commands using either:

- **Task Name**: The human-readable name (e.g., `my-task-name`)
    > Note: Tasks owned by other users can be identified by their owner and name (e.g., `alice/her-task`).
- **Task ID**: The UUID identifier (e.g., `550e8400-e29b-41d4-a716-446655440000`)

## Next Steps

<children></children>
