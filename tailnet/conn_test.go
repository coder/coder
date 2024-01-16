package tailnet_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTailnet(t *testing.T) {
	t.Parallel()
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	t.Run("InstantClose", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		conn, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
			Logger:    logger.Named("w1"),
			DERPMap:   derpMap,
		})
		require.NoError(t, err)
		err = conn.Close()
		require.NoError(t, err)
	})
	t.Run("Connect", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitLong)
		w1IP := tailnet.IP()
		w1, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
			Logger:    logger.Named("w1"),
			DERPMap:   derpMap,
		})
		require.NoError(t, err)

		w2, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
			Logger:    logger.Named("w2"),
			DERPMap:   derpMap,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = w1.Close()
			_ = w2.Close()
		})
		stitch(t, w2, w1)
		stitch(t, w1, w2)
		require.True(t, w2.AwaitReachable(context.Background(), w1IP))
		conn := make(chan struct{}, 1)
		go func() {
			listener, err := w1.Listen("tcp", ":35565")
			assert.NoError(t, err)
			defer listener.Close()
			nc, err := listener.Accept()
			if !assert.NoError(t, err) {
				return
			}
			_ = nc.Close()
			conn <- struct{}{}
		}()

		nc, err := w2.DialContextTCP(context.Background(), netip.AddrPortFrom(w1IP, 35565))
		require.NoError(t, err)
		_ = nc.Close()
		<-conn

		nodes := make(chan *tailnet.Node, 1)
		w2.SetNodeCallback(func(node *tailnet.Node) {
			select {
			case nodes <- node:
			default:
			}
		})
		node := testutil.RequireRecvCtx(ctx, t, nodes)
		// Ensure this connected over DERP!
		require.Len(t, node.DERPForcedWebsocket, 0)

		w1.Close()
		w2.Close()
	})

	t.Run("ForcesWebSockets", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitMedium)

		w1IP := tailnet.IP()
		derpMap := tailnettest.RunDERPOnlyWebSockets(t)
		w1, err := tailnet.NewConn(&tailnet.Options{
			Addresses:      []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
			Logger:         logger.Named("w1"),
			DERPMap:        derpMap,
			BlockEndpoints: true,
		})
		require.NoError(t, err)

		w2, err := tailnet.NewConn(&tailnet.Options{
			Addresses:      []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
			Logger:         logger.Named("w2"),
			DERPMap:        derpMap,
			BlockEndpoints: true,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = w1.Close()
			_ = w2.Close()
		})
		stitch(t, w2, w1)
		stitch(t, w1, w2)
		require.True(t, w2.AwaitReachable(ctx, w1IP))
		conn := make(chan struct{}, 1)
		go func() {
			listener, err := w1.Listen("tcp", ":35565")
			assert.NoError(t, err)
			defer listener.Close()
			nc, err := listener.Accept()
			if !assert.NoError(t, err) {
				return
			}
			_ = nc.Close()
			conn <- struct{}{}
		}()

		nc, err := w2.DialContextTCP(ctx, netip.AddrPortFrom(w1IP, 35565))
		require.NoError(t, err)
		_ = nc.Close()
		<-conn

		nodes := make(chan *tailnet.Node, 1)
		w2.SetNodeCallback(func(node *tailnet.Node) {
			select {
			case nodes <- node:
			default:
			}
		})
		node := <-nodes
		require.Len(t, node.DERPForcedWebsocket, 1)
		// Ensure the reason is valid!
		require.Equal(t, `GET failed with status code 400 (a proxy could be disallowing the use of 'Upgrade: derp'): Invalid "Upgrade" header: DERP`, node.DERPForcedWebsocket[derpMap.RegionIDs()[0]])

		w1.Close()
		w2.Close()
	})
}

// TestConn_PreferredDERP tests that we only trigger the NodeCallback when we have a preferred DERP server.
func TestConn_PreferredDERP(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		Logger:    logger.Named("w1"),
		DERPMap:   derpMap,
	})
	require.NoError(t, err)
	defer func() {
		err := conn.Close()
		require.NoError(t, err)
	}()
	// buffer channel so callback doesn't block
	nodes := make(chan *tailnet.Node, 50)
	conn.SetNodeCallback(func(node *tailnet.Node) {
		nodes <- node
	})
	select {
	case node := <-nodes:
		require.Equal(t, 1, node.PreferredDERP)
	case <-ctx.Done():
		t.Fatal("timed out waiting for node")
	}
}

// TestConn_UpdateDERP tests that when update the DERP map we pick a new
// preferred DERP server and new connections can be made from clients.
func TestConn_UpdateDERP(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	derpMap1, _ := tailnettest.RunDERPAndSTUN(t)
	ip := tailnet.IP()
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:      []netip.Prefix{netip.PrefixFrom(ip, 128)},
		Logger:         logger.Named("w1"),
		DERPMap:        derpMap1,
		BlockEndpoints: true,
	})
	require.NoError(t, err)
	defer func() {
		err := conn.Close()
		assert.NoError(t, err)
	}()

	// Buffer channel so callback doesn't block
	nodes := make(chan *tailnet.Node, 50)
	conn.SetNodeCallback(func(node *tailnet.Node) {
		nodes <- node
	})

	ctx1, cancel1 := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel1()
	select {
	case node := <-nodes:
		require.Equal(t, 1, node.PreferredDERP)
	case <-ctx1.Done():
		t.Fatal("timed out waiting for node")
	}

	// Connect from a different client.
	client1, err := tailnet.NewConn(&tailnet.Options{
		Addresses:      []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		Logger:         logger.Named("client1"),
		DERPMap:        derpMap1,
		BlockEndpoints: true,
	})
	require.NoError(t, err)
	defer func() {
		err := client1.Close()
		assert.NoError(t, err)
	}()
	stitch(t, conn, client1)
	pn, err := tailnet.NodeToProto(conn.Node())
	require.NoError(t, err)
	connID := uuid.New()
	err = client1.UpdatePeers([]*proto.CoordinateResponse_PeerUpdate{{
		Id:   connID[:],
		Node: pn,
		Kind: proto.CoordinateResponse_PeerUpdate_NODE,
	}})
	require.NoError(t, err)

	awaitReachableCtx1, awaitReachableCancel1 := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer awaitReachableCancel1()
	require.True(t, client1.AwaitReachable(awaitReachableCtx1, ip))

	// Update the DERP map and wait for the preferred DERP server to change.
	derpMap2, _ := tailnettest.RunDERPAndSTUN(t)
	// Change the region ID.
	derpMap2.Regions[2] = derpMap2.Regions[1]
	delete(derpMap2.Regions, 1)
	derpMap2.Regions[2].RegionID = 2
	for _, node := range derpMap2.Regions[2].Nodes {
		node.RegionID = 2
	}
	conn.SetDERPMap(derpMap2)

	ctx2, cancel2 := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel2()
parentLoop:
	for {
		select {
		case node := <-nodes:
			if node.PreferredDERP != 2 {
				t.Logf("waiting for preferred DERP server to change, got %v", node.PreferredDERP)
				continue
			}
			t.Log("preferred DERP server changed!")
			break parentLoop
		case <-ctx2.Done():
			t.Fatal("timed out waiting for preferred DERP server to change")
		}
	}

	// Client1 should be dropped...
	awaitReachableCtx2, awaitReachableCancel2 := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer awaitReachableCancel2()
	require.False(t, client1.AwaitReachable(awaitReachableCtx2, ip))

	// ... unless the client updates it's derp map and nodes.
	client1.SetDERPMap(derpMap2)
	pn, err = tailnet.NodeToProto(conn.Node())
	require.NoError(t, err)
	client1.UpdatePeers([]*proto.CoordinateResponse_PeerUpdate{{
		Id:   connID[:],
		Node: pn,
		Kind: proto.CoordinateResponse_PeerUpdate_NODE,
	}})
	awaitReachableCtx3, awaitReachableCancel3 := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer awaitReachableCancel3()
	require.True(t, client1.AwaitReachable(awaitReachableCtx3, ip))

	// Connect from a different different client with up-to-date derp map and
	// nodes.
	client2, err := tailnet.NewConn(&tailnet.Options{
		Addresses:      []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		Logger:         logger.Named("client2"),
		DERPMap:        derpMap2,
		BlockEndpoints: true,
	})
	require.NoError(t, err)
	defer func() {
		err := client2.Close()
		assert.NoError(t, err)
	}()
	stitch(t, conn, client2)
	pn, err = tailnet.NodeToProto(conn.Node())
	require.NoError(t, err)
	client2.UpdatePeers([]*proto.CoordinateResponse_PeerUpdate{{
		Id:   connID[:],
		Node: pn,
		Kind: proto.CoordinateResponse_PeerUpdate_NODE,
	}})

	awaitReachableCtx4, awaitReachableCancel4 := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer awaitReachableCancel4()
	require.True(t, client2.AwaitReachable(awaitReachableCtx4, ip))
}

// stitch sends node updates from src Conn as peer updates to dst Conn.  Sort of
// like the Coordinator would, but without actually needing a Coordinator.
func stitch(t *testing.T, dst, src *tailnet.Conn) {
	srcID := uuid.New()
	src.SetNodeCallback(func(node *tailnet.Node) {
		pn, err := tailnet.NodeToProto(node)
		if !assert.NoError(t, err) {
			return
		}
		err = dst.UpdatePeers([]*proto.CoordinateResponse_PeerUpdate{{
			Id:   srcID[:],
			Node: pn,
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
		}})
		assert.NoError(t, err)
	})
}
