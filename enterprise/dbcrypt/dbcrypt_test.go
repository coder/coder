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

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		// Given: a cipher is loaded
		cipher := &atomic.Pointer[dbcrypt.Cipher]{}
		initCipher(t, cipher)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		rawDB, _ := dbtestutil.NewDB(t)

		// When: we init the crypt db
		cryptDB, err := dbcrypt.New(ctx, rawDB, &dbcrypt.Options{
			ExternalTokenCipher: cipher,
			Logger:              slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		})
		require.NoError(t, err)

		// Then: the sentinel value is encrypted
		cryptVal, err := cryptDB.GetDBCryptSentinelValue(ctx)
		require.NoError(t, err)
		require.Equal(t, "coder", cryptVal)

		rawVal, err := rawDB.GetDBCryptSentinelValue(ctx)
		require.NoError(t, err)
		require.Contains(t, rawVal, dbcrypt.MagicPrefix)
		requireEncryptedEquals(t, cipher, rawVal, "coder")
	})

	t.Run("NoCipher", func(t *testing.T) {
		// Given: no cipher is loaded
		cipher := &atomic.Pointer[dbcrypt.Cipher]{}
		// initCipher(t, cipher)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		rawDB, _ := dbtestutil.NewDB(t)

		// When: we init the crypt db
		cryptDB, err := dbcrypt.New(ctx, rawDB, &dbcrypt.Options{
			ExternalTokenCipher: cipher,
			Logger:              slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		})
		require.NoError(t, err)

		// Then: the sentinel value is not encrypted
		cryptVal, err := cryptDB.GetDBCryptSentinelValue(ctx)
		require.NoError(t, err)
		require.Equal(t, "coder", cryptVal)

		rawVal, err := rawDB.GetDBCryptSentinelValue(ctx)
		require.NoError(t, err)
		require.Equal(t, "coder", rawVal)
	})

	t.Run("CipherChanged", func(t *testing.T) {
		// Given: no cipher is loaded
		cipher := &atomic.Pointer[dbcrypt.Cipher]{}
		initCipher(t, cipher)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		rawDB, _ := dbtestutil.NewDB(t)

		// And: the sentinel value is encrypted with a different cipher
		cipher2 := &atomic.Pointer[dbcrypt.Cipher]{}
		initCipher(t, cipher2)
		field := "coder"
		encrypted, err := (*cipher2.Load()).Encrypt([]byte(field))
		require.NoError(t, err)
		b64encrypted := base64.StdEncoding.EncodeToString(encrypted)
		require.NoError(t, rawDB.SetDBCryptSentinelValue(ctx, b64encrypted))

		// When: we init the crypt db
		_, err = dbcrypt.New(ctx, rawDB, &dbcrypt.Options{
			ExternalTokenCipher: cipher,
			Logger:              slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		})
		// Then: an error is returned
		// TODO: when we implement key rotation, this should not fail.
		require.ErrorContains(t, err, "database is already encrypted with a different key")

		// And the sentinel value should remain unchanged. For now.
		rawVal, err := rawDB.GetDBCryptSentinelValue(ctx)
		require.NoError(t, err)
		require.Equal(t, b64encrypted, rawVal)
	})
}

func requireEncryptedEquals(t *testing.T, cipher *atomic.Pointer[dbcrypt.Cipher], value, expected string) {
	t.Helper()
	c := (*cipher.Load())
	require.NotNil(t, c)
	require.Greater(t, len(value), 16, "value is not encrypted")
	require.Contains(t, value, dbcrypt.MagicPrefix+c.HexDigest()[:7]+"-")
	data, err := base64.StdEncoding.DecodeString(value[16:])
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
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	rawDB, _ := dbtestutil.NewDB(t)

	_, err := rawDB.GetDBCryptSentinelValue(ctx)
	require.ErrorIs(t, err, sql.ErrNoRows)

	cipher = &atomic.Pointer[dbcrypt.Cipher]{}
	initCipher(t, cipher)
	cryptDB, err := dbcrypt.New(ctx, rawDB, &dbcrypt.Options{
		ExternalTokenCipher: cipher,
		Logger:              slogtest.Make(t, nil).Leveled(slog.LevelDebug),
	})
	require.NoError(t, err)

	rawVal, err := rawDB.GetDBCryptSentinelValue(ctx)
	require.NoError(t, err)
	require.Contains(t, rawVal, dbcrypt.MagicPrefix)

	cryptVal, err := cryptDB.GetDBCryptSentinelValue(ctx)
	require.NoError(t, err)
	require.Equal(t, "coder", cryptVal)

	return rawDB, cryptDB, cipher
}
