<!-- DO NOT EDIT | GENERATED CONTENT -->

# server dbcrypt decrypt

Decrypt a previously encrypted database.

## Usage

```console
coder server dbcrypt decrypt [flags]
```

## Options

### --postgres-url

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_PG_CONNECTION_URL</code> |

The connection URL for the Postgres database.

### --keys

|             |                                                            |
| ----------- | ---------------------------------------------------------- |
| Type        | <code>string-array</code>                                  |
| Environment | <code>$CODER_EXTERNAL_TOKEN_ENCRYPTION_DECRYPT_KEYS</code> |

Keys required to decrypt existing data. Must be a comma-separated list of base64-encoded keys.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
