<!-- DO NOT EDIT | GENERATED CONTENT -->
# state push

Push a Terraform state file to a workspace.

## Usage

```console
coder state push [flags] <workspace> <file>
```

## Options

### -b, --build

|      |                  |
|------|------------------|
| Type | <code>int</code> |

Specify a workspace build to target by name. Defaults to latest.

### -n, --no-build

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Update the state without triggering a workspace build. Useful for state-only migrations.
