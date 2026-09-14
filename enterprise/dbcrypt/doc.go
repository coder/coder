// Package dbcrypt provides a database.Store wrapper that encrypts/decrypts
// values stored at rest in the database.
//
// Encryption is done using Ciphers, which is an abstraction over a set of
// encryption keys. Each key has a unique identifier, which is used to
// uniquely identify the key whilst maintaining secrecy.
//
// Currently, AES-256-GCM is the only implemented cipher mode.
// The Cipher is currently used to encrypt/decrypt the following fields:
// - database.UserLink.OAuthAccessToken
// - database.UserLink.OAuthRefreshToken
// - database.GitAuthLink.OAuthAccessToken
// - database.GitAuthLink.OAuthRefreshToken
// - database.DBCryptSentinelValue
//
// Multiple ciphers can be provided to support key rotation. The primary cipher
// is used to encrypt and decrypt all data. Secondary ciphers are only used
// for decryption and, as a general rule, should only be active when rotating
// keys.
//
// Encryption keys are stored in the database in the table `dbcrypt_keys`.
// The table has the following schema:
//   - number: the key number. This is used to avoid conflicts when rotating keys.
//   - created_at: the time the key was created.
//   - active_key_digest: the SHA256 digest of the active key. If null, the key has been revoked.
//   - revoked_key_digest: the SHA256 digest of the revoked key. If null, the key has not been revoked.
//   - revoked_at: the time the key was revoked. If null, the key has not been revoked.
//   - test: the encrypted value of the string "coder". This is used to ensure that the key is valid.
//
// Encrypted fields are stored in the database as a base64-encoded string.
// Each encrypted column MUST have a corresponding _key_id column that is a foreign key
// reference to `dbcrypt_keys.active_key_digest`. This ensures that a key cannot be
// revoked until all rows that use that key have been migrated to a new key.
package dbcrypt
