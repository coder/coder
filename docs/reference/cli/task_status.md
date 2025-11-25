<!-- DO NOT EDIT | GENERATED CONTENT -->
# task status

Show the status of a task.

Aliases:

* stat

## Usage

```console
coder task status [flags]
```

## Description

```console
  - Show the status of a given task.:

     $ coder task status task1

  - Watch the status of a given task until it completes (idle or stopped).:

     $ coder task status task1 --watch
```

## Options

### --watch

|         |                    |
|---------|--------------------|
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Watch the task status output. This will stream updates to the terminal until the underlying workspace is stopped.

### -c, --column

|         |                                                                                                                                                                                                                                                                                                                                                                                                                                  |
|---------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Type    | <code>[id\|organization id\|owner id\|owner name\|owner avatar url\|name\|template id\|template version id\|template name\|template display name\|template icon\|workspace id\|workspace name\|workspace status\|workspace build number\|workspace agent id\|workspace agent lifecycle\|workspace agent health\|workspace app id\|initial prompt\|status\|state\|message\|created at\|updated at\|state changed\|healthy]</code> |
| Default | <code>state changed,status,healthy,state,message</code>                                                                                                                                                                                                                                                                                                                                                                          |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
