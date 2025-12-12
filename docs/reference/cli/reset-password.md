<!-- DO NOT EDIT | GENERATED CONTENT -->
# reset-password

Directly connect to the database to reset a user's password

## Usage

```console
coder reset-password [flags] <username>
```

## Options

### --postgres-url

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_PG_CONNECTION_URL</code> |

URL of a PostgreSQL database to connect to.

### --postgres-url-file

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>string</code>                        |
| Environment | <code>$CODER_PG_CONNECTION_URL_FILE</code> |

Path to a file containing the URL of a PostgreSQL database. The file contents will be read and used as the connection URL. This is an alternative to --postgres-url for cases where the URL is stored in a file, such as a Docker or Kubernetes secret.

### --postgres-connection-auth

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>password\|awsiamrds</code>       |
| Environment | <code>$CODER_PG_CONNECTION_AUTH</code> |
| Default     | <code>password</code>                  |

Type of auth to use when connecting to postgres.
