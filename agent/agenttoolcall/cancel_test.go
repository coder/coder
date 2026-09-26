package agenttoolcall_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCancelHandler(t *testing.T) {
	t.Parallel()

	exitCode := 0
	state := workspacesdk.ToolCallProcess{Canceled: true, Output: "partial", ExitCode: &exitCode, RunAgeMs: 1200}

	t.Run("NeverReceived", func(t *testing.T) {
		t.Parallel()

		next := &countingHandler{}
		h := newHarness(t, 0, next)
		stopper := &fakeStopper{}
		for range 2 {
			resp := h.requireCancel(t, stopper, 1, "call", `{"stop_if_run_age_below_ms":1000}`)
			assert.Equal(t, workspacesdk.CancelToolCallResponse{}, resp)
		}
		assert.True(t, h.store.Current(h.key(1, "call")))
		assert.Empty(t, stopper.calls())

		w := h.do(t.Context(), http.MethodPost, "/start", "", h.headers(1, "call", 0))
		require.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, workspacesdk.ToolCallErrorCanceled, decodeToolCallError(t, w).Code)
		assert.Zero(t, next.calls.Load())
	})

	t.Run("StartedWithProcess", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, 0, &countingHandler{})
		h.requireRun(t, 1, "call", "")
		h.clock.Advance(700 * time.Millisecond).MustWait(testutil.Context(t, testutil.WaitShort))
		stopper := &fakeStopper{state: state, found: true}

		w := h.doCancel(t.Context(), stopper, h.uuid(1, "call"), h.headers(1, "call", 0), `{"stop_if_run_age_below_ms":1500}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "700", w.Header().Get(workspacesdk.CoderToolCallRunAgeMsHeader))
		var resp workspacesdk.CancelToolCallResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, workspacesdk.CancelToolCallResponse{
			Started:     true,
			StatusCode:  http.StatusCreated,
			ContentType: "text/plain; charset=utf-8",
			Body:        []byte("ran 1"),
			Process:     &state,
		}, resp)
		assert.Equal(t, []stopCall{{processID: h.uuid(1, "call"), stopIfRunAgeBelow: 1500 * time.Millisecond}}, stopper.calls())
	})

	t.Run("StartedWithoutProcess", func(t *testing.T) {
		t.Parallel()

		for _, stopper := range []*fakeStopper{{}, nil} {
			h := newHarness(t, 0, &countingHandler{})
			h.requireRun(t, 1, "call", "")
			resp := h.requireCancel(t, stopper, 1, "call", `{"stop_if_run_age_below_ms":1500}`)
			assert.True(t, resp.Started)
			assert.Equal(t, []byte("ran 1"), resp.Body)
			assert.Nil(t, resp.Process)
		}
	})

	t.Run("NegativeThresholdNeverStops", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, 0, &countingHandler{})
		h.requireRun(t, 1, "call", "")
		stopper := &fakeStopper{}
		h.requireCancel(t, stopper, 1, "call", `{"stop_if_run_age_below_ms":-5}`)
		assert.Equal(t, []stopCall{{processID: h.uuid(1, "call"), stopIfRunAgeBelow: 0}}, stopper.calls())
	})

	t.Run("StopperError", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, 0, &countingHandler{})
		h.requireRun(t, 1, "call", "")
		stopper := &fakeStopper{err: xerrors.New("kill failed")}
		w := h.doCancel(t.Context(), stopper, h.uuid(1, "call"), h.headers(1, "call", 0), `{}`)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("StaleMessage", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, 0, &countingHandler{})
		h.requireRun(t, 2, "other", "")
		w := h.doCancel(t.Context(), &fakeStopper{}, h.uuid(1, "call"), h.headers(1, "call", 0), `{}`)
		require.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, workspacesdk.ToolCallErrorStale, decodeToolCallError(t, w).Code)
	})

	t.Run("AgentStartedAfterToolCall", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, 10*time.Second, &countingHandler{})
		w := h.doCancel(t.Context(), &fakeStopper{}, h.uuid(1, "call"), h.headers(1, "call", time.Minute), `{}`)
		require.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, workspacesdk.ToolCallErrorAgentStartedAfterToolCall, decodeToolCallError(t, w).Code)
		assert.False(t, h.store.Current(h.key(1, "call")))
	})

	t.Run("NewerMessageDropsOlderRecords", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, 0, &countingHandler{})
		h.requireRun(t, 1, "call", "")
		h.requireCancel(t, &fakeStopper{}, 2, "call", `{}`)
		assert.False(t, h.store.Current(h.key(1, "call")))
		w := h.do(t.Context(), http.MethodPost, "/start", "", h.headers(1, "call", 0))
		assert.Equal(t, workspacesdk.ToolCallErrorStale, decodeToolCallError(t, w).Code)
	})
}

func TestCancelHandlerBadRequest(t *testing.T) {
	t.Parallel()

	toolCallOnly := func() http.Header {
		headers := http.Header{}
		workspacesdk.ToolCall{MessageID: 1, ID: "call"}.SetHeaders(headers)
		return headers
	}
	tests := []struct {
		name    string
		id      func(h *harness) string
		headers func(h *harness) http.Header
		body    string
	}{
		{
			name:    "WrongID",
			id:      func(*harness) string { return uuid.New().String() },
			headers: func(h *harness) http.Header { return h.headers(1, "call", 0) },
			body:    `{}`,
		},
		{
			name:    "OtherChatID",
			id:      func(h *harness) string { return workspacesdk.ToolCallUUID(uuid.New(), 1, "call").String() },
			headers: func(h *harness) http.Header { return h.headers(1, "call", 0) },
			body:    `{}`,
		},
		{
			name:    "WithoutChat",
			id:      func(h *harness) string { return h.uuid(1, "call") },
			headers: func(*harness) http.Header { return toolCallOnly() },
			body:    `{}`,
		},
		{
			name: "WithoutToolCallHeaders",
			id:   func(h *harness) string { return h.uuid(1, "call") },
			headers: func(h *harness) http.Header {
				return http.Header{workspacesdk.CoderChatIDHeader: {h.chatID.String()}}
			},
			body: `{}`,
		},
		{
			name: "MalformedHeaders",
			id:   func(h *harness) string { return h.uuid(1, "call") },
			headers: func(h *harness) http.Header {
				headers := h.headers(1, "call", 0)
				headers.Set(workspacesdk.CoderToolCallAgeMsHeader, "x")
				return headers
			},
			body: `{}`,
		},
		{
			name:    "MalformedBody",
			id:      func(h *harness) string { return h.uuid(1, "call") },
			headers: func(h *harness) http.Header { return h.headers(1, "call", 0) },
			body:    `{`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &countingHandler{}
			h := newHarness(t, 0, next)
			w := h.doCancel(t.Context(), &fakeStopper{}, tt.id(h), tt.headers(h), tt.body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			// Nothing was recorded, so the tool call still runs.
			h.requireRun(t, 1, "call", "")
			assert.EqualValues(t, 1, next.calls.Load())
		})
	}
}

func TestCancelHandlerWaitsForPendingRun(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		h := newHarness(t, 0, next)
		headers := h.headers(1, "call", 0)
		stopper := &fakeStopper{}

		started := h.goDo(t.Context(), "", headers)
		<-next.entered
		canceled := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			canceled <- h.doCancel(t.Context(), stopper, h.uuid(1, "call"), headers, `{}`)
		}()
		synctest.Wait()
		require.Empty(t, canceled, "cancel must wait for the pending run")
		require.Empty(t, stopper.calls(), "the stopper must not run before the run finishes")

		close(next.release)
		w := <-canceled
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp workspacesdk.CancelToolCallResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Started)
		assert.Equal(t, []byte("done 1"), resp.Body)
		assert.Len(t, stopper.calls(), 1)
		assert.Equal(t, http.StatusCreated, (<-started).Code)
	})
}

func TestCancelHandlerRequestContextEnds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		next := newBlockingHandler()
		h := newHarness(t, 0, next)
		headers := h.headers(1, "call", 0)
		stopper := &fakeStopper{}

		started := h.goDo(t.Context(), "", headers)
		<-next.entered
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		w := h.doCancel(ctx, stopper, h.uuid(1, "call"), headers, `{}`)
		assert.Zero(t, w.Body.Len(), "a cancel whose request ended writes nothing")
		assert.Empty(t, w.Header())
		assert.Empty(t, stopper.calls())

		close(next.release)
		<-started
	})
}

// doCancel serves POST /tool-calls/{id}/cancel through agentchat.Middleware
// and the cancel handler, without test assertions.
func (h *harness) doCancel(ctx context.Context, stopper *fakeStopper, id string, headers http.Header, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	if stopper == nil {
		router.Post("/tool-calls/{id}/cancel", h.store.CancelHandler(nil))
	} else {
		router.Post("/tool-calls/{id}/cancel", h.store.CancelHandler(stopper))
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("/tool-calls/%s/cancel", id), strings.NewReader(body))
	for k, v := range headers {
		r.Header[k] = v
	}
	agentchat.Middleware(router).ServeHTTP(w, r)
	return w
}

// requireCancel cancels a tool call of the harness chat with age zero and
// requires a 200.
func (h *harness) requireCancel(t *testing.T, stopper *fakeStopper, messageID int64, toolCallID, body string) workspacesdk.CancelToolCallResponse {
	t.Helper()

	w := h.doCancel(t.Context(), stopper, h.uuid(messageID, toolCallID), h.headers(messageID, toolCallID, 0), body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp workspacesdk.CancelToolCallResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func (h *harness) uuid(messageID int64, toolCallID string) string {
	return workspacesdk.ToolCallUUID(h.chatID, messageID, toolCallID).String()
}

type stopCall struct {
	processID         string
	stopIfRunAgeBelow time.Duration
}

// fakeStopper records its calls and answers with state, found, and err.
type fakeStopper struct {
	state workspacesdk.ToolCallProcess
	found bool
	err   error

	mu    sync.Mutex
	stops []stopCall
}

func (f *fakeStopper) StopToolCallProcess(_ context.Context, processID string, stopIfRunAgeBelow time.Duration) (workspacesdk.ToolCallProcess, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, stopCall{processID: processID, stopIfRunAgeBelow: stopIfRunAgeBelow})
	return f.state, f.found, f.err
}

func (f *fakeStopper) calls() []stopCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]stopCall(nil), f.stops...)
}
