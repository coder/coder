package agentacp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func dialTestStream(ctx context.Context, t *testing.T, streamURL string) *websocket.Conn {
	t.Helper()
	conn, response, err := websocket.Dial(ctx, streamURL, nil)
	require.NoError(t, err)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func callTestAPI(t *testing.T, handler http.Handler, method, path string, body any, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(testutil.Context(t, testutil.WaitLong), method, path, bytes.NewReader(raw))
	req.Header = headers
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestAPIAndStreamReplay(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	toolCalls := agenttoolcall.New(m.clock, func(context.Context, uuid.UUID, uuid.UUID) {})
	handler := agentchat.Middleware(toolCalls.Middleware(NewAPI(m).Routes()))
	headers := http.Header{workspacesdk.CoderChatIDHeader: {uuid.NewString()}, workspacesdk.CoderToolCallIDHeader: {uuid.NewString()}}
	req := workspacesdk.ACPCreateSessionRequest{HarnessSlug: "fake", WorkingDirectory: dir}
	var created [2]workspacesdk.ACPSession
	for i := range created {
		res := callTestAPI(t, handler, "POST", "/sessions", req, headers)
		require.Equal(t, http.StatusOK, res.Code)
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &created[i]))
	}
	require.Equal(t, created[0].ID, created[1].ID)
	id := created[0].ID
	path := "/sessions/session"
	query := url.Values{"harness_slug": {id.HarnessSlug}, "working_directory": {id.WorkingDirectory}, "session_id": {id.SessionID}}
	headers.Set(workspacesdk.CoderToolCallIDHeader, uuid.NewString())
	res := callTestAPI(t, handler, "POST", path+"/messages?"+query.Encode(), workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"}, headers)
	require.Equal(t, http.StatusOK, res.Code)
	completed := awaitTestSession(t, m, id)
	query.Set("epoch", completed.Session.Cursor.Epoch.String())
	query.Set("after", "1")
	res = callTestAPI(t, handler, "GET", path+"?"+query.Encode(), nil, nil)
	require.Equal(t, http.StatusOK, res.Code)
	var read workspacesdk.ACPSessionResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &read))
	require.Len(t, read.Events, len(completed.Events)-1)
	require.Equal(t, completed.AssistantResponse, read.AssistantResponse)
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx := testutil.Context(t, testutil.WaitLong)
	conn := dialTestStream(ctx, t, server.URL+path+"/stream?"+query.Encode())
	for _, expected := range completed.Events[1:] {
		var event workspacesdk.ACPEvent
		require.NoError(t, wsjson.Read(ctx, conn, &event))
		require.Equal(t, expected, event)
	}
	var snapshot workspacesdk.ACPEvent
	require.NoError(t, wsjson.Read(ctx, conn, &snapshot))
	require.Equal(t, workspacesdk.ACPEventKindSnapshot, snapshot.Kind)
	require.Equal(t, completed.Session, *snapshot.Session)
	require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(wsjson.Read(ctx, conn, &snapshot)))
}

func TestHarnessFailureKeepsDiagnosticsOutOfAPI(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeNone)
	info := createTestSession(t, m, dir)
	failed := messageTestSession(t, m, info.ID, "exit-secret")
	require.Equal(t, workspacesdk.ACPSessionStatusError, failed.Session.Status)
	handler := NewAPI(m).Routes()
	query := url.Values{"harness_slug": {info.ID.HarnessSlug}, "working_directory": {dir}, "session_id": {info.ID.SessionID}}
	for _, path := range []string{"/sessions", "/sessions/session?" + query.Encode()} {
		res := callTestAPI(t, handler, "GET", path, nil, nil)
		require.Equal(t, http.StatusOK, res.Code)
		require.NotContains(t, res.Body.String(), "runtime-secret")
		require.Contains(t, res.Body.String(), "see workspace agent logs")
	}
	setTestHarnessCommand(m, "printf 'credential=%s\\n' 'setup-secret' >&2; exit 1")
	res := callTestAPI(t, handler, "POST", "/sessions", workspacesdk.ACPCreateSessionRequest{HarnessSlug: "fake", WorkingDirectory: dir}, nil)
	require.Equal(t, http.StatusServiceUnavailable, res.Code)
	require.NotContains(t, res.Body.String(), "setup-secret")
	require.Contains(t, res.Body.String(), "see workspace agent logs")
}

func TestAPIStreamCompletion(t *testing.T) {
	t.Parallel()
	for _, ending := range []string{"completed", "disconnected"} {
		t.Run(ending, func(t *testing.T) {
			t.Parallel()
			m, s := newTransportSession(t, workspacesdk.ACPSessionStatusRunning)
			ctx := testutil.Context(t, testutil.WaitLong)
			// Replayed idle statuses must not close a running stream.
			s.mu.Lock()
			s.info.Status = workspacesdk.ACPSessionStatusIdle
			s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
			s.info.Status = workspacesdk.ACPSessionStatusRunning
			s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
			s.mu.Unlock()
			baseline, err := s.Read(ctx, nil)
			require.NoError(t, err)
			server := httptest.NewServer(NewAPI(m).Routes())
			defer server.Close()
			query := url.Values{"harness_slug": {s.id.HarnessSlug}, "working_directory": {s.id.WorkingDirectory}, "session_id": {s.id.SessionID}}
			streamURL := server.URL + "/sessions/session/stream?" + query.Encode()
			conn := dialTestStream(ctx, t, streamURL)
			for _, expected := range baseline.Events {
				var event workspacesdk.ACPEvent
				require.NoError(t, wsjson.Read(ctx, conn, &event))
				require.Equal(t, expected, event)
			}
			var snapshot workspacesdk.ACPEvent
			require.NoError(t, wsjson.Read(ctx, conn, &snapshot))
			require.Equal(t, workspacesdk.ACPEventKindSnapshot, snapshot.Kind)
			require.Equal(t, baseline.Session, *snapshot.Session)
			switch ending {
			case "completed":
				notification := sessionNotification(s.id, "final output")
				require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
				require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, Turn: 1}))
			case "disconnected":
				require.NoError(t, m.Close())
			}
			var live []workspacesdk.ACPEvent
			for {
				var event workspacesdk.ACPEvent
				err = wsjson.Read(ctx, conn, &event)
				if err != nil {
					break
				}
				live = append(live, event)
			}
			if ending == "disconnected" {
				require.Equal(t, websocket.StatusTryAgainLater, websocket.CloseStatus(err))
				require.Empty(t, live)
			} else {
				require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err))
				require.Len(t, live, 2)
				require.Contains(t, string(live[0].Update), "final output")
				require.Equal(t, workspacesdk.ACPEventKindStatus, live[1].Kind)
				require.Equal(t, workspacesdk.ACPSessionStatusIdle, live[1].Session.Status)
			}
		})
	}
}
