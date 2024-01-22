package tailnet_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/tailnet"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestPGCoordinator_MultiAgent tests a single coordinator with a MultiAgent
// connecting to one agent.
//
//	            +--------+
//	agent1 ---> | coord1 | <--- client
//	            +--------+
func TestPGCoordinator_MultiAgent(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()

	agent1 := newTestAgent(t, coord1, "agent1")
	defer agent1.close()
	agent1.sendNode(&agpl.Node{PreferredDERP: 5})

	ma1 := newTestMultiAgent(t, coord1)
	defer ma1.close()

	ma1.subscribeAgent(agent1.id)
	ma1.assertEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.assertEventuallyHasDERPs(ctx, 1)

	ma1.sendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	ma1.close()
	require.NoError(t, agent1.close())

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)
	assertEventuallyLost(ctx, t, store, agent1.id)
}

// TestPGCoordinator_MultiAgent_UnsubscribeRace tests a single coordinator with
// a MultiAgent connecting to one agent. It tries to race a call to Unsubscribe
// with the MultiAgent closing.
//
//	            +--------+
//	agent1 ---> | coord1 | <--- client
//	            +--------+
func TestPGCoordinator_MultiAgent_UnsubscribeRace(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()

	agent1 := newTestAgent(t, coord1, "agent1")
	defer agent1.close()
	agent1.sendNode(&agpl.Node{PreferredDERP: 5})

	ma1 := newTestMultiAgent(t, coord1)
	defer ma1.close()

	ma1.subscribeAgent(agent1.id)
	ma1.assertEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.assertEventuallyHasDERPs(ctx, 1)

	ma1.sendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	ma1.unsubscribeAgent(agent1.id)
	ma1.close()
	require.NoError(t, agent1.close())

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)
	assertEventuallyLost(ctx, t, store, agent1.id)
}

// TestPGCoordinator_MultiAgent_Unsubscribe tests a single coordinator with a
// MultiAgent connecting to one agent. It unsubscribes before closing, and
// ensures node updates are no longer propagated.
//
//	            +--------+
//	agent1 ---> | coord1 | <--- client
//	            +--------+
func TestPGCoordinator_MultiAgent_Unsubscribe(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()

	agent1 := newTestAgent(t, coord1, "agent1")
	defer agent1.close()
	agent1.sendNode(&agpl.Node{PreferredDERP: 5})

	ma1 := newTestMultiAgent(t, coord1)
	defer ma1.close()

	ma1.subscribeAgent(agent1.id)
	ma1.assertEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.assertEventuallyHasDERPs(ctx, 1)

	ma1.sendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	ma1.unsubscribeAgent(agent1.id)
	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)

	func() {
		ctx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow*3)
		defer cancel()
		ma1.sendNodeWithDERP(9)
		assertNeverHasDERPs(ctx, t, agent1, 9)
	}()
	func() {
		ctx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow*3)
		defer cancel()
		agent1.sendNode(&agpl.Node{PreferredDERP: 8})
		ma1.assertNeverHasDERPs(ctx, 8)
	}()

	ma1.close()
	require.NoError(t, agent1.close())

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)
	assertEventuallyLost(ctx, t, store, agent1.id)
}

// TestPGCoordinator_MultiAgent_MultiCoordinator tests two coordinators with a
// MultiAgent connecting to an agent on a separate coordinator.
//
//	            +--------+
//	agent1 ---> | coord1 |
//	            +--------+
//	            +--------+
//	            | coord2 | <--- client
//	            +--------+
func TestPGCoordinator_MultiAgent_MultiCoordinator(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()
	coord2, err := tailnet.NewPGCoord(ctx, logger.Named("coord2"), ps, store)
	require.NoError(t, err)
	defer coord2.Close()

	agent1 := newTestAgent(t, coord1, "agent1")
	defer agent1.close()
	agent1.sendNode(&agpl.Node{PreferredDERP: 5})

	ma1 := newTestMultiAgent(t, coord2)
	defer ma1.close()

	ma1.subscribeAgent(agent1.id)
	ma1.assertEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.assertEventuallyHasDERPs(ctx, 1)

	ma1.sendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	ma1.close()
	require.NoError(t, agent1.close())

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)
	assertEventuallyLost(ctx, t, store, agent1.id)
}

// TestPGCoordinator_MultiAgent_MultiCoordinator_UpdateBeforeSubscribe tests two
// coordinators with a MultiAgent connecting to an agent on a separate
// coordinator. The MultiAgent updates its own node before subscribing.
//
//	            +--------+
//	agent1 ---> | coord1 |
//	            +--------+
//	            +--------+
//	            | coord2 | <--- client
//	            +--------+
func TestPGCoordinator_MultiAgent_MultiCoordinator_UpdateBeforeSubscribe(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()
	coord2, err := tailnet.NewPGCoord(ctx, logger.Named("coord2"), ps, store)
	require.NoError(t, err)
	defer coord2.Close()

	agent1 := newTestAgent(t, coord1, "agent1")
	defer agent1.close()
	agent1.sendNode(&agpl.Node{PreferredDERP: 5})

	ma1 := newTestMultiAgent(t, coord2)
	defer ma1.close()

	ma1.sendNodeWithDERP(3)

	ma1.subscribeAgent(agent1.id)
	ma1.assertEventuallyHasDERPs(ctx, 5)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.assertEventuallyHasDERPs(ctx, 1)

	ma1.close()
	require.NoError(t, agent1.close())

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)
	assertEventuallyLost(ctx, t, store, agent1.id)
}

// TestPGCoordinator_MultiAgent_TwoAgents tests three coordinators with a
// MultiAgent connecting to two agents on separate coordinators.
//
//	            +--------+
//	agent1 ---> | coord1 |
//	            +--------+
//	            +--------+
//	agent2 ---> | coord2 |
//	            +--------+
//	            +--------+
//	            | coord3 | <--- client
//	            +--------+
func TestPGCoordinator_MultiAgent_TwoAgents(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()
	coord2, err := tailnet.NewPGCoord(ctx, logger.Named("coord2"), ps, store)
	require.NoError(t, err)
	defer coord2.Close()
	coord3, err := tailnet.NewPGCoord(ctx, logger.Named("coord3"), ps, store)
	require.NoError(t, err)
	defer coord3.Close()

	agent1 := newTestAgent(t, coord1, "agent1")
	defer agent1.close()
	agent1.sendNode(&agpl.Node{PreferredDERP: 5})

	agent2 := newTestAgent(t, coord2, "agent2")
	defer agent1.close()
	agent2.sendNode(&agpl.Node{PreferredDERP: 6})

	ma1 := newTestMultiAgent(t, coord3)
	defer ma1.close()

	ma1.subscribeAgent(agent1.id)
	ma1.assertEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.assertEventuallyHasDERPs(ctx, 1)

	ma1.subscribeAgent(agent2.id)
	ma1.assertEventuallyHasDERPs(ctx, 6)

	agent2.sendNode(&agpl.Node{PreferredDERP: 2})
	ma1.assertEventuallyHasDERPs(ctx, 2)

	ma1.sendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)
	assertEventuallyHasDERPs(ctx, t, agent2, 3)

	ma1.close()
	require.NoError(t, agent1.close())
	require.NoError(t, agent2.close())

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)
	assertEventuallyLost(ctx, t, store, agent1.id)
}

type testMultiAgent struct {
	t        testing.TB
	id       uuid.UUID
	a        agpl.MultiAgentConn
	nodeKey  []byte
	discoKey string
}

func newTestMultiAgent(t testing.TB, coord agpl.Coordinator) *testMultiAgent {
	nk, err := key.NewNode().Public().MarshalBinary()
	require.NoError(t, err)
	dk, err := key.NewDisco().Public().MarshalText()
	require.NoError(t, err)
	m := &testMultiAgent{t: t, id: uuid.New(), nodeKey: nk, discoKey: string(dk)}
	m.a = coord.ServeMultiAgent(m.id)
	return m
}

func (m *testMultiAgent) sendNodeWithDERP(derp int32) {
	m.t.Helper()
	err := m.a.UpdateSelf(&proto.Node{
		Key:           m.nodeKey,
		Disco:         m.discoKey,
		PreferredDerp: derp,
	})
	require.NoError(m.t, err)
}

func (m *testMultiAgent) close() {
	m.t.Helper()
	err := m.a.Close()
	require.NoError(m.t, err)
}

func (m *testMultiAgent) subscribeAgent(id uuid.UUID) {
	m.t.Helper()
	err := m.a.SubscribeAgent(id)
	require.NoError(m.t, err)
}

func (m *testMultiAgent) unsubscribeAgent(id uuid.UUID) {
	m.t.Helper()
	err := m.a.UnsubscribeAgent(id)
	require.NoError(m.t, err)
}

func (m *testMultiAgent) assertEventuallyHasDERPs(ctx context.Context, expected ...int) {
	m.t.Helper()
	for {
		resp, ok := m.a.NextUpdate(ctx)
		require.True(m.t, ok)
		nodes, err := agpl.OnlyNodeUpdates(resp)
		require.NoError(m.t, err)
		if len(nodes) != len(expected) {
			m.t.Logf("expected %d, got %d nodes", len(expected), len(nodes))
			continue
		}

		derps := make([]int, 0, len(nodes))
		for _, n := range nodes {
			derps = append(derps, n.PreferredDERP)
		}
		for _, e := range expected {
			if !slices.Contains(derps, e) {
				m.t.Logf("expected DERP %d to be in %v", e, derps)
				continue
			}
			return
		}
	}
}

func (m *testMultiAgent) assertNeverHasDERPs(ctx context.Context, expected ...int) {
	m.t.Helper()
	for {
		resp, ok := m.a.NextUpdate(ctx)
		if !ok {
			return
		}
		nodes, err := agpl.OnlyNodeUpdates(resp)
		require.NoError(m.t, err)
		if len(nodes) != len(expected) {
			m.t.Logf("expected %d, got %d nodes", len(expected), len(nodes))
			continue
		}

		derps := make([]int, 0, len(nodes))
		for _, n := range nodes {
			derps = append(derps, n.PreferredDERP)
		}
		for _, e := range expected {
			if !slices.Contains(derps, e) {
				m.t.Logf("expected DERP %d to be in %v", e, derps)
				continue
			}
			return
		}
	}
}
