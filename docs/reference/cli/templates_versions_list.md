<!-- DO NOT EDIT | GENERATED CONTENT -->
# templates versions list

List all the versions of the specified template

## Usage

```console
coder templates versions list [flags] <template>
```

## Options

### --include-archived

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Include archived versions in the result list.

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.

### -c, --column

|         |                                                                       |
|---------|-----------------------------------------------------------------------|
| Type    | <code>[name\|created at\|created by\|status\|active\|archived]</code> |
| Default | <code>name,created at,created by,status,active</code>                 |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
