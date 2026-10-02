package agentacp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestAPIAndStreamReplay(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	handler := http.StripPrefix("/api/v0/acp", NewAPI(m).Routes())
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx := testutil.Context(t, testutil.WaitLong)
	type testResponse struct {
		StatusCode int
		Body       io.Reader
	}
	call := func(method, path string, body any, header string) testResponse {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, method, server.URL+"/api/v0/acp"+path, bytes.NewReader(raw))
		require.NoError(t, err)
		req.Header.Set(workspacesdk.CoderChatIDHeader, header)
		res, err := server.Client().Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		bodyBytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		return testResponse{StatusCode: res.StatusCode, Body: bytes.NewReader(bodyBytes)}
	}
	for _, header := range []string{"", "invalid", uuid.Nil.String()} {
		require.Equal(t, http.StatusOK, call("GET", "/harnesses", nil, header).StatusCode)
	}
	req := workspacesdk.ACPCreateSessionRequest{RequestID: uuid.New(), HarnessSlug: "fake", WorkingDirectory: dir}
	res := call("POST", "/sessions", req, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var info workspacesdk.ACPSession
	require.NoError(t, json.NewDecoder(res.Body).Decode(&info))
	res = call("POST", "/sessions", req, uuid.NewString())
	require.Equal(t, http.StatusOK, res.StatusCode)
	var duplicate workspacesdk.ACPSession
	require.NoError(t, json.NewDecoder(res.Body).Decode(&duplicate))
	require.Equal(t, info.ID, duplicate.ID)
	q := url.Values{"harness_slug": {info.ID.HarnessSlug}, "working_directory": {info.ID.WorkingDirectory}, "session_id": {info.ID.SessionID}}
	path := func(suffix string) string { return "/sessions/session" + suffix + "?" + q.Encode() }
	for _, header := range []string{"", "invalid", uuid.Nil.String(), uuid.NewString()} {
		res := call("GET", path(""), nil, header)
		require.Equal(t, http.StatusOK, res.StatusCode)
		var read workspacesdk.ACPSessionResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&read))
		require.Equal(t, info.ID, read.Session.ID)
		require.Equal(t, info.Cursor, read.Session.Cursor)
	}
	require.Equal(t, http.StatusOK, call("GET", path(""), nil, "").StatusCode)
	require.Equal(t, http.StatusNotFound, call("GET", path("/wait"), nil, "").StatusCode)
	streamURL := strings.Replace(server.URL, "http://", "ws://", 1) + "/api/v0/acp" + path("/stream")
	conn, wsResponse, err := websocket.Dial(ctx, streamURL, nil)
	require.NoError(t, err)
	if wsResponse != nil && wsResponse.Body != nil {
		_ = wsResponse.Body.Close()
	}
	defer func() { _ = conn.CloseNow() }()
	var last uint64
	for {
		var event workspacesdk.ACPEvent
		require.NoError(t, wsjson.Read(ctx, conn, &event))
		require.GreaterOrEqual(t, event.Cursor.Seq, last)
		last = event.Cursor.Seq
		if event.Kind == workspacesdk.ACPEventKindSnapshot {
			require.Equal(t, workspacesdk.ACPSessionStatusIdle, event.Session.Status)
			break
		}
	}
	message := workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"}
	require.Equal(t, http.StatusOK, call("POST", path("/messages"), message, "").StatusCode)
	require.Equal(t, http.StatusOK, call("POST", path("/messages"), message, uuid.NewString()).StatusCode)
	userMessages := 0
	var updates []string
	var completed workspacesdk.ACPSession
	for {
		var event workspacesdk.ACPEvent
		require.NoError(t, wsjson.Read(ctx, conn, &event))
		require.Greater(t, event.Cursor.Seq, last)
		last = event.Cursor.Seq
		if event.Kind == workspacesdk.ACPEventKindUserMessage {
			userMessages++
		}
		if event.Update != nil {
			updates = append(updates, string(event.Update))
		}
		if event.Kind == workspacesdk.ACPEventKindStatus && event.Session.Status != workspacesdk.ACPSessionStatusRunning {
			completed = *event.Session
			break
		}
	}
	require.Equal(t, 1, userMessages)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, completed.Status)
	require.Contains(t, strings.Join(updates, "\n"), "answer:hello:default")
	res = call("GET", path("")+"&epoch="+info.Cursor.Epoch.String()+"&after="+strconv.FormatUint(info.Cursor.Seq, 10), nil, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var result workspacesdk.ACPSessionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	require.Equal(t, "answer:hello:default", result.AssistantResponse)
	require.Equal(t, completed.Cursor, result.Session.Cursor)
	res = call("GET", "/sessions", nil, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var listed []workspacesdk.ACPSession
	require.NoError(t, json.NewDecoder(res.Body).Decode(&listed))
	require.Len(t, listed, 1)
	require.Equal(t, info.ID, listed[0].ID)
	_ = conn.CloseNow()
	replayURL := streamURL + "&epoch=" + result.Session.Cursor.Epoch.String() + "&after=0"
	conn, wsResponse, err = websocket.Dial(ctx, replayURL, nil)
	require.NoError(t, err)
	if wsResponse != nil && wsResponse.Body != nil {
		_ = wsResponse.Body.Close()
	}
	defer func() { _ = conn.CloseNow() }()
	var event workspacesdk.ACPEvent
	require.NoError(t, wsjson.Read(ctx, conn, &event))
	require.Equal(t, uint64(1), event.Cursor.Seq)
	require.Equal(t, http.StatusMethodNotAllowed, call("DELETE", path(""), nil, "").StatusCode)
	require.Equal(t, http.StatusOK, call("GET", path(""), nil, "").StatusCode)
}
