package dbtestutil

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/postgres"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// WillUsePostgres returns true if a call to NewDB() will return a real, postgres-backed Store and Pubsub.
func WillUsePostgres() bool {
	return os.Getenv("DB") != ""
}

type options struct {
	fixedTimezone string
	dumpOnFailure bool
}

type Option func(*options)

// WithTimezone sets the database to the defined timezone.
func WithTimezone(tz string) Option {
	return func(o *options) {
		o.fixedTimezone = tz
	}
}

// WithDumpOnFailure will dump the entire database on test failure.
func WithDumpOnFailure() Option {
	return func(o *options) {
		o.dumpOnFailure = true
	}
}

func NewDB(t testing.TB, opts ...Option) (database.Store, pubsub.Pubsub) {
	t.Helper()

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	db := dbfake.New()
	ps := pubsub.NewInMemory()
	if WillUsePostgres() {
		connectionURL := os.Getenv("CODER_PG_CONNECTION_URL")
		if connectionURL == "" {
			var (
				err     error
				closePg func()
			)
			connectionURL, closePg, err = postgres.Open()
			require.NoError(t, err)
			t.Cleanup(closePg)
		}

		if o.fixedTimezone == "" {
			// To make sure we find timezone-related issues, we set the timezone
			// of the database to a non-UTC one.
			// The below was picked due to the following properties:
			// - It has a non-UTC offset
			// - It has a fractional hour UTC offset
			// - It includes a daylight savings time component
			o.fixedTimezone = "Canada/Newfoundland"
		}
		dbName := dbNameFromConnectionURL(t, connectionURL)
		setDBTimezone(t, connectionURL, dbName, o.fixedTimezone)

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
		if o.dumpOnFailure {
			t.Cleanup(func() { DumpOnFailure(t, connectionURL) })
		}
		db = database.New(sqlDB)

		ps, err = pubsub.New(context.Background(), sqlDB, connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = ps.Close()
		})
	}

	return db, ps
}

// setRandDBTimezone sets the timezone of the database to the given timezone.
// Note that the updated timezone only comes into effect on reconnect, so we
// create our own connection for this and close the DB after we're done.
func setDBTimezone(t testing.TB, dbURL, dbname, tz string) {
	t.Helper()

	sqlDB, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer func() {
		_ = sqlDB.Close()
	}()

	// nolint: gosec // This unfortunately does not work with placeholders.
	_, err = sqlDB.Exec(fmt.Sprintf("ALTER DATABASE %s SET TIMEZONE TO %q", dbname, tz))
	require.NoError(t, err, "failed to set timezone for database")
}

// dbNameFromConnectionURL returns the database name from the given connection URL,
// where connectionURL is of the form postgres://user:pass@host:port/dbname
func dbNameFromConnectionURL(t testing.TB, connectionURL string) string {
	u, err := url.Parse(connectionURL)
	require.NoError(t, err)
	return strings.TrimPrefix(u.Path, "/")
}

// DumpOnFailure exports the database referenced by connectionURL to a file
// corresponding to the current test, with a suffix indicating the time the
// test was run.
// To import this into a new database (assuming you have already run make test-postgres-docker):
//   - Create a new test database:
//     go run ./scripts/migrate-ci/main.go and note the database name it outputs
//   - Import the file into the above database:
//     psql 'postgres://postgres:postgres@127.0.0.1:5432/<dbname>?sslmode=disable' -f <path to file.test.sql>
//   - Run a dev server against that database:
//     ./scripts/coder-dev.sh server --postgres-url='postgres://postgres:postgres@127.0.0.1:5432/<dbname>?sslmode=disable'
func DumpOnFailure(t testing.TB, connectionURL string) {
	if !t.Failed() {
		return
	}
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Errorf("dump on failure: cannot determine current working directory")
		return
	}
	snakeCaseName := regexp.MustCompile("[^a-zA-Z0-9-_]+").ReplaceAllString(t.Name(), "_")
	now := time.Now()
	timeSuffix := fmt.Sprintf("%d%d%d%d%d%d", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())
	outPath := filepath.Join(cwd, snakeCaseName+"."+timeSuffix+".test.sql")
	dump, err := pgDump(connectionURL)
	if err != nil {
		t.Errorf("dump on failure: failed to run pg_dump")
		return
	}
	if err := os.WriteFile(outPath, filterDump(dump), 0o600); err != nil {
		t.Errorf("dump on failure: failed to write: %s", err.Error())
		return
	}
	t.Logf("Dumped database to %q due to failed test. I hope you find what you're looking for!", outPath)
}

// pgDump runs pg_dump against dbURL and returns the output.
func pgDump(dbURL string) ([]byte, error) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return nil, xerrors.Errorf("could not find pg_dump in path: %w", err)
	}
	cmdArgs := []string{
		"pg_dump",
		dbURL,
		"--data-only",
		"--column-inserts",
		"--no-comments",
		"--no-privileges",
		"--no-publication",
		"--no-security-labels",
		"--no-subscriptions",
		"--no-tablespaces",
		// "--no-unlogged-table-data", // some tables are unlogged and may contain data of interest
		"--no-owner",
		"--exclude-table=schema_migrations",
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) // nolint:gosec
	cmd.Env = []string{
		// "PGTZ=UTC", // This is probably not going to be useful if tz has been changed.
		"PGCLIENTENCODING=UTF8",
		"PGDATABASE=", // we should always specify the database name in the connection string
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, xerrors.Errorf("exec pg_dump: %w", err)
	}
	return stdout.Bytes(), nil
}

// Unfortunately, some insert expressions span multiple lines.
// The below may be over-permissive but better that than truncating data.
var insertExpr = regexp.MustCompile(`(?s)\bINSERT[^;]+;`)

func filterDump(dump []byte) []byte {
	var buf bytes.Buffer
	matches := insertExpr.FindAll(dump, -1)
	for _, m := range matches {
		_, _ = buf.Write(m)
		_, _ = buf.WriteRune('\n')
	}
	return buf.Bytes()
}
