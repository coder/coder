# coder server create-admin-user

Create a new admin user with the given username, email and password and adds it to every organization.
## Usage
```console
coder server create-admin-user [flags]
```

## Local Flags
| Name |  Default | Usage |
| ---- |  ------- | ----- |
| --email |  | <code>The email of the new user. If not specified, you will be prompted via stdin. Consumes $CODER_EMAIL.</code>|
| --password |  | <code>The password of the new user. If not specified, you will be prompted via stdin. Consumes $CODER_PASSWORD.</code>|
| --postgres-url |  | <code>URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case). Consumes $CODER_POSTGRES_URL.</code>|
| --ssh-keygen-algorithm | ed25519 | <code>The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096". Consumes $CODER_SSH_KEYGEN_ALGORITHM.</code>|
| --username |  | <code>The username of the new user. If not specified, you will be prompted via stdin. Consumes $CODER_USERNAME.</code>|