package tailnet

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func TestNodeUpdater_setNetInfo_different(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	id := tailcfg.NodeID(1)
	nodeKey := key.NewNode().Public()
	discoKey := key.NewDisco().Public()
	nodeCh := make(chan *Node)
	goCh := make(chan struct{})
	uut := newNodeUpdater(
		logger,
		func(n *Node) {
			nodeCh <- n
			<-goCh
		},
		id, nodeKey, discoKey,
	)
	defer uut.close()

	dl := map[string]float64{"1": 0.025}
	uut.setNetInfo(&tailcfg.NetInfo{
		PreferredDERP: 1,
		DERPLatency:   dl,
	})

	node := testutil.RequireRecvCtx(ctx, t, nodeCh)
	require.Equal(t, nodeKey, node.Key)
	require.Equal(t, discoKey, node.DiscoKey)
	require.Equal(t, 1, node.PreferredDERP)
	require.True(t, maps.Equal(dl, node.DERPLatency))

	// Send in second update to test getting updates in the middle of the
	// callback
	uut.setNetInfo(&tailcfg.NetInfo{
		PreferredDERP: 2,
		DERPLatency:   dl,
	})
	close(goCh) // allows callback to complete

	node = testutil.RequireRecvCtx(ctx, t, nodeCh)
	require.Equal(t, nodeKey, node.Key)
	require.Equal(t, discoKey, node.DiscoKey)
	require.Equal(t, 2, node.PreferredDERP)
	require.True(t, maps.Equal(dl, node.DERPLatency))

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setNetInfo_same(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	id := tailcfg.NodeID(1)
	nodeKey := key.NewNode().Public()
	discoKey := key.NewDisco().Public()
	nodeCh := make(chan *Node)
	uut := newNodeUpdater(
		logger,
		func(n *Node) {
			nodeCh <- n
		},
		id, nodeKey, discoKey,
	)
	defer uut.close()

	// Then: we don't configure
	requireNeverConfigures(ctx, t, &uut.phased)

	// Given: preferred DERP and latency already set
	dl := map[string]float64{"1": 0.025}
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.derpLatency = maps.Clone(dl)
	uut.L.Unlock()

	// When: new update with same info
	uut.setNetInfo(&tailcfg.NetInfo{
		PreferredDERP: 1,
		DERPLatency:   dl,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setDERPForcedWebsocket_different(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	id := tailcfg.NodeID(1)
	nodeKey := key.NewNode().Public()
	discoKey := key.NewDisco().Public()
	nodeCh := make(chan *Node)
	uut := newNodeUpdater(
		logger,
		func(n *Node) {
			nodeCh <- n
		},
		id, nodeKey, discoKey,
	)
	defer uut.close()

	// Given: preferred DERP is 1, so we'll send an update
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.L.Unlock()

	// When: we set a new forced websocket reason
	uut.setDERPForcedWebsocket(1, "test")

	// Then: we receive an update with the reason set
	node := testutil.RequireRecvCtx(ctx, t, nodeCh)
	require.Equal(t, nodeKey, node.Key)
	require.Equal(t, discoKey, node.DiscoKey)
	require.True(t, maps.Equal(map[int]string{1: "test"}, node.DERPForcedWebsocket))

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setDERPForcedWebsocket_same(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	id := tailcfg.NodeID(1)
	nodeKey := key.NewNode().Public()
	discoKey := key.NewDisco().Public()
	nodeCh := make(chan *Node)
	uut := newNodeUpdater(
		logger,
		func(n *Node) {
			nodeCh <- n
		},
		id, nodeKey, discoKey,
	)
	defer uut.close()

	// Then: we don't configure
	requireNeverConfigures(ctx, t, &uut.phased)

	// Given: preferred DERP is 1, so we would send an update on change &&
	//        reason for region 1 is set to "test"
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.derpForcedWebsockets[1] = "test"
	uut.L.Unlock()

	// When: we set region 1 to "test
	uut.setDERPForcedWebsocket(1, "test")

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}
