package agentacp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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

func TestReadRequest(t *testing.T) {
	t.Parallel()
	const emptyMessage = `{"text":""}`
	maxText := strings.Repeat("x", httpapi.DefaultMaxRequestBodyBytes-len(emptyMessage))
	maxBody := `{"text":"` + maxText + `"}`
	for _, tc := range []struct {
		name   string
		body   string
		text   string
		status int
		detail string
	}{
		{
			name: "valid",
			body: `{"text":"hello"}`,
			text: "hello",
		},
		{
			name: "valid_with_whitespace",
			body: "{\"text\":\"hello\"}\n\t ",
			text: "hello",
		},
		{
			name: "at_size_limit",
			body: maxBody,
			text: maxText,
		},
		{
			name:   "unknown_field",
			body:   `{"text":"private-prompt","new_option":"private-config"}`,
			status: http.StatusBadRequest,
			detail: `json: unknown field "new_option". Check the field name. If this field is required, ` +
				"update the workspace agent, typically by restarting the workspace with a current agent.",
		},
		{
			name:   "long_unknown_field",
			body:   `{"text":"private-prompt","` + strings.Repeat("x", 1024) + `":"private-config"}`,
			status: http.StatusBadRequest,
			detail: "update the workspace agent, typically by restarting the workspace with a current agent.",
		},
		{
			name:   "empty",
			body:   "",
			status: http.StatusBadRequest,
			detail: "Request body must contain a JSON value.",
		},
		{
			name:   "malformed_json",
			body:   `{"text": }`,
			status: http.StatusBadRequest,
			detail: "Request body must be valid JSON (byte offset 10).",
		},
		{
			name:   "incomplete_json",
			body:   `{"text":"private-prompt`,
			status: http.StatusBadRequest,
			detail: "Request body contains incomplete JSON.",
		},
		{
			name:   "wrong_type",
			body:   `{"text":1234567890}`,
			status: http.StatusBadRequest,
			detail: `Request field "text" must have type string.`,
		},
		{
			name:   "wrong_body_type",
			body:   `[]`,
			status: http.StatusBadRequest,
			detail: "Request body must be a JSON object.",
		},
		{
			name:   "multiple_values",
			body:   `{"text":"private-prompt"} {}`,
			status: http.StatusBadRequest,
			detail: "Request body must contain exactly one JSON value.",
		},
		{
			name:   "malformed_trailing_json",
			body:   `{"text":"hello"} !`,
			status: http.StatusBadRequest,
			detail: "Request body must be valid JSON (byte offset 18).",
		},
		{
			name:   "incomplete_trailing_json",
			body:   `{"text":"hello"} {`,
			status: http.StatusBadRequest,
			detail: "Request body contains incomplete JSON.",
		},
		{
			name:   "oversized",
			body:   `{"text":"` + maxText + `x"}`,
			status: http.StatusRequestEntityTooLarge,
			detail: "Maximum request body size is 4194304 bytes.",
		},
		{
			name:   "oversized_trailing_whitespace",
			body:   maxBody + " ",
			status: http.StatusRequestEntityTooLarge,
			detail: "Maximum request body size is 4194304 bytes.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sessions/session/messages", strings.NewReader(tc.body))
			tracker := &httpapi.RequestBodyLimitTracker{}
			req = req.WithContext(httpapi.WithRequestBodyLimitTracker(req.Context(), tracker))
			response := httptest.NewRecorder()
			var result workspacesdk.ACPMessageRequest
			ok := readRequest(response, req, &result)
			if tc.status == 0 {
				require.True(t, ok)
				require.Equal(t, tc.text, result.Text)
				return
			}
			require.False(t, ok)
			require.Equal(t, tc.status, response.Code)
			var body codersdk.Response
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Contains(t, body.Detail, tc.detail)
			require.Less(t, len(body.Detail), 512)
			require.NotEmpty(t, body.Message)
			require.NotContains(t, response.Body.String(), "private-prompt")
			require.NotContains(t, response.Body.String(), "private-config")
			require.NotContains(t, response.Body.String(), "1234567890")
			require.Equal(t, tc.status == http.StatusRequestEntityTooLarge, tracker.Exceeded())
		})
	}
}

// deadlineResponseRecorder enforces the HTTP server's write deadline using the
// session manager's clock so recovery delays need no wall-clock waits.
type deadlineResponseRecorder struct {
	*httptest.ResponseRecorder
	clock    quartz.Clock
	deadline time.Time
	writeErr error
}

func (r *deadlineResponseRecorder) SetWriteDeadline(deadline time.Time) error {
	r.deadline = deadline
	return nil
}

func (r *deadlineResponseRecorder) Write(body []byte) (int, error) {
	if !r.deadline.IsZero() && !r.clock.Now().Before(r.deadline) {
		r.writeErr = os.ErrDeadlineExceeded
		return 0, r.writeErr
	}
	return r.ResponseRecorder.Write(body)
}

func TestAPISessionRecoveryWriteDeadline(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		method string
		path   string
		status int
	}{
		{name: "read", method: http.MethodGet, status: http.StatusOK},
		{name: "message", method: http.MethodPost, path: "/messages", status: http.StatusOK},
		{name: "interrupt", method: http.MethodPost, path: "/interrupt", status: http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
			info := createTestSession(t, m, dir)
			m = restartTestManager(t, m, dir)
			api := NewAPI(m)
			toolCalls := agenttoolcall.New(m.clock, func(context.Context, uuid.UUID, uuid.UUID) {})
			router := chi.NewRouter()
			router.Use(agentchat.Middleware, api.WriteDeadlineMiddleware, toolCalls.Middleware)
			router.Mount("/api/v0/acp", api.Routes())
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := m.clock.(*quartz.Mock)
			clock.Set(time.Now()).MustWait(ctx)
			m.updateEnv = func(env []string) ([]string, error) {
				// Restoration outlives the agent HTTP server's 20-second timeout.
				clock.Advance(25 * time.Second).MustWait(ctx)
				return env, nil
			}
			response := &deadlineResponseRecorder{
				ResponseRecorder: httptest.NewRecorder(),
				clock:            clock,
				deadline:         clock.Now().Add(20 * time.Second),
			}
			query := url.Values{"harness_slug": {info.ID.HarnessSlug}, "working_directory": {dir}, "session_id": {info.ID.SessionID}}
			raw, err := json.Marshal(workspacesdk.ACPMessageRequest{Text: "hello"})
			require.NoError(t, err)
			req := httptest.NewRequestWithContext(ctx, tc.method, "/api/v0/acp/sessions/session"+tc.path+"?"+query.Encode(), bytes.NewReader(raw))
			if tc.method == http.MethodPost {
				req.Header.Set(workspacesdk.CoderChatIDHeader, uuid.NewString())
				req.Header.Set(workspacesdk.CoderToolCallIDHeader, uuid.NewString())
			}
			router.ServeHTTP(response, req)
			require.NoError(t, response.writeErr)
			require.Equal(t, tc.status, response.Code)
			switch tc.name {
			case "read":
				var result workspacesdk.ACPSessionResponse
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
				require.Equal(t, info.ID, result.Session.ID)
			case "message":
				var result workspacesdk.ACPMessageResponse
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
				require.Equal(t, workspacesdk.ACPMessageOutcomePrompt, result.Outcome)
			case "interrupt":
				// Restoring an idle session must return its conflict response.
				require.Contains(t, response.Body.String(), "interrupt in idle")
			}
		})
	}
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
	res := callTestAPI(t, handler, "POST", path+"/messages?"+query.Encode(), workspacesdk.ACPMessageRequest{Text: "hello"}, headers)
	require.Equal(t, http.StatusOK, res.Code)
	completed := awaitTestSession(t, m, id)
	query.Set("generation_id", completed.Session.Cursor.GenerationID.String())
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
	res := callTestAPI(t, handler, "GET", "/sessions/session?"+query.Encode(), nil, nil)
	require.Equal(t, http.StatusOK, res.Code)
	require.NotContains(t, res.Body.String(), "runtime-secret")
	require.Contains(t, res.Body.String(), "see workspace agent logs")
	setTestHarnessCommand(m, "printf 'credential=%s\\n' 'setup-secret' >&2; exit 1")
	res = callTestAPI(t, handler, "POST", "/sessions", workspacesdk.ACPCreateSessionRequest{HarnessSlug: "fake", WorkingDirectory: dir}, nil)
	require.Equal(t, http.StatusServiceUnavailable, res.Code)
	require.NotContains(t, res.Body.String(), "setup-secret")
	require.Contains(t, res.Body.String(), "see workspace agent logs")
}

func TestAPIStreamCompletion(t *testing.T) {
	t.Parallel()
	for _, ending := range []string{"idle", "error", "completed", "failed", "interrupted", "disconnected", "canceled"} {
		t.Run(ending, func(t *testing.T) {
			t.Parallel()
			status := workspacesdk.ACPSessionStatusRunning
			switch ending {
			case "idle":
				status = workspacesdk.ACPSessionStatusIdle
			case "error":
				status = workspacesdk.ACPSessionStatusError
			case "interrupted":
				status = workspacesdk.ACPSessionStatusInterrupting
			}
			m, s := newTransportSession(t, status)
			ctx := testutil.Context(t, testutil.WaitLong)
			// Replayed idle statuses must not close a running stream.
			s.mu.Lock()
			s.info.Status = workspacesdk.ACPSessionStatusIdle
			s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
			s.info.Status = status
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
			var expected []workspacesdk.ACPEvent
			switch ending {
			case "completed", "interrupted", "failed":
				notification := sessionNotification(s.id, "final output")
				require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
				if ending == "failed" {
					require.NoError(t, s.adapterError(ctx, xerrors.New("test failure")))
				} else {
					require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, Turn: 1}))
				}
				completed, err := s.Read(ctx, nil)
				require.NoError(t, err)
				expected = completed.Events[len(baseline.Events):]
				if ending != "failed" {
					// A subsequent turn must not extend the current stream.
					_, err = m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{Text: "next turn"})
					require.NoError(t, err)
				}
			case "disconnected":
				require.NoError(t, m.Close())
			case "canceled":
				require.NoError(t, conn.CloseNow())
			}
			var live []workspacesdk.ACPEvent
			if ending != "canceled" {
				for {
					var event workspacesdk.ACPEvent
					err = wsjson.Read(ctx, conn, &event)
					if err != nil {
						break
					}
					live = append(live, event)
				}
				closeStatus := websocket.StatusNormalClosure
				if ending == "disconnected" {
					closeStatus = websocket.StatusTryAgainLater
				}
				require.Equal(t, closeStatus, websocket.CloseStatus(err))
			}
			require.Equal(t, expected, live)
			require.Eventually(t, func() bool {
				s.mu.Lock()
				defer s.mu.Unlock()
				return len(s.subscribers) == 0
			}, testutil.WaitLong, testutil.IntervalFast)
			if ending == "canceled" {
				current, err := s.Read(ctx, nil)
				require.NoError(t, err)
				require.Equal(t, baseline, current)
			}
		})
	}
}
