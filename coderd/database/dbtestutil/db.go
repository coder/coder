package dbtestutil

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
	fixedTimezone bool
}

type Option func(*options)

// WithFixedTimezone disables the random timezone setting for the database.
//
// DEPRECATED: If you need to use this, you may have a timezone-related bug.
func WithFixedTimezone() Option {
	return func(o *options) {
		o.fixedTimezone = true
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

		if !o.fixedTimezone {
			// To make sure we find timezone-related issues, we set the timezone of the database to a random one.
			dbName := dbNameFromConnectionURL(t, connectionURL)
			setRandDBTimezone(t, connectionURL, dbName)
		}

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
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
// The timezone change does not take effect until the next session, so
// we do this in our own connection
func setRandDBTimezone(t testing.TB, dbURL, dbname string) {
	t.Helper()

	sqlDB, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer func() {
		_ = sqlDB.Close()
	}()

	// Pick a random timezone. We can simply pick from pg_timezone_names.
	var tz string
	err = sqlDB.QueryRow("SELECT name FROM pg_timezone_names ORDER BY RANDOM() LIMIT 1").Scan(&tz)
	require.NoError(t, err)

	// Set the timezone for the database.
	t.Logf("setting timezone of database %q to %q", dbname, tz)
	// We apparently can't use placeholders here, sadly.
	// nolint: gosec // This is not user input and this is only executed in tests
	_, err = sqlDB.Exec(fmt.Sprintf("ALTER DATABASE %s SET TIMEZONE TO %q", dbname, tz))
	require.NoError(t, err, "failed to set timezone for database")

	// Ensure the timezone was set.
	// We need to reconnect for this.
	_ = sqlDB.Close()

	sqlDB, err = sql.Open("postgres", dbURL)
	defer func() {
		_ = sqlDB.Close()
	}()
	require.NoError(t, err, "failed to reconnect to database")
	var dbTz string
	err = sqlDB.QueryRow("SHOW TIMEZONE").Scan(&dbTz)
	require.NoError(t, err, "failed to get timezone from database")
	require.Equal(t, tz, dbTz, "database timezone was not set correctly")
}

// dbNameFromConnectionURL returns the database name from the given connection URL.
func dbNameFromConnectionURL(t testing.TB, connectionURL string) string {
	// connectionURL is of the form postgres://user:pass@host:port/dbname
	// We want to extract the dbname part.
	u, err := url.Parse(connectionURL)
	require.NoError(t, err)
	return strings.TrimPrefix(u.Path, "/")
}
