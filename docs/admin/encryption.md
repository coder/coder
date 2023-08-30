# Database Encryption

By default, Coder stores external user tokens in plaintext in the database. This
is undesirable in high-security environments, as an attacker with access to the
database can use these tokens to impersonate users. Database Encryption allows
Coder administrators to encrypt these tokens at-rest, preventing attackers from
using them.

## How it works

Coder allows administrators to specify up to two
[external token encryption keys](../cli/server.md#external-token-encryption-keys).
If configured, Coder will use these keys to encrypt external user tokens before
storing them in the database. The encryption algorithm used is AES-256-GCM with
a 32-byte key length.

Coder will use the first key provided for both encryption and decryption. If a
second key is provided, Coder will use it for decryption only. This allows
administrators to rotate encryption keys without invalidating existing tokens.

The following database fields are currently encrypted:

- `user_links.oauth_access_token`
- `user_links.oauth_refresh_token`
- `git_auth_links.oauth_access_token`
- `git_auth_links.oauth_refresh_token`

Additional database fields may be encrypted in the future.

> Implementation note: there is an additional encrypted database field
> `dbcrypt_sentinel.value`. This field is used to verify that the encryption
> keys are valid for the configured database. It is not used to encrypt any user
> data.

Encrypted data is stored in the following format:

- `encrypted_data = dbcrypt-<b64data>`
- `b64data = <cipher checksum>-<ciphertext>`

All encrypted data is prefixed with the string `dbcrypt-`. The cipher checksum
is the first 7 bytes of the SHA256 hex digest of the encryption key used to
encrypt the data.

## Enabling encryption

1. Ensure you have a valid backup of your database. **Do not skip this step.**
   If you are using the built-in PostgreSQL database, you can run
   [`coder server postgres-builtin-url`](../cli/server_postgres-builtin-url.md)
   to get the connection URL.

1. Generate a 32-byte random key and base64-encode it. For example:

```shell
dd if=/dev/urandom bs=32 count=1 | base64
```

1. Store this key in a secure location (for example, a Kubernetes secret):

```shell
kubectl create secret generate coder-external-token-encryption-keys --from-literal=keys=<key>
```

1. In your Coder configuration set the `external_token_encryption_keys` field to
   a comma-separated list of base64-encoded keys. For example, in your Helm
   `values.yaml`:

```yaml
coder:
  env:
    [...]
    - name: CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS
      valueFrom:
        secretKeyRef:
          name: coder-external-token-encryption-keys
          key: keys
```

## Rotating keys

We recommend only having one active encryption key at a time normally. However,
if you need to rotate keys, you can perform the following procedure:

1. Ensure you have a valid backup of your database. **Do not skip this step.**

1. Generate a new encryption key following the same procedure as above.

1. Add the above key to the list of
   [external token encryption keys](../cli/server.md#external-token-encryption-keys).
   **The new key must appear first in the list**. For example, in the Kubernetes
   secret created above:

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: coder-external-token-encryption-keys
  namespace: coder-namespace
data:
  keys: <new-key>,<old-key>
```

1. After updating the configuration, restart the Coder server. The server will
   now encrypt all new data with the new key, but will be able to decrypt tokens
   encrypted with the old key.

1. To re-encrypt all encrypted database fields with the new key, run
   [`coder dbcrypt-rotate`](../cli/dbcrypt-rotate.md). This command will
   re-encrypt all tokens with the first key in the list of external token
   encryption keys. We recommend performing this action during a maintenance
   window.

   > Note: this command requires direct access to the database. If you are using
   > the built-in PostgreSQL database, you can run
   > [`coder server postgres-builtin-url`](../cli/server_postgres-builtin-url.md)
   > to get the connection URL.

1. Once the above command completes successfully, remove the old encryption key
   from Coder's configuration and restart Coder once more. You can now safely
   delete the old key from your secret store.

## Disabling encryption

Automatically disabling encryption is currently not supported. Encryption can be
disabled by removing the encrypted data manually from the database:

```sql
DELETE FROM user_links WHERE oauth_access_token LIKE 'dbcrypt-%';
DELETE FROM user_links WHERE oauth_refresh_token LIKE 'dbcrypt-%';
DELETE FROM git_auth_links WHERE oauth_access_token LIKE 'dbcrypt-%';
DELETE FROM git_auth_links WHERE oauth_refresh_token LIKE 'dbcrypt-%';
DELETE FROM dbcrypt_sentinel WHERE value LIKE 'dbcrypt-%';
```

Users will then need to re-authenticate with external authentication providers.

## Troubleshooting

- If Coder detects that the data stored in the database under
  `dbcrypt_sentinel.value` was not encrypted with a known key, it will refuse to
  start. If you are seeing this behaviour, ensure that the encryption keys
  provided are correct.
- If Coder is unable to decrypt a token, it will be treated as if the data were
  not present. This means that the user will be prompted to re-authenticate with
  the external provider. If you are seeing this behaviour consistently, ensure
  that the encryption keys are correct.
