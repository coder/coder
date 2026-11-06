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
	"github.com/coder/coder/v2/testutil"
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
			// The commit returned without an error, so the run committed
			// even if PostgreSQL could not be asked afterwards.
			name: "CommittedStatusUnknown",
			in:   outcomeInput{started: true, total: 2, xactStatus: xactUnknown},
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

// TestFinishLockTimeout covers golang-migrate giving up on the migration
// lock. The run never started, so no database is needed.
func TestFinishLockTimeout(t *testing.T) {
	t.Parallel()

	mClock := quartz.NewMock(t)
	sink := testutil.NewFakeSink(t)
	// A long heartbeat interval: no heartbeat may tick, there is no database.
	r := newReporter(nil, newUpOptions(WithLogger(sink.Logger()), WithClock(mClock), WithHeartbeatInterval(time.Hour)))

	r.lockStarted()
	r.setBackend(1, "1")
	r.lockWaitStarted(phaseBatch)
	mClock.Advance(2 * time.Minute)

	res, err := r.finish(migrate.ErrLockTimeout)
	require.ErrorIs(t, err, migrate.ErrLockTimeout)
	require.Equal(t, "up: timeout: can't acquire database lock", err.Error())
	require.Equal(t, outcomeLockTimeout, res.Outcome)

	line := sink.Entries(func(e slog.SinkEntry) bool {
		return e.Message == "gave up waiting for database migration lock"
	})
	require.Len(t, line, 1)
	require.Equal(t, slog.LevelError, line[0].Level)
	require.Equal(t, []string{"status", "count", "waited_ms"}, entryFieldNames(line[0]))
	require.Equal(t, "failure", entryField(line[0], "status"))
	require.Equal(t, 0, entryField(line[0], "count"))
	require.Equal(t, int64(120000), entryField(line[0], "waited_ms"))
}

// TestFinishWithoutReporter covers plain Up: the error is the same as
// without reporting.
func TestFinishWithoutReporter(t *testing.T) {
	t.Parallel()

	var r *reporter
	_, err := r.finish(nil)
	require.NoError(t, err)

	failed := xerrors.New("boom")
	_, err = r.finish(failed)
	require.ErrorIs(t, err, failed)
	require.Equal(t, "up: boom", err.Error())
}

// TestFinishCommittedBeforeFailure covers a commit whose reply was lost:
// golang-migrate returns an error, but PostgreSQL committed.
func TestFinishCommittedBeforeFailure(t *testing.T) {
	t.Parallel()

	sink := testutil.NewFakeSink(t)
	r := newOutcomeReporter(t, sink, &outcomeObserver{status: xactCommitted, version: int64(3)})

	res, err := r.finish(xerrors.New("commit tx on unlock: driver: bad connection"))
	require.EqualError(t, err, "up: commit tx on unlock: driver: bad connection")
	require.Equal(t, outcomeCommittedBeforeFailure, res.Outcome)

	line := onlyOutcomeEntry(t, sink, "committed database migrations before failure")
	require.Equal(t, slog.LevelError, line.Level)
	require.Equal(t, []string{
		"status", "count", "from_version", "to_version", "transaction_status", "total_duration_ms",
	}, entryFieldNames(line))
	require.Equal(t, "failure", entryField(line, "status"))
	require.Equal(t, 1, entryField(line, "count"))
	require.Equal(t, 1, entryField(line, "from_version"))
	require.Equal(t, 3, entryField(line, "to_version"))
	require.Equal(t, xactCommitted, entryField(line, "transaction_status"))
}

// TestFinishUnknown covers a statement that failed without a PostgreSQL
// error, here a broken connection, while PostgreSQL is still finishing the
// transaction. Neither the line nor the returned error may contain the SQL.
func TestFinishUnknown(t *testing.T) {
	t.Parallel()

	sink := testutil.NewFakeSink(t)
	r := newOutcomeReporter(t, sink, &outcomeObserver{status: "in progress", version: int64(1)})
	r.failed = &failedMigration{migrationRef: migrationRef{version: 3, name: "step_3"}, err: io.ErrUnexpectedEOF}

	stmtErr := xerrors.Errorf("run statement: %w", database.Error{
		OrigErr: io.ErrUnexpectedEOF,
		Err:     "migration failed",
		Query:   []byte("INSERT INTO secrets VALUES ('row data')"),
	})
	res, err := r.finish(stmtErr)
	require.Equal(t, outcomeUnknown, res.Outcome)
	require.EqualError(t, err, "up: migration 3 (step_3): migration failed: unexpected EOF")
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)

	line := onlyOutcomeEntry(t, sink, "database migration commit outcome unknown")
	require.Equal(t, slog.LevelWarn, line.Level)
	require.Equal(t, []string{
		"status", "count", "version", "transaction_status", "total_duration_ms", "error",
	}, entryFieldNames(line))
	require.Equal(t, "failure", entryField(line, "status"))
	require.Equal(t, 1, entryField(line, "version"))
	require.Equal(t, "in progress", entryField(line, "transaction_status"))
	require.Equal(t, "migration 3 (step_3): migration failed: unexpected EOF", entryField(line, "error"))
}

// TestFinishCommittedVersionUnreadable covers a commit that succeeded while
// PostgreSQL could not be asked afterwards: the target version was saved.
func TestFinishCommittedVersionUnreadable(t *testing.T) {
	t.Parallel()

	sink := testutil.NewFakeSink(t)
	r := newOutcomeReporter(t, sink, &outcomeObserver{err: xerrors.New("observer unavailable")})

	res, err := r.finish(nil)
	require.NoError(t, err)
	require.Equal(t, outcomeCommitted, res.Outcome)

	line := onlyOutcomeEntry(t, sink, "committed database migrations")
	require.Equal(t, 3, entryField(line, "to_version"))
	require.Equal(t, xactUnknown, entryField(line, "transaction_status"))
	onlyOutcomeEntry(t, sink, "database migration progress details unavailable")
}

func TestSafeErrorText(t *testing.T) {
	t.Parallel()

	// golang-migrate's own statements fail with a *database.Error whose
	// message prints the query.
	err := xerrors.Errorf("set version: %w", &database.Error{OrigErr: io.EOF, Query: []byte("TRUNCATE schema_migrations")})
	require.Equal(t, "EOF", safeErrorText(runResult{}, err))

	err = xerrors.Errorf("run: %w", database.Error{OrigErr: io.EOF, Err: "migration failed", Query: []byte("SELECT 1")})
	require.Equal(t, "migration failed: EOF", safeErrorText(runResult{}, err))

	require.Equal(t, "boom", safeErrorText(runResult{}, xerrors.New("boom")))
}

// newOutcomeReporter returns a reporter in the state of a run that applied
// migration 2 of 1 to 3 and then ended, with the outcome queries answered by
// observer.
func newOutcomeReporter(t *testing.T, sink *testutil.FakeSink, observer *outcomeObserver) *reporter {
	t.Helper()
	db := sql.OpenDB(observer)
	t.Cleanup(func() { _ = db.Close() })

	r := newReporter(nil, newUpOptions(WithLogger(sink.Logger()), WithClock(quartz.NewMock(t)), WithObserverDB(db)))
	r.started = true
	r.fromVersion = 1
	r.targetVersion = 3
	r.total = 2
	r.xid = "42"
	r.applied = []appliedMigration{{migrationRef: migrationRef{version: 2, name: "step_2"}}}
	return r
}

func onlyOutcomeEntry(t *testing.T, sink *testutil.FakeSink, msg string) slog.SinkEntry {
	t.Helper()
	entries := sink.Entries(func(e slog.SinkEntry) bool { return e.Message == msg })
	require.Len(t, entries, 1, "lines with message %q", msg)
	return entries[0]
}

func entryFieldNames(e slog.SinkEntry) []string {
	names := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		names = append(names, f.Name)
	}
	return names
}

func entryField(e slog.SinkEntry, name string) any {
	for _, f := range e.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	return nil
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
