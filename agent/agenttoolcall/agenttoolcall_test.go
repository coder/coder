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

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type testServer struct {
	handler http.Handler
	runs    atomic.Int64
	killed  chan uuid.UUID
	// block, when set, is received from before each run answers.
	block chan struct{}
	// entered receives once per run when the run starts.
	entered chan struct{}
}

func newTestServer(clock quartz.Clock) *testServer {
	s := &testServer{killed: make(chan uuid.UUID, 10), entered: make(chan struct{}, 10)}
	table := agenttoolcall.New(clock, func(_ context.Context, id uuid.UUID) { s.killed <- id })
	r := chi.NewRouter()
	r.Use(agentchat.Middleware, table.Middleware)
	r.Post("/run", func(rw http.ResponseWriter, r *http.Request) {
		n := s.runs.Add(1)
		s.entered <- struct{}{}
		if s.block != nil {
			<-s.block
		}
		httpapi.Write(r.Context(), rw, http.StatusCreated, codersdk.Response{Message: string(rune('0' + n))})
	})
	r.Mount("/tool-calls", table.Routes())
	s.handler = r
	return s
}

func (s *testServer) do(t *testing.T, path string, chatID, toolCallID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
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

func (s *testServer) cancel(t *testing.T, chatID, toolCallID uuid.UUID) workspacesdk.CancelToolCallResponse {
	t.Helper()
	rw := s.do(t, "/tool-calls/"+toolCallID.String()+"/cancel", chatID, uuid.Nil)
	require.Equal(t, http.StatusOK, rw.Code)
	var resp workspacesdk.CancelToolCallResponse
	require.NoError(t, json.NewDecoder(rw.Body).Decode(&resp))
	return resp
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("NoHeaderRunsEachTime", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(quartz.NewReal())
		chatID := uuid.New()
		require.Equal(t, http.StatusCreated, s.run(t, chatID, uuid.Nil).Code)
		require.Equal(t, http.StatusCreated, s.run(t, chatID, uuid.Nil).Code)
		require.EqualValues(t, 2, s.runs.Load())
	})

	t.Run("RepeatReturnsSavedResponse", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(quartz.NewReal())
		chatID, id := uuid.New(), uuid.New()
		first := s.run(t, chatID, id)
		second := s.run(t, chatID, id)
		require.Equal(t, http.StatusCreated, second.Code)
		require.Equal(t, first.Header().Get("Content-Type"), second.Header().Get("Content-Type"))
		require.Equal(t, first.Body.String(), second.Body.String())
		require.EqualValues(t, 1, s.runs.Load())
	})

	t.Run("CanceledRefuses", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(quartz.NewReal())
		chatID, id := uuid.New(), uuid.New()
		s.run(t, chatID, id)
		s.cancel(t, chatID, id)
		<-s.killed
		require.Equal(t, http.StatusConflict, s.run(t, chatID, id).Code)
		require.EqualValues(t, 1, s.runs.Load())
	})

	t.Run("ChatIsolation", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(quartz.NewReal())
		chatA, chatB, id := uuid.New(), uuid.New(), uuid.New()
		require.False(t, s.cancel(t, chatB, id).Received)
		require.Equal(t, http.StatusCreated, s.run(t, chatA, id).Code)
		require.EqualValues(t, 1, s.runs.Load())
		require.Equal(t, http.StatusConflict, s.run(t, chatB, id).Code)
	})

	t.Run("DropsEntriesOlderThanOneHour", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		s := newTestServer(clock)
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

	t.Run("NeverReceived", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(quartz.NewReal())
		chatID, id := uuid.New(), uuid.New()
		require.Equal(t, workspacesdk.CancelToolCallResponse{}, s.cancel(t, chatID, id))
		require.Equal(t, http.StatusConflict, s.run(t, chatID, id).Code)
		require.EqualValues(t, 0, s.runs.Load())
		require.Empty(t, s.killed)
	})

	t.Run("RunningIsKilledAfterRun", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(quartz.NewReal())
		s.block = make(chan struct{})
		chatID, id := uuid.New(), uuid.New()
		runDone := make(chan *httptest.ResponseRecorder)
		go func() { runDone <- s.run(t, chatID, id) }()
		<-s.entered
		cancelDone := make(chan workspacesdk.CancelToolCallResponse)
		go func() { cancelDone <- s.cancel(t, chatID, id) }()
		close(s.block)
		run := <-runDone
		resp := <-cancelDone
		require.Equal(t, id, <-s.killed)
		require.True(t, resp.Received)
		require.Equal(t, http.StatusCreated, resp.Status)
		require.Equal(t, "application/json; charset=utf-8", resp.ContentType)
		require.Equal(t, run.Body.Bytes(), resp.Body)
	})

	t.Run("FinishedReturnsSavedResponse", func(t *testing.T) {
		t.Parallel()
		s := newTestServer(quartz.NewReal())
		chatID, id := uuid.New(), uuid.New()
		run := s.run(t, chatID, id)
		resp := s.cancel(t, chatID, id)
		require.True(t, resp.Received)
		require.Equal(t, run.Code, resp.Status)
		require.Equal(t, run.Body.Bytes(), resp.Body)
	})
}
