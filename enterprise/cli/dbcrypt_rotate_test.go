package cli_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"

	"sync/atomic"
	"testing"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/postgres"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/pty/ptytest"

	"github.com/stretchr/testify/require"
)

func TestDBCryptRotate(t *testing.T) {
	//nolint: paralleltest // use of t.Setenv
	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires a postgres instance")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Setup a postgres database.
	connectionURL, closePg, err := postgres.Open()
	require.NoError(t, err)
	t.Cleanup(closePg)

	sqlDB, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db := database.New(sqlDB)

	// Setup an initial cipher
	keyA := mustString(t, 32)
	cA, err := dbcrypt.CipherAES256([]byte(keyA))
	require.NoError(t, err)
	cipherA := &atomic.Pointer[dbcrypt.Cipher]{}
	cipherB := &atomic.Pointer[dbcrypt.Cipher]{}
	cipherA.Store(&cA)

	// Create an encrypted database
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	cryptdb, err := dbcrypt.New(ctx, db, &dbcrypt.Options{
		PrimaryCipher:   cipherA,
		SecondaryCipher: cipherB,
		Logger:          log,
	})
	require.NoError(t, err)

	// Populate the database with some data encrypted with cipher A.
	var users []database.User
	for i := 0; i < 10; i++ {
		usr := dbgen.User(t, cryptdb, database.User{
			LoginType: database.LoginTypeOIDC,
		})
		_ = dbgen.UserLink(t, cryptdb, database.UserLink{
			UserID:            usr.ID,
			LoginType:         usr.LoginType,
			OAuthAccessToken:  mustString(t, 16),
			OAuthRefreshToken: mustString(t, 16),
		})
		_ = dbgen.GitAuthLink(t, cryptdb, database.GitAuthLink{
			UserID:            usr.ID,
			ProviderID:        "fake",
			OAuthAccessToken:  mustString(t, 16),
			OAuthRefreshToken: mustString(t, 16),
		})
		users = append(users, usr)
	}

	// Run the cmd with ciphers B,A
	keyB := mustString(t, 32)
	cB, err := dbcrypt.CipherAES256([]byte(keyB))
	require.NoError(t, err)
	externalTokensArg := fmt.Sprintf(
		"%s,%s",
		base64.StdEncoding.EncodeToString([]byte(keyB)),
		base64.StdEncoding.EncodeToString([]byte(keyA)),
	)

	inv, _ := newCLI(t, "dbcrypt-rotate",
		"--postgres-url", connectionURL,
		"--external-token-encryption-keys", externalTokensArg,
	)
	pty := ptytest.New(t)
	inv.Stdout = pty.Output()

	err = inv.Run()
	require.NoError(t, err)

	// Validate that all data has been updated with the checksum of the new cipher.
	expectedPrefixA := fmt.Sprintf("dbcrypt-%s-", cA.HexDigest()[:7])
	expectedPrefixB := fmt.Sprintf("dbcrypt-%s-", cB.HexDigest()[:7])
	for _, usr := range users {
		ul, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    usr.ID,
			LoginType: usr.LoginType,
		})
		require.NoError(t, err, "failed to get user link for user %s", usr.ID)
		require.NotContains(t, ul.OAuthAccessToken, expectedPrefixA, "user_link.oauth_access_token should not contain the old cipher checksum")
		require.NotContains(t, ul.OAuthRefreshToken, expectedPrefixA, "user_link.oauth_refresh_token should not contain the old cipher checksum")
		require.Contains(t, ul.OAuthAccessToken, expectedPrefixB, "user_link.oauth_access_token should contain the new cipher checksum")
		require.Contains(t, ul.OAuthRefreshToken, expectedPrefixB, "user_link.oauth_refresh_token should contain the new cipher checksum")

		gal, err := db.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			UserID:     usr.ID,
			ProviderID: "fake",
		})
		require.NoError(t, err, "failed to get git auth link for user %s", usr.ID)
		require.NotContains(t, gal.OAuthAccessToken, expectedPrefixA, "git_auth_link.oauth_access_token should not contain the old cipher checksum")
		require.NotContains(t, gal.OAuthRefreshToken, expectedPrefixA, "git_auth_link.oauth_refresh_token should not contain the old cipher checksum")
		require.Contains(t, gal.OAuthAccessToken, expectedPrefixB, "git_auth_link.oauth_access_token should contain the new cipher checksum")
		require.Contains(t, gal.OAuthRefreshToken, expectedPrefixB, "git_auth_link.oauth_refresh_token should contain the new cipher checksum")
	}
}

func mustString(t *testing.T, n int) string {
	t.Helper()
	s, err := cryptorand.String(n)
	require.NoError(t, err)
	return s
}
