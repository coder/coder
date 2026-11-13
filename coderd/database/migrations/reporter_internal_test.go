package migrations

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestFormatPIDs(t *testing.T) {
	t.Parallel()

	require.Empty(t, formatPIDs(nil))
	require.Equal(t, "6652", formatPIDs(pq.Int64Array{6652}))
	require.Equal(t, "6652,6653", formatPIDs(pq.Int64Array{6652, 6653}))
}

// TestReporterClosed covers a Lock call that golang-migrate abandoned on its
// lock timeout: it keeps waiting after the run returned, and nothing it
// reports afterwards may be logged.
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
	r.close()

	// The abandoned attempt gets the lock later, or another Lock starts.
	r.setBackend(1, "1")
	r.lockAcquired(phaseBatch)
	r.lockStarted()
	r.lockWaitStarted(phaseBatch)
	mClock.Advance(time.Second).MustWait(ctx)

	entries := sink.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, "waiting for database migration lock", entries[0].Message)
}
