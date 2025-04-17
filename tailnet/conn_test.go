package tailnet_test

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestTailnet(t *testing.T) {
	t.Parallel()
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	t.Run("InstantClose", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		conn, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
			Logger:    logger.Named("w1"),
			DERPMap:   derpMap,
		})
		require.NoError(t, err)
		err = conn.Close()
		require.NoError(t, err)
	})
	t.Run("Connect", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		w1IP := tailnet.TailscaleServicePrefix.RandomAddr()
		w1, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
			Logger:    logger.Named("w1"),
			DERPMap:   derpMap,
		})
		require.NoError(t, err)

		w2, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
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
		listenDone := make(chan struct{})
		go func() {
			listener, err := w1.Listen("tcp", ":35565")
			if !assert.NoError(t, err) {
				return
			}
			close(listenDone)
			defer listener.Close()
			nc, err := listener.Accept()
			if !assert.NoError(t, err) {
				return
			}
			_ = nc.Close()
			conn <- struct{}{}
		}()

		_ = testutil.TryReceive(ctx, t, listenDone)
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
		node := testutil.TryReceive(ctx, t, nodes)
		// Ensure this connected over raw (not websocket) DERP!
		require.Len(t, node.DERPForcedWebsocket, 0)

		w1.Close()
		w2.Close()
	})

	t.Run("ForcesWebSockets", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitMedium)

		w1IP := tailnet.TailscaleServicePrefix.RandomAddr()
		derpMap := tailnettest.RunDERPOnlyWebSockets(t)
		w1, err := tailnet.NewConn(&tailnet.Options{
			Addresses:      []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
			Logger:         logger.Named("w1"),
			DERPMap:        derpMap,
			BlockEndpoints: true,
		})
		require.NoError(t, err)

		w2, err := tailnet.NewConn(&tailnet.Options{
			Addresses:      []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
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
		done := make(chan struct{})
		listening := make(chan struct{})
		go func() {
			defer close(done)
			listener, err := w1.Listen("tcp", ":35565")
			if !assert.NoError(t, err) {
				return
			}
			defer listener.Close()
			close(listening)
			nc, err := listener.Accept()
			if !assert.NoError(t, err) {
				return
			}
			_ = nc.Close()
		}()

		testutil.TryReceive(ctx, t, listening)
		nc, err := w2.DialContextTCP(ctx, netip.AddrPortFrom(w1IP, 35565))
		require.NoError(t, err)
		_ = nc.Close()
		testutil.TryReceive(ctx, t, done)

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

	t.Run("PingDirect", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		w1IP := tailnet.TailscaleServicePrefix.RandomAddr()
		w1, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
			Logger:    logger.Named("w1"),
			DERPMap:   derpMap,
		})
		require.NoError(t, err)

		w2, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
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

		require.Eventually(t, func() bool {
			_, direct, pong, err := w2.Ping(ctx, w1IP)
			if err != nil {
				t.Logf("ping error: %s", err.Error())
				return false
			}
			if !direct {
				t.Logf("got pong: %+v", pong)
				return false
			}
			return true
		}, testutil.WaitShort, testutil.IntervalFast)

		w1.Close()
		w2.Close()
	})

	t.Run("PingDERPOnly", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		w1IP := tailnet.TailscaleServicePrefix.RandomAddr()
		w1, err := tailnet.NewConn(&tailnet.Options{
			Addresses:      []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
			Logger:         logger.Named("w1"),
			DERPMap:        derpMap,
			BlockEndpoints: true,
		})
		require.NoError(t, err)

		w2, err := tailnet.NewConn(&tailnet.Options{
			Addresses:      []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
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
		require.True(t, w2.AwaitReachable(context.Background(), w1IP))

		require.Eventually(t, func() bool {
			_, direct, pong, err := w2.Ping(ctx, w1IP)
			if err != nil {
				t.Logf("ping error: %s", err.Error())
				return false
			}
			if direct || pong.DERPRegionID != derpMap.RegionIDs()[0] {
				t.Logf("got pong: %+v", pong)
				return false
			}
			return true
		}, testutil.WaitShort, testutil.IntervalFast)

		w1.Close()
		w2.Close()
	})
}

// TestConn_PreferredDERP tests that we only trigger the NodeCallback when we have a preferred DERP server.
func TestConn_PreferredDERP(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := testutil.Logger(t)
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
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
	logger := testutil.Logger(t)

	derpMap1, _ := tailnettest.RunDERPAndSTUN(t)
	ip := tailnet.TailscaleServicePrefix.RandomAddr()
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
		Addresses:      []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
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
		Addresses:      []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
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

func TestConn_BlockEndpoints(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)

	derpMap, _ := tailnettest.RunDERPAndSTUN(t)

	// Setup conn 1.
	ip1 := tailnet.TailscaleServicePrefix.RandomAddr()
	conn1, err := tailnet.NewConn(&tailnet.Options{
		Addresses:      []netip.Prefix{netip.PrefixFrom(ip1, 128)},
		Logger:         logger.Named("w1"),
		DERPMap:        derpMap,
		BlockEndpoints: true,
	})
	require.NoError(t, err)
	defer func() {
		err := conn1.Close()
		assert.NoError(t, err)
	}()

	// Setup conn 2.
	ip2 := tailnet.TailscaleServicePrefix.RandomAddr()
	conn2, err := tailnet.NewConn(&tailnet.Options{
		Addresses:      []netip.Prefix{netip.PrefixFrom(ip2, 128)},
		Logger:         logger.Named("w2"),
		DERPMap:        derpMap,
		BlockEndpoints: true,
	})
	require.NoError(t, err)
	defer func() {
		err := conn2.Close()
		assert.NoError(t, err)
	}()

	// Connect them together and wait for them to be reachable.
	stitch(t, conn2, conn1)
	stitch(t, conn1, conn2)
	awaitReachableCtx, awaitReachableCancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer awaitReachableCancel()
	require.True(t, conn1.AwaitReachable(awaitReachableCtx, ip2))

	// Wait 10s for endpoints to potentially be sent over Disco. There's no way
	// to force Disco to send endpoints immediately.
	time.Sleep(10 * time.Second)

	// Double check that both peers don't have endpoints for the other peer
	// according to magicsock.
	conn1Status, ok := conn1.Status().Peer[conn2.Node().Key]
	require.True(t, ok)
	require.Empty(t, conn1Status.Addrs)
	require.Empty(t, conn1Status.CurAddr)
	conn2Status, ok := conn2.Status().Peer[conn1.Node().Key]
	require.True(t, ok)
	require.Empty(t, conn2Status.Addrs)
	require.Empty(t, conn2Status.CurAddr)
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

func TestTailscaleServicePrefix(t *testing.T) {
	t.Parallel()
	a := tailnet.TailscaleServicePrefix.RandomAddr()
	require.True(t, strings.HasPrefix(a.String(), "fd7a:115c:a1e0"))
	p := tailnet.TailscaleServicePrefix.RandomPrefix()
	require.True(t, strings.HasPrefix(p.String(), "fd7a:115c:a1e0"))
	require.True(t, strings.HasSuffix(p.String(), "/128"))
	u := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-123456789abc")
	a = tailnet.TailscaleServicePrefix.AddrFromUUID(u)
	require.Equal(t, "fd7a:115c:a1e0:aaaa:aaaa:1234:5678:9abc", a.String())
	p = tailnet.TailscaleServicePrefix.PrefixFromUUID(u)
	require.Equal(t, "fd7a:115c:a1e0:aaaa:aaaa:1234:5678:9abc/128", p.String())
}

func TestCoderServicePrefix(t *testing.T) {
	t.Parallel()
	a := tailnet.CoderServicePrefix.RandomAddr()
	require.True(t, strings.HasPrefix(a.String(), "fd60:627a:a42b"))
	p := tailnet.CoderServicePrefix.RandomPrefix()
	require.True(t, strings.HasPrefix(p.String(), "fd60:627a:a42b"))
	require.True(t, strings.HasSuffix(p.String(), "/128"))
	u := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-123456789abc")
	a = tailnet.CoderServicePrefix.AddrFromUUID(u)
	require.Equal(t, "fd60:627a:a42b:aaaa:aaaa:1234:5678:9abc", a.String())
	p = tailnet.CoderServicePrefix.PrefixFromUUID(u)
	require.Equal(t, "fd60:627a:a42b:aaaa:aaaa:1234:5678:9abc/128", p.String())
}
