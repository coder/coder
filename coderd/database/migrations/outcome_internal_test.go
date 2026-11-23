package migrations

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	failed := xerrors.New("run statement: migration failed")
	for _, tc := range []struct {
		name string
		in   outcomeInput
		want runOutcome
	}{
		{
			name: "LockTimeout",
			in:   outcomeInput{err: migrate.ErrLockTimeout},
			want: outcomeLockTimeout,
		},
		{
			name: "Dirty",
			in:   outcomeInput{err: migrate.ErrDirty{Version: 7}},
			want: outcomeDirty,
		},
		{
			name: "FailedBeforeStart",
			in:   outcomeInput{err: failed},
			want: outcomeFailedBeforeStart,
		},
		{
			name: "NoChange",
			in:   outcomeInput{started: true},
			want: outcomeNoChange,
		},
		{
			name: "Committed",
			in:   outcomeInput{started: true, total: 2, xactStatus: xactCommitted},
			want: outcomeCommitted,
		},
		{
			name: "RolledBack",
			in:   outcomeInput{err: failed, started: true, total: 2, xactStatus: xactAborted},
			want: outcomeRolledBack,
		},
		{
			// For example, the commit's reply was lost, but PostgreSQL did
			// commit.
			name: "CommittedBeforeFailure",
			in:   outcomeInput{err: failed, started: true, total: 2, xactStatus: xactCommitted},
			want: outcomeCommittedBeforeFailure,
		},
		{
			// The connection was lost and PostgreSQL is still finishing the
			// transaction.
			name: "UnknownInProgress",
			in:   outcomeInput{err: failed, started: true, total: 2, xactStatus: "in progress"},
			want: outcomeUnknown,
		},
		{
			name: "UnknownStatusUnreadable",
			in:   outcomeInput{err: failed, started: true, total: 2, xactStatus: xactUnknown},
			want: outcomeUnknown,
		},
		{
			// golang-migrate joins the commit error to the run's error.
			name: "LockTimeoutJoined",
			in:   outcomeInput{err: errors.Join(migrate.ErrLockTimeout, failed)},
			want: outcomeLockTimeout,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, classify(tc.in))
		})
	}
}

// TestFinish covers the outcomes that a run through Up cannot produce
// against a real database: the lock timeout, and failures where only
// PostgreSQL knows whether the transaction committed. All their lines go to
// one golden file.
func TestFinish(t *testing.T) {
	t.Parallel()

	stmtErr := xerrors.Errorf("run statement: %w", database.Error{
		OrigErr: io.ErrUnexpectedEOF,
		Err:     "migration failed",
		Query:   []byte("INSERT INTO secrets VALUES ('row data')"),
	})
	for _, tc := range []struct {
		name string
		// run sets up a reporter that logs to logger, as Up leaves it, and
		// returns finish's error.
		run     func(t *testing.T, logger slog.Logger) error
		wantErr string
	}{
		{
			// golang-migrate gave up on the migration lock. The run never
			// started, so no database is needed.
			name: "lock_timeout",
			run: func(t *testing.T, logger slog.Logger) error {
				mClock := quartz.NewMock(t)
				// No heartbeat may tick: there is no database.
				r := newReporter(nil, newUpOptions(WithLogger(logger), WithClock(mClock), WithHeartbeatInterval(time.Hour)))
				r.lockStarted()
				r.setBackend(1, "1")
				r.upStart = mClock.Now()
				r.lockWaitStarted(phaseBatch)
				mClock.Advance(2 * time.Minute)
				r.cancel()
				return r.finish(migrate.ErrLockTimeout)
			},
			wantErr: "up: timeout: can't acquire database lock",
		},
		{
			// The run failed before the pending migrations were known, for
			// example the batch Lock lost its connection.
			name: "failed_before_start",
			run: func(t *testing.T, logger slog.Logger) error {
				r := newReporter(nil, newUpOptions(WithLogger(logger), WithClock(quartz.NewMock(t))))
				r.cancel()
				return r.finish(xerrors.New("begin tx: driver: bad connection"))
			},
			wantErr: "up: begin tx: driver: bad connection",
		},
		{
			// The commit's reply was lost: golang-migrate returns an error,
			// but PostgreSQL committed.
			name: "committed_before_failure",
			run: func(t *testing.T, logger slog.Logger) error {
				r := newOutcomeReporter(t, logger, &outcomeObserver{status: xactCommitted, version: int64(3)})
				return r.finish(xerrors.New("commit tx on unlock: driver: bad connection"))
			},
			wantErr: "up: commit tx on unlock: driver: bad connection",
		},
		{
			// A statement failed without a PostgreSQL error, here a broken
			// connection, while PostgreSQL is still finishing the
			// transaction. Neither the line nor the error contains the SQL.
			name: "unknown",
			run: func(t *testing.T, logger slog.Logger) error {
				r := newOutcomeReporter(t, logger, &outcomeObserver{status: "in progress", version: int64(1)})
				r.failed = &failedMigration{migrationRef: migrationRef{version: 3, name: "step_3"}, err: io.ErrUnexpectedEOF}
				return r.finish(stmtErr)
			},
			wantErr: "up: migration 3 (step_3): migration failed: unexpected EOF",
		},
	} {
		sink := NewGoldenSink()
		err := tc.run(t, sink.Logger())
		require.EqualError(t, err, tc.wantErr, tc.name)
		RequireGolden(t, "finish_"+tc.name, sink.String())
	}
}

func TestSafeErrorText(t *testing.T) {
	t.Parallel()

	// golang-migrate's own statements fail with a *database.Error whose
	// message prints the query.
	err := xerrors.Errorf("set version: %w", &database.Error{OrigErr: io.EOF, Query: []byte("TRUNCATE schema_migrations")})
	require.Equal(t, "EOF", safeErrorText(nil, err))

	err = xerrors.Errorf("run: %w", database.Error{OrigErr: io.EOF, Err: "migration failed", Query: []byte("SELECT 1")})
	require.Equal(t, "migration failed: EOF", safeErrorText(nil, err))

	require.Equal(t, "boom", safeErrorText(nil, xerrors.New("boom")))
}

// newOutcomeReporter returns a reporter in the state of a run that applied
// migration 2 of 1 to 3 and then ended, with the outcome queries answered by
// observer.
func newOutcomeReporter(t *testing.T, logger slog.Logger, observer *outcomeObserver) *reporter {
	t.Helper()
	db := sql.OpenDB(observer)
	t.Cleanup(func() { _ = db.Close() })

	r := newReporter(nil, newUpOptions(WithLogger(logger), WithClock(quartz.NewMock(t)), WithObserverDB(db)))
	r.started = true
	r.fromVersion = 1
	r.targetVersion = 3
	r.total = 2
	r.xid = "42"
	r.applied = 1
	return r
}

// outcomeObserver is a database that answers the outcome queries with fixed
// values, or fails every query with err.
type outcomeObserver struct {
	status  any // the pg_xact_status result
	version any // the saved schema version, nil for no row
	err     error
}

func (o *outcomeObserver) Connect(context.Context) (driver.Conn, error) {
	return outcomeObserverConn{o: o}, nil
}

func (o *outcomeObserver) Driver() driver.Driver { return outcomeObserverDriver{o: o} }

type outcomeObserverDriver struct{ o *outcomeObserver }

func (d outcomeObserverDriver) Open(string) (driver.Conn, error) { return outcomeObserverConn(d), nil }

type outcomeObserverConn struct{ o *outcomeObserver }

func (outcomeObserverConn) Prepare(string) (driver.Stmt, error) {
	return nil, xerrors.New("not supported")
}
func (outcomeObserverConn) Close() error              { return nil }
func (outcomeObserverConn) Begin() (driver.Tx, error) { return nil, xerrors.New("not supported") }

func (c outcomeObserverConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if c.o.err != nil {
		return nil, c.o.err
	}
	if strings.Contains(query, "pg_xact_status") {
		return &outcomeObserverRows{values: []driver.Value{c.o.status}}, nil
	}
	if c.o.version == nil {
		return &outcomeObserverRows{}, nil
	}
	return &outcomeObserverRows{values: []driver.Value{c.o.version}}, nil
}

// outcomeObserverRows holds at most one row with one column.
type outcomeObserverRows struct {
	values []driver.Value
	read   bool
}

func (*outcomeObserverRows) Columns() []string { return []string{"value"} }
func (*outcomeObserverRows) Close() error      { return nil }

func (r *outcomeObserverRows) Next(dest []driver.Value) error {
	if r.read || r.values == nil {
		return io.EOF
	}
	r.read = true
	dest[0] = r.values[0]
	return nil
}
