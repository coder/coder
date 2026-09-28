package agenttoolcall_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestMiddlewareDecisions(t *testing.T) {
	t.Parallel()

	first := call(cutoff+1, "execute", "call")
	tests := []struct {
		name string
		// handlerStatus is the status the handler answers with. Zero
		// means 201.
		handlerStatus int
		setup         func(t *testing.T, h *harness)
		method        string
		target        string
		toolCall      workspacesdk.ToolCall
		wantStatus    int
		wantCode      workspacesdk.ToolCallErrorCode
		wantBody      string
		wantCalls     int32
	}{
		{
			name:       "NoRecordAboveCutoffRuns",
			toolCall:   first,
			wantStatus: http.StatusCreated,
			wantBody:   "ran 1",
			wantCalls:  1,
		},
		{
			name:       "RecordReplaysResponse",
			setup:      func(t *testing.T, h *harness) { h.start(t.Context(), first) },
			toolCall:   first,
			wantStatus: http.StatusCreated,
			wantBody:   "ran 1",
			wantCalls:  1,
		},
		{
			name:          "RecordReplaysFailure",
			handlerStatus: http.StatusInternalServerError,
			setup:         func(t *testing.T, h *harness) { h.start(t.Context(), first) },
			toolCall:      first,
			wantStatus:    http.StatusInternalServerError,
			wantBody:      "ran 1",
			wantCalls:     1,
		},
		{
			name: "CanceledMarker",
			setup: func(t *testing.T, h *harness) {
				require.Equal(t, http.StatusNoContent, h.cancel(t.Context(), first).Code)
			},
			toolCall:   first,
			wantStatus: http.StatusConflict,
			wantCode:   workspacesdk.ToolCallErrorCanceled,
		},
		{
			name:       "NoRecordAtCutoffUnknown",
			toolCall:   call(cutoff, "execute", "call"),
			wantStatus: http.StatusConflict,
			wantCode:   workspacesdk.ToolCallErrorUnknown,
		},
		{
			name:       "NoRecordBelowCutoffUnknown",
			toolCall:   call(cutoff-1, "execute", "call"),
			wantStatus: http.StatusConflict,
			wantCode:   workspacesdk.ToolCallErrorUnknown,
		},
		{
			name:       "RecordWithOtherMethod",
			setup:      func(t *testing.T, h *harness) { h.start(t.Context(), first) },
			method:     http.MethodPut,
			toolCall:   first,
			wantStatus: http.StatusBadRequest,
			wantCalls:  1,
		},
		{
			name:       "RecordWithOtherPath",
			setup:      func(t *testing.T, h *harness) { h.start(t.Context(), first) },
			target:     "/other",
			toolCall:   first,
			wantStatus: http.StatusBadRequest,
			wantCalls:  1,
		},
		{
			name:       "RecordWithOtherQuery",
			setup:      func(t *testing.T, h *harness) { h.start(t.Context(), first) },
			target:     "/start?wait=true",
			toolCall:   first,
			wantStatus: http.StatusBadRequest,
			wantCalls:  1,
		},
		{
			name:       "SameIDOtherToolRuns",
			setup:      func(t *testing.T, h *harness) { h.start(t.Context(), first) },
			toolCall:   call(cutoff+1, "write_file", "call"),
			wantStatus: http.StatusCreated,
			wantBody:   "ran 2",
			wantCalls:  2,
		},
		{
			name:       "SameIDOtherMessageRuns",
			setup:      func(t *testing.T, h *harness) { h.start(t.Context(), first) },
			toolCall:   call(cutoff+2, "execute", "call"),
			wantStatus: http.StatusCreated,
			wantBody:   "ran 2",
			wantCalls:  2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &countingHandler{status: tt.handlerStatus}
			h := newHarness(t, next)
			if tt.setup != nil {
				tt.setup(t, h)
			}
			method, target := tt.method, tt.target
			if method == "" {
				method = http.MethodPost
			}
			if target == "" {
				target = "/start"
			}

			w := h.do(t.Context(), method, target, strings.NewReader("body"), h.headers(tt.toolCall))
			switch {
			case tt.wantCode != "":
				requireToolCallError(t, w, tt.wantCode)
			case tt.wantBody != "":
				requireRecorded(t, w, tt.wantStatus, tt.wantBody)
			default:
				require.Equal(t, tt.wantStatus, w.Code, w.Body.String())
			}
			require.Equal(t, tt.wantCalls, next.calls.Load())
		})
	}
}

func TestMiddlewareWithoutToolCallHeaders(t *testing.T) {
	t.Parallel()

	var calls int
	next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		calls++
		_, ok := agenttoolcall.FromContext(r.Context())
		require.False(t, ok)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		writeText(rw, http.StatusOK, string(body))
	})
	// Without a cutoff, requests with tool call headers are refused, so
	// this also shows the middleware ignores requests without them.
	h := newHarnessWithoutCutoff(t, next)

	w := h.do(t.Context(), http.MethodPost, "/start", strings.NewReader("one"), nil)
	requireRecorded(t, w, http.StatusOK, "one")
	chatOnly := http.Header{workspacesdk.CoderChatIDHeader: {h.chatID.String()}}
	w = h.do(t.Context(), http.MethodPost, "/start", strings.NewReader("two"), chatOnly)
	requireRecorded(t, w, http.StatusOK, "two")
	require.Equal(t, 2, calls)
}

func TestMiddlewareBadRequest(t *testing.T) {
	t.Parallel()

	next := &countingHandler{}
	h := newHarness(t, next)

	noChat := http.Header{}
	call(cutoff+1, "execute", "call").SetHeaders(noChat)
	w := h.do(t.Context(), http.MethodPost, "/start", http.NoBody, noChat)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	invalid := h.headers(call(cutoff+1, "execute", "call"))
	invalid.Set(workspacesdk.CoderToolCallMessageIDHeader, "not-a-number")
	w = h.do(t.Context(), http.MethodPost, "/start", http.NoBody, invalid)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	require.Zero(t, next.calls.Load())
}

// Without a cutoff message ID, the agent cannot prove it never received a
// tool call, so it never acts on one.
func TestMiddlewareNoCutoff(t *testing.T) {
	t.Parallel()

	next := &countingHandler{}
	h := newHarnessWithoutCutoff(t, next)
	tc := call(150, "execute", "call")

	requireToolCallError(t, h.start(t.Context(), tc), workspacesdk.ToolCallErrorUnknown)
	// The cancel stores nothing: a canceled marker would outlive the
	// missing proof.
	require.Equal(t, http.StatusNoContent, h.cancel(t.Context(), tc).Code)
	require.Zero(t, next.calls.Load())

	// The first manifest's value wins.
	h.store.SetLastChatMessageID(cutoff)
	h.store.SetLastChatMessageID(200)
	requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "ran 1")
}

func TestMiddlewareToolCallContext(t *testing.T) {
	t.Parallel()

	tc := call(cutoff+1, "execute", "call")
	var got *agenttoolcall.ToolCall
	next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var ok bool
		got, ok = agenttoolcall.FromContext(r.Context())
		require.True(t, ok)
		rw.WriteHeader(http.StatusNoContent)
	})
	h := newHarness(t, next)

	require.Equal(t, http.StatusNoContent, h.start(t.Context(), tc).Code)
	require.NotNil(t, got)
	require.Equal(t, h.key(tc), got.Key)
	require.Equal(t, workspacesdk.ToolCallUUID(h.chatID, tc.MessageID, tc.Name, tc.ID), got.UUID)
}

// A replay never reads the request body, so it answers even when the
// body never ends.
func TestMiddlewareReplayDoesNotReadBody(t *testing.T) {
	t.Parallel()

	next := &countingHandler{}
	h := newHarness(t, next)
	tc := call(cutoff+1, "execute", "call")
	requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "ran 1")

	body, bodyWriter := io.Pipe()
	defer bodyWriter.Close()
	w := h.do(t.Context(), http.MethodPost, "/start", body, h.headers(tc))
	requireRecorded(t, w, http.StatusCreated, "ran 1")
}

func TestMiddlewareConcurrentRequestsRunOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		h := newHarness(t, next)
		tc := call(cutoff+1, "execute", "call")

		results := make([]<-chan *httptest.ResponseRecorder, 5)
		for i := range results {
			results[i] = goDo(func() *httptest.ResponseRecorder { return h.start(t.Context(), tc) })
		}
		<-next.entered
		synctest.Wait()
		close(next.release)
		for _, res := range results {
			w := <-res
			requireRecorded(t, w, http.StatusCreated, "done 1")
		}
		require.EqualValues(t, 1, next.calls.Load())
	})
}

func TestMiddlewareWaiterContextEnds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		h := newHarness(t, next)
		tc := call(cutoff+1, "execute", "call")

		first := goDo(func() *httptest.ResponseRecorder { return h.start(t.Context(), tc) })
		<-next.entered
		ctx, cancel := context.WithCancel(t.Context())
		waiter := goDo(func() *httptest.ResponseRecorder { return h.start(ctx, tc) })
		synctest.Wait()
		cancel()
		w := <-waiter
		require.Empty(t, w.Body.String())

		close(next.release)
		requireRecorded(t, <-first, http.StatusCreated, "done 1")
	})
}

// abandonFirst abandons its first run because it "could not read" the
// body, and echoes the body on later runs.
func abandonFirst(rw http.ResponseWriter, r *http.Request, n int32) {
	tc, ok := agenttoolcall.FromContext(r.Context())
	if !ok {
		panic("no tool call in context")
	}
	if n == 1 {
		tc.Abandon()
		writeText(rw, http.StatusBadRequest, "unreadable body")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	writeText(rw, http.StatusCreated, fmt.Sprintf("ran %d with %s", n, body))
}

func TestMiddlewareAbandonedRunRetries(t *testing.T) {
	t.Parallel()

	t.Run("NextRequest", func(t *testing.T) {
		t.Parallel()

		next := newBlockingHandler()
		close(next.release)
		next.then = abandonFirst
		h := newHarness(t, next)
		tc := call(cutoff+1, "execute", "call")

		requireRecorded(t, h.start(t.Context(), tc), http.StatusBadRequest, "unreadable body")
		requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "ran 2 with body")
		requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "ran 2 with body")
	})

	// A request waiting for the abandoned run runs the handler with its
	// own body, which the middleware left unread.
	t.Run("Waiter", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			next := newBlockingHandler()
			next.then = abandonFirst
			h := newHarness(t, next)
			tc := call(cutoff+1, "execute", "call")

			first := goDo(func() *httptest.ResponseRecorder { return h.start(t.Context(), tc) })
			<-next.entered
			waiter := goDo(func() *httptest.ResponseRecorder {
				return h.do(t.Context(), http.MethodPost, "/start", strings.NewReader("waiter body"), h.headers(tc))
			})
			synctest.Wait()
			close(next.release)

			requireRecorded(t, <-first, http.StatusBadRequest, "unreadable body")
			requireRecorded(t, <-waiter, http.StatusCreated, "ran 2 with waiter body")
		})
	})
}

func TestMiddlewareAbandonedRunAfterCancel(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		next.then = abandonFirst
		h := newHarness(t, next)
		tc := call(cutoff+1, "execute", "call")

		first := goDo(func() *httptest.ResponseRecorder { return h.start(t.Context(), tc) })
		<-next.entered
		canceled := goDo(func() *httptest.ResponseRecorder { return h.cancel(t.Context(), tc) })
		waiter := goDo(func() *httptest.ResponseRecorder { return h.start(t.Context(), tc) })
		synctest.Wait()
		close(next.release)

		requireRecorded(t, <-first, http.StatusBadRequest, "unreadable body")
		require.Equal(t, http.StatusNoContent, (<-canceled).Code)
		requireToolCallError(t, <-waiter, workspacesdk.ToolCallErrorCanceled)
		requireToolCallError(t, h.start(t.Context(), tc), workspacesdk.ToolCallErrorCanceled)
		require.EqualValues(t, 1, next.calls.Load())
	})
}

func TestMiddlewareHandlerPanics(t *testing.T) {
	t.Parallel()

	var calls int
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
		panic("handler failed")
	})
	h := newHarness(t, next)
	tc := call(cutoff+1, "execute", "call")

	require.PanicsWithValue(t, "handler failed", func() { h.start(t.Context(), tc) })
	w := h.start(t.Context(), tc)
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.Equal(t, 1, calls)
}

// Records are keyed by chat, so one chat can neither replay nor cancel
// another chat's tool call.
func TestChatIsolation(t *testing.T) {
	t.Parallel()

	next := &countingHandler{}
	h := newHarness(t, next)
	tc := call(cutoff+1, "execute", "call")
	otherChat := uuid.New()

	requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "ran 1")
	w := h.do(t.Context(), http.MethodPost, "/start", strings.NewReader("body"), toolCallHeaders(otherChat, tc))
	requireRecorded(t, w, http.StatusCreated, "ran 2")

	// The other chat addresses the first chat's tool call UUID.
	id := workspacesdk.ToolCallUUID(h.chatID, tc.MessageID, tc.Name, tc.ID)
	w = h.do(t.Context(), http.MethodPost, "/tool-calls/"+id.String()+"/cancel", http.NoBody, toolCallHeaders(otherChat, tc))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// A canceled marker in one chat does not affect the other.
	unsent := call(cutoff+2, "execute", "call")
	require.Equal(t, http.StatusNoContent, h.cancelIn(t.Context(), otherChat, unsent).Code)
	requireRecorded(t, h.start(t.Context(), unsent), http.StatusCreated, "ran 3")
	requireRecorded(t, h.start(t.Context(), tc), http.StatusCreated, "ran 1")
}
