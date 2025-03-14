package tailnet
import (
	"errors"
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	gProto "google.golang.org/protobuf/proto"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/quartz"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)
// UpdateGoldenFiles indicates golden files should be updated.
// To update the golden files:
// make update-golden-files
var UpdateGoldenFiles = flag.Bool("update", false, "update .golden files")
// TestHeartbeats_Cleanup tests the cleanup loop
func TestHeartbeats_Cleanup(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := testutil.Logger(t)
	mStore.EXPECT().CleanTailnetCoordinators(gomock.Any()).Times(2).Return(nil)
	mStore.EXPECT().CleanTailnetLostPeers(gomock.Any()).Times(2).Return(nil)
	mStore.EXPECT().CleanTailnetTunnels(gomock.Any()).Times(2).Return(nil)
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().TickerFunc("heartbeats", "cleanupLoop")
	defer trap.Close()
	uut := &heartbeats{
		ctx:    ctx,
		logger: logger,
		store:  mStore,
		clock:  mClock,
	}
	uut.wg.Add(1)
	go uut.cleanupLoop()
	call, err := trap.Wait(ctx)
	require.NoError(t, err)
	call.Release()
	require.Equal(t, cleanupPeriod, call.Duration)
	mClock.Advance(cleanupPeriod).MustWait(ctx)
}
// TestHeartbeats_recvBeat_resetSkew is a regression test for a bug where heartbeats from two
// coordinators slightly skewed from one another could result in one coordinator failing to get
// expired
func TestHeartbeats_recvBeat_resetSkew(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().Until("heartbeats", "resetExpiryTimerWithLock")
	defer trap.Close()
	uut := heartbeats{
		ctx:          ctx,
		logger:       logger,
		clock:        mClock,
		self:         uuid.UUID{1},
		update:       make(chan hbUpdate, 4),
		coordinators: make(map[uuid.UUID]time.Time),
	}
	coord2 := uuid.UUID{2}
	coord3 := uuid.UUID{3}
	uut.listen(ctx, []byte(coord2.String()), nil)
	// coord 3 heartbeat comes very soon after
	mClock.Advance(time.Millisecond).MustWait(ctx)
	go uut.listen(ctx, []byte(coord3.String()), nil)
	trap.MustWait(ctx).Release()
	// both coordinators are present
	uut.lock.RLock()
	require.Contains(t, uut.coordinators, coord2)
	require.Contains(t, uut.coordinators, coord3)
	uut.lock.RUnlock()
	// no more heartbeats arrive, and coord2 expires
	w := mClock.Advance(MissedHeartbeats*HeartbeatPeriod - time.Millisecond)
	// however, several ms pass between expiring 2 and computing the time until 3 expires
	c := trap.MustWait(ctx)
	mClock.Advance(2 * time.Millisecond).MustWait(ctx) // 3 has now expired _in the past_
	c.Release()
	w.MustWait(ctx)
	// expired in the past means we immediately reschedule checkExpiry, so we get another call
	trap.MustWait(ctx).Release()
	uut.lock.RLock()
	require.NotContains(t, uut.coordinators, coord2)
	require.NotContains(t, uut.coordinators, coord3)
	uut.lock.RUnlock()
}
func TestHeartbeats_LostCoordinator_MarkLost(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)
	mClock := quartz.NewMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := testutil.Logger(t)
	uut := &heartbeats{
		ctx:    ctx,
		logger: logger,
		store:  mStore,
		coordinators: map[uuid.UUID]time.Time{
			uuid.New(): mClock.Now(),
		},
		clock: mClock,
	}
	mpngs := []mapping{{
		peer:        uuid.New(),
		coordinator: uuid.New(),
		updatedAt:   mClock.Now(),
		node:        &proto.Node{},
		kind:        proto.CoordinateResponse_PeerUpdate_NODE,
	}}
	// Filter should still return the mapping without a coordinator, but marked
	// as LOST.
	got := uut.filter(mpngs)
	require.Len(t, got, 1)
	assert.Equal(t, proto.CoordinateResponse_PeerUpdate_LOST, got[0].kind)
}
// TestLostPeerCleanupQueries tests that our SQL queries to clean up lost peers do what we expect,
// that is, clean up peers and associated tunnels that have been lost for over 24 hours.
func TestLostPeerCleanupQueries(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	coordID := uuid.New()
	_, err := store.UpsertTailnetCoordinator(ctx, coordID)
	require.NoError(t, err)
	peerID := uuid.New()
	_, err = store.UpsertTailnetPeer(ctx, database.UpsertTailnetPeerParams{
		ID:            peerID,
		CoordinatorID: coordID,
		Node:          []byte("test"),
		Status:        database.TailnetStatusLost,
	})
	require.NoError(t, err)
	otherID := uuid.New()
	_, err = store.UpsertTailnetTunnel(ctx, database.UpsertTailnetTunnelParams{
		CoordinatorID: coordID,
		SrcID:         peerID,
		DstID:         otherID,
	})
	require.NoError(t, err)
	peers, err := store.GetAllTailnetPeers(ctx)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, peerID, peers[0].ID)
	tunnels, err := store.GetAllTailnetTunnels(ctx)
	require.NoError(t, err)
	require.Len(t, tunnels, 1)
	require.Equal(t, peerID, tunnels[0].SrcID)
	require.Equal(t, otherID, tunnels[0].DstID)
	// this clean is a noop since the peer and tunnel are less than 24h old
	err = store.CleanTailnetLostPeers(ctx)
	require.NoError(t, err)
	err = store.CleanTailnetTunnels(ctx)
	require.NoError(t, err)
	peers, err = store.GetAllTailnetPeers(ctx)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, peerID, peers[0].ID)
	tunnels, err = store.GetAllTailnetTunnels(ctx)
	require.NoError(t, err)
	require.Len(t, tunnels, 1)
	require.Equal(t, peerID, tunnels[0].SrcID)
	require.Equal(t, otherID, tunnels[0].DstID)
	// set the age of the tunnel to >24h
	sqlDB.Exec("UPDATE tailnet_tunnels SET updated_at = $1", time.Now().Add(-25*time.Hour))
	// this clean is still a noop since the peer hasn't been lost for 24 hours
	err = store.CleanTailnetLostPeers(ctx)
	require.NoError(t, err)
	err = store.CleanTailnetTunnels(ctx)
	require.NoError(t, err)
	peers, err = store.GetAllTailnetPeers(ctx)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, peerID, peers[0].ID)
	tunnels, err = store.GetAllTailnetTunnels(ctx)
	require.NoError(t, err)
	require.Len(t, tunnels, 1)
	require.Equal(t, peerID, tunnels[0].SrcID)
	require.Equal(t, otherID, tunnels[0].DstID)
	// set the age of the tunnel to >24h
	sqlDB.Exec("UPDATE tailnet_peers SET updated_at = $1", time.Now().Add(-25*time.Hour))
	// this clean removes the peer and the associated tunnel
	err = store.CleanTailnetLostPeers(ctx)
	require.NoError(t, err)
	err = store.CleanTailnetTunnels(ctx)
	require.NoError(t, err)
	peers, err = store.GetAllTailnetPeers(ctx)
	require.NoError(t, err)
	require.Len(t, peers, 0)
	tunnels, err = store.GetAllTailnetTunnels(ctx)
	require.NoError(t, err)
	require.Len(t, tunnels, 0)
}
func TestDebugTemplate(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("newlines screw up golden files on windows")
	}
	c1 := uuid.MustParse("01000000-1111-1111-1111-111111111111")
	c2 := uuid.MustParse("02000000-1111-1111-1111-111111111111")
	p1 := uuid.MustParse("01000000-2222-2222-2222-222222222222")
	p2 := uuid.MustParse("02000000-2222-2222-2222-222222222222")
	in := HTMLDebug{
		Coordinators: []*HTMLCoordinator{
			{
				ID:           c1,
				HeartbeatAge: 2 * time.Second,
			},
			{
				ID:           c2,
				HeartbeatAge: time.Second,
			},
		},
		Peers: []*HTMLPeer{
			{
				ID:            p1,
				CoordinatorID: c1,
				LastWriteAge:  5 * time.Second,
				Status:        database.TailnetStatusOk,
				Node:          `id:1 preferred_derp:999 endpoints:"192.168.0.49:4449"`,
			},
			{
				ID:            p2,
				CoordinatorID: c2,
				LastWriteAge:  7 * time.Second,
				Status:        database.TailnetStatusLost,
				Node:          `id:2 preferred_derp:999 endpoints:"192.168.0.33:4449"`,
			},
		},
		Tunnels: []*HTMLTunnel{
			{
				CoordinatorID: c1,
				SrcID:         p1,
				DstID:         p2,
				LastWriteAge:  3 * time.Second,
			},
		},
	}
	buf := new(bytes.Buffer)
	err := debugTempl.Execute(buf, in)
	require.NoError(t, err)
	actual := buf.Bytes()
	goldenPath := filepath.Join("testdata", "debug.golden.html")
	if *UpdateGoldenFiles {
		t.Logf("update golden file %s", goldenPath)
		err := os.WriteFile(goldenPath, actual, 0o600)
		require.NoError(t, err, "update golden file")
	}
	expected, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "read golden file, run \"make update-golden-files\" and commit the changes")
	require.Equal(
		t, string(expected), string(actual),
		"golden file mismatch: %s, run \"make update-golden-files\", verify and commit the changes",
		goldenPath,
	)
}
func TestGetDebug(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, _ := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	coordID := uuid.New()
	_, err := store.UpsertTailnetCoordinator(ctx, coordID)
	require.NoError(t, err)
	peerID := uuid.New()
	node := &proto.Node{PreferredDerp: 44}
	nodeb, err := gProto.Marshal(node)
	require.NoError(t, err)
	_, err = store.UpsertTailnetPeer(ctx, database.UpsertTailnetPeerParams{
		ID:            peerID,
		CoordinatorID: coordID,
		Node:          nodeb,
		Status:        database.TailnetStatusLost,
	})
	require.NoError(t, err)
	dstID := uuid.New()
	_, err = store.UpsertTailnetTunnel(ctx, database.UpsertTailnetTunnelParams{
		CoordinatorID: coordID,
		SrcID:         peerID,
		DstID:         dstID,
	})
	require.NoError(t, err)
	debug, err := getDebug(ctx, store)
	require.NoError(t, err)
	require.Len(t, debug.Coordinators, 1)
	require.Len(t, debug.Peers, 1)
	require.Len(t, debug.Tunnels, 1)
	require.Equal(t, coordID, debug.Coordinators[0].ID)
	require.Equal(t, peerID, debug.Peers[0].ID)
	require.Equal(t, coordID, debug.Peers[0].CoordinatorID)
	require.Equal(t, database.TailnetStatusLost, debug.Peers[0].Status)
	require.Equal(t, node.String(), debug.Peers[0].Node)
	require.Equal(t, coordID, debug.Tunnels[0].CoordinatorID)
	require.Equal(t, peerID, debug.Tunnels[0].SrcID)
	require.Equal(t, dstID, debug.Tunnels[0].DstID)
}
// TestPGCoordinatorUnhealthy tests that when the coordinator fails to send heartbeats and is
// unhealthy it disconnects any peers and does not send any extraneous database queries.
func TestPGCoordinatorUnhealthy(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)
	ps := pubsub.NewInMemory()
	mClock := quartz.NewMock(t)
	tfTrap := mClock.Trap().TickerFunc("heartbeats", "sendBeats")
	defer tfTrap.Close()
	// after 3 failed heartbeats, the coordinator is unhealthy
	mStore.EXPECT().
		UpsertTailnetCoordinator(gomock.Any(), gomock.Any()).
		Times(3).
		Return(database.TailnetCoordinator{}, errors.New("badness"))
	// But, in particular we DO NOT want the coordinator to call DeleteTailnetPeer, as this is
	// unnecessary and can spam the database. c.f. https://github.com/coder/coder/issues/12923
	// these cleanup queries run, but we don't care for this test
	mStore.EXPECT().CleanTailnetCoordinators(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().CleanTailnetLostPeers(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().CleanTailnetTunnels(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().UpdateTailnetPeerStatusByCoordinator(gomock.Any(), gomock.Any())
	coordinator, err := newPGCoordInternal(ctx, logger, ps, mStore, mClock)
	require.NoError(t, err)
	expectedPeriod := HeartbeatPeriod
	tfCall, err := tfTrap.Wait(ctx)
	require.NoError(t, err)
	tfCall.Release()
	require.Equal(t, expectedPeriod, tfCall.Duration)
	// Now that the ticker has started, we can advance 2 more beats to get to 3
	// failed heartbeats
	mClock.Advance(HeartbeatPeriod).MustWait(ctx)
	mClock.Advance(HeartbeatPeriod).MustWait(ctx)
	// The querier is informed async about being unhealthy, so we need to wait
	// until it is.
	require.Eventually(t, func() bool {
		return !coordinator.querier.isHealthy()
	}, testutil.WaitShort, testutil.IntervalFast)
	pID := uuid.UUID{5}
	_, resps := coordinator.Coordinate(ctx, pID, "test", agpl.AgentCoordinateeAuth{ID: pID})
	resp := testutil.RequireRecvCtx(ctx, t, resps)
	require.Nil(t, resp, "channel should be closed")
	// give the coordinator some time to process any pending work.  We are
	// testing here that a database call is absent, so we don't want to race to
	// shut down the test.
	time.Sleep(testutil.IntervalMedium)
	_ = coordinator.Close()
	require.Eventually(t, ctrl.Satisfied, testutil.WaitShort, testutil.IntervalFast)
}
