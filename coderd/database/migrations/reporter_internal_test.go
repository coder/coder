package migrations

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
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

// TestReporterClosed covers a Lock call that golang-migrate abandoned on its
// lock timeout: it keeps waiting after the run returned. Its lock lines are
// still logged, but no heartbeat may tick.
func TestReporterClosed(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	mClock := quartz.NewMock(t)
	sink := testutil.NewFakeSink(t)
	// No database: with the heartbeats stopped, no reporting query runs.
	r := newReporter(nil, newUpOptions(WithLogger(sink.Logger()), WithClock(mClock), WithHeartbeatInterval(time.Second)))

	r.lockStarted()
	r.lockWaitStarted(phaseBatch)
	require.Len(t, sink.Entries(), 1)

	// The run returns on the lock timeout while the attempt still waits.
	r.cancel()

	// The abandoned attempt gets the lock later, or another Lock starts and
	// never stops its heartbeat: cancel already stopped both.
	r.setBackend(1, "1")
	r.lockAcquired(phaseBatch)
	r.lockStarted()
	r.lockWaitStarted(phaseBatch)
	mClock.Advance(time.Second).MustWait(ctx)

	var messages []string
	for _, e := range sink.Entries() {
		messages = append(messages, e.Message)
	}
	require.Equal(t, []string{msgAcquiringLock, msgAcquiredLock, msgAcquiringLock}, messages)
}
