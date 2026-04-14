<!-- DO NOT EDIT | GENERATED CONTENT -->
# secret update

Update a secret

## Usage

```console
coder secret update [flags] <name>
```

## Description

```console
At least one of --value, --description, --env, or --file must be specified. Provide the secret value by at most one of --value or non-interactive stdin (pipe or redirect).
```

## Options

### --value

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the secret value. For security reasons, prefer non-interactive stdin (pipe or redirect).

### --description

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the secret description. Pass an empty string to clear it.

### --env

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Name of the workspace environment variable that this secret will set. Pass an empty string to clear it.

### --file

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Workspace file path where this secret will be written. Must start with ~/ or /. Pass an empty string to clear it.
