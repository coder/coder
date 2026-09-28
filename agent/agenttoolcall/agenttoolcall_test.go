package agenttoolcall_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/httpmw"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type killed struct {
	chatID uuid.UUID
	id     uuid.UUID
}

type testServer struct {
	table   *agenttoolcall.Table
	handler http.Handler
	runs    atomic.Int64
	killed  chan killed
	// block, when set, is received from before each run answers.
	block chan struct{}
	// entered receives once per run when the run starts.
	entered chan struct{}
}

func newTestServer(t *testing.T, clock quartz.Clock) *testServer {
	s := &testServer{killed: make(chan killed, 10), entered: make(chan struct{}, 10)}
	s.table = agenttoolcall.New(clock, func(_ context.Context, chatID, id uuid.UUID) {
		s.killed <- killed{chatID: chatID, id: id}
	})
	r := chi.NewRouter()
	r.Use(httpmw.Recover(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})), agentchat.Middleware, s.table.Middleware)
	r.Post("/run", func(rw http.ResponseWriter, r *http.Request) {
		n := s.runs.Add(1)
		s.entered <- struct{}{}
		if s.block != nil {
			<-s.block
		}
		httpapi.Write(r.Context(), rw, http.StatusCreated, codersdk.Response{Message: string(rune('0' + n))})
	})
	r.Post("/panic", func(http.ResponseWriter, *http.Request) {
		s.runs.Add(1)
		panic("handler panic")
	})
	r.Mount("/tool-calls", s.table.Routes())
	s.handler = r
	return s
}

func (s *testServer) do(t *testing.T, path string, chatID, toolCallID uuid.UUID) *httptest.ResponseRecorder {
	ctx := testutil.Context(t, testutil.WaitShort)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, path, nil)
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
	if toolCallID != uuid.Nil {
		req.Header.Set(workspacesdk.CoderToolCallIDHeader, toolCallID.String())
	}
	rw := httptest.NewRecorder()
	s.handler.ServeHTTP(rw, req)
	return rw
}

func (s *testServer) run(t *testing.T, chatID, toolCallID uuid.UUID) *httptest.ResponseRecorder {
	return s.do(t, "/run", chatID, toolCallID)
}

func (s *testServer) cancelRequest(t *testing.T, chatID, toolCallID uuid.UUID) *httptest.ResponseRecorder {
	return s.do(t, "/tool-calls/"+toolCallID.String()+"/cancel", chatID, uuid.Nil)
}

func (s *testServer) cancel(t *testing.T, chatID, toolCallID uuid.UUID) workspacesdk.CancelToolCallResponse {
	t.Helper()
	return decodeCancel(t, s.cancelRequest(t, chatID, toolCallID))
}

func decodeCancel(t *testing.T, rw *httptest.ResponseRecorder) workspacesdk.CancelToolCallResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, rw.Code)
	var resp workspacesdk.CancelToolCallResponse
	require.NoError(t, json.NewDecoder(rw.Body).Decode(&resp))
	return resp
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("NoHeaderRunsEachTime", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(t, quartz.NewReal())
		chatID := uuid.New()
		require.Equal(t, http.StatusCreated, s.run(t, chatID, uuid.Nil).Code)
		require.Equal(t, http.StatusCreated, s.run(t, chatID, uuid.Nil).Code)
		require.EqualValues(t, 2, s.runs.Load())
	})

	t.Run("RepeatReturnsSavedResponse", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(t, quartz.NewReal())
		chatID, id := uuid.New(), uuid.New()
		first := s.run(t, chatID, id)
		second := s.run(t, chatID, id)
		require.Equal(t, http.StatusCreated, second.Code)
		require.Equal(t, first.Header().Get("Content-Type"), second.Header().Get("Content-Type"))
		require.Equal(t, first.Body.String(), second.Body.String())
		require.EqualValues(t, 1, s.runs.Load())
	})

	t.Run("RepeatWhileRunningWaits", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(t, quartz.NewReal())
		s.block = make(chan struct{})
		chatID, id := uuid.New(), uuid.New()
		results := make(chan *httptest.ResponseRecorder, 2)
		go func() { results <- s.run(t, chatID, id) }()
		<-s.entered
		go func() { results <- s.run(t, chatID, id) }()
		close(s.block)
		first, second := <-results, <-results
		require.Equal(t, http.StatusCreated, first.Code)
		require.Equal(t, first.Body.String(), second.Body.String())
		require.EqualValues(t, 1, s.runs.Load())
	})

	t.Run("PanicSavesUnknownOutcome", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(t, quartz.NewReal())
		chatID, id := uuid.New(), uuid.New()
		require.Equal(t, http.StatusInternalServerError, s.do(t, "/panic", chatID, id).Code)
		// The repeat and the cancel answer at once instead of waiting
		// for a response the panicked run never saved.
		repeat := s.do(t, "/panic", chatID, id)
		require.Equal(t, http.StatusInternalServerError, repeat.Code)
		require.Contains(t, repeat.Body.String(), "outcome is unknown")
		resp := s.cancel(t, chatID, id)
		require.True(t, resp.Received)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
		require.EqualValues(t, 1, s.runs.Load())
	})

	t.Run("CanceledRefuses", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(t, quartz.NewReal())
		chatID, id := uuid.New(), uuid.New()
		s.run(t, chatID, id)
		s.cancel(t, chatID, id)
		<-s.killed
		require.Equal(t, http.StatusConflict, s.run(t, chatID, id).Code)
		require.EqualValues(t, 1, s.runs.Load())
	})

	t.Run("ChatIsolation", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(t, quartz.NewReal())
		chatA, chatB, id := uuid.New(), uuid.New(), uuid.New()
		require.False(t, s.cancel(t, chatB, id).Received)
		require.Equal(t, killed{chatID: chatB, id: id}, <-s.killed, "the kill is scoped to the canceling chat")
		require.Equal(t, http.StatusCreated, s.run(t, chatA, id).Code)
		require.EqualValues(t, 1, s.runs.Load())
		require.Equal(t, http.StatusConflict, s.run(t, chatB, id).Code)
	})

	t.Run("DropsEntriesOlderThanOneHour", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		s := newTestServer(t, clock)
		chatID, id := uuid.New(), uuid.New()
		s.run(t, chatID, id)
		clock.Advance(time.Hour + time.Second)
		s.run(t, chatID, uuid.New())
		s.run(t, chatID, id)
		require.EqualValues(t, 3, s.runs.Load())
	})
}

func TestCancel(t *testing.T) {
	t.Parallel()

	t.Run("NoRecord", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(t, quartz.NewReal())
		chatID, id := uuid.New(), uuid.New()
		require.Equal(t, workspacesdk.CancelToolCallResponse{}, s.cancel(t, chatID, id))
		// A process started with the ID outlives the agent's record of
		// the tool call, so the cancel kills it.
		require.Equal(t, killed{chatID: chatID, id: id}, <-s.killed)
		require.Equal(t, http.StatusConflict, s.run(t, chatID, id).Code)
		require.EqualValues(t, 0, s.runs.Load())
	})

	t.Run("WhileRunningKillsAfterRun", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(t, quartz.NewReal())
		s.block = make(chan struct{})
		chatID, id := uuid.New(), uuid.New()
		runDone := make(chan *httptest.ResponseRecorder)
		go func() { runDone <- s.run(t, chatID, id) }()
		<-s.entered
		cancelDone := make(chan *httptest.ResponseRecorder)
		go func() { cancelDone <- s.cancelRequest(t, chatID, id) }()
		// Release the run only after the cancel reached the table.
		testutil.Eventually(testutil.Context(t, testutil.WaitShort), t, func(context.Context) bool {
			return s.table.Canceled(chatID, id)
		}, testutil.IntervalFast)
		require.Empty(t, s.killed, "the cancel waits for the run before killing")
		close(s.block)
		run := <-runDone
		resp := decodeCancel(t, <-cancelDone)
		require.Equal(t, killed{chatID: chatID, id: id}, <-s.killed)
		require.True(t, resp.Received)
		require.Equal(t, http.StatusCreated, resp.Status)
		require.Equal(t, "application/json; charset=utf-8", resp.ContentType)
		require.Equal(t, run.Body.Bytes(), resp.Body)
	})

	t.Run("FinishedReturnsSavedResponse", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(t, quartz.NewReal())
		chatID, id := uuid.New(), uuid.New()
		run := s.run(t, chatID, id)
		resp := s.cancel(t, chatID, id)
		require.True(t, resp.Received)
		require.Equal(t, run.Code, resp.Status)
		require.Equal(t, run.Body.Bytes(), resp.Body)
	})
}
