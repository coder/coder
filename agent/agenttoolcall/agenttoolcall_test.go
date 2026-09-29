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

// The run-once, repeat, cancel-after-run, and refuse-after-cancel
// behavior of each acting route is tested through the real agent in
// agent.TestAgent_ToolCall. These tests cover what that cannot reach.

type killed struct {
	chatID uuid.UUID
	id     uuid.UUID
	// runsFinished is how many runs had finished at the kill.
	runsFinished int64
}

// testServer serves POST /run, which responds with 201 and the number of
// the run as its message, and POST /panic behind the table.
type testServer struct {
	clock    *quartz.Mock
	handler  http.Handler
	runs     atomic.Int64
	finished atomic.Int64
	killed   chan killed
	// entered receives when a run starts.
	entered chan struct{}
	// cancelEntered receives when a cancel request reaches the cancel
	// route, before the table handles it.
	cancelEntered chan struct{}
	// block, when set, holds each run until it is closed.
	block chan struct{}
}

func newTestServer(t *testing.T) *testServer {
	s := &testServer{
		clock:         quartz.NewMock(t),
		killed:        make(chan killed, 10),
		entered:       make(chan struct{}, 10),
		cancelEntered: make(chan struct{}, 10),
	}
	table := agenttoolcall.New(s.clock, func(_ context.Context, chatID, id uuid.UUID) {
		s.killed <- killed{chatID: chatID, id: id, runsFinished: s.finished.Load()}
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

// do sends POST path for chatID, with tool call ID id unless it is
// uuid.Nil.
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
	// Each case sends one request for chatA after before.
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
				// Entries are dropped when a new one is added.
				s.do(ctx, "/run", chatA, uuid.New())
			},
			path:     "/run",
			wantCode: http.StatusCreated, wantMessage: "3", wantRuns: 3,
		},
		{
			// A repeat gets the saved 500 instead of waiting for a
			// response the panicked run never saved.
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

	// However the requests interleave with the run, all get the response
	// of one run.
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
		// A process started with the ID outlives the record, so the
		// cancel still kills it.
		k := testutil.RequireReceive(ctx, t, s.killed)
		require.Equal(t, chatID, k.chatID)
		require.Equal(t, id, k.id)
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
		// Release the run once the cancel request reaches the cancel
		// route. The table may mark the call canceled before or after
		// the run finishes; the assertions hold in either order.
		testutil.RequireReceive(ctx, t, s.cancelEntered)
		close(s.block)

		run := testutil.RequireReceive(ctx, t, runDone)
		resp := testutil.RequireReceive(ctx, t, cancelDone)
		require.True(t, resp.Received)
		require.Equal(t, run.Code, resp.Status)
		require.Equal(t, run.Body.Bytes(), resp.Body)
		k := testutil.RequireReceive(ctx, t, s.killed)
		require.EqualValues(t, 1, k.runsFinished, "the cancel kills after the run, so a process still starting is killed")
	})
}
