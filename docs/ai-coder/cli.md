# Tasks CLI

The Coder CLI provides experimental commands for managing tasks programmatically. These are available under `coder exp task`:

```console
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

## Creating tasks

```console
USAGE:
  coder exp task create [flags] [input]

  Create an experimental task

    - Create a task with direct input:

        $ coder exp task create "Add authentication to the user service"

    - Create a task with stdin input:

        $ echo "Add authentication to the user service" | coder exp task create

    - Create a task with a specific name:

        $ coder exp task create --name task1 "Add authentication to the user service"

    - Create a task from a specific template / preset:

        $ coder exp task create --template backend-dev --preset "My Preset" "Add authentication to the user service"

    - Create a task for another user (requires appropriate permissions):

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

## Deleting Tasks

```console
USAGE:
  coder exp task delete [flags] <task> [<task> ...]

  Delete experimental tasks

  Aliases: rm

    - Delete a single task.:

        $ $ coder exp task delete task1

    - Delete multiple tasks.:

        $ $ coder exp task delete task1 task2 task3

    - Delete a task without confirmation.:

        $ $ coder exp task delete task4 --yes

OPTIONS:
  -y, --yes bool
          Bypass prompts.
```

## Listing tasks

```console
USAGE:
  coder exp task list [flags]

  List experimental tasks

  Aliases: ls

    - List tasks for the current user.:

        $ coder exp task list

    - List tasks for a specific user.:

        $ coder exp task list --user someone-else

    - List all tasks you can view.:

        $ coder exp task list --all

    - List all your running tasks.:

        $ coder exp task list --status running

    - As above, but only show IDs.:

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

## Viewing Task Logs

```console
USAGE:
  coder exp task logs [flags] <task>

  Show a task's logs

    - Show logs for a given task.:

        $ coder exp task logs task1

OPTIONS:
  -c, --column [id|content|type|time] (default: type,content)
          Columns to display in table output.

  -o, --output table|json (default: table)
          Output format.
```

## Sending input to a task

```console
USAGE:
  coder exp task send [flags] <task> [<input> | --stdin]

  Send input to a task

    - Send direct input to a task.:

        $ coder exp task send task1 "Please also add unit tests"

    - Send input from stdin to a task.:

        $ echo "Please also add unit tests" | coder exp task send task1 --stdin

OPTIONS:
      --stdin bool
          Reads the input from stdin.
```

## Viewing Task Status

```console
USAGE:
  coder exp task status [flags]

  Show the status of a task.

  Aliases: stat

    - Show the status of a given task.:

        $ coder exp task status task1

    - Watch the status of a given task until it completes (idle or stopped).:

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

## Identifying Tasks

Tasks can be identified in CLI commands using either:

- **Task Name**: The human-readable name (e.g., `my-task-name`)
    > Note: Tasks owned by other users can be identified by their owner and name (e.g., `alice/her-task`).
- **Task ID**: The UUID identifier (e.g., `550e8400-e29b-41d4-a716-446655440000`)
