# Database Encryption

By default, Coder stores external user tokens in plaintext in the database.
Database Encryption allows Coder administrators to encrypt these tokens at-rest,
preventing attackers with database access from using them to impersonate users.

## How it works

Coder allows administrators to specify
[external token encryption keys](../../reference/cli/server.md#--external-token-encryption-keys).
If configured, Coder will use these keys to encrypt external user tokens before
storing them in the database. The encryption algorithm used is AES-256-GCM with
a 32-byte key length.

Coder will use the first key provided for both encryption and decryption. If
additional keys are provided, Coder will use it for decryption only. This allows
administrators to rotate encryption keys without invalidating existing tokens.

The following database fields are currently encrypted:

- `user_links.oauth_access_token`
- `user_links.oauth_refresh_token`
- `external_auth_links.oauth_access_token`
- `external_auth_links.oauth_refresh_token`
- `crypto_keys.secret`

Additional database fields may be encrypted in the future.

> Implementation notes: each encrypted database column `$C` has a corresponding
> `$C_key_id` column. This column is used to determine which encryption key was
> used to encrypt the data. This allows Coder to rotate encryption keys without
> invalidating existing tokens, and provides referential integrity for encrypted
> data.
>
> The `$C_key_id` column stores the first 7 bytes of the SHA-256 hash of the
> encryption key used to encrypt the data.
>
> Encryption keys in use are stored in `dbcrypt_keys`. This table stores a
> record of all encryption keys that have been used to encrypt data. Active keys
> have a null `revoked_key_id` column, and revoked keys have a non-null
> `revoked_key_id` column. You cannot revoke a key until you have rotated all
> values using that key to a new key.

## Enabling encryption

> NOTE: Enabling encryption does not encrypt all existing data. To encrypt
> existing data, see [rotating keys](#rotating-keys) below.

- Ensure you have a valid backup of your database. **Do not skip this step.** If
  you are using the built-in PostgreSQL database, you can run
  [`coder server postgres-builtin-url`](../../reference/cli/server_postgres-builtin-url.md)
  to get the connection URL.

- Generate a 32-byte random key and base64-encode it. For example:

```shell
dd if=/dev/urandom bs=32 count=1 | base64
```

- Store this key in a secure location (for example, a Kubernetes secret):

```shell
kubectl create secret generic coder-external-token-encryption-keys --from-literal=keys=<key>
```

- In your Coder configuration set `CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS` to a
  comma-separated list of base64-encoded keys. For example, in your Helm
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

- Restart the Coder server. The server will now encrypt all new data with the
  provided key.

## Rotating keys

We recommend only having one active encryption key at a time normally. However,
if you need to rotate keys, you can perform the following procedure:

- Ensure you have a valid backup of your database. **Do not skip this step.**

- Generate a new encryption key following the same procedure as above.

- Add the above key to the list of
  [external token encryption keys](../../reference/cli/server.md#--external-token-encryption-keys).
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
  keys: <new-key>,<old-key1>,<old-key2>,...
```

- After updating the configuration, restart the Coder server. The server will
  now encrypt all new data with the new key, but will be able to decrypt tokens
  encrypted with the old key(s).

- To re-encrypt all encrypted database fields with the new key, run
  [`coder server dbcrypt rotate`](../../reference/cli/server_dbcrypt_rotate.md).
  This command will re-encrypt all tokens with the specified new encryption key.
  We recommend performing this action during a maintenance window.

  > Note: this command requires direct access to the database. If you are using
  > the built-in PostgreSQL database, you can run
  > [`coder server postgres-builtin-url`](../../reference/cli/server_postgres-builtin-url.md)
  > to get the connection URL.

- Once the above command completes successfully, remove the old encryption key
  from Coder's configuration and restart Coder once more. You can now safely
  delete the old key from your secret store.

## Disabling encryption

To disable encryption, perform the following actions:

- Ensure you have a valid backup of your database. **Do not skip this step.**

- Stop all active coderd instances. This will prevent new encrypted data from
  being written, which may cause the next step to fail.

- Run
  [`coder server dbcrypt decrypt`](../../reference/cli/server_dbcrypt_decrypt.md).
  This command will decrypt all encrypted user tokens and revoke all active
  encryption keys.

  > Note: for `decrypt` command, the equivalent environment variable for
  > `--keys` is `CODER_EXTERNAL_TOKEN_ENCRYPTION_DECRYPT_KEYS` and not
  > `CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS`. This is explicitly named differently
  > to help prevent accidentally decrypting data.

- Remove all
  [external token encryption keys](../../reference/cli/server.md#--external-token-encryption-keys)
  from Coder's configuration.

- Start coderd. You can now safely delete the encryption keys from your secret
  store.

## Deleting Encrypted Data

> NOTE: This is a destructive operation.

To delete all encrypted data from your database, perform the following actions:

- Ensure you have a valid backup of your database. **Do not skip this step.**

- Stop all active coderd instances. This will prevent new encrypted data from
  being written.

- Run
  [`coder server dbcrypt delete`](../../reference/cli/server_dbcrypt_delete.md).
  This command will delete all encrypted user tokens and revoke all active
  encryption keys.

- Remove all
  [external token encryption keys](../../reference/cli/server.md#--external-token-encryption-keys)
  from Coder's configuration.

- Start coderd. You can now safely delete the encryption keys from your secret
  store.

## Troubleshooting

- If Coder detects that the data stored in the database was not encrypted with
  any known keys, it will refuse to start. If you are seeing this behavior,
  ensure that the encryption keys provided are correct.
- If Coder detects that the data stored in the database was encrypted with a key
  that is no longer active, it will refuse to start. If you are seeing this
  behavior, ensure that the encryption keys provided are correct and that you
  have not revoked any keys that are still in use.
- Decryption may fail if newly encrypted data is written while decryption is in
  progress. If this happens, ensure that all active coder instances are stopped,
  and retry.

## Next steps

- [Security - best practices](../../tutorials/best-practices/security-best-practices.md)
