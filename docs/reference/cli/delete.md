<!-- DO NOT EDIT | GENERATED CONTENT -->

# delete

Delete a workspace

Aliases:

- rm

## Usage

```console
coder delete [flags] <workspace>
```

## Options

### --orphan

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Delete a workspace without deleting its resources. This can delete a workspace in a broken state, but may also lead to unaccounted cloud resources.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.

### --debug-provisioner

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Sets the provisioner log level to debug.<br/>This will print additional information about the build process.<br/>This is useful for troubleshooting build issues.
