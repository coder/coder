<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder server create-admin-user

Create a new admin user with the given username, email and password and adds it to every organization.

## Usage

```console
coder server create-admin-user [flags]
```

## Flags

### --email

The email of the new user. If not specified, you will be prompted via stdin.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_EMAIL</code> |

### --password

The password of the new user. If not specified, you will be prompted via stdin.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PASSWORD</code> |

### --postgres-url

URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case).
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_POSTGRES_URL</code> |

### --ssh-keygen-algorithm

The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SSH_KEYGEN_ALGORITHM</code> |
| Default | <code>ed25519</code> |

### --username

The username of the new user. If not specified, you will be prompted via stdin.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_USERNAME</code> |
