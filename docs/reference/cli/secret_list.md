<!-- DO NOT EDIT | GENERATED CONTENT -->
# secret list

List secrets, or show one by name

Aliases:

* ls

## Usage

```console
coder secret list [flags] [name]
```

## Description

```console
Secret values are omitted from the output.
```

## Options

### -c, --column

|         |                                                               |
|---------|---------------------------------------------------------------|
| Type    | <code>[created\|name\|updated\|env\|file\|description]</code> |
| Default | <code>name,created,updated,env,file,description</code>        |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
