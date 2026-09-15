package aibridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestInflightGate_Middleware(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Accepted", "Rejected", "Panic"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			gate := NewInflightGate()
			defer gate.Close()
			if name == "Rejected" {
				gate.Close()
			}
			admitted := false
			var requestCtx context.Context
			rec := httptest.NewRecorder()
			handler := gate.Middleware(func() { admitted = true })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCtx = r.Context()
				require.NoError(t, requestCtx.Err())
				gate.mu.Lock()
				active := gate.active
				gate.mu.Unlock()
				require.Equal(t, 1, active)
				if name == "Panic" {
					panic("handler panic")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			serve := func() {
				handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
			}
			if name == "Panic" {
				require.PanicsWithValue(t, "handler panic", serve)
			} else {
				serve()
			}
			gate.mu.Lock()
			active := gate.active
			gate.mu.Unlock()
			require.Zero(t, active)
			require.NoError(t, t.Context().Err(), "cleanup must not cancel the parent")
			if name == "Rejected" {
				require.False(t, admitted)
				require.Nil(t, requestCtx, "rejected requests must not reach the handler")
				require.Equal(t, http.StatusServiceUnavailable, rec.Code)
				require.Equal(t, "AI Gateway is shutting down\n", rec.Body.String())
				return
			}
			require.True(t, admitted)
			require.ErrorIs(t, requestCtx.Err(), context.Canceled)
			if name == "Accepted" {
				require.Equal(t, http.StatusNoContent, rec.Code)
			}
		})
	}
}

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
