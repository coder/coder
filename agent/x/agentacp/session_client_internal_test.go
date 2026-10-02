package agentacp

import (
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
