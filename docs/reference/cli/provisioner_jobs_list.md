<!-- DO NOT EDIT | GENERATED CONTENT -->
# provisioner jobs list

List provisioner jobs

Aliases:

* ls

## Usage

```console
coder provisioner jobs list [flags]
```

## Options

### -s, --status

|             |                                                                                  |
|-------------|----------------------------------------------------------------------------------|
| Type        | <code>[pending\|running\|succeeded\|canceling\|canceled\|failed\|unknown]</code> |
| Environment | <code>$CODER_PROVISIONER_JOB_LIST_STATUS</code>                                  |

Filter by job status.

### -l, --limit

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>int</code>                               |
| Environment | <code>$CODER_PROVISIONER_JOB_LIST_LIMIT</code> |
| Default     | <code>50</code>                                |

Limit the number of jobs returned.

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.

### -c, --column

|         |                                                                                                                                                                                                                                                                                                                                                                                      |
|---------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Type    | <code>[id\|created at\|started at\|completed at\|canceled at\|error\|error code\|status\|worker id\|file id\|tags\|queue position\|queue size\|organization id\|template version id\|workspace build id\|type\|available workers\|template version name\|template id\|template name\|template display name\|template icon\|workspace id\|workspace name\|organization\|queue]</code> |
| Default | <code>id,created at,status,tags,type,organization,queue</code>                                                                                                                                                                                                                                                                                                                       |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
