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
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

// UpdateGoldenFiles indicates golden files should be updated.
// To update the golden files:
// make gen/golden-files
var UpdateGoldenFiles = flag.Bool("update", false, "update .golden files")

func TestDebugTemplate(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("newlines screw up golden files on windows")
	}
	p1 := uuid.MustParse("01000000-2222-2222-2222-222222222222")
	p2 := uuid.MustParse("02000000-2222-2222-2222-222222222222")
	in := HTMLDebug{
		Peers: []HTMLPeer{
			{
				Name:         "Peer 1",
				ID:           p1,
				LastWriteAge: 5 * time.Second,
				Node:         `id:1 preferred_derp:999 endpoints:"192.168.0.49:4449"`,
				CreatedAge:   87 * time.Second,
				Overwrites:   0,
			},
			{
				Name:         "Peer 2",
				ID:           p2,
				LastWriteAge: 7 * time.Second,
				Node:         `id:2 preferred_derp:999 endpoints:"192.168.0.33:4449"`,
				CreatedAge:   time.Hour,
				Overwrites:   2,
			},
		},
		Tunnels: []HTMLTunnel{
			{
				Src: p1,
				Dst: p2,
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

// TestCoreDisconnectIgnoresReadyForHandshake sends Disconnect and
// ReadyForHandshake in one request. Disconnect closes the peer's response
// channel, so the handler must stop there instead of trying to answer the
// handshake on a closed channel. Other peers still see DISCONNECTED.
func TestCoreDisconnectIgnoresReadyForHandshake(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	c := newCore(logger)
	t.Cleanup(func() { require.NoError(t, c.close()) })

	type testPeer struct {
		*peer
		resps chan *proto.CoordinateResponse
	}
	newPeer := func(name string, id uuid.UUID, auth CoordinateeAuth) testPeer {
		resps := make(chan *proto.CoordinateResponse, ResponseBufferSize)
		p := &peer{
			logger: logger.With(slog.F("peer_name", name)),
			id:     id,
			name:   name,
			resps:  resps,
			reqs:   make(chan *proto.CoordinateRequest, RequestBufferSize),
			auth:   auth,
			sent:   make(map[uuid.UUID]*proto.Node),
		}
		require.NoError(t, c.initPeer(p))
		return testPeer{peer: p, resps: resps}
	}
	observer := newPeer("observer", uuid.New(), SingleTailnetCoordinateeAuth{})
	leaverID := uuid.New()
	leaver := newPeer("leaver", leaverID, AgentCoordinateeAuth{ID: leaverID})

	require.NoError(t, c.handleRequest(ctx, observer.peer, &proto.CoordinateRequest{
		AddTunnel: &proto.CoordinateRequest_Tunnel{Id: leaver.id[:]},
	}))
	require.NoError(t, c.handleRequest(ctx, leaver.peer, &proto.CoordinateRequest{
		UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{PreferredDerp: 1}},
	}))
	nodeUpdate := testutil.RequireReceive(ctx, t, observer.resps)
	require.Len(t, nodeUpdate.PeerUpdates, 1)
	require.Equal(t, proto.CoordinateResponse_PeerUpdate_NODE, nodeUpdate.PeerUpdates[0].Kind)

	stranger := uuid.New()
	var err error
	require.NotPanics(t, func() {
		err = c.handleRequest(ctx, leaver.peer, &proto.CoordinateRequest{
			Disconnect:        &proto.CoordinateRequest_Disconnect{},
			ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{{Id: stranger[:]}},
		})
	})
	require.NoError(t, err)

	// The leaver's channel closes without an error response.
	for closed := false; !closed; {
		select {
		case resp, ok := <-leaver.resps:
			if !ok {
				closed = true
				continue
			}
			require.Empty(t, resp.Error)
		case <-ctx.Done():
			t.Fatal("leaver response channel was not closed")
		}
	}
	c.mutex.RLock()
	_, stillPresent := c.peers[leaver.id]
	c.mutex.RUnlock()
	require.False(t, stillPresent)

	disconnected := testutil.RequireReceive(ctx, t, observer.resps)
	require.Len(t, disconnected.PeerUpdates, 1)
	gotID, err := uuid.FromBytes(disconnected.PeerUpdates[0].Id)
	require.NoError(t, err)
	require.Equal(t, leaver.id, gotID)
	require.Equal(t, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, disconnected.PeerUpdates[0].Kind)
}

// TestCoreRemovePeerNestedRemoval sets up two tunneled peers whose response
// buffers are both full, then removes one of them. Notifying the other peer
// fails with ErrWouldBlock, which removes that peer too, and notifying back
// removes the first peer inside the nested call. The outer removePeerLocked
// must notice its peer is already gone instead of closing the channel again.
// Both request handling and lostPeer cleanup must finish without relying on
// panic recovery.
func TestCoreRemovePeerNestedRemoval(t *testing.T) {
	t.Parallel()

	type fullPeers struct {
		core   *core
		a, b   *peer
		aResps chan *proto.CoordinateResponse
		bResps chan *proto.CoordinateResponse
	}
	// setup returns a core with peers a and b sharing a tunnel, where each
	// peer's single-slot response buffer already holds the other's node.
	setup := func(t *testing.T) fullPeers {
		// The full buffers make updateTunnelPeersLocked log "failed to update
		// mapping" at Error, which is the expected trigger. Anything else at
		// Error or Critical, such as "removed non-existent peer", still fails
		// the test.
		logger := slogtest.Make(t, &slogtest.Options{
			IgnoreErrorFn: func(ent slog.SinkEntry) bool {
				return ent.Message == "failed to update mapping"
			},
		}).Leveled(slog.LevelDebug)
		c := newCore(logger)
		t.Cleanup(func() { require.NoError(t, c.close()) })
		newPeer := func(name string) (*peer, chan *proto.CoordinateResponse) {
			resps := make(chan *proto.CoordinateResponse, 1)
			p := &peer{
				logger: logger.With(slog.F("peer_name", name)),
				id:     uuid.New(),
				name:   name,
				resps:  resps,
				reqs:   make(chan *proto.CoordinateRequest, RequestBufferSize),
				auth:   SingleTailnetCoordinateeAuth{},
				sent:   make(map[uuid.UUID]*proto.Node),
			}
			require.NoError(t, c.initPeer(p))
			return p, resps
		}
		a, aResps := newPeer("a")
		b, bResps := newPeer("b")

		c.mutex.Lock()
		defer c.mutex.Unlock()
		// No tunnel yet, so these updates are not sent anywhere.
		require.NoError(t, c.nodeUpdateLocked(a, &proto.Node{PreferredDerp: 1}))
		require.NoError(t, c.nodeUpdateLocked(b, &proto.Node{PreferredDerp: 2}))
		// Adding the tunnel sends each node to the other peer, which fills
		// both buffers.
		require.NoError(t, c.addTunnelLocked(a, b.id))
		require.Len(t, aResps, 1)
		require.Len(t, bResps, 1)
		require.Contains(t, a.sent, b.id)
		require.Contains(t, b.sent, a.id)
		return fullPeers{core: c, a: a, b: b, aResps: aResps, bResps: bResps}
	}

	requireClosed := func(ctx context.Context, t *testing.T, ch chan *proto.CoordinateResponse) {
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return
				}
			case <-ctx.Done():
				t.Fatal("response channel was not closed")
			}
		}
	}

	requireBothRemoved := func(ctx context.Context, t *testing.T, fp fullPeers) {
		fp.core.mutex.RLock()
		_, aPresent := fp.core.peers[fp.a.id]
		_, bPresent := fp.core.peers[fp.b.id]
		aTunnels := fp.core.tunnels.findTunnelPeers(fp.a.id)
		bTunnels := fp.core.tunnels.findTunnelPeers(fp.b.id)
		fp.core.mutex.RUnlock()
		require.False(t, aPresent)
		require.False(t, bPresent)
		require.Empty(t, aTunnels)
		require.Empty(t, bTunnels)
		requireClosed(ctx, t, fp.aResps)
		requireClosed(ctx, t, fp.bResps)
	}

	for _, name := range []string{"UpdateSelf", "UpdateSelfAddTunnel", "UpdateSelfReadyForHandshake"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			fp := setup(t)
			req := &proto.CoordinateRequest{
				UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{PreferredDerp: 3}},
			}
			dstID := uuid.New()
			switch name {
			case "UpdateSelfAddTunnel":
				req.AddTunnel = &proto.CoordinateRequest_Tunnel{Id: dstID[:]}
			case "UpdateSelfReadyForHandshake":
				req.ReadyForHandshake = []*proto.CoordinateRequest_ReadyForHandshake{{Id: dstID[:]}}
			}
			var err error
			require.NotPanics(t, func() {
				err = fp.core.handleRequest(ctx, fp.a, req)
			})
			require.NoError(t, err)
			requireBothRemoved(ctx, t, fp)
		})
	}

	t.Run("LostPeer", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		fp := setup(t)
		var err error
		require.NotPanics(t, func() {
			err = fp.core.lostPeer(fp.a, "")
		})
		require.NoError(t, err)
		requireBothRemoved(ctx, t, fp)
	})
}
