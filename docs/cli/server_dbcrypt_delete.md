<!-- DO NOT EDIT | GENERATED CONTENT -->

# server dbcrypt delete

Delete all encrypted data from the database. THIS IS A DESTRUCTIVE OPERATION.

Aliases:

- rm

## Usage

```console
coder server dbcrypt delete [flags]
```

## Options

### --postgres-url

|             |                                                            |
| ----------- | ---------------------------------------------------------- |
| Type        | <code>string</code>                                        |
| Environment | <code>$CODER_EXTERNAL_TOKEN_ENCRYPTION_POSTGRES_URL</code> |

The connection URL for the Postgres database.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
