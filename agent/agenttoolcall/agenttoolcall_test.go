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

type killed struct {
	chatID uuid.UUID
	id     uuid.UUID
	// runsFinished is how many runs had finished at the kill.
	runsFinished int64
}

// testServer serves POST /run, which answers 201 with the number of the
// run as its message, and POST /panic behind the table.
type testServer struct {
	clock    *quartz.Mock
	handler  http.Handler
	runs     atomic.Int64
	finished atomic.Int64
	killed   chan killed
	// entered receives when a run starts.
	entered chan struct{}
	// block, when set, holds each run until it is closed.
	block chan struct{}
}

func newTestServer(t *testing.T) *testServer {
	s := &testServer{
		clock:   quartz.NewMock(t),
		killed:  make(chan killed, 10),
		entered: make(chan struct{}, 10),
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
	r.Mount("/tool-calls", table.Routes())
	s.handler = r
	return s
}

// do sends POST path for chatID, with toolCallID unless it is uuid.Nil.
func (s *testServer) do(ctx context.Context, path string, chatID, toolCallID uuid.UUID) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, path, nil)
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
	if toolCallID != uuid.Nil {
		req.Header.Set(workspacesdk.CoderToolCallIDHeader, toolCallID.String())
	}
	rw := httptest.NewRecorder()
	s.handler.ServeHTTP(rw, req)
	return rw
}

func (s *testServer) cancel(ctx context.Context, chatID, toolCallID uuid.UUID) *httptest.ResponseRecorder {
	return s.do(ctx, "/tool-calls/"+toolCallID.String()+"/cancel", chatID, uuid.Nil)
}

func message(t *testing.T, body []byte) string {
	t.Helper()
	var resp codersdk.Response
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp.Message
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

	chatA, chatB := uuid.New(), uuid.New()
	// Each case sends one request for chatA after before.
	tests := []struct {
		name   string
		before func(ctx context.Context, s *testServer, id uuid.UUID)
		// path defaults to /run.
		path        string
		noID        bool
		wantCode    int
		wantMessage string
		wantRuns    int64
	}{
		{
			name:     "FirstRequestRuns",
			wantCode: http.StatusCreated, wantMessage: "1", wantRuns: 1,
		},
		{
			name:     "RepeatGetsSavedResponse",
			before:   func(ctx context.Context, s *testServer, id uuid.UUID) { s.do(ctx, "/run", chatA, id) },
			wantCode: http.StatusCreated, wantMessage: "1", wantRuns: 1,
		},
		{
			name:     "WithoutToolCallIDRunsEachTime",
			before:   func(ctx context.Context, s *testServer, _ uuid.UUID) { s.do(ctx, "/run", chatA, uuid.Nil) },
			noID:     true,
			wantCode: http.StatusCreated, wantMessage: "2", wantRuns: 2,
		},
		{
			name: "OtherChatsDoNotShare",
			before: func(ctx context.Context, s *testServer, id uuid.UUID) {
				s.do(ctx, "/run", chatB, id)
				s.cancel(ctx, chatB, id)
			},
			wantCode: http.StatusCreated, wantMessage: "2", wantRuns: 2,
		},
		{
			name: "CanceledAfterRunIsRefused",
			before: func(ctx context.Context, s *testServer, id uuid.UUID) {
				s.do(ctx, "/run", chatA, id)
				s.cancel(ctx, chatA, id)
			},
			wantCode: http.StatusConflict, wantRuns: 1,
		},
		{
			name:     "CanceledBeforeRunIsRefused",
			before:   func(ctx context.Context, s *testServer, id uuid.UUID) { s.cancel(ctx, chatA, id) },
			wantCode: http.StatusConflict, wantRuns: 0,
		},
		{
			name: "ForgottenAfterOneHour",
			before: func(ctx context.Context, s *testServer, id uuid.UUID) {
				s.do(ctx, "/run", chatA, id)
				s.clock.Advance(time.Hour + time.Second)
				// Entries are dropped when a new one is added.
				s.do(ctx, "/run", chatA, uuid.New())
			},
			wantCode: http.StatusCreated, wantMessage: "3", wantRuns: 3,
		},
		{
			name:     "RepeatAfterPanicGetsUnknownOutcome",
			before:   func(ctx context.Context, s *testServer, id uuid.UUID) { s.do(ctx, "/panic", chatA, id) },
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
			if tc.before != nil {
				tc.before(ctx, s, id)
			}
			path := tc.path
			if path == "" {
				path = "/run"
			}
			requestID := id
			if tc.noID {
				requestID = uuid.Nil
			}
			rw := s.do(ctx, path, chatA, requestID)
			require.Equal(t, tc.wantCode, rw.Code)
			if tc.wantMessage != "" {
				require.Contains(t, message(t, rw.Body.Bytes()), tc.wantMessage)
			}
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

	chatID := uuid.New()
	// Each case cancels after before. Every cancel kills a process with
	// the tool call ID, since the process can outlive the record.
	tests := []struct {
		name         string
		before       func(ctx context.Context, s *testServer, id uuid.UUID)
		wantReceived bool
		wantStatus   int
		wantMessage  string
	}{
		{name: "NoRecord"},
		{
			name:         "AfterRun",
			before:       func(ctx context.Context, s *testServer, id uuid.UUID) { s.do(ctx, "/run", chatID, id) },
			wantReceived: true, wantStatus: http.StatusCreated, wantMessage: "1",
		},
		{
			name:         "AfterPanic",
			before:       func(ctx context.Context, s *testServer, id uuid.UUID) { s.do(ctx, "/panic", chatID, id) },
			wantReceived: true, wantStatus: http.StatusInternalServerError, wantMessage: "outcome is unknown",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			s := newTestServer(t)
			id := uuid.New()
			if tc.before != nil {
				tc.before(ctx, s, id)
			}
			resp := decodeCancel(t, s.cancel(ctx, chatID, id))
			require.Equal(t, tc.wantReceived, resp.Received)
			require.Equal(t, tc.wantStatus, resp.Status)
			if tc.wantMessage != "" {
				require.Contains(t, message(t, resp.Body), tc.wantMessage)
			}
			k := testutil.RequireReceive(ctx, t, s.killed)
			require.Equal(t, chatID, k.chatID)
			require.Equal(t, id, k.id)
		})
	}

	t.Run("DuringRun", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		s := newTestServer(t)
		s.block = make(chan struct{})
		id := uuid.New()

		runDone := make(chan *httptest.ResponseRecorder, 1)
		go func() { runDone <- s.do(ctx, "/run", chatID, id) }()
		testutil.RequireReceive(ctx, t, s.entered)
		cancelDone := make(chan *httptest.ResponseRecorder, 1)
		go func() { cancelDone <- s.cancel(ctx, chatID, id) }()

		// Once the cancel lands, a repeat is refused at once while the
		// run goes on. Until then a repeat waits for the run, so each
		// poll gives up quickly.
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			pollCtx, cancel := context.WithTimeout(ctx, testutil.IntervalFast)
			defer cancel()
			return s.do(pollCtx, "/run", chatID, id).Code == http.StatusConflict
		}, testutil.IntervalFast)
		close(s.block)

		run := testutil.RequireReceive(ctx, t, runDone)
		resp := decodeCancel(t, testutil.RequireReceive(ctx, t, cancelDone))
		require.True(t, resp.Received)
		require.Equal(t, run.Code, resp.Status)
		require.Equal(t, run.Body.Bytes(), resp.Body)
		k := testutil.RequireReceive(ctx, t, s.killed)
		require.EqualValues(t, 1, k.runsFinished, "the cancel kills after the run, so a process still starting is killed")
		require.EqualValues(t, 1, s.runs.Load())
	})
}
