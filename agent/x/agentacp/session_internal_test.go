package agentacp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"
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

type recordingWriter struct{ writes chan []byte }

func (w recordingWriter) Write(raw []byte) (int, error) {
	w.writes <- append([]byte(nil), raw...)
	return len(raw), nil
}
func (recordingWriter) Close() error { return nil }

func newRuntimeSession(t *testing.T, status workspacesdk.ACPSessionStatus) (*SessionFSM, <-chan []byte, *io.PipeWriter) {
	t.Helper()
	return newRuntimeSessionWithClient(t, status, func(s *SessionFSM) acp.Client {
		return &sessionClient{session: s}
	})
}

func newRuntimeSessionWithClient(t *testing.T, status workspacesdk.ACPSessionStatus, newClient func(*SessionFSM) acp.Client) (*SessionFSM, <-chan []byte, *io.PipeWriter) {
	t.Helper()
	output, peer := io.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	writer := recordingWriter{writes: make(chan []byte, 512)}
	p := &process{stdin: writer, stdout: output, done: make(chan struct{}), clock: quartz.NewMock(t)}
	p.cancel = sync.OnceFunc(func() { close(p.done) })
	id := workspacesdk.ACPSessionID{HarnessSlug: "test", WorkingDirectory: t.TempDir(), SessionID: uuid.NewString()}
	info := workspacesdk.ACPSession{ID: id, Status: status, Cursor: workspacesdk.ACPCursor{GenerationID: uuid.New()}}
	s := newSessionFSM(context.Background(), sessionInitRequest{harnessSlug: id.HarnessSlug, workingDirectory: id.WorkingDirectory}, info)
	s.turn = 1
	p.conn = acp.NewClientSideConnection(newClient(s), p, output)
	s.startProcess(p, slogtest.Make(t, nil))
	t.Cleanup(s.Close)
	return s, writer.writes, peer
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

func TestRejectedUserOperationsPreserveTranscript(t *testing.T) {
	t.Parallel()
	for _, state := range []workspacesdk.ACPSessionStatus{workspacesdk.ACPSessionStatusInitializing, workspacesdk.ACPSessionStatusIdle, workspacesdk.ACPSessionStatusInterrupting, workspacesdk.ACPSessionStatusError} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			s, writes, _ := newRuntimeSession(t, state)
			ctx := testutil.Context(t, testutil.WaitLong)
			before, err := s.Read(ctx, nil)
			require.NoError(t, err)
			_, err = s.Interrupt(ctx)
			require.ErrorIs(t, err, ErrConflict)
			if state != workspacesdk.ACPSessionStatusIdle {
				_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "rejected"})
				require.ErrorIs(t, err, ErrConflict)
			}
			after, err := s.Read(ctx, nil)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Empty(t, writes)
		})
	}
}

func TestSessionFSMUpdateValidation(t *testing.T) {
	t.Parallel()
	for _, status := range []workspacesdk.ACPSessionStatus{workspacesdk.ACPSessionStatusInitializing, workspacesdk.ACPSessionStatusIdle, workspacesdk.ACPSessionStatusRunning, workspacesdk.ACPSessionStatusInterrupting, workspacesdk.ACPSessionStatusError} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			s, _, _ := newRuntimeSession(t, status)
			ctx := testutil.Context(t, testutil.WaitLong)
			before, err := s.Read(ctx, nil)
			require.NoError(t, err)
			req := sessionNotification(s.id, "final output")
			err = s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req})
			if status == workspacesdk.ACPSessionStatusIdle || status == workspacesdk.ACPSessionStatusError {
				require.ErrorIs(t, err, ErrConflict)
				after, err := s.Read(ctx, nil)
				require.NoError(t, err)
				require.Equal(t, before, after)
				return
			}
			require.NoError(t, err)
			updated, err := s.Read(ctx, nil)
			require.NoError(t, err)
			require.Len(t, updated.Events, len(before.Events)+1)
			require.Equal(t, workspacesdk.ACPEventKindUpdate, updated.Events[len(updated.Events)-1].Kind)
			req.SessionId = "other"
			prior, err := s.Read(ctx, nil)
			require.NoError(t, err)
			require.ErrorIs(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req}), ErrInvalid)
			after, err := s.Read(ctx, nil)
			require.NoError(t, err)
			require.Equal(t, prior, after)
		})
	}
}

func TestSessionFSMInterruptDoesNotRetry(t *testing.T) {
	t.Parallel()
	s, writes, peer := newRuntimeSession(t, workspacesdk.ACPSessionStatusIdle)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "work"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	info, err := s.Interrupt(ctx)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusInterrupting, info.Status)
	testutil.RequireReceive(ctx, t, writes)
	_, err = s.Interrupt(ctx)
	require.ErrorIs(t, err, ErrConflict)
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "too soon"})
	require.ErrorIs(t, err, ErrConflict)
	s.proc.clock.(*quartz.Mock).Advance(10 * stdinWriteTimeout).MustWait(ctx)
	require.Empty(t, writes)
	r, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusInterrupting, r.Session.Status)
	sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	r, err = waitTestSession(ctx, s)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, r.Session.Status)
	require.Equal(t, string(acp.StopReasonEndTurn), r.Session.StopReason)
}

func TestSessionFSMWriteFailurePreservesTranscript(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"message", "interrupt"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
			ctx := testutil.Context(t, testutil.WaitLong)
			req := sessionNotification(s.id, "keep this")
			require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req}))
			if operation == "message" {
				require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, Turn: 1}))
			}
			before, err := s.Read(ctx, nil)
			require.NoError(t, err)
			// No RPC is outstanding, so replacing this writer cannot race a write.
			s.proc.stdin = failingWriter{s.proc.stdin}
			if operation == "message" {
				_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "broken"})
			} else {
				_, err = s.Interrupt(ctx)
			}
			require.ErrorContains(t, err, "broken harness input")
			select {
			case <-s.proc.done:
			default:
				t.Fatal("failed harness was not reaped")
			}
			after, err := waitTestSession(ctx, s)
			require.ErrorContains(t, err, "broken harness input")
			require.Equal(t, workspacesdk.ACPSessionStatusError, after.Session.Status)
			require.Equal(t, before.Session.Cursor.GenerationID, after.Session.Cursor.GenerationID)
			require.Equal(t, before.AssistantResponse, after.AssistantResponse)
			require.Len(t, after.Events, len(before.Events)+2)
			require.Equal(t, before.Events, after.Events[:len(before.Events)])
		})
	}
}

func TestSessionFSMRejectsBusyMessage(t *testing.T) {
	t.Parallel()
	s, writes, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	ctx := testutil.Context(t, testutil.WaitLong)
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "follow-up"})
	require.ErrorIs(t, err, ErrConflict)
	after, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Empty(t, writes)
}

func TestMessageAdmissionWriteDeadline(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusIdle)
	input, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	s.proc.stdin = writer
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := s.proc.clock.(*quartz.Mock)
	trap := clock.Trap().AfterFunc("acp-write")
	done := make(chan error, 1)
	go func() {
		_, err := s.Message(ctx, workspacesdk.ACPMessageRequest{Text: strings.Repeat("x", 1<<19)})
		done <- err
	}()
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	trap.Close()
	clock.Advance(stdinWriteTimeout).MustWait(ctx)
	require.Error(t, testutil.RequireReceive(ctx, t, done))
	_, err = s.Interrupt(ctx)
	require.ErrorIs(t, err, ErrConflict)
}

func TestSessionFSMResetsOnlyOnSuccessfulIdleMessage(t *testing.T) {
	t.Parallel()
	s, writes, peer := newRuntimeSession(t, workspacesdk.ACPSessionStatusIdle)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "first"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	req := sessionNotification(s.id, "old output")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req}))
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "follow-up"})
	require.ErrorIs(t, err, ErrConflict)
	require.Empty(t, writes)
	rejected, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, before, rejected)
	sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	_, err = waitTestSession(ctx, s)
	require.NoError(t, err)
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "second"})
	require.NoError(t, err)
	testutil.RequireReceive(ctx, t, writes)
	after, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.NotEqual(t, before.Session.Cursor.GenerationID, after.Session.Cursor.GenerationID)
	require.Empty(t, after.AssistantResponse)
	require.Len(t, after.Events, 1)
	require.Equal(t, workspacesdk.ACPEventKindStatus, after.Events[0].Kind)
	require.Equal(t, workspacesdk.ACPSessionStatusRunning, after.Events[0].Session.Status)
	_, err = s.Read(ctx, &before.Session.Cursor)
	require.ErrorIs(t, err, ErrConflict)
}

type blockingSessionClient struct {
	*sessionClient
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (c *blockingSessionClient) SessionUpdate(ctx context.Context, req acp.SessionNotification) error {
	c.once.Do(func() {
		close(c.entered)
		<-c.release
	})
	return c.sessionClient.SessionUpdate(ctx, req)
}

func TestFinalACPUpdatesPrecedeCompletion(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		entered, release := make(chan struct{}), make(chan struct{})
		s, writes, peer := newRuntimeSessionWithClient(t, workspacesdk.ACPSessionStatusIdle, func(s *SessionFSM) acp.Client {
			return &blockingSessionClient{sessionClient: &sessionClient{session: s}, entered: entered, release: release}
		})
		unblock := sync.OnceFunc(func() { close(release) })
		t.Cleanup(unblock)
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "prompt"})
		require.NoError(t, err)
		prompt := testutil.RequireReceive(ctx, t, writes)
		const chunks = 200
		for range chunks {
			raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": sessionNotification(s.id, "chunk;")})
			require.NoError(t, err)
			_, err = peer.Write(append(raw, '\n'))
			require.NoError(t, err)
		}
		testutil.TryReceive(ctx, t, entered)
		sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
		// Let completion run as far as it can while the first update is blocked.
		synctest.Wait()
		pending, err := s.Read(ctx, nil)
		require.NoError(t, err)
		require.Equal(t, workspacesdk.ACPSessionStatusRunning, pending.Session.Status)
		require.Empty(t, pending.AssistantResponse)

		unblock()
		synctest.Wait()
		result, err := s.Read(ctx, nil)
		require.NoError(t, err)
		require.Equal(t, workspacesdk.ACPSessionStatusIdle, result.Session.Status)
		require.Equal(t, strings.Repeat("chunk;", chunks), result.AssistantResponse)
		require.Len(t, result.Events, chunks+2)
		require.Equal(t, workspacesdk.ACPEventKindStatus, result.Events[len(result.Events)-1].Kind)
	})
}

func TestPromptFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	failure := xerrors.New("prompt failed")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{}, Error: failure, Turn: 1}))
	select {
	case <-s.proc.done:
	default:
		t.Fatal("failed harness was not reaped")
	}
	result, err := waitTestSession(ctx, s)
	require.ErrorContains(t, err, "prompt failed")
	require.Equal(t, workspacesdk.ACPSessionStatusError, result.Session.Status)
}
