package aibridge

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestInflightGate_Admission(t *testing.T) {
	t.Parallel()
	gate := NewInflightGate()
	calls := 0
	onAdmit := func() { calls++ }

	release, ok := gate.Admit(onAdmit)
	require.True(t, ok)
	require.Equal(t, 1, calls)
	release()

	gate.stopAdmission()
	release, ok = gate.Admit(onAdmit)
	require.False(t, ok)
	require.Nil(t, release)
	require.Equal(t, 1, calls, "rejected admission must not invoke the callback")
}

func TestInflightGate_RequestContext(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"Close", "Cleanup", "Parent"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			gate := NewInflightGate()
			parent, cancel := context.WithCancel(t.Context())
			defer cancel()
			ctx, cleanup := gate.RequestContext(parent)
			defer cleanup()
			require.NoError(t, ctx.Err())

			switch source {
			case "Close":
				gate.Close()
			case "Cleanup":
				cleanup()
				require.ErrorIs(t, ctx.Err(), context.Canceled)
			case "Parent":
				cancel()
			}
			testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, ctx.Done())
			require.ErrorIs(t, ctx.Err(), context.Canceled)
		})
	}
}

func TestInflightGate_Drain(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		requests int
		close    bool
	}{
		{name: "Idle"},
		{name: "Active", requests: 2},
		{name: "Closed", requests: 2, close: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gate := NewInflightGate()
			releases := make([]func(), tc.requests)
			for i := range releases {
				var ok bool
				releases[i], ok = gate.Admit(nil)
				require.True(t, ok)
			}

			if tc.close {
				gate.Close()
				gate.Close() // Repeated Close must be safe.
				_, ok := gate.Admit(nil)
				require.False(t, ok, "Close must stop admission")
			}
			done := gate.stopAdmission()
			require.Equal(t, done, gate.stopAdmission(), "stopping admission is idempotent")
			_, ok := gate.Admit(nil)
			require.False(t, ok)

			// Neither stopping admission nor cancellation releases requests.
			for _, release := range releases {
				select {
				case <-done:
					t.Fatal("drained before the final release")
				default:
				}
				release()
			}
			select {
			case <-done:
			default:
				t.Fatal("an idle gate must drain synchronously")
			}
		})
	}
}

func TestInflightGate_Shutdown(t *testing.T) {
	t.Parallel()
	for _, canceled := range []bool{false, true} {
		name := "Graceful"
		if canceled {
			name = "Canceled"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			gate := NewInflightGate()
			release, ok := gate.Admit(nil)
			require.True(t, ok)
			requestCtx, cleanup := gate.RequestContext(t.Context())
			defer cleanup()
			ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
			defer cancel()

			if canceled {
				cancel()
				require.ErrorIs(t, gate.Shutdown(ctx), context.Canceled)
				require.NoError(t, requestCtx.Err(), "a canceled shutdown must leave requests running")
				_, ok = gate.Admit(nil)
				require.False(t, ok)
				release()
				// Completed drainage takes precedence over an expired context.
				require.NoError(t, gate.Shutdown(ctx))
			} else {
				gate.stopAdmission()
				done := make(chan error, 1)
				go func() { done <- gate.Shutdown(ctx) }()
				release()
				require.NoError(t, testutil.RequireReceive(ctx, t, done))
			}
			require.NoError(t, requestCtx.Err(), "Shutdown must not cancel request contexts")
		})
	}
}

func TestInflightGate_ConcurrentReleaseAndShutdown(t *testing.T) {
	t.Parallel()
	gate := NewInflightGate()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 32 {
		release, ok := gate.Admit(nil)
		require.True(t, ok)
		wg.Go(func() {
			<-start
			release()
		})
		wg.Go(func() {
			<-start
			gate.stopAdmission()
		})
	}
	close(start)
	wg.Wait()
	select {
	case <-gate.stopAdmission():
	default:
		t.Fatal("all requests released but gate is not drained")
	}
	_, ok := gate.Admit(nil)
	require.False(t, ok)
}
