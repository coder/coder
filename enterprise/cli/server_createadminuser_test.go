package cli_test

import (
	"database/sql"
	"encoding/base64"
	"io"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/testutil"
)

// TestServerCreateAdminUserDBCrypt tests that `server create-admin-user`
// encrypts the generated git SSH private key when given external token
// encryption keys, and refuses to write plaintext once encryption is active
// (SEC-1491).
func TestServerCreateAdminUserDBCrypt(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" || testing.Short() {
		// Skip on non-Linux because it spawns a PostgreSQL instance.
		t.SkipNow()
	}

	const (
		username = "encrypted"
		email    = "encrypted@example.com"
		password = "SecurePa$$word123"
	)

	connectionURL, err := dbtestutil.Open(t)
	require.NoError(t, err)

	sqlDB, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	defer sqlDB.Close()
	db := database.New(sqlDB)

	ctx := testutil.Context(t, testutil.WaitLong)

	key := testutil.MustRandString(t, 32)
	encodedKey := base64.StdEncoding.EncodeToString([]byte(key))

	inv, _ := newCLI(t,
		"server", "create-admin-user",
		"--postgres-url", connectionURL,
		"--ssh-keygen-algorithm", "ed25519",
		"--username", username,
		"--email", email,
		"--password", password,
		"--external-token-encryption-keys", encodedKey,
	)
	inv.Stdout, inv.Stderr = io.Discard, io.Discard
	require.NoError(t, inv.WithContext(ctx).Run())

	user, err := db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Email: email,
	})
	require.NoError(t, err)

	// The raw row must be encrypted, not plaintext.
	rawKey, err := db.GetGitSSHKey(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, rawKey.PrivateKeyKeyID.Valid, "private key must have an encryption key ID")
	require.NotContains(t, rawKey.PrivateKey, "PRIVATE KEY", "private key must not be stored in plaintext")

	// The row must decrypt with the provided key.
	ciphers, err := dbcrypt.NewCiphers([]byte(key))
	require.NoError(t, err)
	cryptdb, err := dbcrypt.New(ctx, db, ciphers...)
	require.NoError(t, err)
	sshKey, err := cryptdb.GetGitSSHKey(ctx, user.ID)
	require.NoError(t, err)
	require.Contains(t, sshKey.PrivateKey, "PRIVATE KEY")

	// Once encryption is active, the command must refuse to write plaintext.
	inv, _ = newCLI(t,
		"server", "create-admin-user",
		"--postgres-url", connectionURL,
		"--ssh-keygen-algorithm", "ed25519",
		"--username", "plaintext",
		"--email", "plaintext@example.com",
		"--password", password,
	)
	inv.Stdout, inv.Stderr = io.Discard, io.Discard
	err = inv.WithContext(ctx).Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "dbcrypt")

	// A wrong key must fail loudly rather than write garbage.
	wrongKey := base64.StdEncoding.EncodeToString([]byte(testutil.MustRandString(t, 32)))
	inv, _ = newCLI(t,
		"server", "create-admin-user",
		"--postgres-url", connectionURL,
		"--ssh-keygen-algorithm", "ed25519",
		"--username", "wrongkey",
		"--email", "wrongkey@example.com",
		"--password", password,
		"--external-token-encryption-keys", wrongKey,
	)
	inv.Stdout, inv.Stderr = io.Discard, io.Discard
	err = inv.WithContext(ctx).Run()
	require.Error(t, err)
}
