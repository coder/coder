package migrations

import (
	"bytes"
	"context"
	"database/sql"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// LockID is the migration advisory lock, so tests can hold it.
const LockID = lockID

// To update the golden files:
// make gen/golden-files
var updateGoldenFiles = flag.Bool("update", false, "update .golden files")

// GoldenSink writes log entries in the server's human-readable format
// (sloghuman) without the timestamp, so a run on a mock clock can be
// compared byte for byte. It is shared by the internal and external tests.
type GoldenSink struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	human slog.Sink
	pids  map[int]string
}

// zeroTimePrefix is what sloghuman writes for a zero time.
const zeroTimePrefix = "0001-01-01 00:00:00.000 "

func NewGoldenSink() *GoldenSink {
	s := &GoldenSink{pids: map[int]string{}}
	s.human = sloghuman.Sink(&s.buf)
	return s
}

func (s *GoldenSink) Logger() slog.Logger {
	return slog.Make(s).Leveled(slog.LevelDebug)
}

// NamePID makes String write pid as name. PostgreSQL assigns a new backend
// PID to every connection, so a raw PID would change the output on every
// run. A name keeps it stable and still shows that blocked_by reports the
// session the test made block the migration.
func (s *GoldenSink) NamePID(pid int, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pids[pid] = name
}

func (s *GoldenSink) LogEntry(ctx context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.Time = time.Time{}
	s.human.LogEntry(ctx, e)
}

func (*GoldenSink) Sync() {}

func (s *GoldenSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := strings.ReplaceAll(s.buf.String(), "\n"+zeroTimePrefix, "\n")
	out = strings.TrimPrefix(out, zeroTimePrefix)
	for pid, name := range s.pids {
		// sloghuman writes a single blocked_by PID as [6652].
		out = strings.ReplaceAll(out, "["+strconv.Itoa(pid)+"]", "["+name+"]")
	}
	return out
}

// RequireGolden compares got with testdata/reporter/name.golden.
func RequireGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "reporter", name+".golden")
	if *updateGoldenFiles {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "read golden file, run \"make gen/golden-files\" to create it")
	require.Equal(t, string(want), got, "golden file %s differs, run \"make gen/golden-files\" if the change is intended", path)
}

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
