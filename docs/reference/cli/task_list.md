<!-- DO NOT EDIT | GENERATED CONTENT -->
# task list

List tasks

Aliases:

* ls

## Usage

```console
coder task list [flags]
```

## Description

```console
  - List tasks for the current user.:

     $ coder task list

  - List tasks for a specific user.:

     $ coder task list --user someone-else

  - List all tasks you can view.:

     $ coder task list --all

  - List all your running tasks.:

     $ coder task list --status running

  - As above, but only show IDs.:

     $ coder task list --status running --quiet
```

## Options

### --status

|      |                                                                    |
|------|--------------------------------------------------------------------|
| Type | <code>pending\|initializing\|active\|paused\|error\|unknown</code> |

Filter by task status.

### -a, --all

|         |                    |
|---------|--------------------|
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

List tasks for all users you can view.

### --user

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

List tasks for the specified user (username, "me").

### -q, --quiet

|         |                    |
|---------|--------------------|
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Only display task IDs.

### -c, --column

|         |                                                                                                                                                                                                                                                                                                                                                                                                                                       |
|---------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Type    | <code>[id\|organization id\|owner id\|owner name\|owner avatar url\|name\|display name\|template id\|template version id\|template name\|template display name\|template icon\|workspace id\|workspace name\|workspace status\|workspace build number\|workspace agent id\|workspace agent lifecycle\|workspace agent health\|workspace app id\|initial prompt\|status\|state\|message\|created at\|updated at\|state changed]</code> |
| Default | <code>name,status,state,state changed,message</code>                                                                                                                                                                                                                                                                                                                                                                                  |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
