package aibridge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestInflightGate_AdmitsUntilShutdown asserts Admit succeeds before shutdown
// and is refused after BeginShutdown.
func TestInflightGate_AdmitsUntilShutdown(t *testing.T) {
	t.Parallel()

	gate := NewInflightGate()

	release, ok := gate.Admit(nil)
	require.True(t, ok)
	release()

	gate.BeginShutdown()
	_, ok = gate.Admit(nil)
	require.False(t, ok, "Admit must be refused once shutdown has begun")
}

// TestInflightGate_OnAdmitRunsInsideLock asserts the onAdmit hook runs only on
// admission, between the shutdown check and the count.
func TestInflightGate_OnAdmitRunsInsideLock(t *testing.T) {
	t.Parallel()

	gate := NewInflightGate()

	var ran bool
	release, ok := gate.Admit(func() { ran = true })
	require.True(t, ok)
	require.True(t, ran, "onAdmit must run on admission")
	release()

	// A refused admission must not run the hook.
	gate.BeginShutdown()
	ran = false
	_, ok = gate.Admit(func() { ran = true })
	require.False(t, ok)
	require.False(t, ran, "onAdmit must not run when refused")
}

// TestInflightGate_DrainedWaitsForRelease asserts the drain channel closes only
// after every admitted request releases.
func TestInflightGate_DrainedWaitsForRelease(t *testing.T) {
	t.Parallel()

	gate := NewInflightGate()
	release, ok := gate.Admit(nil)
	require.True(t, ok)

	done := gate.BeginShutdown()
	select {
	case <-done:
		require.Fail(t, "drain completed before the admitted request released")
	default:
	}

	release()
	testCtx := testutil.Context(t, testutil.WaitShort)
	select {
	case <-done:
	case <-testCtx.Done():
		t.Fatal("drain did not complete after the admitted request released")
	}
}

// TestInflightGate_BeginShutdownIdempotent asserts repeated calls return the
// same drain channel and only the first closes admission.
func TestInflightGate_BeginShutdownIdempotent(t *testing.T) {
	t.Parallel()

	gate := NewInflightGate()
	first := gate.BeginShutdown()
	second := gate.BeginShutdown()
	require.Equal(t, first, second, "BeginShutdown must return the same channel")

	testCtx := testutil.Context(t, testutil.WaitShort)
	select {
	case <-first:
	case <-testCtx.Done():
		t.Fatal("drain did not complete for an idle gate")
	}

	_, ok := gate.Admit(nil)
	require.False(t, ok, "Admit must stay refused after repeated BeginShutdown")
}

// TestInflightGate_TrackCancelsOnForcedShutdown asserts a tracked request
// context is canceled by CancelInflight.
func TestInflightGate_TrackCancelsOnForcedShutdown(t *testing.T) {
	t.Parallel()

	gate := NewInflightGate()
	ctx, stop := gate.Track(context.Background())
	defer stop()

	require.NoError(t, ctx.Err())
	gate.CancelInflight()

	testCtx := testutil.Context(t, testutil.WaitShort)
	select {
	case <-ctx.Done():
	case <-testCtx.Done():
		t.Fatal("tracked context was not canceled by CancelInflight")
	}
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}

// TestInflightGate_TrackStopCancels asserts the returned stop cancels the
// tracked context, which callers rely on to release it when a request ends.
func TestInflightGate_TrackStopCancels(t *testing.T) {
	t.Parallel()

	gate := NewInflightGate()
	ctx, stop := gate.Track(context.Background())
	require.NoError(t, ctx.Err())

	stop()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}
