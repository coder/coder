package tailnet_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/tailnet/tailnettest"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTailnet(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	derpMap := tailnettest.RunDERPAndSTUN(t)
	t.Run("InstantClose", func(t *testing.T) {
		t.Parallel()
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
		w1.SetNodeCallback(func(node *tailnet.Node) {
			err := w2.UpdateNodes([]*tailnet.Node{node}, false)
			assert.NoError(t, err)
		})
		w2.SetNodeCallback(func(node *tailnet.Node) {
			err := w1.UpdateNodes([]*tailnet.Node{node}, false)
			assert.NoError(t, err)
		})
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
		node := <-nodes
		// Ensure this connected over DERP!
		require.Len(t, node.DERPForcedWebsocket, 0)

		w1.Close()
		w2.Close()
	})

	t.Run("ForcesWebSockets", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := testutil.Context(t)
		defer cancelFunc()

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
		w1.SetNodeCallback(func(node *tailnet.Node) {
			err := w2.UpdateNodes([]*tailnet.Node{node}, false)
			assert.NoError(t, err)
		})
		w2.SetNodeCallback(func(node *tailnet.Node) {
			err := w1.UpdateNodes([]*tailnet.Node{node}, false)
			assert.NoError(t, err)
		})
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
		require.Equal(t, "GET failed with status code 400: Invalid \"Upgrade\" header: DERP", node.DERPForcedWebsocket[derpMap.RegionIDs()[0]])

		w1.Close()
		w2.Close()
	})
}
