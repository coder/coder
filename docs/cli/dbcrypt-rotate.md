<!-- DO NOT EDIT | GENERATED CONTENT -->

# dbcrypt-rotate

Rotate database encryption keys

## Usage

```console
coder dbcrypt-rotate [flags] --postgres-url <postgres_url> --external-token-encryption-keys <new-key>,<old-keys>
```

## Options

### --external-token-encryption-keys

|             |                                                    |
| ----------- | -------------------------------------------------- |
| Type        | <code>string-array</code>                          |
| Environment | <code>$CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS</code> |

Encrypt OIDC and Git authentication tokens with AES-256-GCM in the database. The value must be a comma-separated list of base64-encoded keys. Each key, when base64-decoded, must be exactly 32 bytes in length. The first key will be used to encrypt new values. Subsequent keys will be used as a fallback when decrypting. During normal operation it is recommended to only set one key.

### --postgres-url

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_PG_CONNECTION_URL</code> |

URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".
