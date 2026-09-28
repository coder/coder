package agenttoolcall_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestCancelDecisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T, h *harness, tc workspacesdk.ToolCall)
		toolCall workspacesdk.ToolCall
		// check sends a request for the tool call after the cancel.
		check     func(t *testing.T, h *harness, tc workspacesdk.ToolCall)
		wantCalls int32
	}{
		{
			name: "RecordWithoutHook",
			setup: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				h.start(t.Context(), tc)
			},
			toolCall: call(cutoff+1, "execute", "call"),
			check: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "ran 1")
			},
			wantCalls: 1,
		},
		{
			name:     "NoRecordAtCutoff",
			toolCall: call(cutoff, "execute", "call"),
			check: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				requireToolCallError(t, h.start(t.Context(), tc), workspacesdk.ToolCallErrorUnknown)
			},
		},
		{
			name:     "NoRecordAboveCutoff",
			toolCall: call(cutoff+1, "execute", "call"),
			check: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				requireToolCallError(t, h.start(t.Context(), tc), workspacesdk.ToolCallErrorCanceled)
			},
		},
		{
			name: "CanceledMarker",
			setup: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				require.Equal(t, http.StatusNoContent, h.cancel(t.Context(), tc).Code)
			},
			toolCall: call(cutoff+1, "execute", "call"),
			check: func(t *testing.T, h *harness, tc workspacesdk.ToolCall) {
				requireToolCallError(t, h.start(t.Context(), tc), workspacesdk.ToolCallErrorCanceled)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &countingHandler{}
			h := newHarness(t, next)
			if tt.setup != nil {
				tt.setup(t, h, tt.toolCall)
			}

			w := h.cancel(t.Context(), tt.toolCall)
			require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
			require.Empty(t, w.Body.String())
			tt.check(t, h, tt.toolCall)
			require.Equal(t, tt.wantCalls, next.calls.Load())
		})
	}
}

func TestCancelBadRequest(t *testing.T) {
	t.Parallel()

	tc := call(cutoff+1, "execute", "call")
	tests := []struct {
		name    string
		id      func(h *harness) uuid.UUID
		headers func(h *harness) http.Header
	}{
		{
			name:    "NoToolCallHeaders",
			headers: func(h *harness) http.Header { return http.Header{workspacesdk.CoderChatIDHeader: {h.chatID.String()}} },
		},
		{
			name: "NoChatHeader",
			headers: func(*harness) http.Header {
				headers := http.Header{}
				tc.SetHeaders(headers)
				return headers
			},
		},
		{
			name: "InvalidToolCallHeaders",
			headers: func(h *harness) http.Header {
				headers := h.headers(tc)
				headers.Set(workspacesdk.CoderToolCallMessageIDHeader, "not-a-number")
				return headers
			},
		},
		{
			name: "IDOfOtherToolCall",
			id: func(h *harness) uuid.UUID {
				return workspacesdk.ToolCallUUID(h.chatID, tc.MessageID, tc.Name, "other")
			},
		},
		{
			name: "IDWithOtherToolName",
			id: func(h *harness) uuid.UUID {
				return workspacesdk.ToolCallUUID(h.chatID, tc.MessageID, "write_file", tc.ID)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &countingHandler{}
			h := newHarness(t, next)
			id := workspacesdk.ToolCallUUID(h.chatID, tc.MessageID, tc.Name, tc.ID)
			if tt.id != nil {
				id = tt.id(h)
			}
			headers := h.headers(tc)
			if tt.headers != nil {
				headers = tt.headers(h)
			}

			w := h.do(t.Context(), http.MethodPost, "/tool-calls/"+id.String()+"/cancel", http.NoBody, headers)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			// Nothing was stored.
			requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "ran 1")
		})
	}
}

// hookHandler registers a cancel hook that counts its calls and returns
// stopErr, then answers 201.
type hookHandler struct {
	stopped chan struct{}
	stopErr error
	stops   atomic.Int32
}

func newHookHandler() *hookHandler {
	return &hookHandler{stopped: make(chan struct{})}
}

func (h *hookHandler) register(rw http.ResponseWriter, r *http.Request, _ int32) {
	tc, ok := agenttoolcall.FromContext(r.Context())
	if !ok {
		panic("no tool call in context")
	}
	tc.OnCancel(func() error {
		h.stops.Add(1)
		return h.stopErr
	}, h.stopped)
	writeText(rw, http.StatusCreated, "started")
}

func TestCancelRunsHookOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		hooks := newHookHandler()
		next := newBlockingHandler()
		close(next.release)
		next.then = hooks.register
		h := newHarness(t, next)
		tc := call(cutoff+1, "execute", "call")

		requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "started")
		require.Zero(t, hooks.stops.Load())

		first := goDo(func() *httptest.ResponseRecorder { return h.cancel(t.Context(), tc) })
		second := goDo(func() *httptest.ResponseRecorder { return h.cancel(t.Context(), tc) })
		synctest.Wait()
		// Both cancels wait for the work to stop.
		require.EqualValues(t, 1, hooks.stops.Load())
		require.Empty(t, first)
		require.Empty(t, second)

		close(hooks.stopped)
		require.Equal(t, http.StatusNoContent, (<-first).Code)
		require.Equal(t, http.StatusNoContent, (<-second).Code)
		require.Equal(t, http.StatusNoContent, h.cancel(t.Context(), tc).Code)
		require.EqualValues(t, 1, hooks.stops.Load())
		requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "started")
	})
}

// A cancel that arrives while the run is in progress marks the record, so
// the hook the run registers afterwards runs at registration, once.
func TestCancelRacesPendingRun(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		hooks := newHookHandler()
		next := newBlockingHandler()
		next.then = hooks.register
		h := newHarness(t, next)
		tc := call(cutoff+1, "execute", "call")

		started := goDo(func() *httptest.ResponseRecorder { return h.start(t.Context(), tc) })
		<-next.entered
		canceled := goDo(func() *httptest.ResponseRecorder { return h.cancel(t.Context(), tc) })
		synctest.Wait()
		require.Zero(t, hooks.stops.Load())

		close(next.release)
		requireRecorded(t, <-started, http.StatusCreated, "started")
		require.EqualValues(t, 1, hooks.stops.Load())
		synctest.Wait()
		require.Empty(t, canceled, "the cancel answers after the work stopped")

		close(hooks.stopped)
		require.Equal(t, http.StatusNoContent, (<-canceled).Code)
		require.EqualValues(t, 1, hooks.stops.Load())
	})
}

func TestCancelHookFails(t *testing.T) {
	t.Parallel()

	hooks := newHookHandler()
	hooks.stopErr = xerrors.New("kill failed")
	next := newBlockingHandler()
	close(next.release)
	next.then = hooks.register
	h := newHarness(t, next)
	tc := call(cutoff+1, "execute", "call")
	requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "started")

	w := h.cancel(t.Context(), tc)
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "kill failed")

	// Once the work stops anyway, the outcome is final.
	close(hooks.stopped)
	require.Equal(t, http.StatusNoContent, h.cancel(t.Context(), tc).Code)
	require.EqualValues(t, 1, hooks.stops.Load())
}
