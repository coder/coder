<!-- DO NOT EDIT | GENERATED CONTENT -->

# server dbcrypt rotate

Rotate database encryption keys.

## Usage

```console
coder server dbcrypt rotate [flags]
```

## Options

### --new-key

|             |                                                               |
| ----------- | ------------------------------------------------------------- |
| Type        | <code>string</code>                                           |
| Environment | <code>$CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_NEW_KEY</code> |

The new external token encryption key. Must be base64-encoded.

### --old-keys

|             |                                                                |
| ----------- | -------------------------------------------------------------- |
| Type        | <code>string-array</code>                                      |
| Environment | <code>$CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_OLD_KEYS</code> |

The old external token encryption keys. Must be a comma-separated list of base64-encoded keys.

### --postgres-url

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_PG_CONNECTION_URL</code> |

The connection URL for the Postgres database.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
