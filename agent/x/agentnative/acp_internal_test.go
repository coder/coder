package agentnative

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

type acpFakeManager struct {
	mu        sync.Mutex
	session   Session
	events    []Event
	changed   chan struct{}
	submitted chan Prompt
	canceled  chan string
	creates   int
	modes     []string
	submit    func(context.Context, Prompt) error
	gap       bool
}

func newACPFake() *acpFakeManager {
	id := uuid.NewString()
	return &acpFakeManager{
		session: Session{ID: id, NativeID: id, Harness: "claude", Cwd: "/workspace", Mode: "automation", State: json.RawMessage(`{"private":"secret receipt"}`)},
		changed: make(chan struct{}), submitted: make(chan Prompt, 10), canceled: make(chan string, 10),
	}
}

func (m *acpFakeManager) List() []Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return []Session{m.session}
}

func (m *acpFakeManager) Session(id string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id != m.session.ID {
		return Session{}, xerrors.New("unknown native session")
	}
	return m.session, nil
}

//nolint:revive // Implements the manager's blocking/nonblocking event interface.
func (m *acpFakeManager) Events(ctx context.Context, _ string, after uint64, wait bool) ([]Event, uint64, error) {
	for {
		m.mu.Lock()
		cursor := m.session.Cursor
		events := append([]Event(nil), m.events[after:]...)
		if len(events) > 0 && m.gap {
			events[0].Seq++
		}
		changed := m.changed
		m.mu.Unlock()
		if len(events) > 0 || !wait {
			return events, cursor, nil
		}
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-changed:
		}
	}
}

func (m *acpFakeManager) Create(_ context.Context, harness, cwd string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creates++
	m.session.Harness, m.session.Cwd = harness, cwd
	return m.session, nil
}

func (m *acpFakeManager) Submit(ctx context.Context, _ string, p Prompt) error {
	m.submitted <- p
	if m.submit != nil {
		return m.submit(ctx, p)
	}
	return nil
}

func (m *acpFakeManager) SetMode(_ context.Context, _ string, mode string) error {
	m.mu.Lock()
	m.modes = append(m.modes, mode)
	m.session.Mode = mode
	m.mu.Unlock()
	m.publish("mode", map[string]any{"mode": mode})
	return nil
}

func (m *acpFakeManager) Cancel(_ context.Context, _ string, request string) error {
	m.canceled <- request
	return nil
}

func (m *acpFakeManager) publish(kind string, data map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session.Cursor++
	m.events = append(m.events, Event{Seq: m.session.Cursor, Type: kind, Data: data})
	close(m.changed)
	m.changed = make(chan struct{})
}

type acpTestClient struct {
	t        *testing.T
	ctx      context.Context
	conn     net.Conn
	messages chan map[string]json.RawMessage
	closed   chan error
	next     int
	updates  []map[string]json.RawMessage
}

func newACPClient(t *testing.T, manager acpManager) *acpTestClient {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	client, server := net.Pipe()
	c := &acpTestClient{t: t, ctx: ctx, conn: client, messages: make(chan map[string]json.RawMessage, 100), closed: make(chan error, 1)}
	go func() { c.closed <- serveACPStream(ctx, server, manager) }()
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		decoder := json.NewDecoder(bufio.NewReader(client))
		for {
			var message map[string]json.RawMessage
			if decoder.Decode(&message) != nil {
				return
			}
			select {
			case c.messages <- message:
			case <-ctx.Done():
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case <-readerDone:
		case <-ctx.Done():
			t.Error("ACP client reader did not close")
		}
		select {
		case <-c.closed:
		case <-ctx.Done():
			t.Error("ACP server did not close")
		}
	})
	return c
}

func (c *acpTestClient) send(method string, params any) int {
	c.t.Helper()
	c.next++
	err := json.NewEncoder(c.conn).Encode(map[string]any{"jsonrpc": "2.0", "id": c.next, "method": method, "params": params})
	require.NoError(c.t, err)
	return c.next
}

func (c *acpTestClient) response(id int) map[string]json.RawMessage {
	c.t.Helper()
	for {
		select {
		case message := <-c.messages:
			if string(message["id"]) == fmt.Sprint(id) {
				return message
			}
			c.updates = append(c.updates, message)
		case <-c.ctx.Done():
			c.t.Fatal("timed out waiting for ACP response")
		}
	}
}

func (c *acpTestClient) request(method string, params any) map[string]json.RawMessage {
	c.t.Helper()
	response := c.response(c.send(method, params))
	require.Empty(c.t, response["error"], string(response["error"]))
	return response
}

func (c *acpTestClient) load(m *acpFakeManager) {
	c.t.Helper()
	c.request("session/load", map[string]any{"sessionId": m.session.ID, "cwd": m.session.Cwd})
}

func TestACPReadOnlyDiscoveryAndReplay(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	m.session.Archived = true
	m.publish("error", map[string]any{"message": "historical failure"})
	m.publish("turn.start", map[string]any{"turnId": "historical", "text": "original task"})
	m.publish("text", map[string]any{"turnId": "historical", "text": "saved answer"})
	m.publish("turn.complete", map[string]any{"turnId": "historical", "answer": "saved answer", "reason": "answer"})
	c := newACPClient(t, m)
	initialized := c.request("initialize", map[string]any{"protocolVersion": 1})
	require.Contains(t, string(initialized["result"]), `"opencode"`)
	listed := c.request("session/list", nil)
	require.Contains(t, string(listed["result"]), m.session.ID)
	require.NotContains(t, string(listed["result"]), "secret receipt")
	c.load(m)
	require.Len(t, c.updates, 2)
	require.Contains(t, string(c.updates[0]["params"]), "original task")
	require.Contains(t, string(c.updates[1]["params"]), "saved answer")
	c.load(m)
	require.Len(t, c.updates, 2, "repeated load must not duplicate history")
	require.Zero(t, m.creates)
	select {
	case <-m.submitted:
		t.Fatal("loading history submitted a prompt")
	default:
	}
}

func TestACPProjectsNativeEventsAndMatchesRoot(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	m.submit = func(_ context.Context, p Prompt) error {
		m.publish("prompt.accepted", map[string]any{"requestId": p.ID})
		m.publish("turn.start", map[string]any{"turnId": "root", "requestId": p.ID, "text": p.Text})
		m.publish("turn.start", map[string]any{"turnId": "child", "requestId": p.ID, "text": "child task"})
		m.publish("turn.complete", map[string]any{"turnId": "child", "requestId": p.ID, "answer": "child answer", "reason": "refusal"})
		m.publish("thought", map[string]any{"turnId": "root", "requestId": p.ID, "text": "thinking"})
		m.publish("text", map[string]any{"turnId": "root", "requestId": p.ID, "text": "**answer**"})
		m.publish("tool.start", map[string]any{"turnId": "root", "requestId": p.ID, "toolCallId": "tool", "title": "pwd", "input": map[string]any{}})
		m.publish("tool.end", map[string]any{"turnId": "root", "requestId": p.ID, "toolCallId": "tool", "input": map[string]any{"command": "pwd"}, "result": "/workspace"})
		m.publish("turn.complete", map[string]any{"turnId": "root", "requestId": p.ID, "answer": "**answer**", "reason": "answer"})
		return nil
	}
	c := newACPClient(t, m)
	c.load(m)
	response := c.request("session/prompt", map[string]any{"sessionId": m.session.ID, "prompt": []any{map[string]any{"type": "text", "text": "do work"}}})
	require.JSONEq(t, `{"stopReason":"end_turn"}`, string(response["result"]))
	var kinds []string
	var all strings.Builder
	for _, update := range c.updates {
		var p struct {
			Update struct {
				Kind string `json:"sessionUpdate"`
			} `json:"update"`
		}
		require.NoError(t, json.Unmarshal(update["params"], &p))
		kinds = append(kinds, p.Update.Kind)
		_, _ = all.Write(update["params"])
	}
	require.Equal(t, []string{"user_message_chunk", "agent_message_chunk", "agent_thought_chunk", "agent_message_chunk", "tool_call", "tool_call_update"}, kinds)
	require.Equal(t, 1, strings.Count(all.String(), "**answer**"), "completion must not duplicate streamed output")
	require.NotContains(t, all.String(), "do work", "the client already renders its submitted prompt")
	require.Contains(t, all.String(), `"rawInput":{"command":"pwd"}`)
}

func TestACPCancelWaitsForDispatchAndUsesRequestID(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	release := make(chan struct{})
	m.submit = func(ctx context.Context, p Prompt) error {
		select {
		case <-release:
			m.publish("prompt.accepted", map[string]any{"requestId": p.ID})
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c := newACPClient(t, m)
	c.load(m)
	promptID := c.send("session/prompt", map[string]any{"sessionId": m.session.ID, "prompt": []any{map[string]any{"type": "text", "text": "task"}}})
	var p Prompt
	select {
	case p = <-m.submitted:
	case <-c.ctx.Done():
		t.Fatal("prompt was not dispatched")
	}
	c.request("session/cancel", map[string]any{"sessionId": m.session.ID})
	select {
	case <-m.canceled:
		t.Fatal("cancellation raced ahead of prompt registration")
	default:
	}
	close(release)
	select {
	case requestID := <-m.canceled:
		require.Equal(t, p.ID, requestID)
	case <-c.ctx.Done():
		t.Fatal("queued cancellation was not dispatched")
	}
	m.publish("prompt.rejected", map[string]any{"requestId": p.ID, "cancelled": true})      //nolint:misspell // Native event field name.
	require.JSONEq(t, `{"stopReason":"cancelled"}`, string(c.response(promptID)["result"])) //nolint:misspell // ACP v1 spelling.
}

func TestACPBackToBackCancelNotification(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	c := newACPClient(t, m)
	c.load(m)
	prompt, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/prompt", "params": map[string]any{"sessionId": m.session.ID, "prompt": []any{map[string]any{"type": "text", "text": "task"}}}})
	require.NoError(t, err)
	cancel, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": m.session.ID}})
	require.NoError(t, err)
	_, err = c.conn.Write(append(append(append(prompt, '\n'), cancel...), '\n'))
	require.NoError(t, err)
	var requestID string
	select {
	case requestID = <-m.canceled:
	case <-c.ctx.Done():
		t.Fatal("back-to-back cancellation was lost before prompt admission")
	}
	m.publish("prompt.rejected", map[string]any{"requestId": requestID, "cancelled": true}) //nolint:misspell // Native event field name.
	require.JSONEq(t, `{"stopReason":"cancelled"}`, string(c.response(2)["result"]))        //nolint:misspell // ACP v1 spelling.
}

func TestACPCompletionBeforeSubmissionAcknowledgement(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	release := make(chan struct{})
	returned := make(chan struct{})
	m.submit = func(ctx context.Context, p Prompt) error {
		defer close(returned)
		m.publish("turn.start", map[string]any{"turnId": "fast", "requestId": p.ID, "text": p.Text})
		m.publish("turn.complete", map[string]any{"turnId": "fast", "requestId": p.ID, "answer": "done", "reason": "answer"})
		select {
		case <-release:
			return xerrors.New("late transport failure")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c := newACPClient(t, m)
	c.load(m)
	response := c.request("session/prompt", map[string]any{"sessionId": m.session.ID, "prompt": []any{map[string]any{"type": "text", "text": "task"}}})
	require.JSONEq(t, `{"stopReason":"end_turn"}`, string(response["result"]))
	close(release)
	select {
	case <-returned:
	case <-c.ctx.Done():
		t.Fatal("submission did not finish")
	}
	c.load(m)
	for _, update := range c.updates {
		require.NotContains(t, string(update["params"]), "late transport failure")
	}
}

func TestACPPersistedCompletionWinsSubmissionError(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	m.submit = func(_ context.Context, p Prompt) error {
		m.publish("turn.start", map[string]any{"turnId": "fast", "requestId": p.ID, "text": p.Text})
		m.publish("turn.complete", map[string]any{"turnId": "fast", "requestId": p.ID, "answer": "done", "reason": "answer"})
		return xerrors.New("lost acknowledgement")
	}
	c := newACPClient(t, m)
	c.load(m)
	response := c.request("session/prompt", map[string]any{"sessionId": m.session.ID, "prompt": []any{map[string]any{"type": "text", "text": "task"}}})
	require.JSONEq(t, `{"stopReason":"end_turn"}`, string(response["result"]))
}

func TestACPBoundsOutstandingSubmissionAcknowledgements(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	m.submit = func(ctx context.Context, p Prompt) error {
		m.publish("turn.start", map[string]any{"turnId": p.ID, "requestId": p.ID, "text": p.Text})
		m.publish("turn.complete", map[string]any{"turnId": p.ID, "requestId": p.ID, "answer": "done", "reason": "answer"})
		<-ctx.Done()
		return ctx.Err()
	}
	c := newACPClient(t, m)
	c.load(m)
	params := map[string]any{"sessionId": m.session.ID, "prompt": []any{map[string]any{"type": "text", "text": "task"}}}
	for range acpMaxSubmissions {
		response := c.request("session/prompt", params)
		require.JSONEq(t, `{"stopReason":"end_turn"}`, string(response["result"]))
		select {
		case <-m.submitted:
		case <-c.ctx.Done():
			t.Fatal("completed submission was not dispatched")
		}
	}
	response := c.response(c.send("session/prompt", params))
	require.Contains(t, string(response["error"]), "submission limit")
	// Admission exhaustion must not block control messages or cleanup.
	c.request("session/cancel", map[string]any{"sessionId": m.session.ID})
	select {
	case <-m.canceled:
		t.Fatal("a completed native turn was canceled")
	default:
	}
}

func TestACPDisconnectDoesNotCancelNativeWork(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	c := newACPClient(t, m)
	c.load(m)
	c.send("session/prompt", map[string]any{"sessionId": m.session.ID, "prompt": []any{map[string]any{"type": "text", "text": "keep working"}}})
	select {
	case <-m.submitted:
	case <-c.ctx.Done():
		t.Fatal("prompt was not dispatched")
	}
	require.NoError(t, c.conn.Close())
	select {
	case <-c.closed:
		c.closed <- nil // Preserve the completion signal for cleanup.
	case <-c.ctx.Done():
		t.Fatal("ACP connection did not close")
	}
	select {
	case <-m.canceled:
		t.Fatal("disconnect canceled native work")
	default:
	}
}

func TestACPRejectsGapBeforeReplayingAnswer(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	m.gap = true
	m.publish("turn.complete", map[string]any{"turnId": "old", "answer": "untrusted answer", "reason": "answer"})
	c := newACPClient(t, m)
	response := c.response(c.send("session/load", map[string]any{"sessionId": m.session.ID, "cwd": m.session.Cwd}))
	require.Contains(t, string(response["error"]), "gap")
	for _, update := range c.updates {
		require.NotContains(t, string(update["params"]), "untrusted answer")
	}
}

func TestACPRejectsUnsupportedInputs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, method string
		params       map[string]any
	}{
		{"relative directory", "session/new", map[string]any{"cwd": "relative"}},
		{"MCP injection", "session/new", map[string]any{"cwd": "/workspace", "mcpServers": []any{map[string]any{"name": "untrusted"}}}},
		{"extra directory", "session/new", map[string]any{"cwd": "/workspace", "additionalDirectories": []string{"/other"}}},
		{"image prompt", "session/prompt", map[string]any{"prompt": []any{map[string]any{"type": "image", "data": "base64"}}}},
		{"empty prompt", "session/prompt", map[string]any{"prompt": []any{map[string]any{"type": "text", "text": "   "}}}},
		{"wrong directory", "session/load", map[string]any{"cwd": "/other"}},
		{"unknown mode", "session/set_mode", map[string]any{"modeId": "unsafe"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newACPFake()
			c := newACPClient(t, m)
			c.load(m)
			tc.params["sessionId"] = m.session.ID
			response := c.response(c.send(tc.method, tc.params))
			require.Contains(t, string(response["error"]), `"code":-32602`)
			require.Zero(t, m.creates)
		})
	}
}

func TestACPFragmentedMessagesAndNotifications(t *testing.T) {
	t.Parallel()
	m := newACPFake()
	c := newACPClient(t, m)
	// A native mutation without a request ID must never run.
	require.NoError(t, json.NewEncoder(c.conn).Encode(map[string]any{"jsonrpc": "2.0", "method": "session/new", "params": map[string]any{"cwd": "/workspace"}}))
	request := []byte(`{"jsonrpc":"2.0","id":"fragmented","method":"initialize","params":{}}` + "\n")
	_, err := c.conn.Write(request[:21])
	require.NoError(t, err)
	_, err = c.conn.Write(request[21:])
	require.NoError(t, err)
	select {
	case message := <-c.messages:
		require.Equal(t, `"fragmented"`, string(message["id"]))
		require.Contains(t, string(message["result"]), `"protocolVersion":1`)
	case <-c.ctx.Done():
		t.Fatal("fragmented request was not parsed")
	}
	require.Zero(t, m.creates)
}
