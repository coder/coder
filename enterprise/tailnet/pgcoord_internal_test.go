package tailnet

import (
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
	"golang.org/x/xerrors"
	gProto "google.golang.org/protobuf/proto"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

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

// TestHeartbeats_Cleanup is internal so that we can overwrite the cleanup period and not wait an hour for the timed
// cleanup.
func TestHeartbeats_Cleanup(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	waitForCleanup := make(chan struct{})
	mStore.EXPECT().CleanTailnetCoordinators(gomock.Any()).MinTimes(2).DoAndReturn(func(_ context.Context) error {
		<-waitForCleanup
		return nil
	})
	mStore.EXPECT().CleanTailnetLostPeers(gomock.Any()).MinTimes(2).DoAndReturn(func(_ context.Context) error {
		<-waitForCleanup
		return nil
	})
	mStore.EXPECT().CleanTailnetTunnels(gomock.Any()).MinTimes(2).DoAndReturn(func(_ context.Context) error {
		<-waitForCleanup
		return nil
	})

	uut := &heartbeats{
		ctx:           ctx,
		logger:        logger,
		store:         mStore,
		cleanupPeriod: time.Millisecond,
	}
	go uut.cleanupLoop()

	for i := 0; i < 6; i++ {
		select {
		case <-ctx.Done():
			t.Fatal("timeout")
		case waitForCleanup <- struct{}{}:
			// ok
		}
	}
	close(waitForCleanup)
}

func TestHeartbeats_LostCoordinator_MarkLost(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	uut := &heartbeats{
		ctx:           ctx,
		logger:        logger,
		store:         mStore,
		cleanupPeriod: time.Millisecond,
		coordinators: map[uuid.UUID]time.Time{
			uuid.New(): time.Now(),
		},
	}

	mpngs := []mapping{{
		peer:        uuid.New(),
		coordinator: uuid.New(),
		updatedAt:   time.Now(),
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

	// after 3 failed heartbeats, the coordinator is unhealthy
	mStore.EXPECT().
		UpsertTailnetCoordinator(gomock.Any(), gomock.Any()).
		MinTimes(3).
		Return(database.TailnetCoordinator{}, xerrors.New("badness"))
	mStore.EXPECT().
		DeleteCoordinator(gomock.Any(), gomock.Any()).
		Times(1).
		Return(nil)
	// But, in particular we DO NOT want the coordinator to call DeleteTailnetPeer, as this is
	// unnecessary and can spam the database. c.f. https://github.com/coder/coder/issues/12923

	// these cleanup queries run, but we don't care for this test
	mStore.EXPECT().CleanTailnetCoordinators(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().CleanTailnetLostPeers(gomock.Any()).AnyTimes().Return(nil)
	mStore.EXPECT().CleanTailnetTunnels(gomock.Any()).AnyTimes().Return(nil)

	coordinator, err := newPGCoordInternal(ctx, logger, ps, mStore)
	require.NoError(t, err)

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
