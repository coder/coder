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
	"github.com/coder/coder/v2/coderd/database/dbtestutil/randtz"
	"github.com/coder/coder/v2/coderd/database/postgres"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// WillUsePostgres returns true if a call to NewDB() will return a real, postgres-backed Store and Pubsub.
func WillUsePostgres() bool {
	return os.Getenv("DB") != ""
}

type options struct {
	fixedTimezone string
}

type Option func(*options)

// WithTimezone sets the database to the defined timezone instead of
// to a random one.
//
// Deprecated: If you need to use this, you may have a timezone-related bug.
func WithTimezone(tz string) Option {
	return func(o *options) {
		o.fixedTimezone = tz
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
			// of the database to a random one.
			o.fixedTimezone = randtz.Name(t)
		}
		dbName := dbNameFromConnectionURL(t, connectionURL)
		setDBTimezone(t, connectionURL, dbName, o.fixedTimezone)

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
func setDBTimezone(t testing.TB, dbURL, dbname, tz string) {
	t.Helper()

	sqlDB, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer func() {
		// The updated timezone only comes into effect on reconnect, so close
		// the DB after we're done.
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
