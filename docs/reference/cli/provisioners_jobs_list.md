<!-- DO NOT EDIT | GENERATED CONTENT -->

# provisioners jobs list

List provisioner jobs

Aliases:

- ls

## Usage

```console
coder provisioners jobs list [flags]
```

## Options

### -s, --status

|             |                                                                                  |
| ----------- | -------------------------------------------------------------------------------- |
| Type        | <code>[pending\|running\|succeeded\|canceling\|canceled\|failed\|unknown]</code> |
| Environment | <code>$CODER_PROVISIONER_JOB_LIST_STATUS</code>                                  |

Filter by job status.

### -l, --limit

|             |                                                |
| ----------- | ---------------------------------------------- |
| Type        | <code>int</code>                               |
| Environment | <code>$CODER_PROVISIONER_JOB_LIST_LIMIT</code> |

Limit the number of jobs returned.

### -O, --org

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.

### -c, --column

|         |                                                                                                                                                                                                                                                               |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Type    | <code>[id\|created at\|started at\|completed at\|canceled at\|error\|error code\|status\|worker id\|file id\|tags\|queue position\|queue size\|organization id\|template version id\|workspace build id\|type\|available workers\|organization\|queue]</code> |
| Default | <code>created at,id,organization,status,type,queue,tags</code>                                                                                                                                                                                                |

Columns to display in table output.

### -o, --output

|         |                          |
| ------- | ------------------------ |
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
