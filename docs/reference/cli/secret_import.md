<!-- DO NOT EDIT | GENERATED CONTENT -->
# secret import

Import secrets from a file

## Usage

```console
coder secret import [flags] <file>
```

## Description

```console
Every key in the file becomes a secret that is injected as an environment variable of the same name. The import is all or nothing, and existing secrets are never overwritten. Pass - to read the file from stdin.
```

## Options

### --input-format

|      |                              |
|------|------------------------------|
| Type | <code>env\|json\|yaml</code> |

Format of the secrets file. Inferred from the file extension when unset, and required when reading from stdin.
