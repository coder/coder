package cli_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/postgres"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/pty/ptytest"
)

// TestServerDBCrypt tests end-to-end encryption, decryption, and deletion
// of encrypted user data.
//
// nolint: paralleltest // use of t.Setenv
func TestServerDBCrypt(t *testing.T) {
	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires a postgres instance")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Setup a postgres database.
	connectionURL, closePg, err := postgres.Open()
	require.NoError(t, err)
	t.Cleanup(closePg)
	t.Cleanup(func() { dbtestutil.DumpOnFailure(t, connectionURL) })

	sqlDB, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db := database.New(sqlDB)

	// Populate the database with some unencrypted data.
	t.Logf("Generating unencrypted data")
	users := genData(t, db)

	// Setup an initial cipher A
	keyA := mustString(t, 32)
	cipherA, err := dbcrypt.NewCiphers([]byte(keyA))
	require.NoError(t, err)

	// Create an encrypted database
	cryptdb, err := dbcrypt.New(ctx, db, cipherA...)
	require.NoError(t, err)

	// Populate the database with some encrypted data using cipher A.
	t.Logf("Generating data encrypted with cipher A")
	newUsers := genData(t, cryptdb)

	// Validate that newly created users were encrypted with cipher A
	for _, usr := range newUsers {
		requireEncryptedWithCipher(ctx, t, db, cipherA[0], usr.ID)
	}
	users = append(users, newUsers...)

	// Encrypt all the data with the initial cipher.
	t.Logf("Encrypting all data with cipher A")
	inv, _ := newCLI(t, "server", "dbcrypt", "rotate",
		"--postgres-url", connectionURL,
		"--new-key", base64.StdEncoding.EncodeToString([]byte(keyA)),
		"--yes",
	)
	pty := ptytest.New(t)
	inv.Stdout = pty.Output()
	err = inv.Run()
	require.NoError(t, err)
	require.NoError(t, pty.Close())

	// Validate that all existing data has been encrypted with cipher A.
	for _, usr := range users {
		requireEncryptedWithCipher(ctx, t, db, cipherA[0], usr.ID)
	}

	// Re-encrypt all existing data with a new cipher.
	keyB := mustString(t, 32)
	cipherBA, err := dbcrypt.NewCiphers([]byte(keyB), []byte(keyA))
	require.NoError(t, err)

	t.Logf("Enrypting all data with cipher B")
	inv, _ = newCLI(t, "server", "dbcrypt", "rotate",
		"--postgres-url", connectionURL,
		"--new-key", base64.StdEncoding.EncodeToString([]byte(keyB)),
		"--old-keys", base64.StdEncoding.EncodeToString([]byte(keyA)),
		"--yes",
	)
	pty = ptytest.New(t)
	inv.Stdout = pty.Output()
	err = inv.Run()
	require.NoError(t, err)
	require.NoError(t, pty.Close())

	// Validate that all data has been re-encrypted with cipher B.
	for _, usr := range users {
		requireEncryptedWithCipher(ctx, t, db, cipherBA[0], usr.ID)
	}

	// Assert that we can revoke the old key.
	t.Logf("Revoking cipher A")
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
	t.Logf("Attempting to revoke cipher B should fail as it is still in use")
	err = db.RevokeDBCryptKey(ctx, cipherBA[0].HexDigest())
	require.Error(t, err, "expected to fail to revoke the new key")
	var pgErr *pq.Error
	require.True(t, xerrors.As(err, &pgErr), "expected a pg error")
	require.EqualValues(t, "23503", pgErr.Code, "expected a foreign key constraint violation error")

	// Decrypt the data using only cipher B. This should result in the key being revoked.
	t.Logf("Decrypting with cipher B")
	inv, _ = newCLI(t, "server", "dbcrypt", "decrypt",
		"--postgres-url", connectionURL,
		"--keys", base64.StdEncoding.EncodeToString([]byte(keyB)),
		"--yes",
	)
	pty = ptytest.New(t)
	inv.Stdout = pty.Output()
	err = inv.Run()
	require.NoError(t, err)
	require.NoError(t, pty.Close())

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
	keyC := mustString(t, 32)
	cipherC, err := dbcrypt.NewCiphers([]byte(keyC))
	require.NoError(t, err)

	t.Logf("Re-encrypting with cipher C")
	inv, _ = newCLI(t, "server", "dbcrypt", "rotate",
		"--postgres-url", connectionURL,
		"--new-key", base64.StdEncoding.EncodeToString([]byte(keyC)),
		"--yes",
	)

	pty = ptytest.New(t)
	inv.Stdout = pty.Output()
	err = inv.Run()
	require.NoError(t, err)
	require.NoError(t, pty.Close())

	// Validate that all data has been re-encrypted with cipher C.
	for _, usr := range users {
		requireEncryptedWithCipher(ctx, t, db, cipherC[0], usr.ID)
	}

	// Now delete all the encrypted data.
	t.Logf("Deleting all encrypted data")
	inv, _ = newCLI(t, "server", "dbcrypt", "delete",
		"--postgres-url", connectionURL,
		"--external-token-encryption-keys", base64.StdEncoding.EncodeToString([]byte(keyC)),
		"--yes",
	)
	pty = ptytest.New(t)
	inv.Stdout = pty.Output()
	err = inv.Run()
	require.NoError(t, err)
	require.NoError(t, pty.Close())

	// Assert that no user links remain.
	for _, usr := range users {
		userLinks, err := db.GetUserLinksByUserID(ctx, usr.ID)
		require.NoError(t, err, "failed to get user links for user %s", usr.ID)
		require.Empty(t, userLinks)
		gitAuthLinks, err := db.GetExternalAuthLinksByUserID(ctx, usr.ID)
		require.NoError(t, err, "failed to get git auth links for user %s", usr.ID)
		require.Empty(t, gitAuthLinks)
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
				randName := mustString(t, 32)
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
				// Fun fact: our schema allows _all_ login types to have
				// a user_link. Even though I'm not sure how it could occur
				// in practice, making sure to test all combinations here.
				_ = dbgen.UserLink(t, db, database.UserLink{
					UserID:            usr.ID,
					LoginType:         usr.LoginType,
					OAuthAccessToken:  "access-" + usr.ID.String(),
					OAuthRefreshToken: "refresh-" + usr.ID.String(),
				})
				users = append(users, usr)
			}
		}
	}
	return users
}

func mustString(t *testing.T, n int) string {
	t.Helper()
	s, err := cryptorand.String(n)
	require.NoError(t, err)
	return s
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
