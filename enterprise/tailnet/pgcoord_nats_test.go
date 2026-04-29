package tailnet_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	xnats "github.com/coder/coder/v2/coderd/x/nats"
	"github.com/coder/coder/v2/enterprise/tailnet"
	agpl "github.com/coder/coder/v2/tailnet"
	agpltest "github.com/coder/coder/v2/tailnet/test"
	"github.com/coder/coder/v2/testutil"
)

// TestPGCoordinatorDual_NATSPubsub validates step 5 of the PG-pubsub →
// NATS-pubsub migration plan: the PGCoordinator behaves identically when
// its tailnet pubsub channels are routed through an embedded NATS
// pubsub via the appPubsub argument, instead of the legacy PG pubsub.
//
// It mirrors TestPGCoordinatorDual_Mainline but constructs each
// coordinator with a separate single-node embedded NATS Pubsub passed
// as appPS. The PG (store, ps) pair is still threaded as the primary
// pubsub argument so that any non-tailnet code paths remain wired,
// however all four tailnet channels (heartbeat, peer update, tunnel
// update, ready-for-handshake) must traverse the NATS pubsubs.
func TestPGCoordinatorDual_NATSPubsub(t *testing.T) {
	t.Parallel()

	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)

	// One single-node embedded NATS Pubsub shared by both
	// coordinators. This mirrors a single NATS node serving multiple
	// coderd processes within a cluster, and ensures all four tailnet
	// channels propagate between coord1 and coord2 via NATS rather
	// than via PG.
	natsLogger := testutil.Logger(t).Named("nats")
	appPS, err := xnats.New(ctx, natsLogger, xnats.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = appPS.Close() })

	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store, appPS)
	require.NoError(t, err)
	defer coord1.Close()
	coord2, err := tailnet.NewPGCoord(ctx, logger.Named("coord2"), ps, store, appPS)
	require.NoError(t, err)
	defer coord2.Close()

	agent1 := agpltest.NewAgent(ctx, t, coord1, "agent1")
	defer agent1.Close(ctx)
	t.Logf("agent1=%s", agent1.ID)
	agent2 := agpltest.NewAgent(ctx, t, coord2, "agent2")
	defer agent2.Close(ctx)
	t.Logf("agent2=%s", agent2.ID)

	client11 := agpltest.NewClient(ctx, t, coord1, "client11", agent1.ID)
	defer client11.Close(ctx)
	t.Logf("client11=%s", client11.ID)
	client12 := agpltest.NewClient(ctx, t, coord1, "client12", agent2.ID)
	defer client12.Close(ctx)
	t.Logf("client12=%s", client12.ID)
	client21 := agpltest.NewClient(ctx, t, coord2, "client21", agent1.ID)
	defer client21.Close(ctx)
	t.Logf("client21=%s", client21.ID)
	client22 := agpltest.NewClient(ctx, t, coord2, "client22", agent2.ID)
	defer client22.Close(ctx)
	t.Logf("client22=%s", client22.ID)

	t.Log("client11 -> Node 11")
	client11.UpdateDERP(11)
	agent1.AssertEventuallyHasDERP(client11.ID, 11)

	t.Log("client21 -> Node 21")
	client21.UpdateDERP(21)
	agent1.AssertEventuallyHasDERP(client21.ID, 21)

	t.Log("client22 -> Node 22")
	client22.UpdateDERP(22)
	agent2.AssertEventuallyHasDERP(client22.ID, 22)

	t.Log("agent2 -> Node 2")
	agent2.UpdateDERP(2)
	client22.AssertEventuallyHasDERP(agent2.ID, 2)
	client12.AssertEventuallyHasDERP(agent2.ID, 2)

	t.Log("client12 -> Node 12")
	client12.UpdateDERP(12)
	agent2.AssertEventuallyHasDERP(client12.ID, 12)

	t.Log("agent1 -> Node 1")
	agent1.UpdateDERP(1)
	client21.AssertEventuallyHasDERP(agent1.ID, 1)
	client11.AssertEventuallyHasDERP(agent1.ID, 1)

	t.Log("close coord2")
	err = coord2.Close()
	require.NoError(t, err)

	// This closes agent2, client22, client21.
	agent2.AssertEventuallyResponsesClosed(agpl.CloseErrCoordinatorClose)
	client22.AssertEventuallyResponsesClosed(agpl.CloseErrCoordinatorClose)
	client21.AssertEventuallyResponsesClosed(agpl.CloseErrCoordinatorClose)
	assertEventuallyLost(ctx, t, store, agent2.ID)
	assertEventuallyLost(ctx, t, store, client21.ID)
	assertEventuallyLost(ctx, t, store, client22.ID)

	err = coord1.Close()
	require.NoError(t, err)
	// This closes agent1, client12, client11.
	agent1.AssertEventuallyResponsesClosed(agpl.CloseErrCoordinatorClose)
	client12.AssertEventuallyResponsesClosed(agpl.CloseErrCoordinatorClose)
	client11.AssertEventuallyResponsesClosed(agpl.CloseErrCoordinatorClose)
	assertEventuallyLost(ctx, t, store, agent1.ID)
	assertEventuallyLost(ctx, t, store, client11.ID)
	assertEventuallyLost(ctx, t, store, client12.ID)
}
