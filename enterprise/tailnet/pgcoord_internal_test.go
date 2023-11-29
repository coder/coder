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

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gProto "google.golang.org/protobuf/proto"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

// UpdateGoldenFiles indicates golden files should be updated.
// To update the golden files:
// make update-golden-files
var UpdateGoldenFiles = flag.Bool("update", false, "update .golden files")

// TestHeartbeat_Cleanup is internal so that we can overwrite the cleanup period and not wait an hour for the timed
// cleanup.
func TestHeartbeat_Cleanup(t *testing.T) {
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

	uut := &heartbeats{
		ctx:           ctx,
		logger:        logger,
		store:         mStore,
		cleanupPeriod: time.Millisecond,
	}
	go uut.cleanupLoop()

	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			t.Fatal("timeout")
		case waitForCleanup <- struct{}{}:
			// ok
		}
	}
	close(waitForCleanup)
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
