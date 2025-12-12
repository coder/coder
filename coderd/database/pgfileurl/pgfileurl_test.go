package pgfileurl_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pgfileurl"
)

func TestRegister(t *testing.T) {
	t.Parallel()

	t.Run("ReadsURLFromFile", func(t *testing.T) {
		t.Parallel()

		connectionURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		// Write connection URL to a temp file.
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "pg_url")
		err = os.WriteFile(filePath, []byte(connectionURL), 0o600)
		require.NoError(t, err)

		// Register the file URL driver.
		driverName, err := pgfileurl.Register("postgres", filePath, slogtest.Make(t, nil))
		require.NoError(t, err)
		require.Contains(t, driverName, "postgres-fileurl-")

		// Open a connection using the file URL driver.
		// The DSN is ignored since the URL is read from the file.
		db, err := sql.Open(driverName, "ignored")
		require.NoError(t, err)
		defer db.Close()

		// Verify we can query the database.
		var result int
		err = db.QueryRow("SELECT 1").Scan(&result)
		require.NoError(t, err)
		require.Equal(t, 1, result)
	})

	t.Run("ReReadsFileOnNewConnection", func(t *testing.T) {
		t.Parallel()

		connectionURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		// Write connection URL to a temp file.
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "pg_url")
		err = os.WriteFile(filePath, []byte(connectionURL), 0o600)
		require.NoError(t, err)

		// Register the file URL driver.
		driverName, err := pgfileurl.Register("postgres", filePath, slogtest.Make(t, nil))
		require.NoError(t, err)

		// Open a connection and verify it works.
		db, err := sql.Open(driverName, "")
		require.NoError(t, err)
		defer db.Close()

		// Force only one connection in the pool so we can test reconnection.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(0)

		var result int
		err = db.QueryRow("SELECT 1").Scan(&result)
		require.NoError(t, err)
		require.Equal(t, 1, result)

		// Update the file with an invalid URL.
		err = os.WriteFile(filePath, []byte("postgres://invalid:5432/db"), 0o600)
		require.NoError(t, err)

		// Close existing connections to force a new one.
		db.SetMaxIdleConns(0)

		// The next query should fail because it will read the invalid URL.
		// We need to force a new connection by closing the pool.
		db2, err := sql.Open(driverName, "")
		require.NoError(t, err)
		defer db2.Close()

		err = db2.Ping()
		require.Error(t, err)
	})

	t.Run("FileNotFound", func(t *testing.T) {
		t.Parallel()

		driverName, err := pgfileurl.Register("postgres", "/nonexistent/path", slogtest.Make(t, nil))
		require.NoError(t, err) // Registration succeeds

		db, err := sql.Open(driverName, "")
		require.NoError(t, err)
		defer db.Close()

		// Connection should fail when trying to read the file.
		err = db.Ping()
		require.Error(t, err)
		require.Contains(t, err.Error(), "read database URL file")
	})
}
