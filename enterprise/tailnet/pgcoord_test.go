package tailnet_test
import (
	"errors"
	"context"
	"database/sql"
	"net/netip"
	"sync"
	"testing"
	"time"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gProto "google.golang.org/protobuf/proto"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/enterprise/tailnet"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	agpltest "github.com/coder/coder/v2/tailnet/test"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}
func TestPGCoordinatorSingle_ClientWithoutAgent(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agentID := uuid.New()
	client := agpltest.NewClient(ctx, t, coordinator, "client", agentID)
	defer client.Close(ctx)
	client.UpdateDERP(10)
	require.Eventually(t, func() bool {
		clients, err := store.GetTailnetTunnelPeerBindings(ctx, agentID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("database error: %v", err)
		}
		if len(clients) == 0 {
			return false
		}
		node := new(proto.Node)
		err = gProto.Unmarshal(clients[0].Node, node)
		assert.NoError(t, err)
		assert.EqualValues(t, 10, node.PreferredDerp)
		return true
	}, testutil.WaitShort, testutil.IntervalFast)
	client.UngracefulDisconnect(ctx)
	assertEventuallyLost(ctx, t, store, client.ID)
}
func TestPGCoordinatorSingle_AgentWithoutClients(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agent := agpltest.NewAgent(ctx, t, coordinator, "agent")
	defer agent.Close(ctx)
	agent.UpdateDERP(10)
	require.Eventually(t, func() bool {
		agents, err := store.GetTailnetPeers(ctx, agent.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("database error: %v", err)
		}
		if len(agents) == 0 {
			return false
		}
		node := new(proto.Node)
		err = gProto.Unmarshal(agents[0].Node, node)
		assert.NoError(t, err)
		assert.EqualValues(t, 10, node.PreferredDerp)
		return true
	}, testutil.WaitShort, testutil.IntervalFast)
	agent.UngracefulDisconnect(ctx)
	assertEventuallyLost(ctx, t, store, agent.ID)
}
func TestPGCoordinatorSingle_AgentInvalidIP(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agent := agpltest.NewAgent(ctx, t, coordinator, "agent")
	defer agent.Close(ctx)
	agent.UpdateNode(&proto.Node{
		Addresses: []string{
			agpl.TailscaleServicePrefix.RandomPrefix().String(),
		},
		PreferredDerp: 10,
	})
	// The agent connection should be closed immediately after sending an invalid addr
	agent.AssertEventuallyResponsesClosed()
	assertEventuallyLost(ctx, t, store, agent.ID)
}
func TestPGCoordinatorSingle_AgentInvalidIPBits(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agent := agpltest.NewAgent(ctx, t, coordinator, "agent")
	defer agent.Close(ctx)
	agent.UpdateNode(&proto.Node{
		Addresses: []string{
			netip.PrefixFrom(agpl.TailscaleServicePrefix.AddrFromUUID(agent.ID), 64).String(),
		},
		PreferredDerp: 10,
	})
	// The agent connection should be closed immediately after sending an invalid addr
	agent.AssertEventuallyResponsesClosed()
	assertEventuallyLost(ctx, t, store, agent.ID)
}
func TestPGCoordinatorSingle_AgentValidIP(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agent := agpltest.NewAgent(ctx, t, coordinator, "agent")
	defer agent.Close(ctx)
	agent.UpdateNode(&proto.Node{
		Addresses: []string{
			agpl.TailscaleServicePrefix.PrefixFromUUID(agent.ID).String(),
		},
		PreferredDerp: 10,
	})
	require.Eventually(t, func() bool {
		agents, err := store.GetTailnetPeers(ctx, agent.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("database error: %v", err)
		}
		if len(agents) == 0 {
			return false
		}
		node := new(proto.Node)
		err = gProto.Unmarshal(agents[0].Node, node)
		assert.NoError(t, err)
		assert.EqualValues(t, 10, node.PreferredDerp)
		return true
	}, testutil.WaitShort, testutil.IntervalFast)
	agent.UngracefulDisconnect(ctx)
	assertEventuallyLost(ctx, t, store, agent.ID)
}
func TestPGCoordinatorSingle_AgentWithClient(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agent := agpltest.NewAgent(ctx, t, coordinator, "original")
	defer agent.Close(ctx)
	agent.UpdateDERP(10)
	client := agpltest.NewClient(ctx, t, coordinator, "client", agent.ID)
	defer client.Close(ctx)
	client.AssertEventuallyHasDERP(agent.ID, 10)
	client.UpdateDERP(11)
	agent.AssertEventuallyHasDERP(client.ID, 11)
	// Ensure an update to the agent node reaches the connIO!
	agent.UpdateDERP(12)
	client.AssertEventuallyHasDERP(agent.ID, 12)
	// Close the agent channel so a new one can connect.
	agent.Close(ctx)
	// Create a new agent connection. This is to simulate a reconnect!
	agent = agpltest.NewPeer(ctx, t, coordinator, "reconnection", agpltest.WithID(agent.ID))
	// Ensure the coordinator sends its client node immediately!
	agent.AssertEventuallyHasDERP(client.ID, 11)
	// Send a bunch of updates in rapid succession, and test that we eventually get the latest.  We don't want the
	// coordinator accidentally reordering things.
	for d := int32(13); d < 36; d++ {
		agent.UpdateDERP(d)
	}
	client.AssertEventuallyHasDERP(agent.ID, 35)
	agent.UngracefulDisconnect(ctx)
	client.UngracefulDisconnect(ctx)
	assertEventuallyLost(ctx, t, store, agent.ID)
	assertEventuallyLost(ctx, t, store, client.ID)
}
func TestPGCoordinatorSingle_MissedHeartbeats(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := testutil.Logger(t)
	mClock := quartz.NewMock(t)
	afTrap := mClock.Trap().AfterFunc("heartbeats", "recvBeat")
	defer afTrap.Close()
	rstTrap := mClock.Trap().TimerReset("heartbeats", "resetExpiryTimerWithLock")
	defer rstTrap.Close()
	coordinator, err := tailnet.NewTestPGCoord(ctx, logger, ps, store, mClock)
	require.NoError(t, err)
	defer coordinator.Close()
	agent := agpltest.NewAgent(ctx, t, coordinator, "agent")
	defer agent.Close(ctx)
	agent.UpdateDERP(10)
	client := agpltest.NewClient(ctx, t, coordinator, "client", agent.ID)
	defer client.Close(ctx)
	client.AssertEventuallyHasDERP(agent.ID, 10)
	client.UpdateDERP(11)
	agent.AssertEventuallyHasDERP(client.ID, 11)
	// simulate a second coordinator via DB calls only --- our goal is to test broken heart-beating, so we can't use a
	// real coordinator
	fCoord2 := &fakeCoordinator{
		ctx:   ctx,
		t:     t,
		store: store,
		id:    uuid.New(),
	}
	fCoord2.heartbeat()
	afTrap.MustWait(ctx).Release() // heartbeat timeout started
	fCoord2.agentNode(agent.ID, &agpl.Node{PreferredDERP: 12})
	client.AssertEventuallyHasDERP(agent.ID, 12)
	fCoord3 := &fakeCoordinator{
		ctx:   ctx,
		t:     t,
		store: store,
		id:    uuid.New(),
	}
	fCoord3.heartbeat()
	rstTrap.MustWait(ctx).Release() // timeout gets reset
	fCoord3.agentNode(agent.ID, &agpl.Node{PreferredDERP: 13})
	client.AssertEventuallyHasDERP(agent.ID, 13)
	// fCoord2 sends in a second heartbeat, one period later (on time)
	mClock.Advance(tailnet.HeartbeatPeriod).MustWait(ctx)
	fCoord2.heartbeat()
	rstTrap.MustWait(ctx).Release() // timeout gets reset
	// when the fCoord3 misses enough heartbeats, the real coordinator should send an update with the
	// node from fCoord2 for the agent.
	mClock.Advance(tailnet.HeartbeatPeriod).MustWait(ctx)
	w := mClock.Advance(tailnet.HeartbeatPeriod)
	rstTrap.MustWait(ctx).Release()
	w.MustWait(ctx)
	client.AssertEventuallyHasDERP(agent.ID, 12)
	// one more heartbeat period will result in fCoord2 being expired, which should cause us to
	// revert to the original agent mapping
	mClock.Advance(tailnet.HeartbeatPeriod).MustWait(ctx)
	// note that the timeout doesn't get reset because both fCoord2 and fCoord3 are expired
	client.AssertEventuallyHasDERP(agent.ID, 10)
	// send fCoord3 heartbeat, which should trigger us to consider that mapping valid again.
	fCoord3.heartbeat()
	rstTrap.MustWait(ctx).Release() // timeout gets reset
	client.AssertEventuallyHasDERP(agent.ID, 13)
	agent.UngracefulDisconnect(ctx)
	client.UngracefulDisconnect(ctx)
	assertEventuallyLost(ctx, t, store, client.ID)
}
func TestPGCoordinatorSingle_MissedHeartbeats_NoDrop(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agentID := uuid.New()
	client := agpltest.NewPeer(ctx, t, coordinator, "client")
	defer client.Close(ctx)
	client.AddTunnel(agentID)
	client.UpdateDERP(11)
	// simulate a second coordinator via DB calls only --- our goal is to test
	// broken heart-beating, so we can't use a real coordinator
	fCoord2 := &fakeCoordinator{
		ctx:   ctx,
		t:     t,
		store: store,
		id:    uuid.New(),
	}
	// simulate a single heartbeat, the coordinator is healthy
	fCoord2.heartbeat()
	fCoord2.agentNode(agentID, &agpl.Node{PreferredDERP: 12})
	// since it's healthy the client should get the new node.
	client.AssertEventuallyHasDERP(agentID, 12)
	// the heartbeat should then timeout and we'll get sent a LOST update, NOT a
	// disconnect.
	client.AssertEventuallyLost(agentID)
	client.UngracefulDisconnect(ctx)
	assertEventuallyLost(ctx, t, store, client.ID)
}
func TestPGCoordinatorSingle_SendsHeartbeats(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	mu := sync.Mutex{}
	heartbeats := []time.Time{}
	unsub, err := ps.SubscribeWithErr(tailnet.EventHeartbeats, func(_ context.Context, _ []byte, err error) {
		assert.NoError(t, err)
		mu.Lock()
		defer mu.Unlock()
		heartbeats = append(heartbeats, time.Now())
	})
	require.NoError(t, err)
	defer unsub()
	start := time.Now()
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		if len(heartbeats) < 2 {
			return false
		}
		require.Greater(t, heartbeats[0].Sub(start), time.Duration(0))
		require.Greater(t, heartbeats[1].Sub(start), time.Duration(0))
		return assert.Greater(t, heartbeats[1].Sub(heartbeats[0]), tailnet.HeartbeatPeriod*3/4)
	}, testutil.WaitMedium, testutil.IntervalMedium)
}
// TestPGCoordinatorDual_Mainline tests with 2 coordinators, one agent connected to each, and 2 clients per agent.
//
//	            +---------+
//	agent1 ---> | coord1  | <--- client11 (coord 1, agent 1)
//	            |         |
//	            |         | <--- client12 (coord 1, agent 2)
//	            +---------+
//	            +---------+
//	agent2 ---> | coord2  | <--- client21 (coord 2, agent 1)
//	            |         |
//	            |         | <--- client22 (coord2, agent 2)
//	            +---------+
func TestPGCoordinatorDual_Mainline(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()
	coord2, err := tailnet.NewPGCoord(ctx, logger.Named("coord2"), ps, store)
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
	// this closes agent2, client22, client21
	agent2.AssertEventuallyResponsesClosed()
	client22.AssertEventuallyResponsesClosed()
	client21.AssertEventuallyResponsesClosed()
	assertEventuallyLost(ctx, t, store, agent2.ID)
	assertEventuallyLost(ctx, t, store, client21.ID)
	assertEventuallyLost(ctx, t, store, client22.ID)
	err = coord1.Close()
	require.NoError(t, err)
	// this closes agent1, client12, client11
	agent1.AssertEventuallyResponsesClosed()
	client12.AssertEventuallyResponsesClosed()
	client11.AssertEventuallyResponsesClosed()
	assertEventuallyLost(ctx, t, store, agent1.ID)
	assertEventuallyLost(ctx, t, store, client11.ID)
	assertEventuallyLost(ctx, t, store, client12.ID)
}
// TestPGCoordinator_MultiCoordinatorAgent tests when a single agent connects to multiple coordinators.
// We use two agent connections, but they share the same AgentID.  This could happen due to a reconnection,
// or an infrastructure problem where an old workspace is not fully cleaned up before a new one started.
//
//	            +---------+
//	agent1 ---> | coord1  |
//	            +---------+
//	            +---------+
//	agent2 ---> | coord2  |
//	            +---------+
//	            +---------+
//	            | coord3  | <--- client
//	            +---------+
func TestPGCoordinator_MultiCoordinatorAgent(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
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
	agent2 := agpltest.NewPeer(ctx, t, coord2, "agent2",
		agpltest.WithID(agent1.ID), agpltest.WithAuth(agpl.AgentCoordinateeAuth{ID: agent1.ID}),
	)
	defer agent2.Close(ctx)
	client := agpltest.NewClient(ctx, t, coord3, "client", agent1.ID)
	defer client.Close(ctx)
	client.UpdateDERP(3)
	agent1.AssertEventuallyHasDERP(client.ID, 3)
	agent2.AssertEventuallyHasDERP(client.ID, 3)
	agent1.UpdateDERP(1)
	client.AssertEventuallyHasDERP(agent1.ID, 1)
	// agent2's update overrides agent1 because it is newer
	agent2.UpdateDERP(2)
	client.AssertEventuallyHasDERP(agent1.ID, 2)
	// agent2 disconnects, and we should revert back to agent1
	agent2.Close(ctx)
	client.AssertEventuallyHasDERP(agent1.ID, 1)
	agent1.UpdateDERP(11)
	client.AssertEventuallyHasDERP(agent1.ID, 11)
	client.UpdateDERP(31)
	agent1.AssertEventuallyHasDERP(client.ID, 31)
	agent1.UngracefulDisconnect(ctx)
	client.UngracefulDisconnect(ctx)
	assertEventuallyLost(ctx, t, store, client.ID)
	assertEventuallyLost(ctx, t, store, agent1.ID)
}
func TestPGCoordinator_Unhealthy(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)
	ps := pubsub.NewInMemory()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	calls := make(chan struct{})
	// first call succeeds, so that our Agent will successfully connect.
	firstSucceeds := mStore.EXPECT().UpsertTailnetCoordinator(gomock.Any(), gomock.Any()).
		Times(1).
		Return(database.TailnetCoordinator{}, nil)
	// next 3 fail, so the Coordinator becomes unhealthy, and we test that it disconnects the agent
	threeMissed := mStore.EXPECT().UpsertTailnetCoordinator(gomock.Any(), gomock.Any()).
		After(firstSucceeds).
		Times(3).
		Do(func(_ context.Context, _ uuid.UUID) { <-calls }).
		Return(database.TailnetCoordinator{}, errors.New("test disconnect"))
	mStore.EXPECT().UpsertTailnetCoordinator(gomock.Any(), gomock.Any()).
		MinTimes(1).
		After(threeMissed).
		Do(func(_ context.Context, _ uuid.UUID) { <-calls }).
		Return(database.TailnetCoordinator{}, nil)
	// extra calls we don't particularly care about for this test
	mStore.EXPECT().CleanTailnetCoordinators(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().CleanTailnetLostPeers(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().CleanTailnetTunnels(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().GetTailnetTunnelPeerIDs(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil)
	mStore.EXPECT().GetTailnetTunnelPeerBindings(gomock.Any(), gomock.Any()).
		AnyTimes().Return(nil, nil)
	mStore.EXPECT().DeleteTailnetPeer(gomock.Any(), gomock.Any()).
		AnyTimes().Return(database.DeleteTailnetPeerRow{}, nil)
	mStore.EXPECT().DeleteAllTailnetTunnels(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().UpdateTailnetPeerStatusByCoordinator(gomock.Any(), gomock.Any())
	uut, err := tailnet.NewPGCoord(ctx, logger, ps, mStore)
	require.NoError(t, err)
	defer func() {
		err := uut.Close()
		require.NoError(t, err)
	}()
	agent1 := agpltest.NewAgent(ctx, t, uut, "agent1")
	defer agent1.Close(ctx)
	for i := 0; i < 3; i++ {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for call %d", i+1)
		case calls <- struct{}{}:
			// OK
		}
	}
	// connected agent should be disconnected
	agent1.AssertEventuallyResponsesClosed()
	// new agent should immediately disconnect
	agent2 := agpltest.NewAgent(ctx, t, uut, "agent2")
	defer agent2.Close(ctx)
	agent2.AssertEventuallyResponsesClosed()
	// next heartbeats succeed, so we are healthy
	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			t.Fatal("timeout")
		case calls <- struct{}{}:
			// OK
		}
	}
	agent3 := agpltest.NewAgent(ctx, t, uut, "agent3")
	defer agent3.Close(ctx)
	agent3.AssertNotClosed(time.Second)
}
func TestPGCoordinator_Node_Empty(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)
	ps := pubsub.NewInMemory()
	logger := testutil.Logger(t)
	id := uuid.New()
	mStore.EXPECT().GetTailnetPeers(gomock.Any(), id).Times(1).Return(nil, nil)
	// extra calls we don't particularly care about for this test
	mStore.EXPECT().UpsertTailnetCoordinator(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(database.TailnetCoordinator{}, nil)
	mStore.EXPECT().CleanTailnetCoordinators(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().CleanTailnetLostPeers(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().CleanTailnetTunnels(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().UpdateTailnetPeerStatusByCoordinator(gomock.Any(), gomock.Any()).Times(1)
	uut, err := tailnet.NewPGCoord(ctx, logger, ps, mStore)
	require.NoError(t, err)
	defer func() {
		err := uut.Close()
		require.NoError(t, err)
	}()
	node := uut.Node(id)
	require.Nil(t, node)
}
// TestPGCoordinator_BidirectionalTunnels tests when peers create tunnels to each other.  We don't
// do this now, but it's schematically possible, so we should make sure it doesn't break anything.
func TestPGCoordinator_BidirectionalTunnels(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agpltest.BidirectionalTunnels(ctx, t, coordinator)
}
func TestPGCoordinator_GracefulDisconnect(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agpltest.GracefulDisconnectTest(ctx, t, coordinator)
}
func TestPGCoordinator_Lost(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agpltest.LostTest(ctx, t, coordinator)
}
func TestPGCoordinator_NoDeleteOnClose(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	coordinator, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator.Close()
	agent := agpltest.NewAgent(ctx, t, coordinator, "original")
	defer agent.Close(ctx)
	agent.UpdateDERP(10)
	client := agpltest.NewClient(ctx, t, coordinator, "client", agent.ID)
	defer client.Close(ctx)
	// Simulate some traffic to generate
	// a peer.
	client.AssertEventuallyHasDERP(agent.ID, 10)
	client.UpdateDERP(11)
	agent.AssertEventuallyHasDERP(client.ID, 11)
	anode := coordinator.Node(agent.ID)
	require.NotNil(t, anode)
	cnode := coordinator.Node(client.ID)
	require.NotNil(t, cnode)
	err = coordinator.Close()
	require.NoError(t, err)
	assertEventuallyLost(ctx, t, store, agent.ID)
	assertEventuallyLost(ctx, t, store, client.ID)
	coordinator2, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer coordinator2.Close()
	anode = coordinator2.Node(agent.ID)
	require.NotNil(t, anode)
	assert.Equal(t, 10, anode.PreferredDERP)
	cnode = coordinator2.Node(client.ID)
	require.NotNil(t, cnode)
	assert.Equal(t, 11, cnode.PreferredDERP)
}
// TestPGCoordinatorDual_FailedHeartbeat tests that peers
// disconnect from a coordinator when they are unhealthy,
// are marked as LOST (not DISCONNECTED), and can reconnect to
// a new coordinator and reestablish their tunnels.
func TestPGCoordinatorDual_FailedHeartbeat(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	dburl, err := dbtestutil.Open(t)
	require.NoError(t, err)
	store1, ps1, sdb1 := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithURL(dburl))
	defer sdb1.Close()
	store2, ps2, sdb2 := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithURL(dburl))
	defer sdb2.Close()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	t.Cleanup(cancel)
	// We do this to avoid failing due errors related to the
	// database connection being close.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	// Create two coordinators, 1 for each peer.
	c1, err := tailnet.NewPGCoord(ctx, logger, ps1, store1)
	require.NoError(t, err)
	c2, err := tailnet.NewPGCoord(ctx, logger, ps2, store2)
	require.NoError(t, err)
	p1 := agpltest.NewPeer(ctx, t, c1, "peer1")
	p2 := agpltest.NewPeer(ctx, t, c2, "peer2")
	// Create a binding between the two.
	p1.AddTunnel(p2.ID)
	// Ensure that messages pass through.
	p1.UpdateDERP(1)
	p2.UpdateDERP(2)
	p1.AssertEventuallyHasDERP(p2.ID, 2)
	p2.AssertEventuallyHasDERP(p1.ID, 1)
	// Close the underlying database connection to induce
	// a heartbeat failure scenario and assert that
	// we eventually disconnect from the coordinator.
	err = sdb1.Close()
	require.NoError(t, err)
	p1.AssertEventuallyResponsesClosed()
	p2.AssertEventuallyLost(p1.ID)
	// This basically checks that peer2 had no update
	// performed on their status since we are connected
	// to coordinator2.
	assertEventuallyStatus(ctx, t, store2, p2.ID, database.TailnetStatusOk)
	// Connect peer1 to coordinator2.
	p1.ConnectToCoordinator(ctx, c2)
	// Reestablish binding.
	p1.AddTunnel(p2.ID)
	// Ensure messages still flow back and forth.
	p1.AssertEventuallyHasDERP(p2.ID, 2)
	p1.UpdateDERP(3)
	p2.UpdateDERP(4)
	p2.AssertEventuallyHasDERP(p1.ID, 3)
	p1.AssertEventuallyHasDERP(p2.ID, 4)
	// Make sure peer2 never got an update about peer1 disconnecting.
	p2.AssertNeverUpdateKind(p1.ID, proto.CoordinateResponse_PeerUpdate_DISCONNECTED)
}
func TestPGCoordinatorDual_PeerReconnect(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := testutil.Logger(t)
	// Create two coordinators, 1 for each peer.
	c1, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	c2, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	p1 := agpltest.NewPeer(ctx, t, c1, "peer1")
	p2 := agpltest.NewPeer(ctx, t, c2, "peer2")
	// Create a binding between the two.
	p1.AddTunnel(p2.ID)
	// Ensure that messages pass through.
	p1.UpdateDERP(1)
	p2.UpdateDERP(2)
	p1.AssertEventuallyHasDERP(p2.ID, 2)
	p2.AssertEventuallyHasDERP(p1.ID, 1)
	// Close coordinator1. Now we will check that we
	// never send a DISCONNECTED update.
	err = c1.Close()
	require.NoError(t, err)
	p1.AssertEventuallyResponsesClosed()
	p2.AssertEventuallyLost(p1.ID)
	// This basically checks that peer2 had no update
	// performed on their status since we are connected
	// to coordinator2.
	assertEventuallyStatus(ctx, t, store, p2.ID, database.TailnetStatusOk)
	// Connect peer1 to coordinator2.
	p1.ConnectToCoordinator(ctx, c2)
	// Reestablish binding.
	p1.AddTunnel(p2.ID)
	// Ensure messages still flow back and forth.
	p1.AssertEventuallyHasDERP(p2.ID, 2)
	p1.UpdateDERP(3)
	p2.UpdateDERP(4)
	p2.AssertEventuallyHasDERP(p1.ID, 3)
	p1.AssertEventuallyHasDERP(p2.ID, 4)
	// Make sure peer2 never got an update about peer1 disconnecting.
	p2.AssertNeverUpdateKind(p1.ID, proto.CoordinateResponse_PeerUpdate_DISCONNECTED)
}
// TestPGCoordinatorPropogatedPeerContext tests that the context for a specific peer
// is propogated through to the `Authorize` method of the coordinatee auth
func TestPGCoordinatorPropogatedPeerContext(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	ctx := testutil.Context(t, testutil.WaitMedium)
	store, ps := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)
	peerCtx := context.WithValue(ctx, agpltest.FakeSubjectKey{}, struct{}{})
	peerID := uuid.UUID{0x01}
	agentID := uuid.UUID{0x02}
	c1, err := tailnet.NewPGCoord(ctx, logger, ps, store)
	require.NoError(t, err)
	defer func() {
		err := c1.Close()
		require.NoError(t, err)
	}()
	ch := make(chan struct{})
	auth := agpltest.FakeCoordinateeAuth{
		Chan: ch,
	}
	reqs, _ := c1.Coordinate(peerCtx, peerID, "peer1", auth)
	testutil.RequireSendCtx(ctx, t, reqs, &proto.CoordinateRequest{AddTunnel: &proto.CoordinateRequest_Tunnel{Id: agpl.UUIDToByteSlice(agentID)}})
	_ = testutil.RequireRecvCtx(ctx, t, ch)
}
func assertEventuallyStatus(ctx context.Context, t *testing.T, store database.Store, agentID uuid.UUID, status database.TailnetStatus) {
	t.Helper()
	assert.Eventually(t, func() bool {
		peers, err := store.GetTailnetPeers(ctx, agentID)
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, peer := range peers {
			if peer.Status != status {
				return false
			}
		}
		return true
	}, testutil.WaitShort, testutil.IntervalFast)
}
func assertEventuallyLost(ctx context.Context, t *testing.T, store database.Store, agentID uuid.UUID) {
	t.Helper()
	assertEventuallyStatus(ctx, t, store, agentID, database.TailnetStatusLost)
}
func assertEventuallyNoClientsForAgent(ctx context.Context, t *testing.T, store database.Store, agentID uuid.UUID) {
	t.Helper()
	assert.Eventually(t, func() bool {
		clients, err := store.GetTailnetTunnelPeerIDs(ctx, agentID)
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			t.Fatal(err)
		}
		return len(clients) == 0
	}, testutil.WaitShort, testutil.IntervalFast)
}
type fakeCoordinator struct {
	ctx   context.Context
	t     *testing.T
	store database.Store
	id    uuid.UUID
}
func (c *fakeCoordinator) heartbeat() {
	c.t.Helper()
	_, err := c.store.UpsertTailnetCoordinator(c.ctx, c.id)
	require.NoError(c.t, err)
}
func (c *fakeCoordinator) agentNode(agentID uuid.UUID, node *agpl.Node) {
	c.t.Helper()
	pNode, err := agpl.NodeToProto(node)
	require.NoError(c.t, err)
	nodeRaw, err := gProto.Marshal(pNode)
	require.NoError(c.t, err)
	_, err = c.store.UpsertTailnetPeer(c.ctx, database.UpsertTailnetPeerParams{
		ID:            agentID,
		CoordinatorID: c.id,
		Node:          nodeRaw,
		Status:        database.TailnetStatusOk,
	})
	require.NoError(c.t, err)
}
