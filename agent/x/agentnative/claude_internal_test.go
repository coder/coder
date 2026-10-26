//go:build linux

package agentnative

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

type claudeTestHost struct {
	mu       sync.Mutex
	sessions map[string]Session
	events   map[string][]Event
	fail     bool
}

func claudeNewTestHost() *claudeTestHost {
	return &claudeTestHost{sessions: make(map[string]Session), events: make(map[string][]Event)}
}
func (h *claudeTestHost) Session(id string) (Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[id]
	if !ok {
		return Session{}, xerrors.New("unknown session")
	}
	s.State = slices.Clone(s.State)
	return s, nil
}
func (h *claudeTestHost) Publish(id string, events []Event, state json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.fail {
		return xerrors.New("disk unavailable")
	}
	s, ok := h.sessions[id]
	if !ok {
		return xerrors.New("manager has not registered session")
	}
	// Mirror the manager's journal budget so adapter tests cannot acknowledge
	// events which the production Host would refuse to persist.
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil || len(data)+32 > maxEventBytes {
			return xerrors.New("event exceeds journal budget")
		}
	}
	s.State = slices.Clone(state)
	h.sessions[id] = s
	for _, event := range events {
		event.Seq = uint64(len(h.events[id])) + 1
		h.events[id] = append(h.events[id], event)
	}
	return nil
}
func (h *claudeTestHost) add(s Session) { h.mu.Lock(); defer h.mu.Unlock(); h.sessions[s.ID] = s }
func (h *claudeTestHost) count(id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.events[id])
}

type claudeTestReply struct {
	Ack      uint64          `json:"ack"`
	Commands []claudeCommand `json:"commands"`
}

func claudePoll(t *testing.T, c *Claude, body claudeHeartbeat, status int) claudeTestReply {
	t.Helper()
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	require.NoError(t, encoder.Encode(body))
	recorder := httptest.NewRecorder()
	c.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/runtime", &data))
	require.Equal(t, status, recorder.Code, recorder.Body.String())
	var reply claudeTestReply
	if status == http.StatusOK {
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &reply))
	}
	return reply
}

func claudeTestOptions(t *testing.T, host Host, clock quartz.Clock) BackendOptions {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(home, 0o700))
	return BackendOptions{StateDir: filepath.Join(base, "state"), Environment: []string{"HOME=" + home, "PATH=/usr/bin:/bin"}, Host: host, Clock: clock, CallbackURL: "http://127.0.0.1:32101", CallbackToken: uuid.NewString()}
}

func claudeTestLive(t *testing.T) (*Claude, *claudeTestHost, *quartz.Mock, Session, claudeHeartbeat) {
	t.Helper()
	clock := quartz.NewMock(t)
	host := claudeNewTestHost()
	options := claudeTestOptions(t, host, clock)
	var c *Claude
	body := claudeHeartbeat{SessionID: uuid.NewString(), RuntimeID: uuid.NewString(), Cwd: claudeEnv(options.Environment, "HOME"), Mode: "automation", Events: []Event{{Seq: 1, Type: "session.start", Data: map[string]any{}}}}
	options.Command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		require.Equal(t, "claude", name)
		require.Equal(t, "--bg", args[0])
		require.NotContains(t, args, "--resume")
		var settings struct {
			Env map[string]string `json:"env"`
		}
		require.NoError(t, json.Unmarshal([]byte(args[slices.Index(args, "--settings")+1]), &settings))
		body.LaunchID = settings.Env["WORKSPACE_BRIDGE_LAUNCH_ID"]
		require.Equal(t, c.descriptor, settings.Env["WORKSPACE_BRIDGE_FILE"])
		reply := claudePoll(t, c, body, http.StatusOK)
		require.Zero(t, reply.Ack)
		return exec.CommandContext(ctx, "/bin/true")
	}
	var err error
	c, err = NewClaude(options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	session, err := c.Create(testutil.Context(t, testutil.WaitLong), body.Cwd)
	require.NoError(t, err)
	require.Equal(t, body.SessionID, session.ID)
	require.Equal(t, body.SessionID, session.NativeID)
	require.Zero(t, host.count(session.ID))
	host.add(session)
	require.NoError(t, c.Resume(testutil.Context(t, testutil.WaitLong), session))
	reply := claudePoll(t, c, body, http.StatusOK)
	require.EqualValues(t, 1, reply.Ack)
	body.Events = nil
	return c, host, clock, session, body
}

func claudeTestWait(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-testutil.Context(t, testutil.WaitLong).Done():
		t.Fatal("Claude operation did not finish")
		return nil
	}
}

func claudeTestCommand(t *testing.T, c *Claude, clock *quartz.Mock, body claudeHeartbeat, operation func() error) (<-chan error, claudeCommand) {
	t.Helper()
	trap := clock.Trap().NewTimer("claude", "wait")
	result := make(chan error, 1)
	go func() { result <- operation() }()
	trap.MustWait(testutil.Context(t, testutil.WaitLong)).MustRelease(testutil.Context(t, testutil.WaitLong))
	trap.Close()
	reply := claudePoll(t, c, body, http.StatusOK)
	require.Len(t, reply.Commands, 1)
	return result, reply.Commands[0]
}

func TestClaudeInstallReplacesExistingAssets(t *testing.T) {
	t.Parallel()
	host := claudeNewTestHost()
	options := claudeTestOptions(t, host, quartz.NewMock(t))
	options.Command = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("install launched a native process")
		return nil
	}
	c, err := NewClaude(options)
	require.NoError(t, err)
	require.NoError(t, c.installLocked())
	require.NoError(t, c.Close())

	paths := []string{"hooks/register.js", "hooks/hooks.json", ".claude-plugin/plugin.json"}
	for _, path := range paths {
		require.NoError(t, os.WriteFile(filepath.Join(c.plugin, path), []byte("stale asset\n"), 0o600))
	}
	// A restarted observer must extract its own embedded bytes, including over
	// an existing installation. Installation itself must not touch native jobs.
	c, err = NewClaude(options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	require.NoError(t, c.installLocked())
	for _, path := range paths {
		want, err := claudeAssets.ReadFile("claudemod/" + path)
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(c.plugin, path))
		require.NoError(t, err)
		require.Equal(t, want, got, path)
	}
}

func TestClaudeLazyCreateAndAtomicHeartbeat(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	host := claudeNewTestHost()
	options := claudeTestOptions(t, host, clock)
	options.Command = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("constructor launched a native process")
		return nil
	}
	c, err := NewClaude(options)
	require.NoError(t, err)
	require.NoError(t, c.Close())

	c, host, _, session, body := claudeTestLive(t)
	count := host.count(session.ID)
	body.Events = []Event{{Seq: 2, Type: "text", Data: map[string]any{"text": "native output"}}}
	host.mu.Lock()
	host.fail = true
	host.mu.Unlock()
	claudePoll(t, c, body, http.StatusInternalServerError)
	require.Equal(t, count, host.count(session.ID))
	host.mu.Lock()
	host.fail = false
	host.mu.Unlock()
	reply := claudePoll(t, c, body, http.StatusOK)
	require.EqualValues(t, 2, reply.Ack)
	require.Equal(t, count+1, host.count(session.ID))
	claudePoll(t, c, body, http.StatusOK)
	require.Equal(t, count+1, host.count(session.ID), "duplicate native sequence must not emit twice")
	body.Events = []Event{{Seq: 4, Type: "text", Data: map[string]any{"text": "missing sequence"}}}
	claudePoll(t, c, body, http.StatusConflict)
	saved, err := host.Session(session.ID)
	require.NoError(t, err)
	var state claudeState
	require.NoError(t, json.Unmarshal(saved.State, &state))
	require.EqualValues(t, 2, state.Seq)
	require.NotEmpty(t, state.Fault)
}

func TestClaudeHeartbeatEventBudgets(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name  string
		kind  string
		text  string
		fault bool
	}{
		{name: "ampersands", kind: "text", text: strings.Repeat("&", 25000)},
		{name: "html-thought", kind: "thought", text: strings.Repeat("<>&", 12000)},
		{name: "unicode-boundary", kind: "text", text: strings.Repeat("&日本語", 10000)},
		{name: "oversized-control", kind: "tool.end", text: strings.Repeat("&", 25000), fault: true},
		{name: "oversized-wire", kind: "text", text: strings.Repeat("x", claudeEventLimit+1), fault: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			c, host, _, session, body := claudeTestLive(t)
			count := host.count(session.ID)
			body.Events = []Event{{Seq: 2, Type: scenario.kind, Data: map[string]any{"text": scenario.text, "requestId": "owned-request", "turnId": "owned-root"}}}
			host.mu.Lock()
			host.fail = true
			host.mu.Unlock()
			claudePoll(t, c, body, http.StatusInternalServerError)
			require.Equal(t, count, host.count(session.ID))
			host.mu.Lock()
			host.fail = false
			host.mu.Unlock()
			require.EqualValues(t, 2, claudePoll(t, c, body, http.StatusOK).Ack)
			host.mu.Lock()
			events := slices.Clone(host.events[session.ID][count:])
			host.mu.Unlock()
			saved, err := host.Session(session.ID)
			require.NoError(t, err)
			var state claudeState
			require.NoError(t, json.Unmarshal(saved.State, &state))
			if scenario.fault {
				require.Len(t, events, 1)
				require.Equal(t, "overflow", events[0].Type)
				require.NotEmpty(t, state.Fault)
				require.Equal(t, "human", state.Mode)
			} else {
				require.Empty(t, state.Fault)
				require.Greater(t, len(events), 1)
				var text strings.Builder
				for _, event := range events {
					require.Equal(t, scenario.kind, event.Type)
					require.Equal(t, "owned-request", event.Data["requestId"])
					require.Equal(t, "owned-root", event.Data["turnId"])
					text.WriteString(event.Data["text"].(string))
				}
				require.Equal(t, scenario.text, text.String())
			}
			count = host.count(session.ID)
			require.EqualValues(t, 2, claudePoll(t, c, body, http.StatusOK).Ack)
			require.Equal(t, count, host.count(session.ID), "chunks and faults must be acknowledged atomically and deduplicated")
		})
	}
}

func TestClaudePermissionResolutionClearsManagerInput(t *testing.T) {
	t.Parallel()
	c, host, _, session, body := claudeTestLive(t)
	body.Events = []Event{
		{Seq: 2, Type: "needs.input", Data: map[string]any{"kind": "permission", "pending": true, "turnId": "root"}},
		{Seq: 3, Type: "needs.input", Data: map[string]any{"kind": "permission", "pending": false, "turnId": "root"}},
		{Seq: 4, Type: "turn.complete", Data: map[string]any{"turnId": "root"}},
	}
	require.EqualValues(t, 4, claudePoll(t, c, body, http.StatusOK).Ack)
	record := &sessionRecord{Session: session}
	host.mu.Lock()
	events := slices.Clone(host.events[session.ID])
	host.mu.Unlock()
	for _, event := range events {
		applyEvent(record, event)
		pending, _ := event.Data["pending"].(bool)
		if event.Type == "needs.input" && pending {
			require.True(t, record.Session.PendingInput)
		}
	}
	require.False(t, record.Session.PendingInput)
	applyEvent(record, Event{Type: "mode", Data: map[string]any{"mode": "automation"}})
	require.Equal(t, "automation", record.Session.Mode)
}

func TestClaudePromptAcknowledgementAndOwnedCancellation(t *testing.T) {
	t.Parallel()
	c, host, clock, session, body := claudeTestLive(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	prompt := Prompt{ID: uuid.NewString(), Text: "do native work"}
	result, command := claudeTestCommand(t, c, clock, body, func() error { return c.Prompt(ctx, session, prompt) })
	require.Equal(t, "prompt", command.Type)
	require.Equal(t, prompt.Text, command.Text)
	select {
	case <-result:
		t.Fatal("prompt returned before native ACK")
	default:
	}
	body.ActiveTurnID = "owned-root"
	body.Events = []Event{
		{Seq: 2, Type: "turn.start", Data: map[string]any{"requestId": prompt.ID, "turnId": "owned-root"}},
		{Seq: 3, Type: "prompt.accepted", Data: map[string]any{"requestId": prompt.ID}},
	}
	claudePoll(t, c, body, http.StatusOK)
	require.NoError(t, claudeTestWait(t, result))
	body.Events = nil
	require.NoError(t, c.Prompt(ctx, session, prompt))
	require.Error(t, c.Prompt(ctx, session, Prompt{ID: prompt.ID, Text: "different"}))
	require.Error(t, c.Cancel(ctx, session, uuid.NewString()))
	require.Empty(t, claudePoll(t, c, body, http.StatusOK).Commands)
	result, command = claudeTestCommand(t, c, clock, body, func() error { return c.Cancel(ctx, session, prompt.ID) })
	require.Equal(t, "cancel", command.Type)
	require.Equal(t, prompt.ID, command.RequestID)
	body.Events = []Event{{Seq: 4, Type: "command.done", Data: map[string]any{"commandId": command.ID}}}
	claudePoll(t, c, body, http.StatusOK)
	require.NoError(t, claudeTestWait(t, result))
	body.ActiveTurnID = ""
	body.Events = []Event{{Seq: 5, Type: "turn.complete", Data: map[string]any{"requestId": prompt.ID, "turnId": "other-root"}}}
	claudePoll(t, c, body, http.StatusOK)
	saved, err := host.Session(session.ID)
	require.NoError(t, err)
	var state claudeState
	require.NoError(t, json.Unmarshal(saved.State, &state))
	require.Equal(t, prompt.ID, state.OwnedRequestID)
	body.Events = []Event{{Seq: 6, Type: "turn.complete", Data: map[string]any{"requestId": prompt.ID, "turnId": "owned-root"}}}
	claudePoll(t, c, body, http.StatusOK)
	require.Error(t, c.Cancel(ctx, session, uuid.NewString()))
}

func TestClaudeCompletionBeforeReceiptIsAtomic(t *testing.T) {
	t.Parallel()
	c, host, clock, session, body := claudeTestLive(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	prompt := Prompt{ID: uuid.NewString(), Text: "native work"}
	result, _ := claudeTestCommand(t, c, clock, body, func() error { return c.Prompt(ctx, session, prompt) })
	body.Events = []Event{
		{Seq: 2, Type: "turn.start", Data: map[string]any{"requestId": prompt.ID, "turnId": "root"}},
		{Seq: 3, Type: "text", Data: map[string]any{"requestId": prompt.ID, "turnId": "root", "text": "done"}},
		{Seq: 4, Type: "turn.complete", Data: map[string]any{"requestId": prompt.ID, "turnId": "root"}},
		{Seq: 5, Type: "prompt.accepted", Data: map[string]any{"requestId": prompt.ID}},
	}
	host.mu.Lock()
	host.fail = true
	host.mu.Unlock()
	claudePoll(t, c, body, http.StatusInternalServerError)
	select {
	case <-result:
		t.Fatal("receipt acknowledged before its held events were durable")
	default:
	}
	host.mu.Lock()
	host.fail = false
	host.mu.Unlock()
	require.EqualValues(t, 5, claudePoll(t, c, body, http.StatusOK).Ack)
	require.NoError(t, claudeTestWait(t, result))
	saved, err := host.Session(session.ID)
	require.NoError(t, err)
	var state claudeState
	require.NoError(t, json.Unmarshal(saved.State, &state))
	require.Equal(t, "completed", state.Receipts[prompt.ID].Result)
	require.Empty(t, state.OwnedRequestID)
	require.Empty(t, state.OwnedTurnID)
	count := host.count(session.ID)
	require.EqualValues(t, 5, claudePoll(t, c, body, http.StatusOK).Ack)
	require.Equal(t, count, host.count(session.ID))
	require.NoError(t, c.Prompt(ctx, session, prompt))
}

func TestClaudeCanceledPromptProtocol(t *testing.T) {
	t.Parallel()
	c, host, clock, session, body := claudeTestLive(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	prompt := Prompt{ID: uuid.NewString(), Text: "canceled before submission"}
	result, _ := claudeTestCommand(t, c, clock, body, func() error { return c.Prompt(ctx, session, prompt) })
	body.Events = []Event{{Seq: 2, Type: "prompt.rejected", Data: map[string]any{
		"requestId": prompt.ID,
		"cancelled": true, //nolint:misspell // Native mod protocol field.
	}}}
	require.EqualValues(t, 2, claudePoll(t, c, body, http.StatusOK).Ack)
	require.Error(t, claudeTestWait(t, result))
	host.mu.Lock()
	events := slices.Clone(host.events[session.ID])
	host.mu.Unlock()
	record := &sessionRecord{Session: session, Requests: map[string]requestRecord{
		prompt.ID: {ID: prompt.ID, Status: "attempted"},
	}}
	for _, event := range events {
		applyEvent(record, event)
	}
	require.Equal(t, "canceled", record.Requests[prompt.ID].Status)
}

func TestClaudeModeAcknowledgementAndStaleHeartbeat(t *testing.T) {
	t.Parallel()
	c, _, clock, session, body := claudeTestLive(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	result, command := claudeTestCommand(t, c, clock, body, func() error { return c.SetMode(ctx, session, "human") })
	require.Equal(t, "mode", command.Type)
	body.Mode = "human"
	body.Events = []Event{{Seq: 2, Type: "mode", Data: map[string]any{"mode": "human"}}, {Seq: 3, Type: "command.done", Data: map[string]any{"commandId": command.ID}}}
	claudePoll(t, c, body, http.StatusOK)
	require.NoError(t, claudeTestWait(t, result))
	clock.Advance(claudeStaleAfter + time.Second).MustWait(ctx)
	require.ErrorContains(t, c.SetMode(ctx, session, "automation"), "stale")
	body.Events = nil
	claudePoll(t, c, body, http.StatusOK)
	result, command = claudeTestCommand(t, c, clock, body, func() error { return c.SetMode(ctx, session, "automation") })
	body.Events = []Event{{Seq: 4, Type: "command.done", Data: map[string]any{"commandId": command.ID}}}
	claudePoll(t, c, body, http.StatusOK)
	require.ErrorContains(t, claudeTestWait(t, result), "not confirmed")
}

func TestClaudeCommandTimeoutDoesNotCancelNativeWork(t *testing.T) {
	t.Parallel()
	c, host, clock, session, body := claudeTestLive(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	result, _ := claudeTestCommand(t, c, clock, body, func() error {
		return c.Prompt(ctx, session, Prompt{ID: uuid.NewString(), Text: "continue independently"})
	})
	clock.Advance(claudeCommandTimeout).MustWait(ctx)
	require.ErrorContains(t, claudeTestWait(t, result), "unknown")
	require.Empty(t, claudePoll(t, c, body, http.StatusOK).Commands)
	saved, err := host.Session(session.ID)
	require.NoError(t, err)
	var state claudeState
	require.NoError(t, json.Unmarshal(saved.State, &state))
	require.NotEmpty(t, state.Fault)
	require.NotEmpty(t, state.OwnedRequestID)
}

func TestClaudeRestartRejoinsSameRuntimeWithoutReplay(t *testing.T) {
	t.Parallel()
	c, host, clock, session, body := claudeTestLive(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	prompt := Prompt{ID: uuid.NewString(), Text: "only once"}
	result, _ := claudeTestCommand(t, c, clock, body, func() error { return c.Prompt(ctx, session, prompt) })
	body.Events = []Event{{Seq: 2, Type: "prompt.accepted", Data: map[string]any{"requestId": prompt.ID}}}
	claudePoll(t, c, body, http.StatusOK)
	require.NoError(t, claudeTestWait(t, result))
	require.NoError(t, c.Close())
	options := c.options
	options.CallbackURL, options.CallbackToken = "http://127.0.0.1:32102", uuid.NewString()
	options.Command = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("live rejoin launched a native process")
		return nil
	}
	restarted, err := NewClaude(options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, restarted.Close()) })
	count := host.count(session.ID)
	// A new observer can validate registration from the durable host before Resume.
	reply := claudePoll(t, restarted, body, http.StatusOK)
	require.EqualValues(t, 2, reply.Ack)
	saved, err := host.Session(session.ID)
	require.NoError(t, err)
	require.NoError(t, restarted.Resume(ctx, saved))
	require.NoError(t, restarted.Prompt(ctx, saved, prompt))
	reply = claudePoll(t, restarted, body, http.StatusOK)
	require.Empty(t, reply.Commands)
	require.Equal(t, count+1, host.count(session.ID), "only the resumed marker is new")
	descriptor, err := os.ReadFile(restarted.descriptor)
	require.NoError(t, err)
	require.Contains(t, string(descriptor), options.CallbackToken)
	require.Contains(t, string(descriptor), options.CallbackURL)
	info, err := os.Stat(restarted.descriptor)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	require.FileExists(t, filepath.Join(restarted.plugin, ".claude-plugin", "plugin.json"))
}

type claudeNativeFixture struct {
	claude     *Claude
	host       *claudeTestHost
	session    Session
	state      map[string]any
	transcript string
}

func claudeTestNative(t *testing.T) claudeNativeFixture {
	t.Helper()
	host := claudeNewTestHost()
	options := claudeTestOptions(t, host, quartz.NewMock(t))
	c, err := NewClaude(options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	state := claudeState{LaunchID: uuid.NewString(), RuntimeID: uuid.NewString(), Mode: "human", Receipts: make(map[string]claudeReceipt)}
	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	id := uuid.NewString()
	session := Session{ID: id, NativeID: id, Harness: "claude", Cwd: claudeEnv(options.Environment, "HOME"), Mode: "human", State: encoded}
	host.add(session)
	settings, err := json.Marshal(map[string]any{"env": map[string]string{"WORKSPACE_BRIDGE_FILE": c.descriptor, "WORKSPACE_BRIDGE_LAUNCH_ID": state.LaunchID}})
	require.NoError(t, err)
	native := map[string]any{"sessionId": id, "cwd": session.Cwd, "state": "completed", "template": "bg", "respawnFlags": []string{"--plugin-dir", c.plugin, "--settings", string(settings)}}
	project := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, session.Cwd)
	transcript := filepath.Join(c.root, "projects", project, id+".jsonl")
	claudeTestWrite(t, filepath.Join(c.root, "jobs", id[:8], "state.json"), native)
	claudeTestWrite(t, transcript, map[string]any{"type": "user", "sessionId": id, "message": map[string]any{"content": "native history"}})
	return claudeNativeFixture{claude: c, host: host, session: session, state: native, transcript: transcript}
}

func claudeTestWrite(t *testing.T, path string, value any) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600))
}

func claudeTestAgentsCommand(t *testing.T, session Session, extra map[string]any) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()
	agent := map[string]any{"id": session.ID[:8], "kind": "background", "sessionId": session.ID, "cwd": session.Cwd, "state": "completed"}
	for key, value := range extra {
		agent[key] = value
	}
	data, err := json.Marshal([]any{agent})
	require.NoError(t, err)
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		require.Equal(t, "claude", name)
		require.Equal(t, []string{"agents", "--json", "--all"}, args, "must not launch native work during validation")
		// Only test-generated JSON is passed as data to a constant format.
		return exec.CommandContext(ctx, "/usr/bin/printf", "%s", string(data)) //nolint:gosec
	}
}

func TestClaudeNativeResumeSafety(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"valid", "missing-history", "metadata-only", "queued-empty", "queued-null", "in-flight", "wrong-plugin", "duplicate-settings", "live-completed", "symlink-history", "public-history"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			fixture := claudeTestNative(t)
			c, session, native, transcript := fixture.claude, fixture.session, fixture.state, fixture.transcript
			extra := map[string]any{}
			switch scenario {
			case "missing-history":
				require.NoError(t, os.Remove(transcript))
			case "metadata-only":
				claudeTestWrite(t, transcript, map[string]any{"type": "summary", "sessionId": session.ID})
			case "queued-empty":
				native["queuedPrompt"] = ""
			case "queued-null":
				native["queuedPrompt"] = nil
			case "in-flight":
				native["inFlight"] = map[string]any{"queued": 1}
			case "wrong-plugin":
				native["respawnFlags"].([]string)[1] = "/unrelated/plugin"
			case "duplicate-settings":
				native["respawnFlags"] = append(native["respawnFlags"].([]string), "--settings", "{}")
			case "live-completed":
				extra["pid"] = os.Getpid()
			case "symlink-history":
				require.NoError(t, os.Rename(transcript, transcript+".real"))
				require.NoError(t, os.Symlink(transcript+".real", transcript))
			case "public-history":
				require.NoError(t, os.Chmod(transcript, 0o644))
			}
			claudeTestWrite(t, filepath.Join(c.root, "jobs", session.ID[:8], "state.json"), native)
			c.options.Command = claudeTestAgentsCommand(t, session, extra)
			var state claudeState
			require.NoError(t, json.Unmarshal(session.State, &state))
			err := c.verifyResume(testutil.Context(t, testutil.WaitLong), session, state.LaunchID)
			switch scenario {
			case "valid":
				require.NoError(t, err)
			case "live-completed":
				require.ErrorIs(t, err, errClaudeLiveWorker)
			default:
				require.ErrorIs(t, err, ErrUnsupported)
				require.Contains(t, err.Error(), "original task was not rerun")
			}
		})
	}
}

func TestClaudeResumeUsesOnlyNativeUUID(t *testing.T) {
	t.Parallel()
	fixture := claudeTestNative(t)
	c, session := fixture.claude, fixture.session
	clock := c.clock.(*quartz.Mock)
	ctx := testutil.Context(t, testutil.WaitLong)
	agents := claudeTestAgentsCommand(t, session, nil)
	var state claudeState
	require.NoError(t, json.Unmarshal(session.State, &state))
	c.mu.Lock()
	runtime, err := c.loadLocked(session)
	require.NoError(t, err)
	oldPrompt := &claudePending{command: claudeCommand{ID: uuid.NewString(), Type: "prompt", Text: "never replay this task"}, done: make(chan struct{})}
	runtime.commands[oldPrompt.command.ID] = oldPrompt
	runtime.state.Receipts[oldPrompt.command.ID] = claudeReceipt{Kind: "prompt", Result: "pending"}
	c.mu.Unlock()
	body := claudeHeartbeat{SessionID: session.ID, RuntimeID: uuid.NewString(), LaunchID: state.LaunchID, Cwd: session.Cwd, Mode: "automation", Events: []Event{{Seq: 1, Type: "session.start", Data: map[string]any{}}}}
	launched := make(chan struct{})
	c.options.Command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if args[0] == "agents" {
			return agents(ctx, name, args...)
		}
		require.Equal(t, []string{"--bg", "--resume", session.ID}, args)
		claudePoll(t, c, body, http.StatusOK)
		close(launched)
		return exec.CommandContext(ctx, "/bin/true")
	}
	trap := clock.Trap().NewTimer("claude", "wait")
	result := make(chan error, 1)
	go func() { result <- c.Resume(ctx, session) }()
	select {
	case <-launched:
	case <-ctx.Done():
		t.Fatal("resume did not launch")
	}
	// Registration wait is already satisfied; the second wait is mode ACK.
	trap.MustWait(ctx).MustRelease(ctx)
	trap.MustWait(ctx).MustRelease(ctx)
	trap.Close()
	body.Events = nil
	reply := claudePoll(t, c, body, http.StatusOK)
	require.Len(t, reply.Commands, 1)
	require.Equal(t, "mode", reply.Commands[0].Type)
	require.Equal(t, "human", reply.Commands[0].Mode)
	body.Mode = "human"
	body.Events = []Event{{Seq: 2, Type: "mode", Data: map[string]any{"mode": "human"}}, {Seq: 3, Type: "command.done", Data: map[string]any{"commandId": reply.Commands[0].ID}}}
	claudePoll(t, c, body, http.StatusOK)
	require.NoError(t, claudeTestWait(t, result))
	require.NotEqual(t, state.RuntimeID, body.RuntimeID)
	select {
	case <-oldPrompt.done:
		require.ErrorContains(t, oldPrompt.result, "unknown")
	default:
		t.Fatal("old runtime command was not invalidated")
	}
	body.RuntimeID = state.RuntimeID
	claudePoll(t, c, body, http.StatusConflict)
}

func TestClaudeResumeWaitsForExistingRuntime(t *testing.T) {
	t.Parallel()
	fixture := claudeTestNative(t)
	c, host, session := fixture.claude, fixture.host, fixture.session
	clock := c.clock.(*quartz.Mock)
	ctx := testutil.Context(t, testutil.WaitLong)
	c.options.Command = claudeTestAgentsCommand(t, session, map[string]any{"pid": os.Getpid()})
	var state claudeState
	require.NoError(t, json.Unmarshal(session.State, &state))
	trap := clock.Trap().NewTimer("claude", "wait")
	result := make(chan error, 1)
	go func() { result <- c.Resume(ctx, session) }()
	trap.MustWait(ctx).MustRelease(ctx)
	trap.Close()
	body := claudeHeartbeat{SessionID: session.ID, RuntimeID: state.RuntimeID, LaunchID: state.LaunchID, Cwd: session.Cwd, Mode: "human", Events: []Event{{Seq: 1, Type: "session.start", Data: map[string]any{}}}}
	require.Zero(t, claudePoll(t, c, body, http.StatusOK).Ack)
	require.NoError(t, claudeTestWait(t, result))
	require.EqualValues(t, 1, claudePoll(t, c, body, http.StatusOK).Ack)
	saved, err := host.Session(session.ID)
	require.NoError(t, err)
	var resumed claudeState
	require.NoError(t, json.Unmarshal(saved.State, &resumed))
	require.Equal(t, state.RuntimeID, resumed.RuntimeID)
	require.Empty(t, resumed.Retired)
	require.False(t, resumed.ResumeAttempted)
}

func claudeTestNativeRegistry(t *testing.T, c *Claude, session Session, pid int, domain string) {
	t.Helper()
	claudeTestWrite(t, filepath.Join(c.root, "daemon.lock"), claudeNativeLock{PID: pid, Version: "2.1.309"})
	claudeTestWrite(t, filepath.Join(c.root, "daemon", "roster.json"), claudeNativeRoster{Proto: 1, SupervisorPID: pid, Workers: map[string]claudeNativeWorker{session.ID[:8]: {PID: pid, SessionID: session.ID, Cwd: session.Cwd, CLIVersion: "2.1.309"}}})
	claudeTestWrite(t, filepath.Join(c.root, "sessions", fmt.Sprint(pid)+".json"), claudeNativeSession{PID: pid, Version: "2.1.309", PIDDomain: domain, SessionID: session.ID, Kind: "bg", JobID: session.ID[:8], Cwd: session.Cwd})
}

func TestClaudeBlockedNativeResumeChecksProcessDomain(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"stopped", "live", "foreign-domain"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			fixture := claudeTestNative(t)
			c, session, native := fixture.claude, fixture.session, fixture.state
			native["state"], native["tempo"], native["backend"], native["cliVersion"] = "blocked", "blocked", "daemon", "2.1.309"
			claudeTestWrite(t, filepath.Join(c.root, "jobs", session.ID[:8], "state.json"), native)
			c.options.Command = claudeTestAgentsCommand(t, session, map[string]any{"state": "blocked"})
			process := exec.CommandContext(testutil.Context(t, testutil.WaitLong), "/bin/true")
			require.NoError(t, process.Run())
			pid := process.Process.Pid
			if scenario == "live" {
				pid = os.Getpid()
			}
			domain, err := claudePIDDomain()
			require.NoError(t, err)
			if scenario == "foreign-domain" {
				domain = "linux:foreign:pid:[123]"
			}
			claudeTestNativeRegistry(t, c, session, pid, domain)
			var state claudeState
			require.NoError(t, json.Unmarshal(session.State, &state))
			err = c.verifyResume(testutil.Context(t, testutil.WaitLong), session, state.LaunchID)
			if scenario == "stopped" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrUnsupported)
			}
		})
	}
}

func TestClaudeShutdownLeavesSharedSupervisorAndUnrelatedPTY(t *testing.T) {
	t.Parallel()
	fixture := claudeTestNative(t)
	c, session := fixture.claude, fixture.session
	ctx := testutil.Context(t, testutil.WaitLong)
	process := exec.CommandContext(ctx, "/bin/true")
	require.NoError(t, process.Run())
	pid := process.Process.Pid
	domain, err := claudePIDDomain()
	require.NoError(t, err)
	claudeTestNativeRegistry(t, c, session, pid, domain)
	// A user-started supervisor can have the bridge as its only roster worker
	// while still owning unrelated PTYs. Neither belongs to the adapter.
	claudeTestWrite(t, filepath.Join(c.root, "daemon.lock"), claudeNativeLock{PID: os.Getpid(), Version: "2.1.309"})
	claudeTestWrite(t, filepath.Join(c.root, "daemon", "roster.json"), claudeNativeRoster{Proto: 1, SupervisorPID: os.Getpid(), Workers: map[string]claudeNativeWorker{session.ID[:8]: {PID: pid, SessionID: session.ID, Cwd: session.Cwd, CLIVersion: "2.1.309"}}})
	foreign := filepath.Join(c.root, "sessions", fmt.Sprint(os.Getpid())+".json")
	claudeTestWrite(t, foreign, claudeNativeSession{PID: os.Getpid(), Version: "2.1.309", PIDDomain: domain, SessionID: uuid.NewString(), Kind: "foreground", Cwd: session.Cwd})
	called := false
	c.options.Command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		called = true
		require.Equal(t, "claude", name)
		require.Equal(t, []string{"stop", session.NativeID[:8]}, args)
		require.NoError(t, os.Remove(filepath.Join(c.root, "sessions", fmt.Sprint(pid)+".json")))
		claudeTestWrite(t, filepath.Join(c.root, "daemon", "roster.json"), claudeNativeRoster{Proto: 1, SupervisorPID: os.Getpid(), Workers: map[string]claudeNativeWorker{}})
		return exec.CommandContext(ctx, "/bin/true")
	}
	require.NoError(t, c.Close())
	require.False(t, called)
	require.NoError(t, c.Shutdown(ctx))
	require.True(t, called)
	require.FileExists(t, filepath.Join(c.root, "daemon.lock"))
	require.FileExists(t, foreign)
}

func TestClaudeShutdownVerifiesWorkerStartIdentity(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"valid", "missing-start", "wrong-start", "missing-record", "wrong-record", "missing-repl"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			start, err := claudeProcessStart(os.Getpid())
			require.NoError(t, err)
			id := uuid.NewString()
			worker := claudeNativeWorker{PID: os.Getpid(), ProcStart: start, SessionID: id, Cwd: t.TempDir()}
			record := claudeNativeSession{PID: os.Getpid(), ProcStart: start, SessionID: id, Cwd: worker.Cwd, Kind: "bg", JobID: id[:8]}
			switch scenario {
			case "missing-start":
				worker.ProcStart = ""
			case "wrong-start":
				worker.ProcStart = "not-the-live-process"
			case "wrong-record":
				record.ProcStart = "not-the-live-process"
			case "missing-repl":
				worker.ReplPID = new(os.Getpid() + 1)
			}
			snapshot := &claudeProcessSnapshot{workers: []claudeNativeWorker{worker}, sessions: []claudeNativeSession{record}}
			if scenario == "missing-record" {
				snapshot.sessions = nil
			}
			pids, err := claudeShutdownPIDs(snapshot, worker)
			if scenario == "valid" {
				require.NoError(t, err)
				require.Equal(t, map[int]bool{os.Getpid(): true}, pids)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestClaudeShutdownRefusesUnverifiedOwnedWorker(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"missing-start", "foreign-domain", "wrong-plugin"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			fixture := claudeTestNative(t)
			c, session, native := fixture.claude, fixture.session, fixture.state
			domain, err := claudePIDDomain()
			require.NoError(t, err)
			if scenario == "foreign-domain" {
				domain = "linux:foreign:pid:[123]"
			}
			if scenario == "wrong-plugin" {
				native["respawnFlags"].([]string)[1] = "/another/plugin"
				claudeTestWrite(t, filepath.Join(c.root, "jobs", session.ID[:8], "state.json"), native)
			}
			claudeTestNativeRegistry(t, c, session, os.Getpid(), domain)
			c.options.Command = func(context.Context, string, ...string) *exec.Cmd {
				t.Fatal("unverified worker must not be stopped")
				return nil
			}
			require.ErrorIs(t, c.Shutdown(testutil.Context(t, testutil.WaitLong)), ErrUnsupported)
		})
	}
}

func TestClaudeShutdownRefusesUnrelatedDaemon(t *testing.T) {
	t.Parallel()
	c := claudeTestNative(t).claude
	c.options.Command = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("must not stop an unrelated native daemon")
		return nil
	}
	id := uuid.NewString()
	claudeTestWrite(t, filepath.Join(c.root, "daemon.lock"), claudeNativeLock{PID: os.Getpid(), Version: "2.1.309"})
	claudeTestWrite(t, filepath.Join(c.root, "daemon", "roster.json"), claudeNativeRoster{Proto: 1, SupervisorPID: os.Getpid(), Workers: map[string]claudeNativeWorker{id[:8]: {PID: os.Getpid(), SessionID: id, Cwd: claudeEnv(c.options.Environment, "HOME"), CLIVersion: "2.1.309"}}})
	require.NoError(t, c.Shutdown(testutil.Context(t, testutil.WaitLong)))
	require.FileExists(t, filepath.Join(c.root, "daemon.lock"))
	require.NoError(t, c.Close(), "closing an observer never touches the daemon")
}
