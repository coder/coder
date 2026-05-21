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
|-------------|------------------------------------|
| Type        | <code>string</code>                |
| Environment | <code>$CODER_TOKEN_LIFETIME</code> |

Duration for the token lifetime. Supports standard Go duration units (ns, us, ms, s, m, h) plus d (days) and y (years). Examples: 8h, 30d, 1y, 1d12h30m.

### -n, --name

|             |                                |
|-------------|--------------------------------|
| Type        | <code>string</code>            |
| Environment | <code>$CODER_TOKEN_NAME</code> |

Specify a human-readable name.

### -u, --user

|             |                                |
|-------------|--------------------------------|
| Type        | <code>string</code>            |
| Environment | <code>$CODER_TOKEN_USER</code> |

Specify the user to create the token for (Only works if logged in user is admin).

### --scope

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

Repeatable scope to attach to the token (e.g. workspace:read).

### --allow

|      |                         |
|------|-------------------------|
| Type | <code>allow-list</code> |

Repeatable allow-list entry (<type>:<uuid>, e.g. workspace:1234-...).
