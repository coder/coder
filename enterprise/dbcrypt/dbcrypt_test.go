package dbcrypt_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"io"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
)

func TestUserLinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("InsertUserLink", func(t *testing.T) {
		t.Parallel()
		db, crypt, cipher := setup(t)
		initCipher(t, cipher)
		user := dbgen.User(t, crypt, database.User{})
		link := dbgen.UserLink(t, crypt, database.UserLink{
			UserID:            user.ID,
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
		})
		link, err := db.GetUserLinkByLinkedID(ctx, link.LinkedID)
		require.NoError(t, err)
		requireEncryptedEquals(t, cipher, link.OAuthAccessToken, "access")
		requireEncryptedEquals(t, cipher, link.OAuthRefreshToken, "refresh")
	})

	t.Run("UpdateUserLink", func(t *testing.T) {
		t.Parallel()
		db, crypt, cipher := setup(t)
		initCipher(t, cipher)
		user := dbgen.User(t, crypt, database.User{})
		link := dbgen.UserLink(t, crypt, database.UserLink{
			UserID: user.ID,
		})
		_, err := crypt.UpdateUserLink(ctx, database.UpdateUserLinkParams{
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
			UserID:            link.UserID,
			LoginType:         link.LoginType,
		})
		require.NoError(t, err)
		link, err = db.GetUserLinkByLinkedID(ctx, link.LinkedID)
		require.NoError(t, err)
		requireEncryptedEquals(t, cipher, link.OAuthAccessToken, "access")
		requireEncryptedEquals(t, cipher, link.OAuthRefreshToken, "refresh")
	})

	t.Run("GetUserLinkByLinkedID", func(t *testing.T) {
		t.Parallel()
		db, crypt, cipher := setup(t)
		initCipher(t, cipher)
		user := dbgen.User(t, crypt, database.User{})
		link := dbgen.UserLink(t, crypt, database.UserLink{
			UserID:            user.ID,
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
		})
		link, err := db.GetUserLinkByLinkedID(ctx, link.LinkedID)
		require.NoError(t, err)
		requireEncryptedEquals(t, cipher, link.OAuthAccessToken, "access")
		requireEncryptedEquals(t, cipher, link.OAuthRefreshToken, "refresh")

		// Reset the key and empty values should be returned!
		initCipher(t, cipher)

		link, err = crypt.GetUserLinkByLinkedID(ctx, link.LinkedID)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("GetUserLinkByUserIDLoginType", func(t *testing.T) {
		t.Parallel()
		db, crypt, cipher := setup(t)
		initCipher(t, cipher)
		user := dbgen.User(t, crypt, database.User{})
		link := dbgen.UserLink(t, crypt, database.UserLink{
			UserID:            user.ID,
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
		})
		link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    link.UserID,
			LoginType: link.LoginType,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, cipher, link.OAuthAccessToken, "access")
		requireEncryptedEquals(t, cipher, link.OAuthRefreshToken, "refresh")

		// Reset the key and empty values should be returned!
		initCipher(t, cipher)

		link, err = crypt.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    link.UserID,
			LoginType: link.LoginType,
		})
		require.ErrorIs(t, err, sql.ErrNoRows)
	})
}

func TestGitAuthLinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("InsertGitAuthLink", func(t *testing.T) {
		t.Parallel()
		db, crypt, cipher := setup(t)
		initCipher(t, cipher)
		link := dbgen.GitAuthLink(t, crypt, database.GitAuthLink{
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
		})
		link, err := db.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, cipher, link.OAuthAccessToken, "access")
		requireEncryptedEquals(t, cipher, link.OAuthRefreshToken, "refresh")
	})

	t.Run("UpdateGitAuthLink", func(t *testing.T) {
		t.Parallel()
		db, crypt, cipher := setup(t)
		initCipher(t, cipher)
		link := dbgen.GitAuthLink(t, crypt, database.GitAuthLink{})
		_, err := crypt.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
			ProviderID:        link.ProviderID,
			UserID:            link.UserID,
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
		})
		require.NoError(t, err)
		link, err = db.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, cipher, link.OAuthAccessToken, "access")
		requireEncryptedEquals(t, cipher, link.OAuthRefreshToken, "refresh")
	})

	t.Run("GetGitAuthLink", func(t *testing.T) {
		t.Parallel()
		db, crypt, cipher := setup(t)
		initCipher(t, cipher)
		link := dbgen.GitAuthLink(t, crypt, database.GitAuthLink{
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
		})
		link, err := db.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			UserID:     link.UserID,
			ProviderID: link.ProviderID,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, cipher, link.OAuthAccessToken, "access")
		requireEncryptedEquals(t, cipher, link.OAuthRefreshToken, "refresh")

		// Reset the key and empty values should be returned!
		initCipher(t, cipher)

		link, err = crypt.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			UserID:     link.UserID,
			ProviderID: link.ProviderID,
		})
		require.ErrorIs(t, err, sql.ErrNoRows)
	})
}

func TestDBCryptSentinelValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, crypt, cipher := setup(t)
	// Initially, the database will not be encrypted.
	_, err := db.GetDBCryptSentinelValue(ctx)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = crypt.GetDBCryptSentinelValue(ctx)
	require.EqualError(t, err, dbcrypt.ErrNotEncrypted.Error())

	// Now, we'll encrypt the value.
	initCipher(t, cipher)
	err = crypt.SetDBCryptSentinelValue(ctx, "coder")
	require.NoError(t, err)

	// The value should be encrypted in the database.
	crypted, err := db.GetDBCryptSentinelValue(ctx)
	require.NoError(t, err)
	require.NotEqual(t, "coder", crypted)
	decrypted, err := crypt.GetDBCryptSentinelValue(ctx)
	require.NoError(t, err)
	require.Equal(t, "coder", decrypted)
	requireEncryptedEquals(t, cipher, crypted, "coder")

	// Reset the key and empty values should be returned!
	initCipher(t, cipher)

	_, err = db.GetDBCryptSentinelValue(ctx) // We can still read the raw value
	require.NoError(t, err)
	_, err = crypt.GetDBCryptSentinelValue(ctx) // Decryption should fail
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func requireEncryptedEquals(t *testing.T, cipher *atomic.Pointer[dbcrypt.Cipher], value, expected string) {
	t.Helper()
	c := (*cipher.Load())
	require.NotNil(t, c)
	require.Greater(t, len(value), len(dbcrypt.MagicPrefix), "value is not encrypted")
	data, err := base64.StdEncoding.DecodeString(value[len(dbcrypt.MagicPrefix):])
	require.NoError(t, err)
	got, err := c.Decrypt(data)
	require.NoError(t, err)
	require.Equal(t, expected, string(got))
}

func initCipher(t *testing.T, cipher *atomic.Pointer[dbcrypt.Cipher]) {
	t.Helper()
	key := make([]byte, 32) // AES-256 key size is 32 bytes
	_, err := io.ReadFull(rand.Reader, key)
	require.NoError(t, err)
	c, err := dbcrypt.CipherAES256(key)
	require.NoError(t, err)
	cipher.Store(&c)
}

func setup(t *testing.T) (db, cryptodb database.Store, cipher *atomic.Pointer[dbcrypt.Cipher]) {
	t.Helper()
	rawDB, _ := dbtestutil.NewDB(t)
	cipher = &atomic.Pointer[dbcrypt.Cipher]{}
	return rawDB, dbcrypt.New(rawDB, &dbcrypt.Options{
		ExternalTokenCipher: cipher,
		Logger:              slogtest.Make(t, nil).Leveled(slog.LevelDebug),
	}), cipher
}
