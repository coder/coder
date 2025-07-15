<!-- DO NOT EDIT | GENERATED CONTENT -->
# delete

Delete a workspace

Aliases:

* rm

## Usage

```console
coder delete [flags] <workspace>
```

## Description

```console
  - Delete a workspace for another user (if you have permission):

     $ coder delete <username>/<workspace_name>
```

## Options

### --orphan

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Delete a workspace without deleting its resources. This can delete a workspace in a broken state, but may also lead to unaccounted cloud resources.

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass prompts.
