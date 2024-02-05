package tailnet_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/tailnet"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
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

	ma1 := tailnettest.NewTestMultiAgent(t, coord1)
	defer ma1.Close()

	ma1.RequireSubscribeAgent(agent1.id)
	ma1.RequireEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.RequireEventuallyHasDERPs(ctx, 1)

	ma1.SendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	ma1.Close()
	require.NoError(t, agent1.close())

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)
	assertEventuallyLost(ctx, t, store, agent1.id)
}

func TestPGCoordinator_MultiAgent_CoordClose(t *testing.T) {
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

	ma1 := tailnettest.NewTestMultiAgent(t, coord1)
	defer ma1.Close()

	err = coord1.Close()
	require.NoError(t, err)

	ma1.RequireEventuallyClosed(ctx)
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

	ma1 := tailnettest.NewTestMultiAgent(t, coord1)
	defer ma1.Close()

	ma1.RequireSubscribeAgent(agent1.id)
	ma1.RequireEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.RequireEventuallyHasDERPs(ctx, 1)

	ma1.SendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	ma1.RequireUnsubscribeAgent(agent1.id)
	ma1.Close()
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

	ma1 := tailnettest.NewTestMultiAgent(t, coord1)
	defer ma1.Close()

	ma1.RequireSubscribeAgent(agent1.id)
	ma1.RequireEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.RequireEventuallyHasDERPs(ctx, 1)

	ma1.SendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	ma1.RequireUnsubscribeAgent(agent1.id)
	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)

	func() {
		ctx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow*3)
		defer cancel()
		ma1.SendNodeWithDERP(9)
		assertNeverHasDERPs(ctx, t, agent1, 9)
	}()
	func() {
		ctx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow*3)
		defer cancel()
		agent1.sendNode(&agpl.Node{PreferredDERP: 8})
		ma1.RequireNeverHasDERPs(ctx, 8)
	}()

	ma1.Close()
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

	ma1 := tailnettest.NewTestMultiAgent(t, coord2)
	defer ma1.Close()

	ma1.RequireSubscribeAgent(agent1.id)
	ma1.RequireEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.RequireEventuallyHasDERPs(ctx, 1)

	ma1.SendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	ma1.Close()
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

	ma1 := tailnettest.NewTestMultiAgent(t, coord2)
	defer ma1.Close()

	ma1.SendNodeWithDERP(3)

	ma1.RequireSubscribeAgent(agent1.id)
	ma1.RequireEventuallyHasDERPs(ctx, 5)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.RequireEventuallyHasDERPs(ctx, 1)

	ma1.Close()
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

	ma1 := tailnettest.NewTestMultiAgent(t, coord3)
	defer ma1.Close()

	ma1.RequireSubscribeAgent(agent1.id)
	ma1.RequireEventuallyHasDERPs(ctx, 5)

	agent1.sendNode(&agpl.Node{PreferredDERP: 1})
	ma1.RequireEventuallyHasDERPs(ctx, 1)

	ma1.RequireSubscribeAgent(agent2.id)
	ma1.RequireEventuallyHasDERPs(ctx, 6)

	agent2.sendNode(&agpl.Node{PreferredDERP: 2})
	ma1.RequireEventuallyHasDERPs(ctx, 2)

	ma1.SendNodeWithDERP(3)
	assertEventuallyHasDERPs(ctx, t, agent1, 3)
	assertEventuallyHasDERPs(ctx, t, agent2, 3)

	ma1.Close()
	require.NoError(t, agent1.close())
	require.NoError(t, agent2.close())

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.id)
	assertEventuallyLost(ctx, t, store, agent1.id)
}
