package agentproc

import (
	"bytes"
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

func TestToolCallProcess(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	clock := quartz.NewMock(t)
	api := NewAPI(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), agentexec.DefaultExecer, nil, nil, nil, nil, nil)
	api.manager.clock = clock
	t.Cleanup(func() { _ = api.Close() })
	handler := agentchat.Middleware(api.Routes())

	chatID, id := uuid.NewString(), uuid.New()
	serve := func(method, path string, body any) *httptest.ResponseRecorder {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(b))
		req.Header.Set(workspacesdk.CoderChatIDHeader, chatID)
		req.Header.Set(workspacesdk.CoderToolCallIDHeader, id.String())
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)
		return rw
	}
	start := func(timeout time.Duration) {
		var resp workspacesdk.StartProcessResponse
		rw := serve(http.MethodPost, "/start", workspacesdk.StartProcessRequest{Command: "sleep 300", TimeoutMs: timeout.Milliseconds()})
		require.NoError(t, json.NewDecoder(rw.Body).Decode(&resp))
		require.Equal(t, id.String(), resp.ID)
	}
	output := func() (resp workspacesdk.ProcessOutputResponse) {
		require.NoError(t, json.NewDecoder(serve(http.MethodGet, "/"+id.String()+"/output?wait=true", nil).Body).Decode(&resp))
		return resp
	}

	start(time.Second)
	// A repeat attaches to the process and keeps its waitUntil.
	start(time.Hour)

	trap := clock.Trap().AfterFunc()
	waited := make(chan workspacesdk.ProcessOutputResponse)
	go func() { waited <- output() }()
	call := trap.MustWait(ctx)
	require.Equal(t, time.Second, call.Duration, "the wait ends at waitUntil, before the cap")
	call.MustRelease(ctx)
	trap.Close()
	clock.Advance(time.Second).MustWait(ctx)
	resp := testutil.RequireReceive(ctx, t, waited)
	require.True(t, resp.Running)
	require.True(t, resp.TimedOut)
	require.False(t, resp.Canceled)

	api.KillToolCall(ctx, id)
	proc, ok := api.manager.get(id.String())
	require.True(t, ok)
	testutil.TryReceive(ctx, t, proc.done)
	resp = output()
	require.True(t, resp.Canceled)
	require.False(t, resp.TimedOut)
}
