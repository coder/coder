package agentacp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	acp "github.com/coder/acp-go-sdk"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type failingWriter struct{ io.WriteCloser }

func (failingWriter) Write([]byte) (int, error) { return 0, xerrors.New("broken harness input") }

func TestMessageWriteFailureCanRetry(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	s, err := m.get(ctx, info.ID)
	require.NoError(t, err)
	s.mu.Lock()
	p := s.proc
	s.mu.Unlock()
	p.stdin = failingWriter{p.stdin}
	message := workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "retry after broken input"}
	_, err = m.Message(ctx, info.ID, message)
	require.ErrorContains(t, err, "broken harness input")
	_, err = m.Message(ctx, info.ID, message)
	require.NoError(t, err)
	result, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, "answer:"+message.Text+":other", result.AssistantResponse)
}

func TestMessageAdmissionWriteDeadline(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	m := NewManager(context.Background(), Options{Clock: clock, Filesystem: afero.NewOsFs()})
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	s := m.newSession(workspacesdk.ACPCreateSessionRequest{HarnessSlug: "blocked", WorkingDirectory: t.TempDir()}, uuid.NewString())
	s.info.Status = workspacesdk.ACPSessionStatusIdle
	close(s.ready)
	input, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	output, peer := io.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	p := &process{stdin: writer, stdout: output, cancel: func() {}, done: make(chan struct{}), clock: clock}
	close(p.done)
	p.conn = acp.NewClientSideConnection(&client{session: s}, p, output)
	s.proc = p
	m.sessions[s.info.ID] = s
	ctx := testutil.Context(t, testutil.WaitLong)
	caller, cancel := context.WithCancel(ctx)
	defer cancel()
	trap := clock.Trap().AfterFunc("acp-write")
	done := make(chan error, 1)
	go func() {
		_, err := m.Message(caller, s.info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: strings.Repeat("x", 1<<19)})
		done <- err
	}()
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	trap.Close()
	cancel()
	clock.Advance(setupTimeout).MustWait(ctx)
	require.Error(t, testutil.RequireReceive(ctx, t, done))
	// Admission has released opMu, so interruption can return an unavailable
	// runtime instead of hanging behind the blocked transport.
	_, err = m.Interrupt(ctx, s.info.ID)
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestObsoleteCallbacksCannotChangeRecoveredHistory(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeLoad)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	s, err := m.get(ctx, info.ID)
	require.NoError(t, err)
	s.mu.Lock()
	old := &client{session: s, generation: s.generation}
	s.mu.Unlock()
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
	require.NoError(t, err)
	_, err = awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"})
	require.NoError(t, err)
	_, err = awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	before, err := m.Read(ctx, info.ID, nil)
	require.NoError(t, err)
	require.NotEqual(t, info.Cursor.Epoch, before.Session.Cursor.Epoch)
	for _, kind := range []string{"agent_message_chunk", "user_message_chunk"} {
		var update acp.SessionUpdate
		require.NoError(t, json.Unmarshal([]byte(`{"sessionUpdate":"`+kind+`","content":{"type":"text","text":"stale"}}`), &update))
		require.NoError(t, old.SessionUpdate(ctx, acp.SessionNotification{SessionId: acp.SessionId(info.ID.SessionID), Update: update}))
	}
	after, err := m.Read(ctx, info.ID, nil)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestPromptCompletionPreservesCurrentRuntime(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"failed", "replaced", "new_turn", "completed"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			m := NewManager(context.Background(), Options{})
			t.Cleanup(func() { require.NoError(t, m.Close()) })
			s := m.newSession(workspacesdk.ACPCreateSessionRequest{}, uuid.NewString())
			p := &process{}
			s.proc, s.turn, s.info.Status = p, 1, workspacesdk.ACPSessionStatusRunning
			switch scenario {
			case "failed":
				s.failLocked(ErrUnavailable)
			case "replaced":
				s.proc = &process{}
			case "new_turn":
				s.turn++
			}
			before := s.infoLocked()
			s.completePromptLocked(p, 1, acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil)
			if scenario == "completed" {
				require.Equal(t, workspacesdk.ACPSessionStatusIdle, s.info.Status)
			} else {
				require.Equal(t, before, s.info)
			}
		})
	}
}

func TestRecoverySharesFailuresAndBacksOff(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"})
	require.NoError(t, err)
	_, err = awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	m.mu.Lock()
	cfg := m.configs["fake"]
	cfg.command = fakeCommand(t, fakeHarnessModeHang)
	cfg.hash = sha256.Sum256([]byte(cfg.command))
	m.configs["fake"] = cfg
	m.mu.Unlock()
	clock := m.clock.(*quartz.Mock)
	trap := clock.Trap().AfterFunc("acp-timeout")
	caller, cancel := context.WithCancel(ctx)
	defer cancel()
	canceled := make(chan error, 1)
	go func() { _, err := m.recover(caller, info.ID); canceled <- err }()
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	trap.Close()
	cancel()
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, canceled), context.Canceled)
	s, err := m.get(ctx, info.ID)
	require.NoError(t, err)
	s.mu.Lock()
	generation := s.generation
	s.mu.Unlock()
	results := make(chan error, 10)
	for range 10 {
		go func() { _, err := m.recover(ctx, info.ID); results <- err }()
	}
	clock.Advance(setupTimeout).MustWait(ctx)
	for range 10 {
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, results), ErrUnavailable)
	}
	_, err = m.recover(ctx, info.ID)
	require.ErrorIs(t, err, ErrUnavailable)
	s.mu.Lock()
	require.Equal(t, generation, s.generation)
	s.mu.Unlock()
	clock.Advance(discoveryInterval).MustWait(ctx)
	trap = clock.Trap().AfterFunc("acp-timeout")
	retried := make(chan error, 1)
	go func() { _, err := m.recover(ctx, info.ID); retried <- err }()
	call = trap.MustWait(ctx)
	call.MustRelease(ctx)
	trap.Close()
	clock.Advance(setupTimeout).MustWait(ctx)
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, retried), ErrUnavailable)
	s.mu.Lock()
	require.Equal(t, generation+1, s.generation)
	s.mu.Unlock()
}

func TestDetachedSteeringCompletion(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, events, unsubscribe, err := m.Subscribe(ctx, info.ID, &info.Cursor)
	require.NoError(t, err)
	defer unsubscribe()
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hold"})
	require.NoError(t, err)
	for {
		event := testutil.RequireReceive(ctx, t, events)
		if strings.Contains(string(event.Update), `"text":"started"`) {
			break
		}
	}
	result, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "new-complete"})
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPMessageOutcomeStartedNewTurn, result.Outcome)
	completed, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, completed.Session.Status)
	require.Contains(t, completed.AssistantResponse, "answer:new-complete:other")
	// Messages following detached work must wait for its adapter-owned cleanup.
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "after detached"})
	require.NoError(t, err)
	completed, err = awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Contains(t, completed.AssistantResponse, "answer:after detached:other")
}

type recordingWriter struct{ writes chan []byte }

func (w recordingWriter) Write(raw []byte) (int, error) {
	w.writes <- append([]byte(nil), raw...)
	return len(raw), nil
}

func (recordingWriter) Close() error { return nil }

func TestInterruptRetrySerializesWithAdmission(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	m := NewManager(context.Background(), Options{Clock: clock})
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	s := m.newSession(workspacesdk.ACPCreateSessionRequest{HarnessSlug: "retry", WorkingDirectory: t.TempDir()}, uuid.NewString())
	s.info.Status = workspacesdk.ACPSessionStatusIdle
	close(s.ready)
	output, peer := io.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	writer := recordingWriter{writes: make(chan []byte, 16)}
	p := &process{stdin: writer, stdout: output, cancel: func() {}, done: make(chan struct{}), clock: clock}
	close(p.done)
	p.conn = acp.NewClientSideConnection(&client{session: s}, p, output)
	s.proc = p
	m.sessions[s.info.ID] = s
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, s.info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "first"})
	require.NoError(t, err)
	var prompt struct{ ID json.RawMessage }
	require.NoError(t, json.Unmarshal(testutil.RequireReceive(ctx, t, writer.writes), &prompt))
	timerTrap := clock.Trap().NewTimer("acp-interrupt")
	_, err = m.Interrupt(ctx, s.info.ID)
	require.NoError(t, err)
	testutil.RequireReceive(ctx, t, writer.writes)
	call := timerTrap.MustWait(ctx)
	call.MustRelease(ctx)
	timerTrap.Close()
	writeTrap := clock.Trap().AfterFunc("acp-write")
	defer func() {
		if writeTrap != nil {
			writeTrap.Close()
		}
	}()
	advanced := clock.Advance(time.Second)
	call = writeTrap.MustWait(ctx)
	// At the transport boundary of the retry, a new Message must not be able
	// to change the turn that this cancellation was checked against.
	locked := s.opMu.TryLock()
	if locked {
		s.opMu.Unlock()
	}
	require.False(t, locked, "retry cancellation must hold the admission lock")
	call.MustRelease(ctx)
	writeTrap.Close()
	writeTrap = nil
	advanced.MustWait(ctx)
	testutil.RequireReceive(ctx, t, writer.writes)
	_, err = peer.Write([]byte(`{"jsonrpc":"2.0","id":` + string(prompt.ID) + `,"result":{"stopReason":"` + string(acp.StopReasonCancelled) + `"}}` + "\n"))
	require.NoError(t, err)
	result, err := awaitTestSession(ctx, m, s.info.ID)
	require.NoError(t, err)
	require.Equal(t, string(acp.StopReasonCancelled), result.Session.StopReason)
	_, err = m.Message(ctx, s.info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "second"})
	require.NoError(t, err)
	result, err = m.Read(ctx, s.info.ID, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusRunning, result.Session.Status)
}

func TestSuccessfulRecoveryAllowsAnotherImmediateRecovery(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	for _, text := range []string{"first", "second"} {
		_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"})
		require.NoError(t, err)
		failed, err := awaitTestSession(ctx, m, info.ID)
		require.NoError(t, err)
		require.Equal(t, workspacesdk.ACPSessionStatusError, failed.Session.Status)
		_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: text})
		require.NoError(t, err)
		completed, err := awaitTestSession(ctx, m, info.ID)
		require.NoError(t, err)
		require.Equal(t, "answer:"+text+":other", completed.AssistantResponse)
	}
}

func TestDisconnectDrainsQueuedUpdates(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	m := NewManager(context.Background(), Options{Clock: clock, Logger: slogtest.Make(t, nil)})
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	s := m.newSession(workspacesdk.ACPCreateSessionRequest{HarnessSlug: "queued", WorkingDirectory: t.TempDir()}, uuid.NewString())
	s.info.Status = workspacesdk.ACPSessionStatusRunning
	s.generation = 1
	close(s.ready)
	output, peer := io.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	p := &process{stdin: recordingWriter{writes: make(chan []byte, 1)}, stdout: output, cancel: func() {}, done: make(chan struct{}), clock: clock}
	p.conn, p.updatesDone = newClientConnection(&client{session: s, generation: s.generation}, p, output)
	s.proc = p
	m.sessions[s.info.ID] = s
	ctx := testutil.Context(t, testutil.WaitLong)
	trap := clock.Trap().NewTimer("acp-drain")
	defer func() {
		if trap != nil {
			trap.Close()
		}
	}()
	// Queue all updates while callbacks are blocked on the session lock. The
	// SDK's disconnect signal must precede their processing in this scenario.
	s.mu.Lock()
	require.True(t, m.run(func() { m.watchProcess(s, p, harnessConfig{slug: "queued"}) }))
	go func() {
		for range 200 {
			_, _ = peer.Write([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"` + s.info.ID.SessionID + `","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"chunk;"}}}}` + "\n"))
		}
		_ = peer.Close()
		close(p.done)
	}()
	select {
	case <-p.conn.Done():
	case <-ctx.Done():
		s.mu.Unlock()
		t.Fatal(ctx.Err())
	}
	call, err := trap.Wait(ctx)
	// The watcher is draining and has not invalidated the queued callbacks.
	generation := s.generation
	s.mu.Unlock()
	require.NoError(t, err)
	require.Equal(t, uint64(1), generation)
	call.MustRelease(ctx)
	trap.Close()
	trap = nil
	result, err := awaitTestSession(ctx, m, s.info.ID)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusError, result.Session.Status)
	require.Equal(t, strings.Repeat("chunk;", 200), result.AssistantResponse)
}

func TestNotificationDrainTimeout(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	m := NewManager(context.Background(), Options{Clock: clock, Logger: slogtest.Make(t, nil)})
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	ctx := testutil.Context(t, testutil.WaitLong)
	trap := clock.Trap().NewTimer("acp-drain")
	defer func() {
		if trap != nil {
			trap.Close()
		}
	}()
	done := make(chan struct{}, 1)
	go func() {
		m.drainUpdates(ctx, &process{updatesDone: make(chan struct{})})
		done <- struct{}{}
	}()
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	trap.Close()
	trap = nil
	clock.Advance(setupTimeout).MustWait(ctx)
	testutil.RequireReceive(ctx, t, done)
}
