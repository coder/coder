package agenttoolcall_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// Per-route behavior is tested through the real agent in
// agent.TestAgent_ToolCall.

type canceledCall struct {
	chatID       uuid.UUID
	id           uuid.UUID
	runsFinished int64 // runs finished when cancel was called
}

// testServer serves /run and /panic behind a Table and reports the Table's
// cancel calls on canceled.
type testServer struct {
	clock         *quartz.Mock
	handler       http.Handler
	runs          atomic.Int64
	finished      atomic.Int64
	canceled      chan canceledCall
	entered       chan struct{} // receives when a run starts
	cancelEntered chan struct{} // receives when a cancel arrives, before the Table handles it
	block         chan struct{} // if set, runs wait until it is closed
}

func newTestServer(t *testing.T) *testServer {
	s := &testServer{
		clock:         quartz.NewMock(t),
		canceled:      make(chan canceledCall, 10),
		entered:       make(chan struct{}, 10),
		cancelEntered: make(chan struct{}, 10),
	}
	table := agenttoolcall.New(s.clock, func(_ context.Context, chatID, id uuid.UUID) {
		s.canceled <- canceledCall{chatID: chatID, id: id, runsFinished: s.finished.Load()}
	})
	r := chi.NewRouter()
	r.Use(httpmw.Recover(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})), agentchat.Middleware, table.Middleware)
	r.Post("/run", func(rw http.ResponseWriter, r *http.Request) {
		n := s.runs.Add(1)
		s.entered <- struct{}{}
		if s.block != nil {
			<-s.block
		}
		httpapi.Write(r.Context(), rw, http.StatusCreated, codersdk.Response{Message: strconv.FormatInt(n, 10)})
		s.finished.Add(1)
	})
	r.Post("/panic", func(http.ResponseWriter, *http.Request) {
		s.runs.Add(1)
		panic("handler panic")
	})
	cancels := table.Routes()
	r.Mount("/tool-calls", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		s.cancelEntered <- struct{}{}
		cancels.ServeHTTP(rw, r)
	}))
	s.handler = r
	return s
}

func (s *testServer) do(ctx context.Context, path string, chatID, id uuid.UUID) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, path, nil)
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
	if id != uuid.Nil {
		req.Header.Set(workspacesdk.CoderToolCallIDHeader, id.String())
	}
	rw := httptest.NewRecorder()
	s.handler.ServeHTTP(rw, req)
	return rw
}

func (s *testServer) cancel(ctx context.Context, t *testing.T, chatID, id uuid.UUID) workspacesdk.CancelToolCallResponse {
	t.Helper()
	rw := s.do(ctx, "/tool-calls/"+id.String()+"/cancel", chatID, uuid.Nil)
	require.Equal(t, http.StatusOK, rw.Code)
	var resp workspacesdk.CancelToolCallResponse
	require.NoError(t, json.NewDecoder(rw.Body).Decode(&resp))
	return resp
}

func message(t *testing.T, body []byte) string {
	t.Helper()
	var resp codersdk.Response
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp.Message
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	chatA, chatB := uuid.New(), uuid.New()
	tests := []struct {
		name        string
		before      func(ctx context.Context, t *testing.T, s *testServer, id uuid.UUID)
		path        string
		wantCode    int
		wantMessage string
		wantRuns    int64
	}{
		{
			name: "OtherChatsDoNotShare",
			before: func(ctx context.Context, t *testing.T, s *testServer, id uuid.UUID) {
				s.do(ctx, "/run", chatB, id)
				s.cancel(ctx, t, chatB, id)
			},
			path:     "/run",
			wantCode: http.StatusCreated, wantMessage: "2", wantRuns: 2,
		},
		{
			name: "ForgottenAfterOneHour",
			before: func(ctx context.Context, _ *testing.T, s *testServer, id uuid.UUID) {
				s.do(ctx, "/run", chatA, id)
				s.clock.Advance(time.Hour + time.Second)
				// Expired entries are deleted only when an entry is added.
				s.do(ctx, "/run", chatA, uuid.New())
			},
			path:     "/run",
			wantCode: http.StatusCreated, wantMessage: "3", wantRuns: 3,
		},
		{
			name: "RepeatKeepsRecord",
			before: func(ctx context.Context, _ *testing.T, s *testServer, id uuid.UUID) {
				s.do(ctx, "/run", chatA, id)
				s.clock.Advance(50 * time.Minute)
				s.do(ctx, "/run", chatA, id)
				s.clock.Advance(20 * time.Minute)
				s.do(ctx, "/run", chatA, uuid.New())
			},
			path:     "/run",
			wantCode: http.StatusCreated, wantMessage: "1", wantRuns: 2,
		},
		{
			name: "CancelKeepsRecord",
			before: func(ctx context.Context, t *testing.T, s *testServer, id uuid.UUID) {
				s.cancel(ctx, t, chatA, id)
				s.clock.Advance(50 * time.Minute)
				s.cancel(ctx, t, chatA, id)
				s.clock.Advance(20 * time.Minute)
				s.do(ctx, "/run", chatA, uuid.New())
			},
			path:     "/run",
			wantCode: http.StatusConflict, wantMessage: "canceled", wantRuns: 1,
		},
		{
			name: "RefusedRequestKeepsRecord",
			before: func(ctx context.Context, t *testing.T, s *testServer, id uuid.UUID) {
				s.cancel(ctx, t, chatA, id)
				s.clock.Advance(50 * time.Minute)
				s.do(ctx, "/run", chatA, id)
				s.clock.Advance(20 * time.Minute)
				s.do(ctx, "/run", chatA, uuid.New())
			},
			path:     "/run",
			wantCode: http.StatusConflict, wantMessage: "canceled", wantRuns: 1,
		},
		{
			name: "RepeatAfterPanicGetsUnknownOutcome",
			before: func(ctx context.Context, _ *testing.T, s *testServer, id uuid.UUID) {
				s.do(ctx, "/panic", chatA, id)
			},
			path:     "/panic",
			wantCode: http.StatusInternalServerError, wantMessage: "outcome is unknown", wantRuns: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			s := newTestServer(t)
			id := uuid.New()
			tc.before(ctx, t, s, id)
			rw := s.do(ctx, tc.path, chatA, id)
			require.Equal(t, tc.wantCode, rw.Code)
			require.Contains(t, message(t, rw.Body.Bytes()), tc.wantMessage)
			require.Equal(t, tc.wantRuns, s.runs.Load())
		})
	}
}

func TestConcurrentRequestsRunOnce(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	s := newTestServer(t)
	s.block = make(chan struct{})
	chatID, id := uuid.New(), uuid.New()

	const requests = 10
	results := make(chan *httptest.ResponseRecorder, requests)
	for range requests {
		go func() { results <- s.do(ctx, "/run", chatID, id) }()
	}
	testutil.RequireReceive(ctx, t, s.entered)
	close(s.block)
	for range requests {
		rw := testutil.RequireReceive(ctx, t, results)
		require.Equal(t, http.StatusCreated, rw.Code)
		require.Equal(t, "1", message(t, rw.Body.Bytes()))
	}
	require.EqualValues(t, 1, s.runs.Load())
}

func TestCancel(t *testing.T) {
	t.Parallel()

	t.Run("NoRecord", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		s := newTestServer(t)
		chatID, id := uuid.New(), uuid.New()

		require.False(t, s.cancel(ctx, t, chatID, id).Received)
		c := testutil.RequireReceive(ctx, t, s.canceled)
		require.Equal(t, chatID, c.chatID)
		require.Equal(t, id, c.id)
		require.Equal(t, http.StatusConflict, s.do(ctx, "/run", chatID, id).Code)
		require.Zero(t, s.runs.Load())
	})

	t.Run("DuringRun", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		s := newTestServer(t)
		s.block = make(chan struct{})
		chatID, id := uuid.New(), uuid.New()

		runDone := make(chan *httptest.ResponseRecorder, 1)
		go func() { runDone <- s.do(ctx, "/run", chatID, id) }()
		testutil.RequireReceive(ctx, t, s.entered)
		cancelDone := make(chan workspacesdk.CancelToolCallResponse, 1)
		go func() {
			rw := s.do(ctx, "/tool-calls/"+id.String()+"/cancel", chatID, uuid.Nil)
			var resp workspacesdk.CancelToolCallResponse
			_ = json.NewDecoder(rw.Body).Decode(&resp)
			cancelDone <- resp
		}()
		// The cancel may mark the call before or after the run finishes;
		// the assertions hold either way.
		testutil.RequireReceive(ctx, t, s.cancelEntered)
		close(s.block)

		run := testutil.RequireReceive(ctx, t, runDone)
		resp := testutil.RequireReceive(ctx, t, cancelDone)
		require.True(t, resp.Received)
		require.Equal(t, run.Code, resp.Status)
		require.Equal(t, run.Body.Bytes(), resp.Body)
		c := testutil.RequireReceive(ctx, t, s.canceled)
		require.EqualValues(t, 1, c.runsFinished, "the cancel hook runs after the run, so a process still starting is stopped")
	})
}
