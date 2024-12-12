<!-- DO NOT EDIT | GENERATED CONTENT -->
# tokens list

List tokens

Aliases:

* ls

## Usage

```console
coder tokens list [flags]
```

## Options

### -a, --all

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Specifies whether all users' tokens will be listed or not (must have Owner role to see all tokens).

### -c, --column

|         |                                                                   |
|---------|-------------------------------------------------------------------|
| Type    | <code>[id\|name\|last used\|expires at\|created at\|owner]</code> |
| Default | <code>id,name,last used,expires at,created at</code>              |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
