<!-- DO NOT EDIT | GENERATED CONTENT -->
# provisioner list

List provisioner daemons in an organization

Aliases:

* ls

## Usage

```console
coder provisioner list [flags]
```

## Options

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.

### -c, --column

|         |                                                                                                                                                                                                                                                                                                                                                                                               |
|---------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Type    | <code>[id\|organization id\|created at\|last seen at\|name\|version\|api version\|tags\|key name\|status\|current job id\|current job status\|current job template name\|current job template icon\|current job template display name\|previous job id\|previous job status\|previous job template name\|previous job template icon\|previous job template display name\|organization]</code> |
| Default | <code>name,organization,status,key name,created at,last seen at,version,tags</code>                                                                                                                                                                                                                                                                                                           |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
