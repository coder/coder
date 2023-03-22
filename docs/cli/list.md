<!-- DO NOT EDIT | GENERATED CONTENT -->

# list

List workspaces

Aliases:

- ls

## Usage

```console
coder list [flags]
```

## Options

### -a, --all

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Specifies whether all workspaces will be listed or not.

### -c, --column

|         |                                                                                  |
| ------- | -------------------------------------------------------------------------------- |
| Type    | <code>string-array</code>                                                        |
| Default | <code>workspace,template,status,last built,outdated,starts at,stops after</code> |

Columns to display in table output. Available columns: workspace, template, status, last built, outdated, starts at, stops after.

### -o, --output

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>table</code>  |

Output format. Available formats: table, json.

### --search

|         |                       |
| ------- | --------------------- |
| Type    | <code>string</code>   |
| Default | <code>owner:me</code> |

Search for a workspace with a query.
