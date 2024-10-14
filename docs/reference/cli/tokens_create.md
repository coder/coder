<!-- DO NOT EDIT | GENERATED CONTENT -->

# tokens create

Create a token

## Usage

```console
coder tokens create [flags]
```

## Options

### --lifetime

|             |                                    |
| ----------- | ---------------------------------- |
| Type        | <code>duration</code>              |
| Environment | <code>$CODER_TOKEN_LIFETIME</code> |
| Default     | <code>720h0m0s</code>              |

Specify a duration for the lifetime of the token.

### -n, --name

|             |                                |
| ----------- | ------------------------------ |
| Type        | <code>string</code>            |
| Environment | <code>$CODER_TOKEN_NAME</code> |

Specify a human-readable name.

### -u, --user

|             |                                |
| ----------- | ------------------------------ |
| Type        | <code>string</code>            |
| Environment | <code>$CODER_TOKEN_USER</code> |

Specify the user to create the token for (Only works if logged in user is admin).
