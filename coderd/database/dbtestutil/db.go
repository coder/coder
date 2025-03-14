package dbtestutil

import (
	"errors"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"

	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)
// WillUsePostgres returns true if a call to NewDB() will return a real, postgres-backed Store and Pubsub.
func WillUsePostgres() bool {
	return os.Getenv("DB") != ""

}
type options struct {
	fixedTimezone string
	dumpOnFailure bool
	returnSQLDB   func(*sql.DB)

	logger        slog.Logger
	url           string
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
func WithLogger(logger slog.Logger) Option {

	return func(o *options) {
		o.logger = logger
	}
}
func WithURL(u string) Option {
	return func(o *options) {
		o.url = u

	}
}
func withReturnSQLDB(f func(*sql.DB)) Option {
	return func(o *options) {
		o.returnSQLDB = f
	}

}
func NewDBWithSQLDB(t testing.TB, opts ...Option) (database.Store, pubsub.Pubsub, *sql.DB) {
	t.Helper()
	if !WillUsePostgres() {
		t.Fatal("cannot use NewDBWithSQLDB without PostgreSQL, consider adding `if !dbtestutil.WillUsePostgres() { t.Skip() }` to this test")
	}

	var sqlDB *sql.DB
	opts = append(opts, withReturnSQLDB(func(db *sql.DB) {
		sqlDB = db
	}))
	db, ps := NewDB(t, opts...)
	return db, ps, sqlDB

}
var DefaultTimezone = "Canada/Newfoundland"
// NowInDefaultTimezone returns the current time rounded to the nearest microsecond in the default timezone

// used by postgres in tests. Useful for object equality checks.
func NowInDefaultTimezone() time.Time {
	loc, err := time.LoadLocation(DefaultTimezone)
	if err != nil {

		panic(err)
	}
	return time.Now().In(loc).Round(time.Microsecond)
}
func NewDB(t testing.TB, opts ...Option) (database.Store, pubsub.Pubsub) {
	t.Helper()
	o := options{logger: testutil.Logger(t).Named("pubsub")}
	for _, opt := range opts {

		opt(&o)
	}

	var db database.Store
	var ps pubsub.Pubsub
	if WillUsePostgres() {
		connectionURL := os.Getenv("CODER_PG_CONNECTION_URL")
		if connectionURL == "" && o.url != "" {
			connectionURL = o.url
		}
		if connectionURL == "" {
			var err error
			connectionURL, err = Open(t)

			require.NoError(t, err)
		}
		if o.fixedTimezone == "" {

			// To make sure we find timezone-related issues, we set the timezone
			// of the database to a non-UTC one.
			// The below was picked due to the following properties:
			// - It has a non-UTC offset
			// - It has a fractional hour UTC offset

			// - It includes a daylight savings time component
			o.fixedTimezone = DefaultTimezone
		}
		dbName := dbNameFromConnectionURL(t, connectionURL)
		setDBTimezone(t, connectionURL, dbName, o.fixedTimezone)
		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
		if o.returnSQLDB != nil {
			o.returnSQLDB(sqlDB)
		}

		if o.dumpOnFailure {
			t.Cleanup(func() { DumpOnFailure(t, connectionURL) })
		}
		// Unit tests should not retry serial transaction failures.
		db = database.New(sqlDB, database.WithSerialRetryCount(1))
		ps, err = pubsub.New(context.Background(), o.logger, sqlDB, connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = ps.Close()
		})
	} else {
		db = dbmem.New()

		ps = pubsub.NewInMemory()
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
	dump, err := PGDump(connectionURL)
	if err != nil {
		t.Errorf("dump on failure: failed to run pg_dump")
		return

	}
	if err := os.WriteFile(outPath, normalizeDump(dump), 0o600); err != nil {
		t.Errorf("dump on failure: failed to write: %s", err.Error())
		return
	}
	t.Logf("Dumped database to %q due to failed test. I hope you find what you're looking for!", outPath)
}
// PGDump runs pg_dump against dbURL and returns the output.
// It is used by DumpOnFailure().
func PGDump(dbURL string) ([]byte, error) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return nil, fmt.Errorf("could not find pg_dump in path: %w", err)
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
		return nil, fmt.Errorf("exec pg_dump: %w", err)
	}
	return stdout.Bytes(), nil
}
const minimumPostgreSQLVersion = 13
// PGDumpSchemaOnly is for use by gen/dump only.
// It runs pg_dump against dbURL and sets a consistent timezone and encoding.
func PGDumpSchemaOnly(dbURL string) ([]byte, error) {
	hasPGDump := false
	if _, err := exec.LookPath("pg_dump"); err == nil {
		out, err := exec.Command("pg_dump", "--version").Output()
		if err == nil {
			// Parse output:
			// pg_dump (PostgreSQL) 14.5 (Ubuntu 14.5-0ubuntu0.22.04.1)
			parts := strings.Split(string(out), " ")
			if len(parts) > 2 {
				version, err := strconv.Atoi(strings.Split(parts[2], ".")[0])
				if err == nil && version >= minimumPostgreSQLVersion {
					hasPGDump = true
				}
			}
		}
	}
	cmdArgs := []string{
		"pg_dump",
		"--schema-only",
		dbURL,
		"--no-privileges",
		"--no-owner",
		"--no-privileges",
		"--no-publication",
		"--no-security-labels",
		"--no-subscriptions",

		"--no-tablespaces",
		// We never want to manually generate

		// queries executing against this table.
		"--exclude-table=schema_migrations",
	}
	if !hasPGDump {
		cmdArgs = append([]string{
			"docker",
			"run",
			"--rm",
			"--network=host",
			fmt.Sprintf("gcr.io/coder-dev-1/postgres:%d", minimumPostgreSQLVersion),
		}, cmdArgs...)
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) //#nosec
	cmd.Env = append(os.Environ(), []string{
		"PGTZ=UTC",
		"PGCLIENTENCODING=UTF8",
	}...)
	var output bytes.Buffer
	cmd.Stdout = &output

	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	return normalizeDump(output.Bytes()), nil
}
func normalizeDump(schema []byte) []byte {
	// Remove all comments.
	schema = regexp.MustCompile(`(?im)^(--.*)$`).ReplaceAll(schema, []byte{})
	// Public is implicit in the schema.
	schema = regexp.MustCompile(`(?im)( |::|'|\()public\.`).ReplaceAll(schema, []byte(`$1`))

	// Remove database settings.
	schema = regexp.MustCompile(`(?im)^(SET.*;)`).ReplaceAll(schema, []byte(``))
	// Remove select statements
	schema = regexp.MustCompile(`(?im)^(SELECT.*;)`).ReplaceAll(schema, []byte(``))
	// Removes multiple newlines.

	schema = regexp.MustCompile(`(?im)\n{3,}`).ReplaceAll(schema, []byte("\n\n"))
	return schema
}
// Deprecated: disable foreign keys was created to aid in migrating off
// of the test-only in-memory database. Do not use this in new code.
func DisableForeignKeysAndTriggers(t *testing.T, db database.Store) {
	err := db.DisableForeignKeysAndTriggers(context.Background())
	if t != nil {
		require.NoError(t, err)
	}
	if err != nil {
		panic(err)
	}
}
