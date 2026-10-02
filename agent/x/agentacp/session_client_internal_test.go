package agentacp

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSessionClientAttachment(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	c := &sessionClient{}
	ctx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, c.SessionUpdate(ctx, sessionNotification(s.id, "setup")))
	require.NoError(t, c.attachSession(s))
	require.NoError(t, c.SessionUpdate(ctx, sessionNotification(s.id, "live")))
	read, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, "live", read.AssistantResponse)
	require.Len(t, read.Events, 1)
}

func TestSessionClientHistoryReplay(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusIdle)
	c := &sessionClient{}
	ctx := testutil.Context(t, testutil.WaitLong)
	c.beginHistoryReplay(s.id.SessionID)
	require.NoError(t, c.SessionUpdate(ctx, sessionNotification(s.id, "history")))
	wrongID := s.id
	wrongID.SessionID = "other"
	require.ErrorIs(t, c.SessionUpdate(ctx, sessionNotification(wrongID, "wrong session")), ErrInvalid)
	before, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, before.Events)
	require.NoError(t, c.attachSession(s))
	after, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, "history", after.AssistantResponse)
	require.Len(t, after.Events, 1)
	// Once attached, idle sessions reject live updates through the FSM.
	require.ErrorIs(t, c.SessionUpdate(ctx, sessionNotification(s.id, "late")), ErrConflict)
}

func TestSessionClientConcurrentHistoryHandoff(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	c := &sessionClient{}
	ctx := testutil.Context(t, testutil.WaitLong)
	c.beginHistoryReplay(s.id.SessionID)
	require.NoError(t, c.SessionUpdate(ctx, sessionNotification(s.id, "history")))
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := c.SessionUpdate(ctx, sessionNotification(s.id, "live")); err != nil {
			t.Errorf("deliver notification: %v", err)
		}
	})
	require.NoError(t, c.attachSession(s))
	wg.Wait()
	read, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, "historylive", read.AssistantResponse)
	require.Len(t, read.Events, 2)
}
