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
	agpltest "github.com/coder/coder/v2/tailnet/test"
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

	agent1 := agpltest.NewAgent(ctx, t, coord1, "agent1")
	defer agent1.Close(ctx)
	agent1.UpdateDERP(5)

	ma1 := agpltest.NewPeer(ctx, t, coord1, "client")
	defer ma1.Close(ctx)

	ma1.AddTunnel(agent1.ID)
	ma1.AssertEventuallyHasDERP(agent1.ID, 5)

	agent1.UpdateDERP(1)
	ma1.AssertEventuallyHasDERP(agent1.ID, 1)

	ma1.UpdateDERP(3)
	agent1.AssertEventuallyHasDERP(ma1.ID, 3)

	ma1.Disconnect()
	agent1.UngracefulDisconnect(ctx)

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.ID)
	assertEventuallyLost(ctx, t, store, agent1.ID)
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

	ma1 := agpltest.NewPeer(ctx, t, coord1, "client")
	defer ma1.Close(ctx)

	err = coord1.Close()
	require.NoError(t, err)

	ma1.AssertEventuallyResponsesClosed(agpl.CloseErrCoordinatorClose)
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

	agent1 := agpltest.NewAgent(ctx, t, coord1, "agent1")
	defer agent1.Close(ctx)
	agent1.UpdateDERP(5)

	ma1 := agpltest.NewPeer(ctx, t, coord1, "client")
	defer ma1.Close(ctx)

	ma1.AddTunnel(agent1.ID)
	ma1.AssertEventuallyHasDERP(agent1.ID, 5)

	agent1.UpdateDERP(1)
	ma1.AssertEventuallyHasDERP(agent1.ID, 1)

	ma1.UpdateDERP(3)
	agent1.AssertEventuallyHasDERP(ma1.ID, 3)

	ma1.RemoveTunnel(agent1.ID)
	ma1.Close(ctx)
	agent1.UngracefulDisconnect(ctx)

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.ID)
	assertEventuallyLost(ctx, t, store, agent1.ID)
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

	agent1 := agpltest.NewAgent(ctx, t, coord1, "agent1")
	defer agent1.Close(ctx)
	agent1.UpdateDERP(5)

	ma1 := agpltest.NewPeer(ctx, t, coord1, "client")
	defer ma1.Close(ctx)

	ma1.AddTunnel(agent1.ID)
	ma1.AssertEventuallyHasDERP(agent1.ID, 5)

	agent1.UpdateDERP(1)
	ma1.AssertEventuallyHasDERP(agent1.ID, 1)

	ma1.UpdateDERP(3)
	agent1.AssertEventuallyHasDERP(ma1.ID, 3)

	ma1.RemoveTunnel(agent1.ID)
	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.ID)

	func() {
		ctx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow*3)
		defer cancel()
		ma1.UpdateDERP(9)
		agent1.AssertNeverHasDERPs(ctx, ma1.ID, 9)
	}()
	func() {
		ctx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow*3)
		defer cancel()
		agent1.UpdateDERP(8)
		ma1.AssertNeverHasDERPs(ctx, agent1.ID, 8)
	}()

	ma1.Disconnect()
	agent1.UngracefulDisconnect(ctx)

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.ID)
	assertEventuallyLost(ctx, t, store, agent1.ID)
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

	agent1 := agpltest.NewAgent(ctx, t, coord1, "agent1")
	defer agent1.Close(ctx)
	agent1.UpdateDERP(5)

	ma1 := agpltest.NewPeer(ctx, t, coord2, "client")
	defer ma1.Close(ctx)

	ma1.AddTunnel(agent1.ID)
	ma1.AssertEventuallyHasDERP(agent1.ID, 5)

	agent1.UpdateDERP(1)
	ma1.AssertEventuallyHasDERP(agent1.ID, 1)

	ma1.UpdateDERP(3)
	agent1.AssertEventuallyHasDERP(ma1.ID, 3)

	ma1.Disconnect()
	agent1.UngracefulDisconnect(ctx)

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.ID)
	assertEventuallyLost(ctx, t, store, agent1.ID)
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

	agent1 := agpltest.NewAgent(ctx, t, coord1, "agent1")
	defer agent1.Close(ctx)
	agent1.UpdateDERP(5)

	ma1 := agpltest.NewPeer(ctx, t, coord2, "client")
	defer ma1.Close(ctx)

	ma1.UpdateDERP(3)

	ma1.AddTunnel(agent1.ID)
	ma1.AssertEventuallyHasDERP(agent1.ID, 5)
	agent1.AssertEventuallyHasDERP(ma1.ID, 3)

	agent1.UpdateDERP(1)
	ma1.AssertEventuallyHasDERP(agent1.ID, 1)

	ma1.Disconnect()
	agent1.UngracefulDisconnect(ctx)

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.ID)
	assertEventuallyLost(ctx, t, store, agent1.ID)
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

	agent1 := agpltest.NewAgent(ctx, t, coord1, "agent1")
	defer agent1.Close(ctx)
	agent1.UpdateDERP(5)

	agent2 := agpltest.NewAgent(ctx, t, coord2, "agent2")
	defer agent2.Close(ctx)
	agent2.UpdateDERP(6)

	ma1 := agpltest.NewPeer(ctx, t, coord3, "client")
	defer ma1.Close(ctx)

	ma1.AddTunnel(agent1.ID)
	ma1.AssertEventuallyHasDERP(agent1.ID, 5)

	agent1.UpdateDERP(1)
	ma1.AssertEventuallyHasDERP(agent1.ID, 1)

	ma1.AddTunnel(agent2.ID)
	ma1.AssertEventuallyHasDERP(agent2.ID, 6)

	agent2.UpdateDERP(2)
	ma1.AssertEventuallyHasDERP(agent2.ID, 2)

	ma1.UpdateDERP(3)
	agent1.AssertEventuallyHasDERP(ma1.ID, 3)
	agent2.AssertEventuallyHasDERP(ma1.ID, 3)

	ma1.Disconnect()
	agent1.UngracefulDisconnect(ctx)
	agent2.UngracefulDisconnect(ctx)

	assertEventuallyNoClientsForAgent(ctx, t, store, agent1.ID)
	assertEventuallyLost(ctx, t, store, agent1.ID)
}
