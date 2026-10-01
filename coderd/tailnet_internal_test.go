package coderd

import (
	"context"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMultiAgentController_ExpireRunsOnExpire(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	var expired []uuid.UUID
	m := NewMultiAgentController(ctx, testutil.Logger(t), noop.NewTracerProvider().Tracer(""), nil, func(agentID uuid.UUID) {
		expired = append(expired, agentID)
	})
	defer m.Close()

	idle, inUse := uuid.New(), uuid.New()
	require.NoError(t, m.ensureAgent(idle))
	require.NoError(t, m.ensureAgent(inUse))
	release := m.acquireTicket(inUse)
	defer release()

	m.doExpireOldAgents(ctx, 0)
	require.Equal(t, []uuid.UUID{idle}, expired)
}

func TestAgentTransports_Forget(t *testing.T) {
	t.Parallel()

	dial := func(context.Context, uint16) (net.Conn, error) { return nil, xerrors.New("unused") }
	logger := testutil.Logger(t)
	pair := func(agentID uuid.UUID) agentTransportPair {
		return agentTransportPair{
			api:  workspacesdk.NewAgentAPITransport(agentID, dial, logger),
			apps: workspacesdk.NewAgentAppTransport(agentID, dial, logger),
		}
	}
	forgotten, kept := uuid.New(), uuid.New()
	ts := &agentTransports{agents: map[uuid.UUID]agentTransportPair{
		forgotten: pair(forgotten),
		kept:      pair(kept),
	}}
	ts.forget(forgotten)
	require.NotContains(t, ts.agents, forgotten)
	require.Contains(t, ts.agents, kept)
}

func TestPollingDERPClient_FirstRecvDoesNotWaitForTick(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	derpMap := &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{1: {RegionID: 1}}}
	// The mock clock never advances, so a poll loop that waits for the
	// first tick would block forever.
	client := newPollingDERPClient(func() *tailcfg.DERPMap { return derpMap }, testutil.Logger(t), quartz.NewMock(t))
	defer client.Close()

	got := make(chan *tailcfg.DERPMap, 1)
	go func() {
		dm, err := client.Recv()
		if err != nil {
			t.Error("recv derp map:", err)
			return
		}
		got <- dm
	}()
	require.Equal(t, derpMap, testutil.RequireReceive(ctx, t, got))
}
