<!-- DO NOT EDIT | GENERATED CONTENT -->
# organizations roles update

Update an organization custom role

## Usage

```console
coder organizations roles update [flags] <role_name>
```

## Description

```console
  - Run with an input.json file:

     $ coder roles update --stdin < role.json
```

## Options

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass prompts.

### --dry-run

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Does all the work, but does not submit the final updated role.

### --stdin

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Reads stdin for the json role definition to upload.

### -c, --column

|         |                                                                                                                  |
|---------|------------------------------------------------------------------------------------------------------------------|
| Type    | <code>[name\|display name\|organization id\|site permissions\|organization permissions\|user permissions]</code> |
| Default | <code>name,display name,site permissions,organization permissions,user permissions</code>                        |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
