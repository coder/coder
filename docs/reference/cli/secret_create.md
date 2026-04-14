<!-- DO NOT EDIT | GENERATED CONTENT -->
# secret create

Create a secret

## Usage

```console
coder secret create [flags] <name>
```

## Description

```console
Provide the secret value with --value, --value-env, or non-interactive stdin (pipe or redirect).
```

## Options

### --value

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Set the secret value. For security reasons, prefer --value-env or non-interactive stdin (pipe or redirect).

### --value-env

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Read the secret value from the named environment variable.

### --trim-trailing-newline

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Trim a single trailing newline from stdin-provided secret values.

### --description

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Set the secret description.

### --env

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Inject the secret into workspaces as an environment variable.

### --file

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Inject the secret into workspaces as a file.
