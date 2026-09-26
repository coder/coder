package agenttoolcall_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const (
	// ageMargin is the margin the spec requires for request transit time.
	ageMargin = 2 * time.Second
	// longRunning is an agent uptime that makes every tool call with a
	// small age provably new to the agent.
	longRunning = time.Hour
)

func TestMiddlewareDecisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// uptime is how long the agent has been running. Zero means
		// longRunning.
		uptime time.Duration
		// handlerStatus is the status the handler answers with. Zero
		// means 201.
		handlerStatus int
		// setup sends earlier requests for the chat.
		setup       func(t *testing.T, h *harness)
		messageID   int64
		age         time.Duration
		body        string
		wantStatus  int
		wantError   workspacesdk.ToolCallErrorCode
		wantBody    string
		wantCalls   int32
		wantCurrent bool
	}{
		{
			name:        "NoRecordRuns",
			messageID:   1,
			wantStatus:  http.StatusCreated,
			wantBody:    "ran 1",
			wantCalls:   1,
			wantCurrent: true,
		},
		{
			name: "RecordReturnsRecordedResponse",
			setup: func(t *testing.T, h *harness) {
				h.requireRun(t, 1, "call", "")
			},
			messageID:   1,
			wantStatus:  http.StatusCreated,
			wantBody:    "ran 1",
			wantCalls:   1,
			wantCurrent: true,
		},
		{
			name:          "RecordReturnsRecordedFailure",
			handlerStatus: http.StatusInternalServerError,
			setup: func(t *testing.T, h *harness) {
				h.requireRun(t, 1, "call", "")
			},
			messageID:   1,
			wantStatus:  http.StatusInternalServerError,
			wantBody:    "ran 1",
			wantCalls:   1,
			wantCurrent: true,
		},
		{
			name: "RecordIgnoresAge",
			setup: func(t *testing.T, h *harness) {
				h.requireRun(t, 1, "call", "")
			},
			messageID:   1,
			age:         2 * time.Hour,
			wantStatus:  http.StatusCreated,
			wantBody:    "ran 1",
			wantCalls:   1,
			wantCurrent: true,
		},
		{
			name: "InputMismatch",
			setup: func(t *testing.T, h *harness) {
				h.requireRun(t, 1, "call", "first")
			},
			messageID:   1,
			body:        "second",
			wantStatus:  http.StatusConflict,
			wantError:   workspacesdk.ToolCallErrorInputMismatch,
			wantCalls:   1,
			wantCurrent: true,
		},
		{
			name: "StaleMessage",
			setup: func(t *testing.T, h *harness) {
				h.requireRun(t, 2, "other", "")
			},
			messageID:  1,
			wantStatus: http.StatusConflict,
			wantError:  workspacesdk.ToolCallErrorStale,
			wantCalls:  1,
		},
		{
			name: "StaleBeforeAgentStartedAfterToolCall",
			setup: func(t *testing.T, h *harness) {
				h.requireRun(t, 2, "other", "")
			},
			messageID:  1,
			age:        2 * time.Hour,
			wantStatus: http.StatusConflict,
			wantError:  workspacesdk.ToolCallErrorStale,
			wantCalls:  1,
		},
		{
			name: "OtherToolCallInLatestMessage",
			setup: func(t *testing.T, h *harness) {
				h.requireRun(t, 1, "other", "")
			},
			messageID:   1,
			wantStatus:  http.StatusCreated,
			wantBody:    "ran 2",
			wantCalls:   2,
			wantCurrent: true,
		},
		{
			name: "SameToolCallIDInNewerMessageRuns",
			setup: func(t *testing.T, h *harness) {
				h.requireRun(t, 1, "call", "first")
			},
			messageID:   2,
			body:        "second",
			wantStatus:  http.StatusCreated,
			wantBody:    "ran 2",
			wantCalls:   2,
			wantCurrent: true,
		},
		{
			name:       "AgentStartedAfterToolCall",
			uptime:     10 * time.Second,
			messageID:  1,
			age:        time.Minute,
			wantStatus: http.StatusConflict,
			wantError:  workspacesdk.ToolCallErrorAgentStartedAfterToolCall,
		},
		{
			name:       "AgentStartedAtMargin",
			uptime:     10 * time.Second,
			messageID:  1,
			age:        10*time.Second - ageMargin,
			wantStatus: http.StatusConflict,
			wantError:  workspacesdk.ToolCallErrorAgentStartedAfterToolCall,
		},
		{
			// The header carries whole milliseconds, so this is the
			// smallest age below the margin boundary.
			name:        "AgentStartedPastMargin",
			uptime:      10 * time.Second,
			messageID:   1,
			age:         10*time.Second - ageMargin - time.Millisecond,
			wantStatus:  http.StatusCreated,
			wantBody:    "ran 1",
			wantCalls:   1,
			wantCurrent: true,
		},
		{
			name:       "AgentStartedAfterMaxAge",
			messageID:  1,
			age:        math.MaxInt64,
			wantStatus: http.StatusConflict,
			wantError:  workspacesdk.ToolCallErrorAgentStartedAfterToolCall,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &countingHandler{status: tt.handlerStatus}
			h := newHarness(t, tt.uptime, next)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			w := h.do(t.Context(), http.MethodPost, "/start", tt.body, h.headers(tt.messageID, "call", tt.age))
			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())
			if tt.wantError != "" {
				assert.Equal(t, tt.wantError, decodeToolCallError(t, w).Code)
			} else {
				assert.Equal(t, tt.wantBody, w.Body.String())
			}
			assert.Equal(t, tt.wantCalls, next.calls.Load())
			assert.Equal(t, tt.wantCurrent, h.store.Current(h.key(tt.messageID, "call")))
		})
	}
}

func TestMiddlewareInputMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
	}{
		{name: "SameRequest", method: http.MethodPost, target: "/start?x=1", body: "a", wantStatus: http.StatusCreated},
		{name: "DifferentBody", method: http.MethodPost, target: "/start?x=1", body: "b", wantStatus: http.StatusConflict},
		{name: "DifferentQuery", method: http.MethodPost, target: "/start?x=2", body: "a", wantStatus: http.StatusConflict},
		{name: "DifferentPath", method: http.MethodPost, target: "/other?x=1", body: "a", wantStatus: http.StatusConflict},
		{name: "DifferentMethod", method: http.MethodPut, target: "/start?x=1", body: "a", wantStatus: http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &countingHandler{}
			h := newHarness(t, 0, next)
			headers := h.headers(1, "call", 0)
			w := h.do(t.Context(), http.MethodPost, "/start?x=1", "a", headers)
			require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

			w = h.do(t.Context(), tt.method, tt.target, tt.body, headers)
			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())
			if tt.wantStatus == http.StatusConflict {
				assert.Equal(t, workspacesdk.ToolCallErrorInputMismatch, decodeToolCallError(t, w).Code)
			}
			assert.EqualValues(t, 1, next.calls.Load())
		})
	}
}

func TestMiddlewareWithoutToolCallHeaders(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, ok := agenttoolcall.FromContext(r.Context())
		body, _ := io.ReadAll(r.Body)
		rw.Header().Set("X-Custom", "value")
		rw.WriteHeader(http.StatusTeapot)
		_, _ = fmt.Fprintf(rw, "%s %t", body, ok)
	})
	h := newHarness(t, 0, next)

	for _, headers := range []http.Header{
		nil,
		{workspacesdk.CoderChatIDHeader: {h.chatID.String()}},
	} {
		for range 2 {
			w := h.do(t.Context(), http.MethodPost, "/start", "payload", headers)
			assert.Equal(t, http.StatusTeapot, w.Code)
			assert.Equal(t, "payload false", w.Body.String())
			assert.Equal(t, "value", w.Header().Get("X-Custom"))
			assert.Empty(t, w.Header().Get(workspacesdk.CoderToolCallRunAgeMsHeader))
		}
	}
}

func TestMiddlewareBadRequest(t *testing.T) {
	t.Parallel()

	toolCallOnly := func() http.Header {
		headers := http.Header{}
		workspacesdk.ToolCall{MessageID: 1, ID: "call"}.SetHeaders(headers)
		return headers
	}
	tests := []struct {
		name    string
		headers func(chatID uuid.UUID) http.Header
	}{
		{
			name:    "WithoutChat",
			headers: func(uuid.UUID) http.Header { return toolCallOnly() },
		},
		{
			name: "PartialHeaders",
			headers: func(chatID uuid.UUID) http.Header {
				return http.Header{
					workspacesdk.CoderChatIDHeader:            {chatID.String()},
					workspacesdk.CoderToolCallMessageIDHeader: {"1"},
				}
			},
		},
		{
			name: "MalformedAge",
			headers: func(chatID uuid.UUID) http.Header {
				headers := toolCallOnly()
				headers.Set(workspacesdk.CoderChatIDHeader, chatID.String())
				headers.Set(workspacesdk.CoderToolCallAgeMsHeader, "-1")
				return headers
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &countingHandler{}
			h := newHarness(t, 0, next)
			w := h.do(t.Context(), http.MethodPost, "/start", "", tt.headers(h.chatID))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Zero(t, next.calls.Load())
			// Nothing was recorded, so a valid request still runs.
			h.requireRun(t, 1, "call", "")
		})
	}
}

func TestMiddlewareToolCallContext(t *testing.T) {
	t.Parallel()

	var (
		got     agenttoolcall.ToolCall
		gotOK   bool
		gotBody string
	)
	next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		got, gotOK = agenttoolcall.FromContext(r.Context())
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		rw.WriteHeader(http.StatusNoContent)
	})
	h := newHarness(t, 0, next)

	w := h.do(t.Context(), http.MethodPost, "/start", "payload", h.headers(1, "call", 0))
	require.Equal(t, http.StatusNoContent, w.Code)
	require.True(t, gotOK)
	assert.Equal(t, h.key(1, "call"), got.Key)
	assert.Equal(t, workspacesdk.ToolCallUUID(h.chatID, 1, "call"), got.UUID)
	assert.Equal(t, "payload", gotBody, "the handler reads the whole body")
}

func TestMiddlewareRecordsResponse(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("X-Not-Recorded", "value")
		rw.WriteHeader(http.StatusAccepted)
		_, _ = rw.Write([]byte(`{"id":"x"}`))
	})
	h := newHarness(t, 0, next)
	headers := h.headers(1, "call", 0)

	for i := range 2 {
		w := h.do(t.Context(), http.MethodPost, "/start", "", headers)
		assert.Equal(t, http.StatusAccepted, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Equal(t, `{"id":"x"}`, w.Body.String())
		if i == 1 {
			assert.Empty(t, w.Header().Get("X-Not-Recorded"), "only Content-Type is recorded")
		}
	}
}

func TestMiddlewareRunAge(t *testing.T) {
	t.Parallel()

	var clock *quartz.Mock
	next := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		clock.Advance(300 * time.Millisecond)
		rw.WriteHeader(http.StatusCreated)
	})
	h := newHarness(t, 0, next)
	clock = h.clock
	headers := h.headers(1, "call", 0)

	w := h.do(t.Context(), http.MethodPost, "/start", "", headers)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "300", w.Header().Get(workspacesdk.CoderToolCallRunAgeMsHeader))

	clock.Advance(1200 * time.Millisecond).MustWait(testutil.Context(t, testutil.WaitShort))
	w = h.do(t.Context(), http.MethodPost, "/start", "", headers)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "1500", w.Header().Get(workspacesdk.CoderToolCallRunAgeMsHeader))
}

func TestNewerMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// age is the age of the request for the newer message.
		age         time.Duration
		wantDropped bool
	}{
		{name: "DropsOlderRecords", wantDropped: true},
		{name: "AgentStartedAfterToolCallKeepsRecords", age: 2 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &countingHandler{}
			h := newHarness(t, 0, next)
			h.requireRun(t, 1, "first", "")
			h.requireRun(t, 1, "second", "")
			require.True(t, h.store.Current(h.key(1, "first")))
			require.True(t, h.store.Current(h.key(1, "second")))

			w := h.do(t.Context(), http.MethodPost, "/start", "", h.headers(2, "newer", tt.age))
			assert.Equal(t, tt.wantDropped, h.store.Current(h.key(2, "newer")))
			if !tt.wantDropped {
				require.Equal(t, http.StatusConflict, w.Code)
				assert.True(t, h.store.Current(h.key(1, "first")))
				assert.True(t, h.store.Current(h.key(1, "second")))
				return
			}
			require.Equal(t, http.StatusCreated, w.Code)
			assert.False(t, h.store.Current(h.key(1, "first")))
			assert.False(t, h.store.Current(h.key(1, "second")))
			w = h.do(t.Context(), http.MethodPost, "/start", "", h.headers(1, "first", 0))
			require.Equal(t, http.StatusConflict, w.Code)
			assert.Equal(t, workspacesdk.ToolCallErrorStale, decodeToolCallError(t, w).Code)
		})
	}
}

func TestChatsAreIndependent(t *testing.T) {
	t.Parallel()

	next := &countingHandler{}
	h := newHarness(t, 0, next)
	chatB := uuid.New()
	headersB := toolCallHeaders(chatB, 1, "call", 0)

	h.requireRun(t, 5, "call", "")
	w := h.do(t.Context(), http.MethodPost, "/start", "", headersB)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, "ran 2", w.Body.String())
	assert.True(t, h.store.Current(h.key(5, "call")))
	assert.True(t, h.store.Current(agenttoolcall.Key{ChatID: chatB, MessageID: 1, ToolCallID: "call"}))
}

func TestMiddlewareDoneContext(t *testing.T) {
	t.Parallel()

	t.Run("NoRecordRuns", func(t *testing.T) {
		t.Parallel()

		next := &countingHandler{}
		h := newHarness(t, 0, next)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		w := h.do(ctx, http.MethodPost, "/start", "", h.headers(1, "call", 0))
		require.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "ran 1", w.Body.String())
	})

	// A done ctx and a published response are both ready, so the response
	// must be checked first. Repeating makes a random choice between the
	// two fail with near certainty.
	t.Run("PublishedResponseWins", func(t *testing.T) {
		t.Parallel()

		next := &countingHandler{}
		h := newHarness(t, 0, next)
		h.requireRun(t, 1, "call", "")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		for range 50 {
			w := h.do(ctx, http.MethodPost, "/start", "", h.headers(1, "call", 0))
			require.Equal(t, http.StatusCreated, w.Code)
			require.Equal(t, "ran 1", w.Body.String())
		}
	})
}

func TestMiddlewareConcurrentRunsOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		h := newHarness(t, 0, next)
		headers := h.headers(1, "call", 0)

		results := []<-chan *httptest.ResponseRecorder{h.goDo(t.Context(), "", headers)}
		<-next.entered
		for range 7 {
			results = append(results, h.goDo(t.Context(), "", headers))
		}
		synctest.Wait()
		for _, res := range results {
			require.Empty(t, res, "no request may be answered before the handler returns")
		}
		require.True(t, h.store.Current(h.key(1, "call")), "a pending record is current")

		close(next.release)
		for _, res := range results {
			w := <-res
			assert.Equal(t, http.StatusCreated, w.Code)
			assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
			assert.Equal(t, "done 1", w.Body.String())
		}
		assert.EqualValues(t, 1, next.calls.Load())
	})
}

func TestMiddlewareInputMismatchDoesNotWait(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		h := newHarness(t, 0, next)
		headers := h.headers(1, "call", 0)

		first := h.goDo(t.Context(), "a", headers)
		<-next.entered
		// Waiting for the pending run here would deadlock the bubble.
		w := h.do(t.Context(), http.MethodPost, "/start", "b", headers)
		require.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, workspacesdk.ToolCallErrorInputMismatch, decodeToolCallError(t, w).Code)

		close(next.release)
		assert.Equal(t, http.StatusCreated, (<-first).Code)
	})
}

func TestMiddlewareWaiterContextEnds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		h := newHarness(t, 0, next)
		headers := h.headers(1, "call", 0)

		first := h.goDo(t.Context(), "", headers)
		<-next.entered
		ctx, cancel := context.WithCancel(t.Context())
		waiter := h.goDo(ctx, "", headers)
		synctest.Wait()

		cancel()
		w := <-waiter
		assert.Zero(t, w.Body.Len(), "a waiter whose context ended writes nothing")
		assert.Empty(t, w.Header())

		close(next.release)
		assert.Equal(t, "done 1", (<-first).Body.String())
		w = h.do(t.Context(), http.MethodPost, "/start", "", headers)
		assert.Equal(t, "done 1", w.Body.String(), "the record is intact for later requests")
	})
}

func TestMiddlewareHandlerPanics(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		var calls atomic.Int32
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			if calls.Add(1) == 1 {
				close(entered)
			}
			<-release
			panic("boom")
		})
		h := newHarness(t, 0, next)
		headers := h.headers(1, "call", 0)

		panicked := make(chan any, 1)
		go func() {
			defer func() { panicked <- recover() }()
			h.do(t.Context(), http.MethodPost, "/start", "", headers)
		}()
		<-entered
		waiter := h.goDo(t.Context(), "", headers)
		synctest.Wait()

		close(release)
		assert.Equal(t, "boom", <-panicked)
		w := <-waiter
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		w = h.do(t.Context(), http.MethodPost, "/start", "", headers)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.EqualValues(t, 1, calls.Load())
	})
}

func TestMiddlewareDroppedPendingRecordWakesWaiters(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		h := newHarness(t, 0, next)
		older := h.headers(1, "call", 0)

		first := h.goDo(t.Context(), "", older)
		<-next.entered
		waiter := h.goDo(t.Context(), "", older)
		synctest.Wait()

		// The newer message's request runs the handler too, which is
		// released below with the first run.
		newer := h.goDo(t.Context(), "", h.headers(2, "call", 0))
		synctest.Wait()
		require.False(t, h.store.Current(h.key(1, "call")))

		close(next.release)
		assert.Equal(t, "done 1", (<-first).Body.String())
		assert.Equal(t, "done 1", (<-waiter).Body.String())
		assert.Equal(t, "done 2", (<-newer).Body.String())
		assert.True(t, h.store.Current(h.key(2, "call")), "publishing a dropped record must not touch current records")
		w := h.do(t.Context(), http.MethodPost, "/start", "", older)
		assert.Equal(t, workspacesdk.ToolCallErrorStale, decodeToolCallError(t, w).Code)
	})
}

// harness serves requests through agentchat.Middleware and the store's
// middleware, the way the agent mounts them.
type harness struct {
	store   *agenttoolcall.Store
	clock   *quartz.Mock
	handler http.Handler
	chatID  uuid.UUID
}

// newHarness returns a harness for an agent that has been running for
// uptime, or longRunning when uptime is zero.
func newHarness(t *testing.T, uptime time.Duration, next http.Handler) *harness {
	t.Helper()

	clock := quartz.NewMock(t)
	store := agenttoolcall.NewStore(clock)
	if uptime == 0 {
		uptime = longRunning
	}
	clock.Advance(uptime).MustWait(testutil.Context(t, testutil.WaitShort))
	return &harness{
		store:   store,
		clock:   clock,
		handler: agentchat.Middleware(store.Middleware(next)),
		chatID:  uuid.New(),
	}
}

func (h *harness) key(messageID int64, toolCallID string) agenttoolcall.Key {
	return agenttoolcall.Key{ChatID: h.chatID, MessageID: messageID, ToolCallID: toolCallID}
}

func (h *harness) headers(messageID int64, toolCallID string, age time.Duration) http.Header {
	return toolCallHeaders(h.chatID, messageID, toolCallID, age)
}

// do serves a request without test assertions, so it can run in a
// goroutine.
func (h *harness) do(ctx context.Context, method, target, body string, headers http.Header) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	for k, v := range headers {
		r.Header[k] = v
	}
	h.handler.ServeHTTP(w, r)
	return w
}

// goDo serves a POST /start in a goroutine.
func (h *harness) goDo(ctx context.Context, body string, headers http.Header) <-chan *httptest.ResponseRecorder {
	res := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		res <- h.do(ctx, http.MethodPost, "/start", body, headers)
	}()
	return res
}

// requireRun sends a POST /start with age zero and requires that it ran
// the handler or returned a recorded response.
func (h *harness) requireRun(t *testing.T, messageID int64, toolCallID, body string) {
	t.Helper()

	w := h.do(t.Context(), http.MethodPost, "/start", body, h.headers(messageID, toolCallID, 0))
	require.NotEqual(t, http.StatusConflict, w.Code, w.Body.String())
	require.NotEqual(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func toolCallHeaders(chatID uuid.UUID, messageID int64, toolCallID string, age time.Duration) http.Header {
	headers := http.Header{workspacesdk.CoderChatIDHeader: {chatID.String()}}
	workspacesdk.ToolCall{MessageID: messageID, ID: toolCallID, Age: age}.SetHeaders(headers)
	return headers
}

func decodeToolCallError(t *testing.T, w *httptest.ResponseRecorder) workspacesdk.ToolCallError {
	t.Helper()

	var resp workspacesdk.ToolCallError
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

// countingHandler answers with status (201 when zero), a plain text
// content type, and a body that counts its runs.
type countingHandler struct {
	status int
	calls  atomic.Int32
}

func (h *countingHandler) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	n := h.calls.Add(1)
	status := h.status
	if status == 0 {
		status = http.StatusCreated
	}
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rw.WriteHeader(status)
	_, _ = fmt.Fprintf(rw, "ran %d", n)
}

// blockingHandler closes entered on its first run and answers once
// release is closed.
type blockingHandler struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{entered: make(chan struct{}), release: make(chan struct{})}
}

func (h *blockingHandler) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	n := h.calls.Add(1)
	h.once.Do(func() { close(h.entered) })
	<-h.release
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rw.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(rw, "done %d", n)
}
