package agentnative

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/testutil"
)

type codexTestHost struct {
	mu       sync.Mutex
	sessions map[string]Session
	events   map[string][]Event
	fail     bool
}

func (h *codexTestHost) Session(id string) (Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[id]
	if !ok {
		return Session{}, xerrors.New("unregistered session")
	}
	s.State = append(json.RawMessage(nil), s.State...)
	return s, nil
}
func (h *codexTestHost) Publish(id string, events []Event, state json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.fail {
		return xerrors.New("test disk failure")
	}
	s, ok := h.sessions[id]
	if !ok {
		return xerrors.New("published before registration")
	}
	s.State = append(json.RawMessage(nil), state...)
	for _, event := range events {
		if event.Type == "mode" {
			s.Mode = event.Data["mode"].(string)
		}
	}
	h.sessions[id] = s
	data, _ := json.Marshal(events)
	var copied []Event
	_ = json.Unmarshal(data, &copied)
	h.events[id] = append(h.events[id], copied...)
	return nil
}
func (h *codexTestHost) register(s Session) { h.mu.Lock(); defer h.mu.Unlock(); h.sessions[s.ID] = s }
func (h *codexTestHost) list(id string) []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Event(nil), h.events[id]...)
}

type codexTestCall struct {
	method string
	params map[string]any
}
type codexTestRPC struct {
	t            *testing.T
	mu           sync.Mutex
	calls        []codexTestCall
	onMessage    func(codexMessage)
	onDisconnect func()
	handler      func(string, map[string]any) (any, error)
	closed       bool
}

func (r *codexTestRPC) call(_ context.Context, method string, params, out any) error {
	data, err := json.Marshal(params)
	require.NoError(r.t, err)
	var decoded map[string]any
	require.NoError(r.t, json.Unmarshal(data, &decoded))
	r.mu.Lock()
	r.calls = append(r.calls, codexTestCall{method, decoded})
	r.mu.Unlock()
	result, err := r.handler(method, decoded)
	if err != nil {
		return err
	}
	data, err = json.Marshal(result)
	require.NoError(r.t, err)
	return json.Unmarshal(data, out)
}
func (r *codexTestRPC) notify(_ context.Context, method string) error {
	require.Equal(r.t, "initialized", method)
	return nil
}
func (r *codexTestRPC) close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.onDisconnect()
	return nil
}
func (r *codexTestRPC) emit(method string, params any) {
	data, err := json.Marshal(params)
	require.NoError(r.t, err)
	r.onMessage(codexMessage{Method: method, Params: data})
}
func (r *codexTestRPC) methods() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var names []string
	for _, call := range r.calls {
		names = append(names, call.method)
	}
	return names
}

type codexTestFixture struct {
	codex   *Codex
	host    *codexTestHost
	rpc     *codexTestRPC
	session Session
	clock   *quartz.Mock
}

func newCodexTest(t *testing.T) (*Codex, *codexTestHost, *codexTestRPC, Session, *quartz.Mock) {
	t.Helper()
	f := newCodexFixture(t)
	return f.codex, f.host, f.rpc, f.session, f.clock
}

func newCodexFixture(t *testing.T) codexTestFixture {
	t.Helper()
	clock := quartz.NewMock(t)
	host := &codexTestHost{sessions: make(map[string]Session), events: make(map[string][]Event)}
	stateDir := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.Mkdir(stateDir, 0o700))
	c, err := NewCodex(BackendOptions{StateDir: stateDir, Host: host, Clock: clock, Command: func(context.Context, string, ...string) *exec.Cmd {
		t.Error("unexpected CLI invocation")
		return exec.Command("false")
	}})
	require.NoError(t, err)
	s := Session{ID: "codex:" + uuid.NewString(), Harness: "codex", Cwd: t.TempDir(), Mode: "human"}
	s.NativeID = s.ID[len("codex:"):]
	s.State, err = json.Marshal(codexState{Version: codexVersion, Requests: map[string]*codexReceipt{}})
	require.NoError(t, err)
	rpc := &codexTestRPC{t: t}
	rpc.handler = func(method string, _ map[string]any) (any, error) {
		switch method {
		case "initialize":
			return map[string]any{"userAgent": "codex/0.164.0", "platformFamily": "unix", "codexHome": t.TempDir()}, nil
		case "thread/start", "thread/read":
			return map[string]any{"thread": codexTestThread(s, "idle")}, nil
		case "thread/resume":
			return map[string]any{"thread": codexTestThread(s, "idle"), "initialTurnsPage": map[string]any{"data": []any{}}}, nil
		default:
			return nil, xerrors.New("unexpected test RPC: " + method)
		}
	}
	c.dial = func(_ context.Context, onMessage func(codexMessage), onDisconnect func()) (codexRPC, error) {
		rpc.onMessage = onMessage
		rpc.onDisconnect = onDisconnect
		return rpc, nil
	}
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	return codexTestFixture{codex: c, host: host, rpc: rpc, session: s, clock: clock}
}
func codexTestThread(s Session, status string) any {
	return map[string]any{"id": s.NativeID, "cwd": s.Cwd, "status": map[string]any{"type": status}}
}
func codexTestUser(clientID string) any {
	return map[string]any{"id": uuid.NewString(), "type": "userMessage", "clientId": clientID, "content": []any{map[string]any{"type": "text", "text": "hello"}}}
}
func codexTestTurn(id, status string, items ...any) any {
	if items == nil {
		items = []any{}
	}
	return map[string]any{"id": id, "status": status, "items": items}
}
func codexTestLive(t *testing.T, c *Codex, host *codexTestHost, s Session) {
	t.Helper()
	host.register(s)
	require.NoError(t, c.Resume(t.Context(), s))
	require.NoError(t, c.SetMode(t.Context(), s, "automation"))
}

func TestCodexEarlyCreateEvents(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, _ := newCodexTest(t)
	base := rpc.handler
	turnID := uuid.NewString()
	rpc.handler = func(method string, params map[string]any) (any, error) {
		if method == "thread/start" {
			rpc.emit("turn/started", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(turnID, "inProgress", codexTestUser("human"))})
		}
		return base(method, params)
	}
	created, err := c.Create(t.Context(), s.Cwd)
	require.NoError(t, err)
	require.Equal(t, s.ID, created.ID)
	require.Empty(t, host.list(s.ID))
	host.register(created)
	require.NoError(t, c.Resume(t.Context(), created))
	var starts []Event
	for _, event := range host.list(s.ID) {
		if event.Type == "turn.start" {
			starts = append(starts, event)
		}
	}
	require.Len(t, starts, 1)
	require.Equal(t, turnID, starts[0].Data["turnId"])
	require.NotContains(t, starts[0].Data, "requestId")
}

func TestCodexPromptEarlyCompletion(t *testing.T) {
	t.Parallel()
	for _, lateIdentity := range []bool{false, true} {
		t.Run(map[bool]string{false: "identity-in-completion", true: "completion-before-identity"}[lateIdentity], func(t *testing.T) {
			t.Parallel()
			c, host, rpc, s, clock := newCodexTest(t)
			codexTestLive(t, c, host, s)
			base := rpc.handler
			prompt := Prompt{ID: uuid.NewString(), Text: "hello"}
			turnID := uuid.NewString()
			rpc.handler = func(method string, params map[string]any) (any, error) {
				if method != "turn/start" {
					return base(method, params)
				}
				require.Equal(t, s.Cwd, params["cwd"], "native turns must retain the verified working directory")
				saved, err := host.Session(s.ID)
				require.NoError(t, err)
				var state codexState
				require.NoError(t, json.Unmarshal(saved.State, &state))
				require.Equal(t, params["clientUserMessageId"], state.Requests[prompt.ID].ClientID)
				require.Equal(t, "queued", state.Requests[prompt.ID].Status)
				user := codexTestUser(params["clientUserMessageId"].(string))
				rpc.emit("item/agentMessage/delta", map[string]any{"threadId": s.NativeID, "turnId": turnID, "itemId": "answer", "delta": "answer"})
				items := []any{user}
				if lateIdentity {
					items = nil
				}
				rpc.emit("turn/completed", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(turnID, "completed", items...)})
				if lateIdentity {
					rpc.emit("turn/started", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(turnID, "inProgress", user)})
				}
				return map[string]any{"turn": codexTestTurn(turnID, "inProgress")}, nil
			}
			require.NoError(t, c.Prompt(t.Context(), s, prompt))
			var events []Event
			for _, event := range host.list(s.ID) {
				if event.Type == "turn.start" || event.Type == "text" || event.Type == "turn.complete" {
					events = append(events, event)
					require.Equal(t, prompt.ID, event.Data["requestId"])
				}
			}
			require.Len(t, events, 3)
			require.Equal(t, "turn.start", events[0].Type)
			require.Equal(t, "text", events[1].Type)
			require.Equal(t, "turn.complete", events[2].Type)
			runtime := c.sessions[s.NativeID]
			require.Empty(t, runtime.active)
			require.Equal(t, "idle", runtime.status)
			require.Equal(t, "completed", runtime.state.Requests[prompt.ID].Status)
			_, pending := clock.Peek()
			require.False(t, pending, "completed turn must not retain an ownership timer")
			require.NoError(t, c.Cancel(t.Context(), s, prompt.ID))
			require.NotContains(t, rpc.methods(), "turn/interrupt")
		})
	}
}

func TestCodexResponseDoesNotGrantOwnership(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, clock := newCodexTest(t)
	codexTestLive(t, c, host, s)
	base := rpc.handler
	rpc.handler = func(method string, params map[string]any) (any, error) {
		if method == "turn/start" {
			return map[string]any{"turn": codexTestTurn(uuid.NewString(), "inProgress")}, nil
		}
		return base(method, params)
	}
	prompt := Prompt{ID: uuid.NewString(), Text: "hello"}
	require.NoError(t, c.Prompt(t.Context(), s, prompt))
	require.NoError(t, c.Cancel(t.Context(), s, prompt.ID))
	clock.Advance(10 * time.Second).MustWait(t.Context())
	require.True(t, c.sessions[s.NativeID].fault)
	require.Equal(t, "unknown", c.sessions[s.NativeID].state.Requests[prompt.ID].Status)
	require.NotContains(t, rpc.methods(), "turn/interrupt")
	for _, event := range host.list(s.ID) {
		require.NotEqual(t, "turn.start", event.Type)
	}
}

func TestCodexCancelExactOwnedTurn(t *testing.T) {
	t.Parallel()
	for _, human := range []bool{false, true} {
		t.Run(map[bool]string{false: "owned", true: "human-takes-control"}[human], func(t *testing.T) {
			t.Parallel()
			c, host, rpc, s, _ := newCodexTest(t)
			codexTestLive(t, c, host, s)
			base := rpc.handler
			turnID := uuid.NewString()
			rpc.handler = func(method string, params map[string]any) (any, error) {
				switch method {
				case "turn/start":
					turn := codexTestTurn(turnID, "inProgress", codexTestUser(params["clientUserMessageId"].(string)))
					rpc.emit("turn/started", map[string]any{"threadId": s.NativeID, "turn": turn})
					return map[string]any{"turn": turn}, nil
				case "turn/interrupt":
					require.Equal(t, map[string]any{"threadId": s.NativeID, "turnId": turnID}, params)
					return map[string]any{}, nil
				default:
					return base(method, params)
				}
			}
			prompt := Prompt{ID: uuid.NewString(), Text: "hello"}
			require.NoError(t, c.Prompt(t.Context(), s, prompt))
			require.Error(t, c.Cancel(t.Context(), s, uuid.NewString()))
			if human {
				rpc.emit("item/started", map[string]any{"threadId": s.NativeID, "turnId": turnID, "item": codexTestUser("native-human")})
				require.ErrorIs(t, c.Cancel(t.Context(), s, prompt.ID), ErrUnsupported)
				require.NotContains(t, rpc.methods(), "turn/interrupt")
				saved, err := host.Session(s.ID)
				require.NoError(t, err)
				require.Equal(t, "human", saved.Mode)
			} else {
				require.NoError(t, c.Cancel(t.Context(), s, prompt.ID))
				require.NoError(t, c.Cancel(t.Context(), s, prompt.ID))
				c.tasks.Wait()
				count := 0
				for _, method := range rpc.methods() {
					if method == "turn/interrupt" {
						count++
					}
				}
				require.Equal(t, 1, count)
			}
		})
	}
}

func TestCodexCancelWaitsForOwnedRoot(t *testing.T) {
	t.Parallel()
	for _, completeBeforeIdentity := range []bool{false, true} {
		t.Run(map[bool]string{false: "interrupt-after-proof", true: "already-complete-before-proof"}[completeBeforeIdentity], func(t *testing.T) {
			t.Parallel()
			ctx := codexTestContext(t)
			c, host, rpc, s, _ := newCodexTest(t)
			codexTestLive(t, c, host, s)
			base := rpc.handler
			turnID := uuid.NewString()
			started := make(chan string, 1)
			release := make(chan struct{})
			interrupted := make(chan map[string]any, 2)
			rpc.handler = func(method string, params map[string]any) (any, error) {
				switch method {
				case "turn/start":
					started <- params["clientUserMessageId"].(string)
					select {
					case <-release:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					return map[string]any{"turn": codexTestTurn(turnID, "inProgress")}, nil
				case "turn/interrupt":
					interrupted <- params
					return map[string]any{}, nil
				default:
					return base(method, params)
				}
			}
			prompt := Prompt{ID: uuid.NewString(), Text: "hello"}
			done := make(chan error, 1)
			go func() { done <- c.Prompt(ctx, s, prompt) }()
			var clientID string
			select {
			case clientID = <-started:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.NoError(t, c.Cancel(ctx, s, prompt.ID))
			require.NoError(t, c.Cancel(ctx, s, prompt.ID))
			snapshot, err := host.Session(s.ID)
			require.NoError(t, err)
			var saved codexState
			require.NoError(t, json.Unmarshal(snapshot.State, &saved))
			require.True(t, saved.Requests[prompt.ID].CancelRequested)
			require.False(t, saved.Requests[prompt.ID].CancelSent)
			rpc.emit("turn/started", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(turnID, "inProgress")})
			// A matching client ID on a different native thread grants no control.
			rpc.emit("turn/started", map[string]any{"threadId": uuid.NewString(), "turn": codexTestTurn("child", "inProgress", codexTestUser(clientID))})
			require.NotContains(t, rpc.methods(), "turn/interrupt")
			if completeBeforeIdentity {
				rpc.emit("turn/completed", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(turnID, "completed")})
			}
			rpc.emit("item/started", map[string]any{"threadId": s.NativeID, "turnId": turnID, "item": codexTestUser(clientID)})
			if !completeBeforeIdentity {
				select {
				case params := <-interrupted:
					require.Equal(t, map[string]any{"threadId": s.NativeID, "turnId": turnID}, params)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				rpc.emit("turn/completed", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(turnID, "interrupted")})
			}
			// Both native completion and cancellation precede the start RPC response.
			close(release)
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.NoError(t, c.Cancel(ctx, s, prompt.ID))
			c.tasks.Wait()
			require.Empty(t, interrupted, "there must be no second or unowned interrupt")
			c.mu.Lock()
			runtime := c.sessions[s.NativeID]
			require.Empty(t, runtime.active)
			require.False(t, runtime.fault)
			require.Equal(t, "completed", runtime.state.Requests[prompt.ID].Status)
			c.mu.Unlock()
			for _, event := range host.list(s.ID) {
				if event.Type == "turn.start" || event.Type == "turn.complete" {
					require.Equal(t, prompt.ID, event.Data["requestId"])
				}
			}
		})
	}
}

func TestCodexColdResumeChecks(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"queue", "queue-cursor", "goal", "goal-error", "missing-goal", "disabled", "complete", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			c, host, rpc, s, _ := newCodexTest(t)
			host.register(s)
			base := rpc.handler
			rpc.handler = func(method string, params map[string]any) (any, error) {
				switch method {
				case "thread/read":
					return map[string]any{"thread": codexTestThread(s, "notLoaded")}, nil
				case "thread/queue/list":
					require.Equal(t, float64(1), params["limit"])
					if scenario == "queue" {
						return map[string]any{"data": []any{"work"}, "nextCursor": nil}, nil
					}
					if scenario == "queue-cursor" {
						return map[string]any{"data": []any{}, "nextCursor": "next"}, nil
					}
					return map[string]any{"data": []any{}, "nextCursor": nil}, nil
				case "thread/goal/get":
					switch scenario {
					case "goal":
						return map[string]any{"goal": map[string]any{"status": "active"}}, nil
					case "goal-error":
						return nil, &codexRPCError{}
					case "missing-goal":
						return map[string]any{}, nil
					case "disabled":
						return nil, &codexRPCError{rejected: true, goalsDisabled: true}
					case "complete":
						return map[string]any{"goal": map[string]any{"status": "complete"}}, nil
					default:
						return map[string]any{"goal": nil}, nil
					}
				case "thread/resume":
					require.Equal(t, map[string]any{"threadId": s.NativeID, "excludeTurns": true, "initialTurnsPage": map[string]any{"limit": float64(1), "itemsView": "summary", "sortDirection": "desc"}}, params)
				}
				return base(method, params)
			}
			err := c.Resume(t.Context(), s)
			if scenario == "disabled" || scenario == "complete" || scenario == "empty" {
				require.NoError(t, err)
				require.Contains(t, rpc.methods(), "thread/resume")
			} else {
				require.Error(t, err)
				require.NotContains(t, rpc.methods(), "thread/resume")
			}
			require.NotContains(t, rpc.methods(), "turn/start")
		})
	}
}

func TestCodexResumeDoesNotReplayHistory(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, _ := newCodexTest(t)
	host.register(s)
	base := rpc.handler
	rpc.handler = func(method string, params map[string]any) (any, error) {
		if method == "thread/resume" {
			return map[string]any{"thread": codexTestThread(s, "idle"), "initialTurnsPage": map[string]any{"data": []any{codexTestTurn(uuid.NewString(), "completed", codexTestUser("human"), map[string]any{"type": "agentMessage", "id": "old", "text": "must not replay"})}}}, nil
		}
		return base(method, params)
	}
	require.NoError(t, c.Resume(t.Context(), s))
	for _, event := range host.list(s.ID) {
		require.Equal(t, "mode", event.Type)
	}
}

func TestCodexPersistenceFailureBlocksDispatch(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, _ := newCodexTest(t)
	codexTestLive(t, c, host, s)
	host.mu.Lock()
	host.fail = true
	host.mu.Unlock()
	require.Error(t, c.Prompt(t.Context(), s, Prompt{ID: uuid.NewString(), Text: "hello"}))
	require.NotContains(t, rpc.methods(), "turn/start")
}

func TestCodexCloseIsPassive(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, _ := newCodexTest(t)
	codexTestLive(t, c, host, s)
	before := rpc.methods()
	require.NoError(t, c.Close())
	require.NoError(t, c.Close())
	require.Equal(t, before, rpc.methods())
	require.True(t, rpc.closed)
	for _, event := range host.list(s.ID) {
		require.Equal(t, "mode", event.Type, "an intentional idle close is not an observation failure")
	}
	require.False(t, c.sessions[s.NativeID].fault)
	require.False(t, c.sessions[s.NativeID].ready)
	saved, err := host.Session(s.ID)
	require.NoError(t, err)
	require.Equal(t, "human", saved.Mode)
	require.ErrorIs(t, c.Shutdown(t.Context()), errCodexOwnership, "a reused server must not be silently treated as stopped")
}

func TestCodexClosePreservesReceiptOutcomes(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"completed", "rejected", "queued", "running", "unknown"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			c, host, rpc, s, _ := newCodexTest(t)
			codexTestLive(t, c, host, s)
			requestID := uuid.NewString()
			receipt := codexReceipt{ClientID: uuid.NewString(), TextHash: strings.Repeat("a", 64), Status: status, CancelRequested: true, CancelSent: true}
			expected := receipt
			c.mu.Lock()
			c.sessions[s.NativeID].state.Requests[requestID] = &receipt
			c.mu.Unlock()
			before := rpc.methods()
			require.NoError(t, c.Close())
			require.Equal(t, before, rpc.methods(), "closing must not interrupt or redispatch native work")
			saved, err := host.Session(s.ID)
			require.NoError(t, err)
			var state codexState
			require.NoError(t, json.Unmarshal(saved.State, &state))
			want := status
			if status == "queued" || status == "running" {
				want = "unknown"
			}
			require.Equal(t, want, state.Requests[requestID].Status)
			expected.Status = want
			require.Equal(t, &expected, state.Requests[requestID], "preserve correlation and cancellation receipts")
			wantFault := want == "unknown"
			require.Equal(t, wantFault, c.sessions[s.NativeID].fault)
			var overflows int
			for _, event := range host.list(s.ID) {
				if event.Type == "overflow" {
					overflows++
				}
			}
			if wantFault {
				require.Equal(t, 1, overflows)
			} else {
				require.Zero(t, overflows)
			}
		})
	}
}

func TestCodexCloseRetainsUnfinishedNativeWorkFault(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"active", "unattributed-completion", "approval", "disconnected"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			c, host, rpc, s, _ := newCodexTest(t)
			codexTestLive(t, c, host, s)
			switch phase {
			case "active":
				rpc.emit("turn/started", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(uuid.NewString(), "inProgress", codexTestUser("human"))})
			case "unattributed-completion":
				rpc.emit("turn/completed", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(uuid.NewString(), "completed")})
			case "approval":
				params, err := json.Marshal(map[string]any{"threadId": s.NativeID})
				require.NoError(t, err)
				rpc.onMessage(codexMessage{ID: json.RawMessage(`"pending"`), Method: "mcpServer/elicitation/request", Params: params})
			case "disconnected":
				rpc.onDisconnect()
			}
			before := rpc.methods()
			require.NoError(t, c.Close())
			require.Equal(t, before, rpc.methods())
			require.True(t, c.sessions[s.NativeID].fault)
			var overflows int
			for _, event := range host.list(s.ID) {
				if event.Type == "overflow" {
					overflows++
				}
			}
			require.Equal(t, 1, overflows, "retain the failure exactly once")
		})
	}
}

func TestCodexAttachVerifiesWithoutResume(t *testing.T) {
	t.Parallel()
	c, _, rpc, s, _ := newCodexTest(t)
	command, err := c.Attach(t.Context(), s)
	require.NoError(t, err)
	require.Equal(t, []string{"resume", "--remote", "unix://" + filepath.Join(c.options.StateDir, "codex.sock"), s.NativeID}, command.Args)
	require.Equal(t, []string{"initialize", "thread/read"}, rpc.methods())
	base := rpc.handler
	rpc.handler = func(method string, params map[string]any) (any, error) {
		if method == "thread/read" {
			wrong := s
			wrong.Cwd = t.TempDir()
			return map[string]any{"thread": codexTestThread(wrong, "idle")}, nil
		}
		return base(method, params)
	}
	_, err = c.Attach(t.Context(), s)
	require.Error(t, err)
}

func TestCodexVersionMismatchDoesNotStartService(t *testing.T) {
	t.Parallel()
	c, _, rpc, s, _ := newCodexTest(t)
	rpc.handler = func(string, map[string]any) (any, error) {
		return map[string]any{"userAgent": "codex/0.165.0", "platformFamily": "unix", "codexHome": t.TempDir()}, nil
	}
	_, err := c.Create(t.Context(), s.Cwd)
	var mismatch *codexVersionError
	require.ErrorAs(t, err, &mismatch)
	require.Equal(t, []string{"initialize"}, rpc.methods())
}

func TestCodexPromptRefusalAndUnknownOutcome(t *testing.T) {
	t.Parallel()
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown", true: "rejected"}[rejected], func(t *testing.T) {
			t.Parallel()
			c, host, rpc, s, _ := newCodexTest(t)
			codexTestLive(t, c, host, s)
			base := rpc.handler
			rpc.handler = func(method string, params map[string]any) (any, error) {
				if method == "turn/start" {
					return nil, &codexRPCError{rejected: rejected}
				}
				return base(method, params)
			}
			prompt := Prompt{ID: uuid.NewString(), Text: "hello"}
			require.Error(t, c.Prompt(t.Context(), s, prompt))
			want := "unknown"
			if rejected {
				want = "rejected"
			}
			require.Equal(t, want, c.sessions[s.NativeID].state.Requests[prompt.ID].Status)
			require.Error(t, c.Prompt(t.Context(), s, prompt))
			starts := 0
			for _, method := range rpc.methods() {
				if method == "turn/start" {
					starts++
				}
			}
			require.Equal(t, 1, starts)
		})
	}
}

func TestCodexNativeNotificationProjection(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, _ := newCodexTest(t)
	codexTestLive(t, c, host, s)
	turnID := uuid.NewString()
	rpc.emit("turn/started", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(turnID, "inProgress", codexTestUser("human"))})
	rpc.emit("item/reasoning/textDelta", map[string]any{"threadId": s.NativeID, "turnId": turnID, "itemId": "reasoning", "delta": "thinking"})
	rpc.emit("item/agentMessage/delta", map[string]any{"threadId": s.NativeID, "turnId": turnID, "itemId": "answer", "delta": "answer"})
	rpc.emit("item/completed", map[string]any{"threadId": s.NativeID, "turnId": turnID, "item": map[string]any{"type": "agentMessage", "id": "answer", "text": "answer"}})
	rpc.emit("item/completed", map[string]any{"threadId": s.NativeID, "turnId": turnID, "item": map[string]any{"type": "commandExecution", "id": "tool", "command": "pwd", "aggregatedOutput": s.Cwd, "status": "completed"}})
	params, err := json.Marshal(map[string]any{"threadId": s.NativeID, "turnId": turnID})
	require.NoError(t, err)
	rpc.onMessage(codexMessage{ID: json.RawMessage(`"approval"`), Method: "item/commandExecution/requestApproval", Params: params})
	rpc.emit("serverRequest/resolved", map[string]any{"threadId": s.NativeID, "requestId": "approval"})
	rpc.emit("turn/completed", map[string]any{"threadId": s.NativeID, "turn": codexTestTurn(turnID, "interrupted")})
	var kinds []string
	for _, event := range host.list(s.ID) {
		if event.Type != "mode" {
			kinds = append(kinds, event.Type)
		}
		if event.Type == "turn.complete" {
			require.Equal(t, "aborted", event.Data["reason"], "native interrupted status must retain the internal cancellation reason")
		}
	}
	require.Equal(t, []string{"turn.start", "thought", "text", "tool.start", "tool.end", "needs.input", "needs.input", "turn.complete"}, kinds)
}

func TestCodexRestoredUnknownReceiptPreventsRedispatch(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, _ := newCodexTest(t)
	requestID := uuid.NewString()
	s.State, _ = json.Marshal(codexState{Version: codexVersion, Requests: map[string]*codexReceipt{requestID: {ClientID: uuid.NewString(), TextHash: strings.Repeat("a", 64), Status: "running"}}})
	codexTestLive(t, c, host, s)
	require.Error(t, c.Prompt(t.Context(), s, Prompt{ID: uuid.NewString(), Text: "do not repeat unknown work"}))
	require.NotContains(t, rpc.methods(), "turn/start")
}

func TestCodexSettingsCwdChangeRefusesAutomation(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, _ := newCodexTest(t)
	codexTestLive(t, c, host, s)
	rpc.emit("thread/settings/updated", map[string]any{"threadId": s.NativeID, "threadSettings": map[string]any{"cwd": t.TempDir()}})
	require.Error(t, c.Prompt(t.Context(), s, Prompt{ID: uuid.NewString(), Text: "must not run"}))
	require.Error(t, c.SetMode(t.Context(), s, "automation"))
	require.NotContains(t, rpc.methods(), "turn/start")
}

func TestCodexPromptRevalidatesCwd(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, _ := newCodexTest(t)
	codexTestLive(t, c, host, s)
	base := rpc.handler
	rpc.handler = func(method string, params map[string]any) (any, error) {
		if method == "thread/read" {
			other := s
			other.Cwd = t.TempDir()
			return map[string]any{"thread": codexTestThread(other, "idle")}, nil
		}
		return base(method, params)
	}
	require.Error(t, c.Prompt(t.Context(), s, Prompt{ID: uuid.NewString(), Text: "must not run"}))
	require.NotContains(t, rpc.methods(), "turn/start")
}

func TestCodexFaultedSessionCanReturnToVerifiedHumanControl(t *testing.T) {
	t.Parallel()
	c, host, rpc, s, _ := newCodexTest(t)
	codexTestLive(t, c, host, s)
	c.mu.Lock()
	c.fault(c.sessions[s.NativeID], "lost observation")
	c.mu.Unlock()
	require.NoError(t, c.SetMode(t.Context(), s, "human"))
	_, err := c.Attach(t.Context(), s)
	require.NoError(t, err)
	require.Error(t, c.SetMode(t.Context(), s, "automation"))
	require.True(t, c.sessions[s.NativeID].fault)
	require.NotContains(t, rpc.methods(), "turn/start")
}

func TestCodexColdResumeFencesObservedConcurrentWork(t *testing.T) {
	t.Parallel()
	for _, duringResume := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-dispatch", true: "after-dispatch"}[duringResume], func(t *testing.T) {
			t.Parallel()
			c, host, rpc, s, _ := newCodexTest(t)
			host.register(s)
			base := rpc.handler
			rpc.handler = func(method string, params map[string]any) (any, error) {
				switch method {
				case "thread/read":
					return map[string]any{"thread": codexTestThread(s, "notLoaded")}, nil
				case "thread/queue/list":
					return map[string]any{"data": []any{}, "nextCursor": nil}, nil
				case "thread/goal/get":
					if !duringResume {
						rpc.emit("thread/queue/changed", map[string]any{"threadId": s.NativeID})
					}
					return map[string]any{"goal": nil}, nil
				case "thread/resume":
					rpc.emit("thread/queue/changed", map[string]any{"threadId": s.NativeID})
				}
				return base(method, params)
			}
			require.Error(t, c.Resume(t.Context(), s))
			if !duringResume {
				require.NotContains(t, rpc.methods(), "thread/resume")
			}
			require.NotContains(t, rpc.methods(), "turn/interrupt", "never interrupt unowned work from the unavoidable native resume race")
		})
	}
}

func TestCodexColdResumeAllowsInactiveGoalSnapshot(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"thread/goal/cleared", "thread/goal/updated"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			c, host, rpc, s, _ := newCodexTest(t)
			host.register(s)
			base := rpc.handler
			rpc.handler = func(methodName string, params map[string]any) (any, error) {
				switch methodName {
				case "thread/read":
					return map[string]any{"thread": codexTestThread(s, "notLoaded")}, nil
				case "thread/queue/list":
					return map[string]any{"data": []any{}, "nextCursor": nil}, nil
				case "thread/goal/get":
					return map[string]any{"goal": nil}, nil
				case "thread/resume":
					rpc.emit(method, map[string]any{"threadId": s.NativeID, "goal": map[string]any{"status": "complete"}})
				}
				return base(methodName, params)
			}
			require.NoError(t, c.Resume(t.Context(), s))
			require.NotContains(t, rpc.methods(), "turn/start")
		})
	}
}

func codexTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)
	return ctx
}
