<!-- DO NOT EDIT | GENERATED CONTENT -->
# secret create

Create a secret

## Usage

```console
coder secret create [flags] <name>
```

## Description

```console
Provide the secret value with --value or non-interactive stdin (pipe or redirect).
```

## Options

### --value

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Set the secret value. For security reasons, prefer non-interactive stdin (pipe or redirect).

### --description

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Set the secret description.

### --env

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Name of the workspace environment variable that this secret will set.

### --file

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Workspace file path where this secret will be written. Must start with ~/ or /.
