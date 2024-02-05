<!-- DO NOT EDIT | GENERATED CONTENT -->

# schedule show

Show workspace schedules

## Usage

```console
coder schedule show [flags] <workspace | --search <query> | --all>
```

## Description

```console
Shows the following information for the given workspace(s):
  * The automatic start schedule
  * The next scheduled start time
  * The duration after which it will stop
  * The next scheduled stop time

```

## Options

### -a, --all

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Specifies whether all workspaces will be listed or not.

### -c, --column

|         |                                                                     |
| ------- | ------------------------------------------------------------------- |
| Type    | <code>string-array</code>                                           |
| Default | <code>workspace,starts at,starts next,stops after,stops next</code> |

Columns to display in table output. Available columns: workspace, starts at, starts next, stops after, stops next.

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
