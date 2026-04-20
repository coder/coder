package coderd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type mcpLSAgentConn struct {
	workspacesdk.AgentConn
	closeCalls int
}

func (c *mcpLSAgentConn) Close() error {
	c.closeCalls++
	return nil
}

func TestSharedTailnetAgentDialerRequiresProvider(t *testing.T) {
	t.Parallel()

	_, err := (sharedTailnetAgentDialer{}).DialAgent(t.Context(), uuid.New(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent provider is unavailable")
}

func TestSharedTailnetAgentDialerRejectsIncompleteProviderResult(t *testing.T) {
	t.Parallel()

	t.Run("nil conn", func(t *testing.T) {
		t.Parallel()

		dialer := sharedTailnetAgentDialer{provider: fakeAgentProvider{
			agentConn: func(context.Context, uuid.UUID) (_ workspacesdk.AgentConn, release func(), _ error) {
				return nil, func() {}, nil
			},
		}}

		_, err := dialer.DialAgent(t.Context(), uuid.New(), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "nil connection")
	})

	t.Run("nil release", func(t *testing.T) {
		t.Parallel()

		dialer := sharedTailnetAgentDialer{provider: fakeAgentProvider{
			agentConn: func(context.Context, uuid.UUID) (_ workspacesdk.AgentConn, release func(), _ error) {
				return &mcpLSAgentConn{}, nil, nil
			},
		}}

		_, err := dialer.DialAgent(t.Context(), uuid.New(), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "nil release function")
	})
}

func TestSharedTailnetAgentDialerCloseReleasesLease(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()
	baseConn := &mcpLSAgentConn{}
	releaseCalls := 0

	dialer := sharedTailnetAgentDialer{provider: fakeAgentProvider{
		agentConn: func(ctx context.Context, gotAgentID uuid.UUID) (_ workspacesdk.AgentConn, release func(), _ error) {
			require.Equal(t, agentID, gotAgentID)
			return baseConn, func() {
				releaseCalls++
			}, nil
		},
	}}

	conn, err := dialer.DialAgent(t.Context(), agentID, nil)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.Equal(t, 1, releaseCalls)
	require.Equal(t, 1, baseConn.closeCalls)
}
