<!-- DO NOT EDIT | GENERATED CONTENT -->
# secret update

Update a secret

## Usage

```console
coder secret update [flags] <name>
```

## Description

```console
At least one of --value, --value-env, --description, --env, or --file must be specified. Provide the secret value by at most one of --value, --value-env, or non-interactive stdin (pipe or redirect).
```

## Options

### --value

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the secret value. For security reasons, prefer --value-env or non-interactive stdin (pipe or redirect).

### --value-env

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Read the updated secret value from the named environment variable.

### --trim-trailing-newline

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Trim a single trailing newline from stdin-provided secret values.

### --description

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the secret description. Pass an empty string to clear it.

### --env

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the environment variable injection target. Pass an empty string to clear it.

### --file

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the file injection target. Pass an empty string to clear it.
