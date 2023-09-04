package cli_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"testing"

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

// nolint: paralleltest // use of t.Setenv
func TestDBCryptRotate(t *testing.T) {
	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires a postgres instance")
	}
	//
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	//
	// Setup a postgres database.
	connectionURL, closePg, err := postgres.Open()
	require.NoError(t, err)
	t.Cleanup(closePg)
	//
	sqlDB, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db := database.New(sqlDB)

	// Populate the database with some unencrypted data.
	users := genData(t, db, 10)

	// Setup an initial cipher
	keyA := mustString(t, 32)
	cipherA, err := dbcrypt.NewCiphers([]byte(keyA))
	require.NoError(t, err)

	// Encrypt all the data with the initial cipher.
	inv, _ := newCLI(t, "dbcrypt-rotate",
		"--postgres-url", connectionURL,
		"--external-token-encryption-keys", base64.StdEncoding.EncodeToString([]byte(keyA)),
	)
	pty := ptytest.New(t)
	inv.Stdout = pty.Output()
	err = inv.Run()
	require.NoError(t, err)

	// Validate that all existing data has been encrypted with cipher A.
	requireDataDecryptsWithCipher(ctx, t, db, cipherA[0], users)

	// Create an encrypted database
	cryptdb, err := dbcrypt.New(ctx, db, cipherA...)
	require.NoError(t, err)

	// Populate the database with some encrypted data using cipher A.
	users = append(users, genData(t, cryptdb, 10)...)

	// Re-encrypt all existing data with a new cipher.
	keyB := mustString(t, 32)
	cipherBA, err := dbcrypt.NewCiphers([]byte(keyB), []byte(keyA))
	require.NoError(t, err)
	externalTokensArg := fmt.Sprintf(
		"%s,%s",
		base64.StdEncoding.EncodeToString([]byte(keyB)),
		base64.StdEncoding.EncodeToString([]byte(keyA)),
	)

	inv, _ = newCLI(t, "dbcrypt-rotate",
		"--postgres-url", connectionURL,
		"--external-token-encryption-keys", externalTokensArg,
	)
	pty = ptytest.New(t)
	inv.Stdout = pty.Output()
	err = inv.Run()
	require.NoError(t, err)

	// Validate that all data has been re-encrypted with cipher B.
	requireDataDecryptsWithCipher(ctx, t, db, cipherBA[0], users)

	// Assert that we can revoke the old key.
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
	err = db.RevokeDBCryptKey(ctx, cipherBA[0].HexDigest())
	require.Error(t, err, "expected to fail to revoke the new key")
	var pgErr *pq.Error
	require.True(t, xerrors.As(err, &pgErr), "expected a pg error")
	require.EqualValues(t, "23503", pgErr.Code, "expected a foreign key constraint violation error")
}

func genData(t *testing.T, db database.Store, n int) []database.User {
	t.Helper()
	var users []database.User
	for i := 0; i < n; i++ {
		usr := dbgen.User(t, db, database.User{
			LoginType: database.LoginTypeOIDC,
		})
		_ = dbgen.UserLink(t, db, database.UserLink{
			UserID:            usr.ID,
			LoginType:         usr.LoginType,
			OAuthAccessToken:  mustString(t, 16),
			OAuthRefreshToken: mustString(t, 16),
		})
		_ = dbgen.GitAuthLink(t, db, database.GitAuthLink{
			UserID:            usr.ID,
			ProviderID:        "fake",
			OAuthAccessToken:  mustString(t, 16),
			OAuthRefreshToken: mustString(t, 16),
		})
		users = append(users, usr)
	}
	return users
}

func mustString(t *testing.T, n int) string {
	t.Helper()
	s, err := cryptorand.String(n)
	require.NoError(t, err)
	return s
}

func requireDecryptWithCipher(t *testing.T, c dbcrypt.Cipher, s string) {
	t.Helper()
	decodedVal, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err, "failed to decode base64 string")
	_, err = c.Decrypt(decodedVal)
	require.NoError(t, err, "failed to decrypt value")
}

func requireDataDecryptsWithCipher(ctx context.Context, t *testing.T, db database.Store, c dbcrypt.Cipher, users []database.User) {
	t.Helper()
	for _, usr := range users {
		ul, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    usr.ID,
			LoginType: usr.LoginType,
		})
		require.NoError(t, err, "failed to get user link for user %s", usr.ID)
		requireDecryptWithCipher(t, c, ul.OAuthAccessToken)
		requireDecryptWithCipher(t, c, ul.OAuthRefreshToken)
		require.Equal(t, c.HexDigest(), ul.OAuthAccessTokenKeyID.String)
		require.Equal(t, c.HexDigest(), ul.OAuthRefreshTokenKeyID.String)
		//
		gal, err := db.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			UserID:     usr.ID,
			ProviderID: "fake",
		})
		require.NoError(t, err, "failed to get git auth link for user %s", usr.ID)
		requireDecryptWithCipher(t, c, gal.OAuthAccessToken)
		requireDecryptWithCipher(t, c, gal.OAuthRefreshToken)
		require.Equal(t, c.HexDigest(), gal.OAuthAccessTokenKeyID.String)
		require.Equal(t, c.HexDigest(), gal.OAuthRefreshTokenKeyID.String)
	}
}
