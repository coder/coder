package agentacp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestACPHelperProcess(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_ACP_TEST_HELPER") != "1" {
		return
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	var pending any
	for {
		var req struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if decoder.Decode(&req) != nil {
			os.Exit(0) //nolint:revive // Keep the test runner's output out of the ACP transport.
		}
		result := any(map[string]any{})
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}}
		case "session/new":
			result = map[string]any{"sessionId": "test-session"}
		case "session/prompt":
			var params struct {
				Prompt []struct {
					Text string `json:"text"`
				} `json:"prompt"`
			}
			_ = json.Unmarshal(req.Params, &params)
			text := params.Prompt[0].Text
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "test-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "answer: " + text}}}})
			if text == "block" {
				pending = req.ID
				continue
			}
			result = map[string]any{"stopReason": "end_turn"}
		case "session/cancel":
			if pending != nil {
				_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": pending, "result": map[string]any{"stopReason": "cancelled"}}) //nolint:misspell // ACP protocol value.
				pending = nil
			}
			continue
		}
		if req.ID != nil {
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		}
	}
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	m := New(t.Context(), func() string { return t.TempDir() }, func(env []string) ([]string, error) { return append(env, "CODER_ACP_TEST_HELPER=1"), nil }, uuid.New)
	m.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, executable, "-test.run=^TestACPHelperProcess$") // #nosec G204: Run this test binary as the fake adapter.
	}
	t.Cleanup(m.Close)
	return m
}
func awaitStatus(t *testing.T, s *session, status string) codersdk.ACPSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	for {
		s.mu.Lock()
		v := s.snapshotLocked()
		changed := s.changed
		s.mu.Unlock()
		if v.Status == status {
			return v
		}
		select {
		case <-ctx.Done():
			t.Fatalf("expected %s, got %+v", status, v)
		case <-changed:
		}
	}
}
func TestACPProcessLifecycle(t *testing.T) {
	t.Parallel()
	m := testManager(t)
	parent, org := uuid.New(), uuid.New()
	ctx, cancel := context.WithCancel(t.Context())
	first, err := m.Spawn(ctx, parent, org, codersdk.ACPSpawnRequest{Agent: "codex", Prompt: "block"})
	require.NoError(t, err)
	s, err := m.lookup(first.SessionID, parent, org)
	require.NoError(t, err)
	cancel()
	require.NoError(t, s.ctx.Err(), "spawn request does not own the adapter")
	require.NoError(t, s.message("follow-up", true))
	done := awaitStatus(t, s, "waiting")
	require.Len(t, done.Entries, 4)
	require.Equal(t, "answer: follow-up", done.Entries[3].Text)
	require.NoError(t, s.message("again", false))
	done = awaitStatus(t, s, "waiting")
	require.Len(t, done.Entries, 6)
	_, err = m.lookup(first.SessionID, uuid.New(), org)
	require.Error(t, err)
	_, err = m.lookup(first.SessionID, parent, uuid.New())
	require.Error(t, err)
	m.Close()
	_, err = m.lookup(first.SessionID, parent, org)
	require.Error(t, err)
}
func TestACPAPI(t *testing.T) {
	t.Parallel()
	m := testManager(t)
	parent, org := uuid.New(), uuid.New()
	server := httptest.NewServer(m.Routes())
	defer server.Close()
	base := fmt.Sprintf("%s/%s/%s/", server.URL, org, parent)
	request := func(method, url, body string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return server.Client().Do(req)
	}
	response, err := request(http.MethodPost, base, `{"agent":"claude_code","prompt":"hello"}`)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var created codersdk.ACPSession
	require.NoError(t, json.NewDecoder(response.Body).Decode(&created))
	response, err = request(http.MethodGet, base+created.SessionID.String()+"/wait", "")
	require.NoError(t, err)
	defer response.Body.Close()
	var done codersdk.ACPSession
	require.NoError(t, json.NewDecoder(response.Body).Decode(&done))
	require.Equal(t, "waiting", done.Status)
	require.Len(t, done.Entries, 2)
	response, err = request(http.MethodPost, base+created.SessionID.String()+"/messages", `{"message":"next"}`)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	response, err = request(http.MethodGet, base+"?limit=1", "")
	require.NoError(t, err)
	defer response.Body.Close()
	var list codersdk.ACPListResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&list))
	require.Equal(t, 1, list.Total)
}
func TestACPCommands(t *testing.T) {
	t.Parallel()
	for _, agent := range []string{"claude_code", "codex"} {
		script, err := command(agent)
		require.NoError(t, err)
		require.Contains(t, script, "exec npx -y @agentclientprotocol/")
	}
	_, err := command("codex; exit")
	require.Error(t, err)
}

func TestACPStreamReconnect(t *testing.T) {
	t.Parallel()
	m := testManager(t)
	parent, org := uuid.New(), uuid.New()
	created, err := m.Spawn(t.Context(), parent, org, codersdk.ACPSpawnRequest{Agent: "codex", Prompt: "block"})
	require.NoError(t, err)
	router := chi.NewRouter()
	router.Mount("/api/v0/acp", m.Routes())
	server := httptest.NewServer(router)
	defer server.Close()
	url := fmt.Sprintf("ws%s/api/v0/acp/%s/%s/%s/stream", strings.TrimPrefix(server.URL, "http"), org, parent, created.SessionID)
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	socket, _, err := websocket.Dial(ctx, url, nil) //nolint:bodyclose // websocket.Dial owns the response body.
	require.NoError(t, err)
	var first codersdk.ACPSession
	require.NoError(t, wsjson.Read(ctx, socket, &first))
	socket.CloseNow()
	s, err := m.lookup(created.SessionID, parent, org)
	require.NoError(t, err)
	require.NoError(t, s.message("next", true))
	done := awaitStatus(t, s, "waiting")
	socket, _, err = websocket.Dial(ctx, url, nil) //nolint:bodyclose // websocket.Dial owns the response body.
	require.NoError(t, err)
	defer socket.CloseNow()
	var replay codersdk.ACPSession
	require.NoError(t, wsjson.Read(ctx, socket, &replay))
	require.Equal(t, done.Entries, replay.Entries)
	require.GreaterOrEqual(t, replay.Version, first.Version)
	require.Len(t, replay.Entries, 4)
}

func TestACPAutomaticPermissions(t *testing.T) {
	t.Parallel()
	s := &session{data: codersdk.ACPSession{Status: "running"}}
	req := acp.RequestPermissionRequest{Options: []acp.PermissionOption{{OptionId: "deny", Kind: "reject_once", Name: "Deny"}, {OptionId: "allow", Kind: "allow_once", Name: "Allow"}}}
	response, err := s.RequestPermission(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, acp.PermissionOptionId("allow"), response.Outcome.Selected.OptionId)
	s.data.Status = "interrupting"
	response, err = s.RequestPermission(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, response.Outcome.Cancelled) //nolint:misspell // The ACP SDK uses the protocol's spelling.
}

func TestACPRealAdapters(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_ACP_SMOKE") != "1" {
		t.Skip("set CODER_ACP_SMOKE=1 in a workspace with adapter credentials")
	}
	for _, name := range []string{"claude_code", "codex"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitSuperLong)
			defer cancel()
			dir := t.TempDir()
			m := New(ctx, func() string { return dir }, func(env []string) ([]string, error) { return env, nil }, uuid.New)
			defer m.Close()
			parent, org := uuid.New(), uuid.New()
			created, err := m.Spawn(ctx, parent, org, codersdk.ACPSpawnRequest{Agent: name, Prompt: "Use your shell tool to run printf ACP_OK, then reply with exactly ACP_OK."})
			require.NoError(t, err)
			s, err := m.lookup(created.SessionID, parent, org)
			require.NoError(t, err)
			for {
				s.mu.Lock()
				state := s.snapshotLocked()
				changed := s.changed
				s.mu.Unlock()
				if state.Status == "error" {
					t.Fatalf("adapter failed: %s", state.Error)
				}
				if state.Status == "waiting" {
					var response string
					for _, entry := range state.Entries {
						if entry.Role == "assistant" && entry.Kind == "text" {
							response += entry.Text
						}
					}
					require.Contains(t, response, "ACP_OK")
					require.True(t, slices.ContainsFunc(state.Entries, func(entry codersdk.ACPEntry) bool { return entry.Kind == "tool" && entry.Status == "completed" }), "adapter must execute a workspace tool")
					return
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-changed:
				}
			}
		})
	}
}

func TestACPWaitTimeoutDoesNotStopSession(t *testing.T) {
	t.Parallel()
	m := testManager(t)
	clock := quartz.NewMock(t)
	m.clock = clock
	parent, org := uuid.New(), uuid.New()
	created, err := m.Spawn(t.Context(), parent, org, codersdk.ACPSpawnRequest{Agent: "codex", Prompt: "block"})
	require.NoError(t, err)
	ctx := testutil.Context(t, testutil.WaitLong)
	trap := clock.Trap().NewTimer("acp", "wait")
	defer trap.Close()
	router := chi.NewRouter()
	router.Mount("/api/v0/acp", m.Routes())
	response := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/api/v0/acp/%s/%s/%s/wait?timeout_seconds=1", org, parent, created.SessionID), nil)
	done := make(chan struct{})
	go func() { defer close(done); router.ServeHTTP(response, request) }()
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(time.Second).MustWait(ctx)
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-done:
	}
	require.Equal(t, 200, response.Code)
	var result codersdk.ACPSession
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Equal(t, "running", result.Status)
	s, err := m.lookup(created.SessionID, parent, org)
	require.NoError(t, err)
	require.NoError(t, s.ctx.Err())
	require.NoError(t, s.interrupt())
	awaitStatus(t, s, "waiting")
}
