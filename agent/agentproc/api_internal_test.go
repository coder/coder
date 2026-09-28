package agentproc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type toolCallServer struct {
	api     *API
	clock   *quartz.Mock
	handler http.Handler
}

func newToolCallServer(t *testing.T) *toolCallServer {
	clock := quartz.NewMock(t)
	api := NewAPI(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), agentexec.DefaultExecer, nil, nil, nil, nil, nil)
	api.manager.clock = clock
	t.Cleanup(func() { _ = api.Close() })
	return &toolCallServer{api: api, clock: clock, handler: agentchat.Middleware(api.Routes())}
}

// serve sends a request for chat chatID with tool call ID id.
func (s *toolCallServer) serve(ctx context.Context, chatID uuid.UUID, id uuid.UUID, method, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(b))
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
	req.Header.Set(workspacesdk.CoderToolCallIDHeader, id.String())
	rw := httptest.NewRecorder()
	s.handler.ServeHTTP(rw, req)
	return rw
}

func (s *toolCallServer) start(ctx context.Context, chatID, id uuid.UUID, timeout time.Duration) *httptest.ResponseRecorder {
	return s.serve(ctx, chatID, id, http.MethodPost, "/start", workspacesdk.StartProcessRequest{Command: "sleep 300", TimeoutMs: timeout.Milliseconds()})
}

func TestProcessOutputWaitWithStartTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		// afterTimeout runs once the clock passes the start timeout.
		afterTimeout func(s *toolCallServer, chatID, id uuid.UUID)
		want         workspacesdk.ProcessOutputResponse
	}{
		{
			// The execute tool's wait returns at the start timeout, and
			// the process keeps running.
			name:  "StopAtStartTimeout",
			query: "?wait=true&stop_at_start_timeout=true",
			want:  workspacesdk.ProcessOutputResponse{Running: true, TimedOut: true},
		},
		{
			// Other waits, such as process_output's, outlast the start
			// timeout and return at exit.
			name:  "PlainWait",
			query: "?wait=true",
			afterTimeout: func(s *toolCallServer, chatID, id uuid.UUID) {
				s.api.KillToolCall(context.Background(), chatID, id)
			},
			want: workspacesdk.ProcessOutputResponse{Canceled: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			s := newToolCallServer(t)
			chatID, id := uuid.New(), uuid.New()

			require.Equal(t, http.StatusOK, s.start(ctx, chatID, id, time.Second).Code)
			// A repeat start does not move the start timeout.
			require.Equal(t, http.StatusOK, s.start(ctx, chatID, id, time.Hour).Code)

			trap := s.clock.Trap().AfterFunc()
			waited := make(chan *httptest.ResponseRecorder, 1)
			go func() { waited <- s.serve(ctx, chatID, id, http.MethodGet, "/"+id.String()+"/output"+tc.query, nil) }()
			trap.MustWait(ctx).MustRelease(ctx)
			trap.Close()
			s.clock.Advance(time.Second).MustWait(ctx)
			if tc.afterTimeout != nil {
				tc.afterTimeout(s, chatID, id)
			}

			var got workspacesdk.ProcessOutputResponse
			require.NoError(t, json.NewDecoder(testutil.RequireReceive(ctx, t, waited).Body).Decode(&got))
			require.Equal(t, tc.want.Running, got.Running)
			require.Equal(t, tc.want.TimedOut, got.TimedOut)
			require.Equal(t, tc.want.Canceled, got.Canceled)
		})
	}
}

func TestToolCallProcessChat(t *testing.T) {
	t.Parallel()

	// Each case starts a tool call's process for the owner chat, then
	// attaches to it and kills it as the requesting chat.
	tests := []struct {
		name      string
		sameChat  bool
		wantStart int
		wantKill  bool
	}{
		{name: "SameChat", sameChat: true, wantStart: http.StatusOK, wantKill: true},
		{name: "OtherChat", wantStart: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			s := newToolCallServer(t)
			owner, id := uuid.New(), uuid.New()
			requester := uuid.New()
			if tc.sameChat {
				requester = owner
			}

			require.Equal(t, http.StatusOK, s.start(ctx, owner, id, 0).Code)
			rw := s.start(ctx, requester, id, 0)
			require.Equal(t, tc.wantStart, rw.Code)
			if tc.wantStart == http.StatusOK {
				var resp workspacesdk.StartProcessResponse
				require.NoError(t, json.NewDecoder(rw.Body).Decode(&resp))
				require.Equal(t, id.String(), resp.ID, "a repeat start attaches to the process")
			}

			s.api.KillToolCall(ctx, requester, id)
			proc, ok := s.api.manager.get(id.String())
			require.True(t, ok)
			if tc.wantKill {
				testutil.TryReceive(ctx, t, proc.done)
				require.True(t, proc.canceled.Load())
				return
			}
			require.True(t, proc.info().Running)
			require.False(t, proc.canceled.Load())
		})
	}
}
