package agentacp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

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

type recordingWriter struct{ writes chan []byte }

func (w recordingWriter) Write(raw []byte) (int, error) {
	w.writes <- append([]byte(nil), raw...)
	return len(raw), nil
}
func (recordingWriter) Close() error { return nil }

func newTransportSession(t *testing.T, status workspacesdk.ACPSessionStatus, steering bool) (*Manager, *SessionFSM, <-chan []byte, *io.PipeWriter) {
	t.Helper()
	m := NewManager(context.Background(), Options{Clock: quartz.NewMock(t), Filesystem: afero.NewOsFs(), Logger: slogtest.Make(t, nil)})
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	output, peer := io.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	writer := recordingWriter{writes: make(chan []byte, 512)}
	p := &process{stdin: writer, stdout: output, cancel: func() {}, done: make(chan struct{}), clock: m.clock}
	close(p.done)
	id := workspacesdk.ACPSessionID{HarnessSlug: "test", WorkingDirectory: t.TempDir(), SessionID: uuid.NewString()}
	info := workspacesdk.ACPSession{ID: id, Status: status, Cursor: workspacesdk.ACPCursor{Epoch: uuid.New()}, Harness: workspacesdk.ACPHarness{Steering: steering}}
	s := newSessionFSM(m.ctx, workspacesdk.ACPCreateSessionRequest{HarnessSlug: id.HarnessSlug, WorkingDirectory: id.WorkingDirectory}, info, p, m.run)
	s.turn = 1
	p.conn = acp.NewClientSideConnection(&client{session: s}, p, output)
	m.sessions[id] = s
	m.instances = append(m.instances, s)
	require.True(t, m.run(s.loop))
	return m, s, writer.writes, peer
}

func sendTransportResult(t *testing.T, peer *io.PipeWriter, request []byte, result any) {
	t.Helper()
	var req struct{ ID json.RawMessage }
	require.NoError(t, json.Unmarshal(request, &req))
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	require.NoError(t, err)
	_, err = peer.Write(append(raw, '\n'))
	require.NoError(t, err)
}

func sessionNotification(id workspacesdk.ACPSessionID, text string) acp.SessionNotification {
	return acp.SessionNotification{SessionId: acp.SessionId(id.SessionID), Update: acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock(text)}}}
}

func TestSessionFSMTransitions(t *testing.T) {
	t.Parallel()
	// Expected transitions are specified independently of the implementation.
	cases := []struct {
		state workspacesdk.ACPSessionStatus
		want  map[sessionOperation]workspacesdk.ACPSessionStatus
	}{
		{workspacesdk.ACPSessionStatusIdle, map[sessionOperation]workspacesdk.ACPSessionStatus{operationMessage: workspacesdk.ACPSessionStatusRunning, operationSetError: workspacesdk.ACPSessionStatusError}},
		{workspacesdk.ACPSessionStatusRunning, map[sessionOperation]workspacesdk.ACPSessionStatus{operationMessage: workspacesdk.ACPSessionStatusRunning, operationInterrupt: workspacesdk.ACPSessionStatusInterrupting, operationFinishRunning: workspacesdk.ACPSessionStatusIdle, operationSetError: workspacesdk.ACPSessionStatusError}},
		{workspacesdk.ACPSessionStatusInterrupting, map[sessionOperation]workspacesdk.ACPSessionStatus{operationFinishInterruption: workspacesdk.ACPSessionStatusIdle, operationSetError: workspacesdk.ACPSessionStatusError}},
		{workspacesdk.ACPSessionStatusError, nil},
	}
	for _, tc := range cases {
		for _, op := range []sessionOperation{operationMessage, operationInterrupt, operationFinishRunning, operationFinishInterruption, operationSetError} {
			t.Run(string(tc.state)+"/"+string(op), func(t *testing.T) {
				t.Parallel()
				s := &SessionFSM{info: workspacesdk.ACPSession{Status: tc.state}}
				err := s.transition(op)
				if target, ok := tc.want[op]; ok {
					require.NoError(t, err)
					require.Equal(t, target, s.info.Status)
				} else {
					require.ErrorIs(t, err, ErrConflict)
					require.Equal(t, tc.state, s.info.Status)
				}
			})
		}
	}
}

func TestRejectedUserOperationsPreserveTranscript(t *testing.T) {
	t.Parallel()
	for _, state := range []workspacesdk.ACPSessionStatus{workspacesdk.ACPSessionStatusIdle, workspacesdk.ACPSessionStatusInterrupting, workspacesdk.ACPSessionStatusError} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			_, s, writes, _ := newTransportSession(t, state, true)
			ctx := testutil.Context(t, testutil.WaitLong)
			before, changed, err := s.Watch(ctx, nil)
			require.NoError(t, err)
			reply, err := s.Interrupt(ctx)
			require.NoError(t, err)
			require.ErrorIs(t, testutil.RequireReceive(ctx, t, reply).Error, ErrConflict)
			if state != workspacesdk.ACPSessionStatusIdle {
				reply, err = s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "rejected"})
				require.NoError(t, err)
				require.ErrorIs(t, testutil.RequireReceive(ctx, t, reply).Error, ErrConflict)
			}
			after, err := s.Read(ctx, nil)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Empty(t, writes)
			select {
			case <-changed:
				t.Fatal("rejected operation woke reader")
			default:
			}
		})
	}
}

func TestSessionFSMUpdatesAndSnapshots(t *testing.T) {
	t.Parallel()
	for _, status := range []workspacesdk.ACPSessionStatus{workspacesdk.ACPSessionStatusIdle, workspacesdk.ACPSessionStatusRunning, workspacesdk.ACPSessionStatusInterrupting, workspacesdk.ACPSessionStatusError} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			_, s, _, _ := newTransportSession(t, status, true)
			ctx := testutil.Context(t, testutil.WaitLong)
			before, changed, err := s.Watch(ctx, nil)
			require.NoError(t, err)
			req := sessionNotification(s.id, "final output")
			err = s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req})
			if status == workspacesdk.ACPSessionStatusIdle || status == workspacesdk.ACPSessionStatusError {
				require.ErrorIs(t, err, ErrConflict)
				after, err := s.Read(ctx, nil)
				require.NoError(t, err)
				require.Equal(t, before, after)
				select {
				case <-changed:
					t.Fatal("rejected update woke reader")
				default:
				}
				return
			}
			require.NoError(t, err)
			select {
			case <-changed:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			req.SessionId = "other"
			prior, err := s.Read(ctx, nil)
			require.NoError(t, err)
			require.ErrorIs(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req}), ErrInvalid)
			copySnapshot, err := s.Read(ctx, nil)
			require.NoError(t, err)
			require.Equal(t, prior, copySnapshot)
			copySnapshot.Events[0].Update[0] = 'x'
			copySnapshot.Session.Harness.ConfigOptions = append(copySnapshot.Session.Harness.ConfigOptions, workspacesdk.ACPConfigOption{ID: "mutated"})
			after, err := s.Read(ctx, nil)
			require.NoError(t, err)
			require.Equal(t, prior, after)
			for _, reason := range []acp.StopReason{acp.StopReasonEndTurn} {
				require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{StopReason: reason}, Turn: 1}))
				completed, err := s.Wait(ctx)
				require.NoError(t, err)
				require.Equal(t, "final output", completed.AssistantResponse)
				require.Equal(t, string(reason), completed.Session.StopReason)
				require.Equal(t, workspacesdk.ACPSessionStatusIdle, completed.Session.Status)
			}
		})
	}
}

func TestSessionFSMInterruptDoesNotRetry(t *testing.T) {
	t.Parallel()
	m, s, writes, peer := newTransportSession(t, workspacesdk.ACPSessionStatusIdle, true)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "work"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	info, err := m.Interrupt(ctx, s.id)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusInterrupting, info.Status)
	testutil.RequireReceive(ctx, t, writes)
	_, err = m.Interrupt(ctx, s.id)
	require.ErrorIs(t, err, ErrConflict)
	_, err = m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "too soon"})
	require.ErrorIs(t, err, ErrConflict)
	m.clock.(*quartz.Mock).Advance(10 * setupTimeout).MustWait(ctx)
	require.Empty(t, writes)
	r, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusInterrupting, r.Session.Status)
	sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	r, err = s.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, r.Session.Status)
	require.Equal(t, string(acp.StopReasonEndTurn), r.Session.StopReason)
}

func TestSteeringResponseDoesNotBlockInterrupt(t *testing.T) {
	t.Parallel()
	m, s, writes, peer := newTransportSession(t, workspacesdk.ACPSessionStatusIdle, true)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "first"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	result, err := m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "steer"})
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPMessageOutcomeSteering, result.Outcome)
	steer := testutil.RequireReceive(ctx, t, writes)
	_, err = m.Interrupt(ctx, s.id)
	require.NoError(t, err)
	testutil.RequireReceive(ctx, t, writes)
	sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonCancelled})
	sendTransportResult(t, peer, steer, workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomeInjected})
	r, err := s.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, string(acp.StopReasonCancelled), r.Session.StopReason)
}

func TestSessionFSMResetAndWriteFailure(t *testing.T) {
	t.Parallel()
	m, s, writes, peer := newTransportSession(t, workspacesdk.ACPSessionStatusIdle, false)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "first"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	req := sessionNotification(s.id, "keep this")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req}))
	sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	before, err := s.Wait(ctx)
	require.NoError(t, err)
	// No RPC is outstanding, so replacing this writer cannot race a write.
	s.proc.stdin = failingWriter{s.proc.stdin}
	_, err = m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "broken"})
	require.ErrorContains(t, err, "broken harness input")
	after, err := s.Wait(ctx)
	require.ErrorContains(t, err, "broken harness input")
	require.Equal(t, before.Session.Cursor.Epoch, after.Session.Cursor.Epoch)
	require.Equal(t, before.AssistantResponse, after.AssistantResponse)
	require.Len(t, after.Events, len(before.Events)+2)
}

func TestSessionFSMUnsupportedSteering(t *testing.T) {
	t.Parallel()
	_, s, writes, _ := newTransportSession(t, workspacesdk.ACPSessionStatusRunning, false)
	ctx := testutil.Context(t, testutil.WaitLong)
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	reply, err := s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "steer"})
	require.NoError(t, err)
	require.ErrorContains(t, testutil.RequireReceive(ctx, t, reply).Error, "harness does not support steering")
	after, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Empty(t, writes)
}

func TestUnreadOperationResultsAndLaterFailure(t *testing.T) {
	t.Parallel()
	_, s, writes, _ := newTransportSession(t, workspacesdk.ACPSessionStatusIdle, true)
	ctx := testutil.Context(t, testutil.WaitLong)
	reply, err := s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
	require.NoError(t, err)
	testutil.RequireReceive(ctx, t, writes)
	failure := xerrors.New("adapter lost")
	require.NoError(t, s.adapterError(ctx, failure))
	r, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusError, r.Session.Status)
	require.NoError(t, testutil.RequireReceive(ctx, t, reply).Error)
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, reply).Error, failure)
	_, open := <-reply
	require.False(t, open)
}

func TestMessageAdmissionWriteDeadline(t *testing.T) {
	t.Parallel()
	m, s, _, _ := newTransportSession(t, workspacesdk.ACPSessionStatusIdle, true)
	input, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	s.proc.stdin = writer
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := m.clock.(*quartz.Mock)
	trap := clock.Trap().AfterFunc("acp-write")
	done := make(chan error, 1)
	go func() {
		_, err := m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: strings.Repeat("x", 1<<19)})
		done <- err
	}()
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	trap.Close()
	clock.Advance(setupTimeout).MustWait(ctx)
	require.Error(t, testutil.RequireReceive(ctx, t, done))
	_, err = m.Interrupt(ctx, s.id)
	require.ErrorIs(t, err, ErrConflict)
}

func TestRecoveryReplacesInstanceAndIsolatesCallbacks(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	old, err := m.get(ctx, info.ID)
	require.NoError(t, err)
	crash := workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"}
	_, err = m.Message(ctx, info.ID, crash)
	require.NoError(t, err)
	failed, err := old.Wait(ctx)
	require.Error(t, err)
	var wg sync.WaitGroup
	results := make(chan *SessionFSM, 10)
	for range 10 {
		wg.Go(func() { s, err := m.recover(ctx, info.ID); require.NoError(t, err); results <- s })
	}
	wg.Wait()
	close(results)
	var current *SessionFSM
	for s := range results {
		if current == nil {
			current = s
		}
		require.Same(t, current, s)
	}
	require.NotSame(t, old, current)
	req := sessionNotification(info.ID, "stale")
	require.ErrorIs(t, (&client{session: old}).SessionUpdate(ctx, req), ErrConflict)
	stillFailed, err := old.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, failed, stillFailed)
	restored, err := current.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, restored.Session.Status)
	require.NotEqual(t, failed.Session.Cursor.Epoch, restored.Session.Cursor.Epoch)
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
	results := make(chan error, 10)
	for range 10 {
		go func() { _, err := m.recover(ctx, info.ID); results <- err }()
	}
	clock.Advance(setupTimeout).MustWait(ctx)
	for range 10 {
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, results), ErrUnavailable)
	}
	m.mu.Lock()
	attempt := m.initializing[info.ID]
	m.mu.Unlock()
	_, err = m.recover(ctx, info.ID)
	require.ErrorIs(t, err, ErrUnavailable)
	m.mu.Lock()
	require.Same(t, attempt, m.initializing[info.ID])
	m.mu.Unlock()
	clock.Advance(discoveryInterval).MustWait(ctx)
	trap = clock.Trap().AfterFunc("acp-timeout")
	retried := make(chan error, 1)
	go func() { _, err := m.recover(ctx, info.ID); retried <- err }()
	call = trap.MustWait(ctx)
	call.MustRelease(ctx)
	trap.Close()
	clock.Advance(setupTimeout).MustWait(ctx)
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, retried), ErrUnavailable)
}

func TestSessionFSMResetsOnlyOnSuccessfulIdleMessage(t *testing.T) {
	t.Parallel()
	m, s, writes, peer := newTransportSession(t, workspacesdk.ACPSessionStatusIdle, true)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "first"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	req := sessionNotification(s.id, "old output")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req}))
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	_, err = m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "steer"})
	require.NoError(t, err)
	steer := testutil.RequireReceive(ctx, t, writes)
	steered, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, before.Session.Cursor.Epoch, steered.Session.Cursor.Epoch)
	require.Equal(t, "old output", steered.AssistantResponse)
	sendTransportResult(t, peer, steer, workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomeInjected})
	sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	_, err = s.Wait(ctx)
	require.NoError(t, err)
	_, err = m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "second"})
	require.NoError(t, err)
	testutil.RequireReceive(ctx, t, writes)
	after, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.NotEqual(t, before.Session.Cursor.Epoch, after.Session.Cursor.Epoch)
	require.Empty(t, after.AssistantResponse)
	require.Len(t, after.Events, 2)
	require.Equal(t, "second", after.Events[0].Text)
	require.Equal(t, workspacesdk.ACPSessionStatusRunning, after.Events[1].Session.Status)
	_, err = s.Read(ctx, &before.Session.Cursor)
	require.ErrorIs(t, err, ErrConflict)
}

func TestSessionFSMInterruptWriteFailure(t *testing.T) {
	t.Parallel()
	m, s, _, _ := newTransportSession(t, workspacesdk.ACPSessionStatusRunning, true)
	ctx := testutil.Context(t, testutil.WaitLong)
	req := sessionNotification(s.id, "partial")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req}))
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	s.proc.stdin = failingWriter{s.proc.stdin}
	_, err = m.Interrupt(ctx, s.id)
	require.ErrorContains(t, err, "broken harness input")
	after, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusError, after.Session.Status)
	require.Equal(t, before.Session.Cursor.Epoch, after.Session.Cursor.Epoch)
	require.Equal(t, "partial", after.AssistantResponse)
}

func TestSessionFSMUnexpectedSteeringFailure(t *testing.T) {
	t.Parallel()
	_, s, writes, peer := newTransportSession(t, workspacesdk.ACPSessionStatusRunning, true)
	ctx := testutil.Context(t, testutil.WaitLong)
	reply, err := s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "steer"})
	require.NoError(t, err)
	request := testutil.RequireReceive(ctx, t, writes)
	require.NoError(t, testutil.RequireReceive(ctx, t, reply).Error)
	// A malformed response is an unexpected work failure after admission.
	sendTransportResult(t, peer, request, map[string]string{"outcome": "rejected"})
	require.ErrorContains(t, testutil.RequireReceive(ctx, t, reply).Error, "steering was not accepted")
	result, err := s.Wait(ctx)
	require.Error(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusError, result.Session.Status)
	require.Equal(t, "steer", result.Events[0].Text)
}

func TestFinalACPUpdatesPrecedeCompletion(t *testing.T) {
	t.Parallel()
	m, s, writes, peer := newTransportSession(t, workspacesdk.ACPSessionStatusIdle, true)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "prompt"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	for range 200 {
		raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": sessionNotification(s.id, "chunk;")})
		require.NoError(t, err)
		_, err = peer.Write(append(raw, '\n'))
		require.NoError(t, err)
	}
	sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	result, err := s.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("chunk;", 200), result.AssistantResponse)
	require.Equal(t, workspacesdk.ACPEventKindStatus, result.Events[len(result.Events)-1].Kind)
}

func TestPromptFailureDoesNotWaitForSteeringResponse(t *testing.T) {
	t.Parallel()
	_, s, writes, _ := newTransportSession(t, workspacesdk.ACPSessionStatusRunning, true)
	ctx := testutil.Context(t, testutil.WaitLong)
	reply, err := s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "steer"})
	require.NoError(t, err)
	testutil.RequireReceive(ctx, t, writes)
	require.NoError(t, testutil.RequireReceive(ctx, t, reply).Error)
	failure := xerrors.New("prompt failed")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{}, Turn: 1, Error: failure}))
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, reply).Error, failure)
	result, err := s.Wait(ctx)
	require.ErrorContains(t, err, "prompt failed")
	require.Equal(t, workspacesdk.ACPSessionStatusError, result.Session.Status)
}

func TestSteeringNewTurnsKeepCompletionOwnership(t *testing.T) {
	t.Parallel()
	m, s, writes, peer := newTransportSession(t, workspacesdk.ACPSessionStatusIdle, true)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "first"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	status := func(kind string) {
		var update acp.SessionUpdate
		require.NoError(t, json.Unmarshal([]byte(`{"sessionUpdate":"session_info_update","_meta":{"codex":{"threadStatus":{"type":"`+kind+`"}}}}`), &update))
		req := acp.SessionNotification{SessionId: acp.SessionId(s.id.SessionID), Update: update}
		require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req}))
	}
	status("active")
	_, err = m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "new one"})
	require.NoError(t, err)
	firstSteer := testutil.RequireReceive(ctx, t, writes)
	_, err = m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "new two"})
	require.NoError(t, err)
	secondSteer := testutil.RequireReceive(ctx, t, writes)
	status("idle")
	status("active")
	status("idle")
	// Both responses start new turns. The third turn's status has not yet
	// arrived, and reversing response order must not complete that turn.
	sendTransportResult(t, peer, secondSteer, workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomeStartedNewTurn})
	sendTransportResult(t, peer, firstSteer, workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomeStartedNewTurn})
	sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	status("active")
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusRunning, before.Session.Status)
	status("idle")
	result, err := s.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, result.Session.Status)
	require.Len(t, result.Events, len(before.Events)+2)
}
