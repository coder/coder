package migrations

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

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
	since, stop := r.lockWaitStarted(phaseBatch)
	require.Len(t, sink.Entries(), 1)

	// The run returns on the lock timeout while the attempt still waits.
	r.close()

	// The abandoned attempt gets the lock later, or another Lock starts and
	// never stops its heartbeat: close already stopped both.
	r.setBackend(1, "1")
	stop()
	r.lockAcquired(phaseBatch, since)
	r.lockStarted()
	_, _ = r.lockWaitStarted(phaseBatch)
	mClock.Advance(time.Second).MustWait(ctx)

	entries := sink.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, msgAcquiringLock, entries[0].Message)
}
