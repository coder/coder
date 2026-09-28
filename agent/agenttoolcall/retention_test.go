package agenttoolcall_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestRetentionActivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		create   func(t *testing.T, h *harness, tc workspacesdk.ToolCall)
		activity func(t *testing.T, h *harness, tc workspacesdk.ToolCall)
	}{
		{
			name:   "RequestExtendsRecord",
			create: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) { h.start(t.Context(), tc) },
			activity: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "ran 1")
			},
		},
		{
			name:   "CancelExtendsRecord",
			create: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) { h.start(t.Context(), tc) },
			activity: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				require.Equal(t, http.StatusNoContent, h.cancel(t.Context(), tc).Code)
			},
		},
		{
			name: "RequestExtendsCanceledMarker",
			create: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				require.Equal(t, http.StatusNoContent, h.cancel(t.Context(), tc).Code)
			},
			activity: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				requireToolCallError(t, h.start(t.Context(), tc), workspacesdk.ToolCallErrorCanceled)
			},
		},
		{
			name:   "OtherToolCallInMessageExtendsRecord",
			create: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) { h.start(t.Context(), tc) },
			activity: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				requireRecorded(t, h.start(t.Context(), call(tc.MessageID, tc.Name, "sibling")), http.StatusCreated, "ran 2")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &countingHandler{}
			h := newHarness(t, next)
			tc := call(cutoff+1, "execute", "call")
			tt.create(t, h, tc)

			h.advance(t, 59*time.Minute)
			tt.activity(t, h, tc)
			h.advance(t, 59*time.Minute)
			h.store.Sweep()
			require.True(t, h.store.Retained(h.key(tc)))

			h.advance(t, time.Minute)
			h.store.Sweep()
			require.False(t, h.store.Retained(h.key(tc)))
			// Eviction raised the cutoff to the message ID.
			requireToolCallError(t, h.start(t.Context(), tc), workspacesdk.ToolCallErrorUnknown)
		})
	}
}

func TestRetentionPendingRunHolds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		h := newHarness(t, next)
		tc := call(cutoff+1, "execute", "call")

		started := goDo(func() *httptest.ResponseRecorder { return h.start(t.Context(), tc) })
		<-next.entered
		h.advance(t, 2*time.Hour)
		h.store.Sweep()
		require.True(t, h.store.Retained(h.key(tc)))

		// The end of the run is activity.
		close(next.release)
		requireRecorded(t, <-started, http.StatusCreated, "done 1")
		h.advance(t, 59*time.Minute)
		h.store.Sweep()
		require.True(t, h.store.Retained(h.key(tc)))
		h.advance(t, time.Minute)
		h.store.Sweep()
		require.False(t, h.store.Retained(h.key(tc)))
	})
}

func TestRetentionRunningHookHolds(t *testing.T) {
	t.Parallel()

	hooks := newHookHandler()
	next := newBlockingHandler()
	close(next.release)
	next.then = hooks.register
	h := newHarness(t, next)
	tc := call(cutoff+1, "execute", "call")
	requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "started")

	h.advance(t, 2*time.Hour)
	h.store.Sweep()
	require.True(t, h.store.Retained(h.key(tc)))

	// The last sweep that saw the work running counts as activity.
	close(hooks.stopped)
	h.advance(t, 59*time.Minute)
	h.store.Sweep()
	require.True(t, h.store.Retained(h.key(tc)))
	h.advance(t, time.Minute)
	h.store.Sweep()
	require.False(t, h.store.Retained(h.key(tc)))
}

func TestRetentionEvictionRaisesCutoff(t *testing.T) {
	t.Parallel()

	next := &countingHandler{}
	h := newHarness(t, next)
	high := call(150, "execute", "call")
	low := call(120, "execute", "call")

	requireRecorded(t, h.start(t.Context(), high), http.StatusCreated, "ran 1")
	h.advance(t, 30*time.Minute)
	requireRecorded(t, h.start(t.Context(), low), http.StatusCreated, "ran 2")
	h.advance(t, 30*time.Minute)
	h.store.Sweep()
	require.False(t, h.store.Retained(h.key(high)))
	require.True(t, h.store.Retained(h.key(low)))

	// A retained record below the new cutoff still replays.
	requireRecorded(t, h.start(t.Context(), low), http.StatusCreated, "ran 2")
	requireToolCallError(t, h.start(t.Context(), call(120, "execute", "new")), workspacesdk.ToolCallErrorUnknown)
	requireToolCallError(t, h.start(t.Context(), call(150, "execute", "new")), workspacesdk.ToolCallErrorUnknown)
	requireToolCallError(t, h.start(t.Context(), call(130, "execute", "new")), workspacesdk.ToolCallErrorUnknown)

	// Evicting a lower message never lowers the cutoff.
	h.advance(t, time.Hour)
	h.store.Sweep()
	require.False(t, h.store.Retained(h.key(low)))
	requireToolCallError(t, h.start(t.Context(), call(140, "execute", "new")), workspacesdk.ToolCallErrorUnknown)
	requireRecorded(t, h.start(t.Context(), call(151, "execute", "new")), http.StatusCreated, "ran 3")
}
