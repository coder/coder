package agentacp

import (
	"io"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

//nolint:misspell // ACP names this protocol variant Cancelled.
func TestSessionPermissionAfterInterrupt(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	s := newSessionFSM(ctx, sessionInitRequest{}, workspacesdk.ACPSession{
		ID:     workspacesdk.ACPSessionID{SessionID: uuid.NewString()},
		Status: workspacesdk.ACPSessionStatusRunning,
	})
	local, remote := net.Pipe()
	defer func() {
		s.cancel()
		_ = local.Close()
		_ = remote.Close()
		s.wg.Wait()
	}()
	go func() { _, _ = io.Copy(io.Discard, remote) }()
	client := &sessionClient{session: s}
	s.proc = &process{stdin: local, stdout: local, clock: quartz.NewMock(t)}
	s.proc.conn = acp.NewClientSideConnection(client, s.proc, local)
	req := acp.RequestPermissionRequest{SessionId: acp.SessionId(s.id.SessionID), Options: []acp.PermissionOption{{
		OptionId: "allow", Kind: acp.PermissionOptionKindAllowAlways, Name: "Allow",
	}}}

	_, err := s.Interrupt(ctx)
	require.NoError(t, err)
	// A queued SDK callback can run after interruption with its context active.
	require.NoError(t, ctx.Err())
	response, err := client.RequestPermission(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, response.Outcome.Cancelled)
	require.Nil(t, response.Outcome.Selected)
	require.NoError(t, s.ctx.Err(), "interruption must preserve the session")

	err = s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{StopReason: acp.StopReasonCancelled}})
	require.NoError(t, err)
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{Text: "next turn"})
	require.NoError(t, err)
	response, err = client.RequestPermission(ctx, req)
	require.NoError(t, err)
	require.Nil(t, response.Outcome.Cancelled)
	require.NotNil(t, response.Outcome.Selected)
	require.Equal(t, acp.PermissionOptionId("allow"), response.Outcome.Selected.OptionId)
}

func TestSessionClientUpdates(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	c := &sessionClient{session: s}
	ctx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, c.SessionUpdate(ctx, sessionNotification(s.id, "live")))
	read, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, "live", read.AssistantResponse)
	require.Len(t, read.Events, 1)
}

func TestSessionClientHistoryReplay(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusInitializing)
	c := &sessionClient{session: s}
	ctx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, c.SessionUpdate(ctx, sessionNotification(s.id, "old response")))
	for _, text := range []string{"first user chunk", "second user chunk"} {
		require.NoError(t, c.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: acp.SessionId(s.id.SessionID),
			Update:    acp.SessionUpdate{UserMessageChunk: &acp.SessionUpdateUserMessageChunk{Content: acp.TextBlock(text)}},
		}))
	}
	require.NoError(t, c.SessionUpdate(ctx, sessionNotification(s.id, "history")))
	wrongID := s.id
	wrongID.SessionID = "other"
	require.ErrorIs(t, c.SessionUpdate(ctx, sessionNotification(wrongID, "wrong session")), ErrInvalid)
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, "history", before.AssistantResponse)
	require.Len(t, before.Events, 1)
	require.Equal(t, workspacesdk.ACPSessionStatusInitializing, before.Session.Status)
	require.NoError(t, s.finishInitialization(s.id.SessionID, before.Session.Harness))
	after, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, "history", after.AssistantResponse)
	require.Len(t, after.Events, 1)
	require.Equal(t, before.Events, after.Events)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, after.Session.Status)
	// Once initialized, idle sessions reject live updates through the FSM.
	require.ErrorIs(t, c.SessionUpdate(ctx, sessionNotification(s.id, "late")), ErrConflict)
}

func TestSessionClientSetupNotifications(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusInitializing)
	id := s.id
	s.mu.Lock()
	s.id.SessionID = ""
	s.info.ID = s.id
	s.mu.Unlock()
	c := &sessionClient{session: s}
	ctx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, c.SessionUpdate(ctx, sessionNotification(id, "setup")))
	require.ErrorIs(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{}, Turn: 1}), ErrConflict)
	require.NoError(t, s.finishInitialization(id.SessionID, workspacesdk.ACPHarness{}))
	read, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, id, read.Session.ID)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, read.Session.Status)
	require.Empty(t, read.AssistantResponse)
	require.Empty(t, read.Events)
}

func TestSessionClientInitializationFailure(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusInitializing)
	c := &sessionClient{session: s}
	ctx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, s.adapterError(ctx, xerrors.New("harness disconnected")))
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusError, before.Session.Status)
	require.ErrorIs(t, s.finishInitialization(s.id.SessionID, workspacesdk.ACPHarness{}), ErrConflict)
	require.ErrorIs(t, c.SessionUpdate(ctx, sessionNotification(s.id, "late setup")), ErrConflict)
	after, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
