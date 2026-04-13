<!-- DO NOT EDIT | GENERATED CONTENT -->
# secret update

Update a secret

## Usage

```console
coder secret update [flags] <name>
```

## Description

```console
At least one of --value, --description, --inject-env, or --inject-file must be specified.
```

## Options

### --value

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the secret value.

### --description

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the secret description. Pass an empty string to clear it.

### --inject-env

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the environment variable injection target. Pass an empty string to clear it.

### --inject-file

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the file injection target. Pass an empty string to clear it.
