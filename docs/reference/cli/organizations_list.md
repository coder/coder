<!-- DO NOT EDIT | GENERATED CONTENT -->
# organizations list

List all organizations

Aliases:

* ls

## Usage

```console
coder organizations list [flags]
```

## Description

```console
List all organizations. Requires a role which grants ResourceOrganization: read.
```

## Options

### -c, --column

|         |                                                                                           |
|---------|-------------------------------------------------------------------------------------------|
| Type    | <code>[id\|name\|display name\|icon\|description\|created at\|updated at\|default]</code> |
| Default | <code>name,display name,id,default</code>                                                 |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
