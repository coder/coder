<!-- DO NOT EDIT | GENERATED CONTENT -->

# groups list

List user groups

## Usage

```console
coder groups list [flags]
```

## Options

### -c, --column

|         |                                                                   |
| ------- | ----------------------------------------------------------------- |
| Type    | <code>string-array</code>                                         |
| Default | <code>name,display name,organization id,members,avatar url</code> |

Columns to display in table output. Available columns: name, display name,
organization id, members, avatar url.

### -o, --output

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>table</code>  |

Output format. Available formats: table, json.

### -O, --org

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.
