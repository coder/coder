package agentacp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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
	output, peer := io.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	writer := recordingWriter{writes: make(chan []byte, 512)}
	p := &process{stdin: writer, stdout: output, cancel: func() {}, done: make(chan struct{}), clock: quartz.NewMock(t)}
	close(p.done)
	id := workspacesdk.ACPSessionID{HarnessSlug: "test", WorkingDirectory: t.TempDir(), SessionID: uuid.NewString()}
	info := workspacesdk.ACPSession{ID: id, Status: status, Cursor: workspacesdk.ACPCursor{Epoch: uuid.New()}}
	var wg sync.WaitGroup
	s := newSessionFSM(context.Background(), workspacesdk.ACPCreateSessionRequest{HarnessSlug: id.HarnessSlug, WorkingDirectory: id.WorkingDirectory}, info, p, func(fn func()) bool {
		wg.Go(fn)
		return true
	})
	s.mu.Lock()
	s.turn = 1
	s.mu.Unlock()
	p.conn = acp.NewClientSideConnection(&client{session: s}, p, output)
	t.Cleanup(func() {
		s.cancel()
		p.close()
		s.shutdown()
		wg.Wait()
	})
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
	for _, state := range []workspacesdk.ACPSessionStatus{workspacesdk.ACPSessionStatusIdle, workspacesdk.ACPSessionStatusInterrupting, workspacesdk.ACPSessionStatusError} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			s, writes, _ := newRuntimeSession(t, state)
			ctx := testutil.Context(t, testutil.WaitLong)
			before, events, err := s.Watch(ctx, nil)
			require.NoError(t, err)
			_, err = s.Interrupt(ctx)
			require.ErrorIs(t, err, ErrConflict)
			if state != workspacesdk.ACPSessionStatusIdle {
				_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "rejected"})
				require.ErrorIs(t, err, ErrConflict)
			}
			after, err := s.Read(ctx, nil)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Empty(t, writes)
			select {
			case <-events:
				t.Fatal("rejected operation published an event")
			default:
			}
		})
	}
}

func TestSessionFSMUpdateValidation(t *testing.T) {
	t.Parallel()
	for _, status := range []workspacesdk.ACPSessionStatus{workspacesdk.ACPSessionStatusIdle, workspacesdk.ACPSessionStatusRunning, workspacesdk.ACPSessionStatusInterrupting, workspacesdk.ACPSessionStatusError} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			s, _, _ := newRuntimeSession(t, status)
			ctx := testutil.Context(t, testutil.WaitLong)
			before, events, err := s.Watch(ctx, nil)
			require.NoError(t, err)
			req := sessionNotification(s.id, "final output")
			err = s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req})
			if status == workspacesdk.ACPSessionStatusIdle || status == workspacesdk.ACPSessionStatusError {
				require.ErrorIs(t, err, ErrConflict)
				after, err := s.Read(ctx, nil)
				require.NoError(t, err)
				require.Equal(t, before, after)
				select {
				case <-events:
					t.Fatal("rejected update published an event")
				default:
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, workspacesdk.ACPEventKindUpdate, testutil.RequireReceive(ctx, t, events).Kind)
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
	_, err := s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "work"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	info, err := s.Interrupt(ctx)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusInterrupting, info.Status)
	testutil.RequireReceive(ctx, t, writes)
	_, err = s.Interrupt(ctx)
	require.ErrorIs(t, err, ErrConflict)
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "too soon"})
	require.ErrorIs(t, err, ErrConflict)
	s.proc.clock.(*quartz.Mock).Advance(10 * setupTimeout).MustWait(ctx)
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
				_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "broken"})
			} else {
				_, err = s.Interrupt(ctx)
			}
			require.ErrorContains(t, err, "broken harness input")
			after, err := s.Wait(ctx)
			require.ErrorContains(t, err, "broken harness input")
			require.Equal(t, workspacesdk.ACPSessionStatusError, after.Session.Status)
			require.Equal(t, before.Session.Cursor.Epoch, after.Session.Cursor.Epoch)
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
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "follow-up"})
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
		_, err := s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: strings.Repeat("x", 1<<19)})
		done <- err
	}()
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	trap.Close()
	clock.Advance(setupTimeout).MustWait(ctx)
	require.Error(t, testutil.RequireReceive(ctx, t, done))
	_, err = s.Interrupt(ctx)
	require.ErrorIs(t, err, ErrConflict)
}

func TestSessionFSMResetsOnlyOnSuccessfulIdleMessage(t *testing.T) {
	t.Parallel()
	s, writes, peer := newRuntimeSession(t, workspacesdk.ACPSessionStatusIdle)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "first"})
	require.NoError(t, err)
	prompt := testutil.RequireReceive(ctx, t, writes)
	req := sessionNotification(s.id, "old output")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req}))
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "follow-up"})
	require.ErrorIs(t, err, ErrConflict)
	require.Empty(t, writes)
	rejected, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, before, rejected)
	sendTransportResult(t, peer, prompt, acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	_, err = s.Wait(ctx)
	require.NoError(t, err)
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "second"})
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

func TestFinalACPUpdatesPrecedeCompletion(t *testing.T) {
	t.Parallel()
	s, writes, peer := newRuntimeSession(t, workspacesdk.ACPSessionStatusIdle)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "prompt"})
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

func TestPromptFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	s := newSessionFSM(ctx, workspacesdk.ACPCreateSessionRequest{}, workspacesdk.ACPSession{Status: workspacesdk.ACPSessionStatusRunning}, nil, nil)
	failure := xerrors.New("prompt failed")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{}, Error: failure}))
	result, err := s.Wait(ctx)
	require.ErrorContains(t, err, "prompt failed")
	require.Equal(t, workspacesdk.ACPSessionStatusError, result.Session.Status)
}
