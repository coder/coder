<!-- DO NOT EDIT | GENERATED CONTENT -->
# organizations roles create

Create a new organization custom role

## Usage

```console
coder organizations roles create [flags] <role_name>
```

## Description

```console
  - Run with an input.json file:

     $ coder organization -O <organization_name> roles create --stidin < role.json
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
