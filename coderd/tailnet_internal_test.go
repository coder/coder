package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMultiAgentController_ExpiresAgentsWithoutTickets(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	m := NewMultiAgentController(ctx, testutil.Logger(t), noop.NewTracerProvider().Tracer(""), nil)
	defer m.Close()

	idle, inUse := uuid.New(), uuid.New()
	releaseIdle, err := m.acquire(idle, nil)
	require.NoError(t, err)
	releaseIdle()
	releaseInUse, err := m.acquire(inUse, nil)
	require.NoError(t, err)

	m.doExpireOldAgents(ctx, 0)
	require.NotContains(t, m.agents, idle)
	require.Contains(t, m.agents, inUse)

	releaseInUse()
	releaseInUse() // Releasing twice is harmless.
	m.doExpireOldAgents(ctx, 0)
	require.NotContains(t, m.agents, inUse)
}

// TestMultiAgentController_KeepsStateWhileTicketHeld checks that a caller
// holding a ticket keeps using the state, and so the transports, that the
// controller holds for the agent.
func TestMultiAgentController_KeepsStateWhileTicketHeld(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	m := NewMultiAgentController(ctx, testutil.Logger(t), noop.NewTracerProvider().Tracer(""), nil)
	defer m.Close()

	agentID := uuid.New()
	acquire := func() (*agentState, func()) {
		var got *agentState
		release, err := m.acquire(agentID, func(a *agentState) error {
			got = a
			return nil
		})
		require.NoError(t, err)
		return got, release
	}

	first, release := acquire()
	m.doExpireOldAgents(ctx, 0)
	second, releaseSecond := acquire()
	require.Same(t, first, second)
	require.Same(t, first, m.agents[agentID])

	release()
	releaseSecond()
	m.doExpireOldAgents(ctx, 0)
	third, releaseThird := acquire()
	defer releaseThird()
	require.NotSame(t, first, third, "an expired agent gets new state")
}

func TestMultiAgentController_UseErrorHoldsNoTicket(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	m := NewMultiAgentController(ctx, testutil.Logger(t), noop.NewTracerProvider().Tracer(""), nil)
	defer m.Close()

	agentID := uuid.New()
	_, err := m.acquire(agentID, func(*agentState) error { return xerrors.New("build failed") })
	require.ErrorContains(t, err, "build failed")
	require.Empty(t, m.agents[agentID].tickets)
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
