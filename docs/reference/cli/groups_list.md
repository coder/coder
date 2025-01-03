<!-- DO NOT EDIT | GENERATED CONTENT -->
# groups list

List user groups

## Usage

```console
coder groups list [flags]
```

## Options

### -c, --column

|         |                                                                         |
|---------|-------------------------------------------------------------------------|
| Type    | <code>[name\|display name\|organization id\|members\|avatar url]</code> |
| Default | <code>name,display name,organization id,members,avatar url</code>       |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.
