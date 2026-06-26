package cli_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/cli"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/testutil"
)

// TestServerDBCrypt tests end-to-end encryption, decryption, and deletion
// of encrypted user data.
//
// nolint: paralleltest // use of t.Setenv
func TestServerDBCrypt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Setup a postgres database.
	connectionURL, err := dbtestutil.Open(t)
	require.NoError(t, err)
	t.Cleanup(func() { dbtestutil.DumpOnFailure(t, connectionURL) })

	sqlDB, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db := database.New(sqlDB)

	// Populate the database with some unencrypted data.
	t.Log("Generating unencrypted data")
	users := genData(t, db)

	// Setup an initial cipher A
	keyA := testutil.MustRandString(t, 32)
	cipherA, err := dbcrypt.NewCiphers([]byte(keyA))
	require.NoError(t, err)

	// Create an encrypted database
	cryptdb, err := dbcrypt.New(ctx, db, cipherA...)
	require.NoError(t, err)

	// Populate the database with some encrypted data using cipher A.
	t.Log("Generating data encrypted with cipher A")
	newUsers := genData(t, cryptdb)

	// Validate that newly created users were encrypted with cipher A
	for _, usr := range newUsers {
		requireEncryptedWithCipher(ctx, t, db, cipherA[0], usr.ID)
	}
	users = append(users, newUsers...)

	// Encrypt all the data with the initial cipher.
	t.Log("Encrypting all data with cipher A")
	inv, _ := newCLI(t, "server", "dbcrypt", "rotate",
		"--postgres-url", connectionURL,
		"--new-key", base64.StdEncoding.EncodeToString([]byte(keyA)),
		"--yes",
	)
	err = inv.Run()
	require.NoError(t, err)

	// Validate that all existing data has been encrypted with cipher A.
	for _, usr := range users {
		requireEncryptedWithCipher(ctx, t, db, cipherA[0], usr.ID)
	}

	// Re-encrypt all existing data with a new cipher.
	keyB := testutil.MustRandString(t, 32)
	cipherBA, err := dbcrypt.NewCiphers([]byte(keyB), []byte(keyA))
	require.NoError(t, err)

	t.Log("Enrypting all data with cipher B")
	inv, _ = newCLI(t, "server", "dbcrypt", "rotate",
		"--postgres-url", connectionURL,
		"--new-key", base64.StdEncoding.EncodeToString([]byte(keyB)),
		"--old-keys", base64.StdEncoding.EncodeToString([]byte(keyA)),
		"--yes",
	)
	err = inv.Run()
	require.NoError(t, err)

	// Validate that all data has been re-encrypted with cipher B.
	for _, usr := range users {
		requireEncryptedWithCipher(ctx, t, db, cipherBA[0], usr.ID)
	}

	// Assert that we can revoke the old key.
	t.Log("Revoking cipher A")
	err = db.RevokeDBCryptKey(ctx, cipherA[0].HexDigest())
	require.NoError(t, err, "failed to revoke old key")

	// Assert that the key has been revoked in the database.
	keys, err := db.GetDBCryptKeys(ctx)
	oldKey := keys[0] // ORDER BY number ASC;
	newKey := keys[1]
	require.NoError(t, err, "failed to get db crypt keys")
	require.Len(t, keys, 2, "expected exactly 2 keys")
	require.Equal(t, cipherBA[0].HexDigest(), newKey.ActiveKeyDigest.String, "expected the new key to be the active key")
	require.Empty(t, newKey.RevokedKeyDigest.String, "expected the new key to not be revoked")
	require.Equal(t, cipherBA[1].HexDigest(), oldKey.RevokedKeyDigest.String, "expected the old key to be revoked")
	require.Empty(t, oldKey.ActiveKeyDigest.String, "expected the old key to not be active")

	// Revoking the new key should fail.
	t.Log("Attempting to revoke cipher B should fail as it is still in use")
	err = db.RevokeDBCryptKey(ctx, cipherBA[0].HexDigest())
	require.Error(t, err, "expected to fail to revoke the new key")
	var pgErr *pq.Error
	require.True(t, xerrors.As(err, &pgErr), "expected a pg error")
	require.EqualValues(t, "23503", pgErr.Code, "expected a foreign key constraint violation error")

	// Decrypt the data using only cipher B. This should result in the key being revoked.
	t.Log("Decrypting with cipher B")
	inv, _ = newCLI(t, "server", "dbcrypt", "decrypt",
		"--postgres-url", connectionURL,
		"--keys", base64.StdEncoding.EncodeToString([]byte(keyB)),
		"--yes",
	)
	err = inv.Run()
	require.NoError(t, err)

	// Validate that both keys have been revoked.
	keys, err = db.GetDBCryptKeys(ctx)
	require.NoError(t, err, "failed to get db crypt keys")
	require.Len(t, keys, 2, "expected exactly 2 keys")
	for _, key := range keys {
		require.Empty(t, key.ActiveKeyDigest.String, "expected the new key to not be active")
	}

	// Validate that all data has been decrypted.
	for _, usr := range users {
		requireEncryptedWithCipher(ctx, t, db, &nullCipher{}, usr.ID)
	}

	// Re-encrypt all existing data with a new cipher.
	keyC := testutil.MustRandString(t, 32)
	cipherC, err := dbcrypt.NewCiphers([]byte(keyC))
	require.NoError(t, err)

	t.Log("Re-encrypting with cipher C")
	inv, _ = newCLI(t, "server", "dbcrypt", "rotate",
		"--postgres-url", connectionURL,
		"--new-key", base64.StdEncoding.EncodeToString([]byte(keyC)),
		"--yes",
	)
	err = inv.Run()
	require.NoError(t, err)

	// Validate that all data has been re-encrypted with cipher C.
	for _, usr := range users {
		requireEncryptedWithCipher(ctx, t, db, cipherC[0], usr.ID)
	}

	// Now delete all the encrypted data.
	t.Log("Deleting all encrypted data")
	inv, _ = newCLI(t, "server", "dbcrypt", "delete",
		"--postgres-url", connectionURL,
		"--external-token-encryption-keys", base64.StdEncoding.EncodeToString([]byte(keyC)),
		"--yes",
	)
	err = inv.Run()
	require.NoError(t, err)

	// Assert that no user links remain.
	for _, usr := range users {
		userLinks, err := db.GetUserLinksByUserID(ctx, usr.ID)
		require.NoError(t, err, "failed to get user links for user %s", usr.ID)
		require.Empty(t, userLinks)
		gitAuthLinks, err := db.GetExternalAuthLinksByUserID(ctx, usr.ID)
		require.NoError(t, err, "failed to get git auth links for user %s", usr.ID)
		require.Empty(t, gitAuthLinks)

		userSecrets, err := db.ListUserSecretsWithValues(ctx, usr.ID)
		require.NoError(t, err, "failed to get user secrets for user %s", usr.ID)
		require.Empty(t, userSecrets)

		// gitsshkey rows are preserved so the user can regenerate; only the ciphertext is wiped.
		sshKey, err := db.GetGitSSHKey(ctx, usr.ID)
		require.NoError(t, err, "expected gitsshkey row to remain for user %s", usr.ID)
		require.Empty(t, sshKey.PrivateKey, "expected private_key to be cleared for user %s", usr.ID)
		require.False(t, sshKey.PrivateKeyKeyID.Valid, "expected private_key_key_id to be cleared for user %s", usr.ID)
	}

	// Validate that the key has been revoked in the database.
	keys, err = db.GetDBCryptKeys(ctx)
	require.NoError(t, err, "failed to get db crypt keys")
	require.Len(t, keys, 3, "expected exactly 3 keys")
	for _, k := range keys {
		require.Empty(t, k.ActiveKeyDigest.String, "expected the key to not be active")
		require.NotEmpty(t, k.RevokedKeyDigest.String, "expected the key to be revoked")
	}
}

func genData(t *testing.T, db database.Store) []database.User {
	t.Helper()
	var users []database.User
	// Make some users
	for _, status := range database.AllUserStatusValues() {
		for _, loginType := range database.AllLoginTypeValues() {
			for _, deleted := range []bool{false, true} {
				randName := testutil.MustRandString(t, 32)
				usr := dbgen.User(t, db, database.User{
					Username:  randName,
					Email:     randName + "@notcoder.com",
					LoginType: loginType,
					Status:    status,
					Deleted:   deleted,
				})
				_ = dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
					UserID:            usr.ID,
					ProviderID:        "fake",
					OAuthAccessToken:  "access-" + usr.ID.String(),
					OAuthRefreshToken: "refresh-" + usr.ID.String(),
				})
				provider := dbgen.AIProvider(t, db, database.AIProvider{
					Name:     "ai-provider-" + usr.ID.String(),
					Settings: sql.NullString{String: "settings-" + usr.ID.String(), Valid: true},
				})
				_ = dbgen.AIProviderKey(t, db, database.AIProviderKey{
					ProviderID: provider.ID,
					APIKey:     "provider-key-" + usr.ID.String(),
				})
				// gitsshkeys are not removed by the user soft-delete trigger,
				// so seed one for every user including deleted ones.
				_ = dbgen.GitSSHKey(t, db, database.GitSSHKey{
					UserID:     usr.ID,
					PrivateKey: "private-" + usr.ID.String(),
					PublicKey:  "public-" + usr.ID.String(),
				})
				now := time.Now()
				_, err := db.UpsertUserAIProviderKey(context.Background(), database.UpsertUserAIProviderKeyParams{
					ID:           uuid.New(),
					UserID:       usr.ID,
					AIProviderID: provider.ID,
					APIKey:       "user-ai-provider-key-" + usr.ID.String(),
					CreatedAt:    now,
					UpdatedAt:    now,
				})
				require.NoError(t, err)

				// Deleted users cannot have user_links or user_secrets.
				if !deleted {
					// Fun fact: our schema allows _all_ login types to have
					// a user_link. Even though I'm not sure how it could occur
					// in practice, making sure to test all combinations here.
					_ = dbgen.UserLink(t, db, database.UserLink{
						UserID:            usr.ID,
						LoginType:         usr.LoginType,
						OAuthAccessToken:  "access-" + usr.ID.String(),
						OAuthRefreshToken: "refresh-" + usr.ID.String(),
					})

					_ = dbgen.UserSecret(t, db, database.UserSecret{
						UserID:   usr.ID,
						Name:     "secret-" + usr.ID.String(),
						Value:    "value-" + usr.ID.String(),
						EnvName:  "",
						FilePath: "",
					})
				}
				users = append(users, usr)
			}
		}
	}
	return users
}

func requireEncryptedEquals(t *testing.T, c dbcrypt.Cipher, expected, actual string) {
	t.Helper()
	var decodedVal []byte
	var err error
	if _, ok := c.(*nullCipher); !ok {
		decodedVal, err = base64.StdEncoding.DecodeString(actual)
		require.NoError(t, err, "failed to decode base64 string")
	} else {
		// If a nullCipher is being used, we expect the value not to be encrypted.
		decodedVal = []byte(actual)
	}
	val, err := c.Decrypt(decodedVal)
	require.NoError(t, err, "failed to decrypt value")
	require.Equal(t, expected, string(val))
}

func requireEncryptedWithCipher(ctx context.Context, t *testing.T, db database.Store, c dbcrypt.Cipher, userID uuid.UUID) {
	t.Helper()
	userLinks, err := db.GetUserLinksByUserID(ctx, userID)
	require.NoError(t, err, "failed to get user links for user %s", userID)
	for _, ul := range userLinks {
		requireEncryptedEquals(t, c, "access-"+userID.String(), ul.OAuthAccessToken)
		requireEncryptedEquals(t, c, "refresh-"+userID.String(), ul.OAuthRefreshToken)
		require.Equal(t, c.HexDigest(), ul.OAuthAccessTokenKeyID.String)
		require.Equal(t, c.HexDigest(), ul.OAuthRefreshTokenKeyID.String)
	}
	gitAuthLinks, err := db.GetExternalAuthLinksByUserID(ctx, userID)
	require.NoError(t, err, "failed to get git auth links for user %s", userID)
	for _, gal := range gitAuthLinks {
		requireEncryptedEquals(t, c, "access-"+userID.String(), gal.OAuthAccessToken)
		requireEncryptedEquals(t, c, "refresh-"+userID.String(), gal.OAuthRefreshToken)
		require.Equal(t, c.HexDigest(), gal.OAuthAccessTokenKeyID.String)
		require.Equal(t, c.HexDigest(), gal.OAuthRefreshTokenKeyID.String)
	}

	userSecrets, err := db.ListUserSecretsWithValues(ctx, userID)
	require.NoError(t, err, "failed to get user secrets for user %s", userID)
	for _, s := range userSecrets {
		requireEncryptedEquals(t, c, "value-"+userID.String(), s.Value)
		require.Equal(t, c.HexDigest(), s.ValueKeyID.String)
	}

	sshKey, err := db.GetGitSSHKey(ctx, userID)
	require.NoError(t, err, "failed to get gitsshkey for user %s", userID)
	requireEncryptedEquals(t, c, "private-"+userID.String(), sshKey.PrivateKey)
	require.Equal(t, c.HexDigest(), sshKey.PrivateKeyKeyID.String)
	// Public key is never encrypted.
	require.Equal(t, "public-"+userID.String(), sshKey.PublicKey)

	providers, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
		IncludeDeleted:  true,
		IncludeDisabled: true,
	})
	require.NoError(t, err, "failed to get ai providers")
	providerName := "ai-provider-" + userID.String()
	var provider database.AIProvider
	for _, p := range providers {
		if p.Name == providerName {
			provider = p
			break
		}
	}
	require.NotEqual(t, uuid.Nil, provider.ID, "expected ai provider for user %s", userID)
	require.True(t, provider.Settings.Valid)
	requireEncryptedEquals(t, c, "settings-"+userID.String(), provider.Settings.String)
	require.Equal(t, c.HexDigest(), provider.SettingsKeyID.String)

	providerKeys, err := db.GetAIProviderKeysByProviderID(ctx, provider.ID)
	require.NoError(t, err, "failed to get ai provider keys for provider %s", provider.ID)
	require.Len(t, providerKeys, 1)
	requireEncryptedEquals(t, c, "provider-key-"+userID.String(), providerKeys[0].APIKey)
	require.Equal(t, c.HexDigest(), providerKeys[0].ApiKeyKeyID.String)

	userAIProviderKeys, err := db.GetUserAIProviderKeysByUserID(ctx, userID)
	require.NoError(t, err, "failed to get user ai provider keys for user %s", userID)
	require.Len(t, userAIProviderKeys, 1)
	requireEncryptedEquals(t, c, "user-ai-provider-key-"+userID.String(), userAIProviderKeys[0].APIKey)
	require.Equal(t, c.HexDigest(), userAIProviderKeys[0].ApiKeyKeyID.String)
}

// TestServerAIProviderKeysEncryptedWithDBCrypt starts a real enterprise server
// with external token encryption and AI provider config, then verifies that
// seeded AI provider keys are encrypted at rest.
func TestServerAIProviderKeysEncryptedWithDBCrypt(t *testing.T) {
	t.Parallel()

	// Given: a 32-byte encryption key, base64-encoded.
	rawKey := testutil.MustRandString(t, 32)
	b64Key := base64.StdEncoding.EncodeToString([]byte(rawKey))

	ciphers, err := dbcrypt.NewCiphers([]byte(rawKey))
	require.NoError(t, err)
	expectedDigest := ciphers[0].HexDigest()

	dbURL, err := dbtestutil.Open(t)
	require.NoError(t, err)

	const testAPIKey = "sk-test-key-that-must-be-encrypted-at-rest"

	// Given: enterprise server with encryption and a legacy AI provider.
	var root cli.RootCmd
	cmd, err := root.Command(root.EnterpriseSubcommands())
	require.NoError(t, err)

	inv, cfg := clitest.NewWithCommand(t, cmd,
		"server",
		"--postgres-url="+dbURL,
		"--http-address", ":0",
		"--access-url", "http://example.com",
		"--external-token-encryption-keys", b64Key,
		"--aibridge-enabled",
		"--aibridge-openai-key", testAPIKey,
	)

	// When: the server starts up and seeds ai providers from env
	ctx := testutil.Context(t, testutil.WaitLong)
	clitest.Start(t, inv.WithContext(ctx))
	_ = waitAccessURL(t, cfg)

	// Open a RAW database connection to inspect the actual stored values.
	sqlDB, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	rawDB := database.New(sqlDB)

	// Then: we expect a single provider to be seeded in the db.
	providers, err := rawDB.GetAIProviders(ctx, database.GetAIProvidersParams{
		IncludeDeleted:  true,
		IncludeDisabled: true,
	})
	require.NoError(t, err)
	require.Len(t, providers, 1, "expected exactly one provider")
	provider := providers[0]
	require.Equal(t, "openai", provider.Name, "unexpected provider name")

	// Then: provider must exist.
	require.NotEmpty(t, provider.ID,
		"seeded AI provider 'openai' should exist in database")

	keys, err := rawDB.GetAIProviderKeysByProviderID(ctx, provider.ID)
	require.NoError(t, err)
	require.Len(t, keys, 1, "should have exactly one provider key")

	rawKeyRow := keys[0]

	// Then: key_id must be populated
	require.True(t, rawKeyRow.ApiKeyKeyID.Valid,
		"api_key_key_id must be set when dbcrypt is active; NULL means the key was written without encryption (the bug from PR #25699)")
	require.Equal(t, expectedDigest, rawKeyRow.ApiKeyKeyID.String,
		"api_key_key_id should match the active cipher's hex digest")

	// Then: the stored value must NOT be plaintext.
	require.NotEqual(t, testAPIKey, rawKeyRow.APIKey,
		"raw stored api_key must not be plaintext when encryption is active")

	// Then: the stored value decrypts to the original key.
	ciphertext, err := base64.StdEncoding.DecodeString(rawKeyRow.APIKey)
	require.NoError(t, err, "encrypted api_key should be valid base64")

	plaintext, err := ciphers[0].Decrypt(ciphertext)
	require.NoError(t, err, "should be able to decrypt the stored key with the configured cipher")
	require.Equal(t, testAPIKey, string(plaintext),
		"decrypted value should match original API key")
}

// nullCipher is a dbcrypt.Cipher that does not encrypt or decrypt.
// used for testing
type nullCipher struct{}

func (*nullCipher) Encrypt(b []byte) ([]byte, error) {
	return b, nil
}

func (*nullCipher) Decrypt(b []byte) ([]byte, error) {
	return b, nil
}

func (*nullCipher) HexDigest() string {
	return "" // This co-incidentally happens to be the value of sql.NullString{}.String...
}

var _ dbcrypt.Cipher = (*nullCipher)(nil)
