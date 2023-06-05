<!-- DO NOT EDIT | GENERATED CONTENT -->

# server create-admin-user

Create a new admin user with the given username, email and password and adds it to every organization.

## Usage

```console
coder server create-admin-user [flags]
```

## Options

### --email

|             |                           |
| ----------- | ------------------------- |
| Type        | <code>string</code>       |
| Environment | <code>$CODER_EMAIL</code> |

The email of the new user. If not specified, you will be prompted via stdin.

### --password

|             |                              |
| ----------- | ---------------------------- |
| Type        | <code>string</code>          |
| Environment | <code>$CODER_PASSWORD</code> |

The password of the new user. If not specified, you will be prompted via stdin.

### --postgres-url

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_PG_CONNECTION_URL</code> |

URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case).

### --raw-url

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Output the raw connection URL instead of a psql command.

### --ssh-keygen-algorithm

|             |                                          |
| ----------- | ---------------------------------------- |
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_SSH_KEYGEN_ALGORITHM</code> |
| Default     | <code>ed25519</code>                     |

The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".

### --username

|             |                              |
| ----------- | ---------------------------- |
| Type        | <code>string</code>          |
| Environment | <code>$CODER_USERNAME</code> |

The username of the new user. If not specified, you will be prompted via stdin.
