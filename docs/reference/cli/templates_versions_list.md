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
| ---- | ----------------- |
| Type | <code>bool</code> |

Include archived versions in the result list.

### -c, --column

|         |                                                       |
| ------- | ----------------------------------------------------- |
| Type    | <code>string-array</code>                             |
| Default | <code>Name,Created At,Created By,Status,Active</code> |

Columns to display in table output. Available columns: name, created at, created by, status, active, archived.

### -o, --output

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>table</code>  |

Output format. Available formats: table, json.
