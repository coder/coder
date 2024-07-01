<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates list

List all the templates available for the organization

Aliases:

- ls

## Usage

```console
coder templates list [flags]
```

## Options

### -c, --column

|         |                                        |
| ------- | -------------------------------------- |
| Type    | <code>string-array</code>              |
| Default | <code>name,last updated,used by</code> |

Columns to display in table output. Available columns: name, created at, last updated, organization id, organization name, provisioner, active version id, used by, default ttl.

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
