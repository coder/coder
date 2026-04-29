package tailnet

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"golang.org/x/xerrors"
	gProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// UpdateGoldenFiles indicates golden files should be updated.
// To update the golden files:
// make gen/golden-files
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
	mStore.EXPECT().CleanTailnetLostPeers(gomock.Any()).Times(2).Return(nil, nil)
	mStore.EXPECT().CleanTailnetTunnels(gomock.Any()).Times(2).Return(nil, nil)

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

	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
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
	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)
	mStore.EXPECT().GetAllTailnetCoordinators(gomock.Any()).
		Return(nil, nil).AnyTimes()
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().Until("heartbeats", "resetExpiryTimerWithLock")
	defer trap.Close()

	uut := heartbeats{
		ctx:                 ctx,
		logger:              logger,
		store:               mStore,
		clock:               mClock,
		self:                uuid.UUID{1},
		update:              make(chan hbUpdate, 4),
		coordinators:        make(map[uuid.UUID]time.Time),
		lastDBHeartbeat:     make(map[uuid.UUID]time.Time),
		expiredCoordinators: make(map[uuid.UUID]struct{}),
	}
	uut.timer = mClock.AfterFunc(MissedHeartbeats*HeartbeatPeriod, uut.checkExpiry, "heartbeats", "newHeartbeats")

	coord2 := uuid.UUID{2}
	coord3 := uuid.UUID{3}

	go uut.listen(ctx, []byte(coord2.String()), nil)
	trap.MustWait(ctx).MustRelease(ctx)

	// coord 3 heartbeat comes very soon after
	mClock.Advance(time.Millisecond).MustWait(ctx)
	go uut.listen(ctx, []byte(coord3.String()), nil)
	trap.MustWait(ctx).MustRelease(ctx)

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
	c.MustRelease(ctx)
	w.MustWait(ctx)

	// expired in the past means we immediately reschedule checkExpiry, so we get another call
	trap.MustWait(ctx).MustRelease(ctx)

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
	_, err = store.CleanTailnetLostPeers(ctx)
	require.NoError(t, err)
	_, err = store.CleanTailnetTunnels(ctx)
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
	_, err = store.CleanTailnetLostPeers(ctx)
	require.NoError(t, err)
	_, err = store.CleanTailnetTunnels(ctx)
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
	_, err = store.CleanTailnetLostPeers(ctx)
	require.NoError(t, err)
	_, err = store.CleanTailnetTunnels(ctx)
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
	require.NoError(t, err, "read golden file, run \"make gen/golden-files\" and commit the changes")

	require.Equal(
		t, string(expected), string(actual),
		"golden file mismatch: %s, run \"make gen/golden-files\", verify and commit the changes",
		goldenPath,
	)
}

func TestGetDebug(t *testing.T) {
	t.Parallel()

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
		Return(database.TailnetCoordinator{}, xerrors.New("badness"))
	// But, in particular we DO NOT want the coordinator to call DeleteTailnetPeer, as this is
	// unnecessary and can spam the database. c.f. https://github.com/coder/coder/issues/12923

	// these cleanup queries run, but we don't care for this test
	mStore.EXPECT().CleanTailnetCoordinators(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().CleanTailnetLostPeers(gomock.Any()).AnyTimes().Return(nil, nil)
	mStore.EXPECT().CleanTailnetTunnels(gomock.Any()).AnyTimes().Return(nil, nil)
	mStore.EXPECT().UpdateTailnetPeerStatusByCoordinator(gomock.Any(), gomock.Any()).Return(nil, nil)

	coordinator, err := newPGCoordInternal(ctx, logger, ps, mStore, mClock, nil)
	require.NoError(t, err)

	expectedPeriod := HeartbeatPeriod
	tfCall, err := tfTrap.Wait(ctx)
	require.NoError(t, err)
	tfCall.MustRelease(ctx)
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
	resp := testutil.RequireReceive(ctx, t, resps)
	require.Equal(t, CloseErrUnhealthy, resp.Error)
	resp = testutil.TryReceive(ctx, t, resps)
	require.Nil(t, resp, "channel should be closed")

	// give the coordinator some time to process any pending work.  We are
	// testing here that a database call is absent, so we don't want to race to
	// shut down the test.
	time.Sleep(testutil.IntervalMedium)
	_ = coordinator.Close()
	require.Eventually(t, ctrl.Satisfied, testutil.WaitShort, testutil.IntervalFast)
}

func TestWorkQ_AcquireBatch_RespectsMax(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := newWorkQ[uuid.UUID](ctx)

	for i := 0; i < 5; i++ {
		q.enqueue(uuid.New())
	}

	batch, err := q.acquireBatch(3)
	require.NoError(t, err)
	assert.Len(t, batch, 3, "should respect max parameter")

	for _, k := range batch {
		q.done(k)
	}

	// Remaining 2 should be available.
	batch, err = q.acquireBatch(10)
	require.NoError(t, err)
	assert.Len(t, batch, 2)

	for _, k := range batch {
		q.done(k)
	}
}

func TestWorkQ_AcquireBatch_SkipsInProgress(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := newWorkQ[uuid.UUID](ctx)

	peer1 := uuid.New()
	peer2 := uuid.New()
	q.enqueue(peer1)
	q.enqueue(peer2)

	// Acquire one item.
	key, err := q.acquire()
	require.NoError(t, err)
	assert.Equal(t, peer1, key)

	// Re-enqueue peer1 (simulating a new update while in progress).
	q.enqueue(peer1)

	// acquireBatch should only return peer2 (peer1 is in progress).
	batch, err := q.acquireBatch(10)
	require.NoError(t, err)
	require.Len(t, batch, 1)
	assert.Equal(t, peer2, batch[0])

	q.done(key)
	for _, k := range batch {
		q.done(k)
	}

	// Now peer1 (re-enqueued) should be available.
	batch, err = q.acquireBatch(10)
	require.NoError(t, err)
	require.Len(t, batch, 1)
	assert.Equal(t, peer1, batch[0])
}

func TestWorkQ_Acquire_WrapsAcquireBatch(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := newWorkQ[uuid.UUID](ctx)

	peer := uuid.New()
	q.enqueue(peer)

	key, err := q.acquire()
	require.NoError(t, err)
	assert.Equal(t, peer, key)
	q.done(key)
}

// TestPeriodicRefresh_EnqueuesAllMapperKeys verifies that periodicRefresh
// enqueues all mapper keys into mappingQ on each tick, without requiring
// a database, pubsub, or real coordinator.
func TestQuerier_periodicRefresh(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := testutil.Logger(t)
	mClock := quartz.NewMock(t)

	// Trap the TickerFunc so we can control when it fires.
	trap := mClock.Trap().TickerFunc("querier", "periodicRefresh")
	defer trap.Close()

	// Build a minimal querier with a few mapper entries.
	q := &querier{
		ctx:      ctx,
		logger:   logger.Named("querier"),
		clock:    mClock,
		mappingQ: newWorkQ[mKey](ctx),
		mappers:  make(map[mKey]*mapper),
	}

	key1 := mKey(uuid.New())
	key2 := mKey(uuid.New())
	key3 := mKey(uuid.New())
	q.mappers[key1] = nil
	q.mappers[key2] = nil
	q.mappers[key3] = nil

	q.wg.Add(1)
	go q.periodicRefresh()

	// Wait for the TickerFunc to be registered, then release it.
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	require.Equal(t, mapperRefreshInterval, call.Duration)

	// Advance the clock to trigger the tick callback.
	mClock.Advance(mapperRefreshInterval).MustWait(ctx)

	// All three keys should now be enqueued in mappingQ.
	batch, err := q.mappingQ.acquireBatch(10)
	require.NoError(t, err)
	require.Len(t, batch, 3)

	got := make(map[mKey]struct{})
	for _, k := range batch {
		got[k] = struct{}{}
	}
	require.Contains(t, got, key1)
	require.Contains(t, got, key2)
	require.Contains(t, got, key3)

	// Clean up: cancel context so periodicRefresh exits.
	cancel()
	q.wg.Wait()
}

func TestNodesEqual(t *testing.T) {
	t.Parallel()

	t.Run("BothNil", func(t *testing.T) {
		t.Parallel()
		assert.True(t, nodesEqual(nil, nil))
	})

	t.Run("OneNil", func(t *testing.T) {
		t.Parallel()
		assert.False(t, nodesEqual(&proto.Node{PreferredDerp: 1}, nil))
		assert.False(t, nodesEqual(nil, &proto.Node{PreferredDerp: 1}))
	})

	t.Run("SameIgnoringAsOf", func(t *testing.T) {
		t.Parallel()
		a := &proto.Node{
			PreferredDerp: 1,
			AsOf:          timestamppb.Now(),
		}
		b := &proto.Node{
			PreferredDerp: 1,
			AsOf:          timestamppb.New(time.Now().Add(-time.Hour)),
		}
		assert.True(t, nodesEqual(a, b))
		// Verify AsOf fields are restored.
		assert.NotNil(t, a.AsOf)
		assert.NotNil(t, b.AsOf)
	})

	t.Run("DifferentPreferredDERP", func(t *testing.T) {
		t.Parallel()
		a := &proto.Node{PreferredDerp: 1}
		b := &proto.Node{PreferredDerp: 2}
		assert.False(t, nodesEqual(a, b))
	})
}

func TestBinderSkipsNoopUpsert(t *testing.T) {
	t.Parallel()

	key := bKey(uuid.New())
	node := &proto.Node{PreferredDerp: 1}

	b := &binder{
		latest: make(map[bKey]binding),
	}

	bnd := binding{bKey: key, node: node, kind: proto.CoordinateResponse_PeerUpdate_NODE}

	// First store should succeed.
	assert.True(t, b.storeBinding(bnd))

	// Same node (even with different AsOf) should be skipped.
	bnd2 := binding{
		bKey: key,
		node: &proto.Node{PreferredDerp: 1, AsOf: timestamppb.Now()},
		kind: proto.CoordinateResponse_PeerUpdate_NODE,
	}
	assert.False(t, b.storeBinding(bnd2))
}

func TestBinderAllowsChangedNode(t *testing.T) {
	t.Parallel()

	key := bKey(uuid.New())

	b := &binder{
		latest: make(map[bKey]binding),
	}

	bnd1 := binding{bKey: key, node: &proto.Node{PreferredDerp: 1}, kind: proto.CoordinateResponse_PeerUpdate_NODE}
	assert.True(t, b.storeBinding(bnd1))

	bnd2 := binding{bKey: key, node: &proto.Node{PreferredDerp: 2}, kind: proto.CoordinateResponse_PeerUpdate_NODE}
	assert.True(t, b.storeBinding(bnd2))
}

// newTestQuerier constructs a minimal querier suitable for testing the
// pubsub decode and dedup paths in isolation. It does not start any
// workers; callers drive the methods they care about directly.
func newTestQuerier(t *testing.T, store database.Store) (*querier, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	// Some tests intentionally feed malformed payloads; the
	// listenPeer fallback path logs at ERROR level when a UUID-string
	// parse fails, which is the desired production behavior.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	q := &querier{
		ctx:                ctx,
		logger:             logger,
		store:              store,
		mappers:            make(map[mKey]*mapper),
		peerUpdateQ:        newWorkQ[uuid.UUID](ctx),
		mappingQ:           newWorkQ[mKey](ctx),
		enrichedUpdates:    make(map[uuid.UUID]*proto.TailnetPeerUpdate),
		peerUpdateLastSeen: make(map[uuid.UUID]time.Time),
		clock:              quartz.NewMock(t),
		healthy:            true,
	}
	return q, cancel
}

// encodeEnrichedPeerUpdate produces the on-the-wire bytes for a
// TailnetPeerUpdate as published over PG pubsub: a base64-encoded
// proto. This mirrors what publishPeerUpdate emits.
func encodeEnrichedPeerUpdate(t *testing.T, peerID, coordID uuid.UUID, status int32, node []byte, updatedAt time.Time) []byte {
	t.Helper()
	raw, err := gProto.Marshal(&proto.TailnetPeerUpdate{
		PeerId:        peerID[:],
		CoordinatorId: coordID[:],
		Status:        status,
		Node:          node,
		UpdatedAt:     timestamppb.New(updatedAt),
	})
	require.NoError(t, err)
	encoded := []byte(base64.StdEncoding.EncodeToString(raw))
	return encoded
}

// TestQuerier_ListenPeer_EnrichedStaleness verifies that an enriched
// peer update with an older updated_at than a previously seen update
// is dropped before reaching peerUpdateQ, while strictly-newer updates
// replace the stored payload and re-enqueue.
func TestQuerier_ListenPeer_EnrichedStaleness(t *testing.T) {
	t.Parallel()
	q, cancel := newTestQuerier(t, nil)
	defer cancel()

	peerID := uuid.New()
	coordID := uuid.New()
	t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	t0 := t1.Add(-time.Minute)

	nodeNew, err := gProto.Marshal(&proto.Node{PreferredDerp: 9})
	require.NoError(t, err)
	nodeOld, err := gProto.Marshal(&proto.Node{PreferredDerp: 1})
	require.NoError(t, err)

	// Newer update first.
	q.listenPeer(context.Background(),
		encodeEnrichedPeerUpdate(t, peerID, coordID, peerUpdateStatusOK, nodeNew, t1), nil)

	q.peerUpdatesMu.Lock()
	stored, ok := q.enrichedUpdates[peerID]
	require.True(t, ok)
	require.Equal(t, nodeNew, stored.GetNode())
	require.Equal(t, t1, q.peerUpdateLastSeen[peerID])
	q.peerUpdatesMu.Unlock()

	// Older update should be dropped: stored payload must remain the
	// newer one and last-seen must not regress.
	q.listenPeer(context.Background(),
		encodeEnrichedPeerUpdate(t, peerID, coordID, peerUpdateStatusOK, nodeOld, t0), nil)
	q.peerUpdatesMu.Lock()
	require.Equal(t, nodeNew, q.enrichedUpdates[peerID].GetNode(),
		"older update should not overwrite newer payload")
	require.Equal(t, t1, q.peerUpdateLastSeen[peerID])
	q.peerUpdatesMu.Unlock()

	// A strictly-newer update should replace.
	t2 := t1.Add(time.Minute)
	nodeNewer, err := gProto.Marshal(&proto.Node{PreferredDerp: 42})
	require.NoError(t, err)
	q.listenPeer(context.Background(),
		encodeEnrichedPeerUpdate(t, peerID, coordID, peerUpdateStatusOK, nodeNewer, t2), nil)
	q.peerUpdatesMu.Lock()
	require.Equal(t, nodeNewer, q.enrichedUpdates[peerID].GetNode())
	require.Equal(t, t2, q.peerUpdateLastSeen[peerID])
	q.peerUpdatesMu.Unlock()

	// A "lost" status should drop the last-seen entry so the map is
	// bounded by currently-active peers.
	q.listenPeer(context.Background(),
		encodeEnrichedPeerUpdate(t, peerID, coordID, peerUpdateStatusLost, nil, t2.Add(time.Second)), nil)
	q.peerUpdatesMu.Lock()
	_, hasLastSeen := q.peerUpdateLastSeen[peerID]
	require.False(t, hasLastSeen, "lost status should remove peer from last-seen map")
	q.peerUpdatesMu.Unlock()
}

// TestQuerier_ListenPeer_DecodeFailureFallback verifies that:
//   - garbage bytes do not panic and do not populate enrichedUpdates
//   - a legacy UUID-string update enqueues the peer ID for the worker
//   - the worker (peerUpdate) runs the full DB-query path
//     (GetTailnetTunnelPeerIDsBatch) when no enriched payload is
//     present, preserving today's behavior
//   - a subsequent valid enriched update is processed normally.
func TestQuerier_ListenPeer_DecodeFailureFallback(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)
	q, cancel := newTestQuerier(t, mStore)
	defer cancel()

	peerID := uuid.New()
	coordID := uuid.New()

	// 1. Garbage bytes: must not panic, must not populate enriched
	// state. The bytes below are not valid base64 and not a UUID.
	q.listenPeer(context.Background(), []byte{0xff, 0xfe, 0x00, 0x01}, nil)
	q.peerUpdatesMu.Lock()
	require.Empty(t, q.enrichedUpdates)
	require.Empty(t, q.peerUpdateLastSeen)
	q.peerUpdatesMu.Unlock()

	// 2. Legacy UUID-string update: drives the fallback path and
	// enqueues the peer ID. Verify by acquiring from peerUpdateQ.
	q.listenPeer(context.Background(), []byte(peerID.String()), nil)
	q.peerUpdatesMu.Lock()
	require.Empty(t, q.enrichedUpdates,
		"UUID-string fallback must not populate enrichedUpdates")
	q.peerUpdatesMu.Unlock()

	got, err := q.peerUpdateQ.acquireBatch(10)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{peerID}, got)
	q.peerUpdateQ.done(got...)

	// 3. The worker (peerUpdate) for a UUID-only fallback must run
	// the full DB query path, preserving pre-§6b behavior.
	mStore.EXPECT().
		GetTailnetTunnelPeerIDsBatch(gomock.Any(), []uuid.UUID{peerID}).
		Times(1).
		Return(nil, nil)
	require.NoError(t, q.peerUpdate([]uuid.UUID{peerID}))

	// 4. A subsequent valid enriched update is processed normally
	// (no panic, populates enrichedUpdates).
	updatedAt := time.Now().UTC()
	q.listenPeer(context.Background(),
		encodeEnrichedPeerUpdate(t, peerID, coordID, peerUpdateStatusOK, nil, updatedAt), nil)
	q.peerUpdatesMu.Lock()
	stored, ok := q.enrichedUpdates[peerID]
	require.True(t, ok, "valid enriched update after decode failure must be stored")
	require.Equal(t, peerID[:], stored.GetPeerId())
	q.peerUpdatesMu.Unlock()
}

func TestBinderLostToNodeTransition(t *testing.T) {
	t.Parallel()

	key := bKey(uuid.New())

	b := &binder{
		latest: make(map[bKey]binding),
	}

	node := &proto.Node{PreferredDerp: 1}

	// NODE should enqueue.
	bnd1 := binding{bKey: key, node: node, kind: proto.CoordinateResponse_PeerUpdate_NODE}
	assert.True(t, b.storeBinding(bnd1))

	// LOST should enqueue (transitions state).
	bnd2 := binding{bKey: key, kind: proto.CoordinateResponse_PeerUpdate_LOST}
	assert.True(t, b.storeBinding(bnd2))

	// NODE again should enqueue (transitioning back from LOST).
	bnd3 := binding{bKey: key, node: node, kind: proto.CoordinateResponse_PeerUpdate_NODE}
	assert.True(t, b.storeBinding(bnd3))
}
