<!-- DO NOT EDIT | GENERATED CONTENT -->
# server regenerate-vapid-keypair

Regenerate the VAPID keypair used for push notifications.

## Usage

```console
coder server regenerate-vapid-keypair [flags]
```

## Options

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass prompts.

### --postgres-url

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_PG_CONNECTION_URL</code> |

URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case).

### --postgres-connection-auth

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>password\|awsiamrds</code>       |
| Environment | <code>$CODER_PG_CONNECTION_AUTH</code> |
| Default     | <code>password</code>                  |

Type of auth to use when connecting to postgres.
