package migrations

import (
	"database/sql"
	"time"

	"github.com/coder/quartz"
)

// LockID is the migration advisory lock, so tests can hold it.
const LockID = lockID

// Options that only tests use. They live here, compiled only into tests, so
// that package migrations_test can use them; it cannot hold internal tests
// because dbtestutil imports this package.

// WithAppName overrides the application_name of the migration connection, to
// find the connection in pg_stat_activity.
func WithAppName(name string) Option {
	return func(o *upOptions) {
		o.appName = name
	}
}

// WithClock overrides the clock of the heartbeat and all durations.
func WithClock(clock quartz.Clock) Option {
	return func(o *upOptions) {
		o.clock = clock
	}
}

// WithHeartbeatInterval overrides how often a wait or a running migration
// reports that it is still alive.
func WithHeartbeatInterval(interval time.Duration) Option {
	return func(o *upOptions) {
		o.heartbeatInterval = interval
	}
}

// WithObserverDB overrides the pool used for reporting queries.
func WithObserverDB(db *sql.DB) Option {
	return func(o *upOptions) {
		o.observerDB = db
	}
}
