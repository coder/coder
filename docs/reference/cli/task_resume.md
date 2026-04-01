<!-- DO NOT EDIT | GENERATED CONTENT -->
# task resume

Resume a task

## Usage

```console
coder task resume [flags] <task>
```

## Description

```console
  - Resume a task by name:

     $ coder task resume my-task

  - Resume another user's task:

     $ coder task resume alice/my-task

  - Resume a task without confirmation:

     $ coder task resume my-task --yes
```

## Options

### --no-wait

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Return immediately after resuming the task.

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass confirmation prompts.
