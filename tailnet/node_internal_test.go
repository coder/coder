package tailnet

import (
	"net/netip"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"golang.org/x/exp/slices"

	"tailscale.com/wgengine"

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

func TestNodeUpdater_setStatus_different(t *testing.T) {
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

	// When: we set a new status
	asof := time.Date(2024, 1, 10, 8, 0o0, 1, 1, time.UTC)
	uut.setStatus(&wgengine.Status{
		LocalAddrs: []tailcfg.Endpoint{
			{Addr: netip.MustParseAddrPort("[fe80::1]:5678")},
		},
		AsOf: asof,
	}, nil)

	// Then: we receive an update with the endpoint
	node := testutil.RequireRecvCtx(ctx, t, nodeCh)
	require.Equal(t, nodeKey, node.Key)
	require.Equal(t, discoKey, node.DiscoKey)
	require.Equal(t, []string{"[fe80::1]:5678"}, node.Endpoints)

	// Then: we store the AsOf time as lastStatus
	uut.L.Lock()
	require.Equal(t, uut.lastStatus, asof)
	uut.L.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setStatus_same(t *testing.T) {
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
	//        endpoints set to {"[fe80::1]:5678"}
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.endpoints = []string{"[fe80::1]:5678"}
	uut.L.Unlock()

	// When: we set a status with endpoints {[fe80::1]:5678}
	uut.setStatus(&wgengine.Status{LocalAddrs: []tailcfg.Endpoint{
		{Addr: netip.MustParseAddrPort("[fe80::1]:5678")},
	}}, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setStatus_error(t *testing.T) {
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

	// Given: preferred DERP is 1, so we would send an update on change && empty endpoints
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.L.Unlock()

	// When: we set a status with endpoints {[fe80::1]:5678}, with an error
	uut.setStatus(&wgengine.Status{LocalAddrs: []tailcfg.Endpoint{
		{Addr: netip.MustParseAddrPort("[fe80::1]:5678")},
	}}, xerrors.New("test"))

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setStatus_outdated(t *testing.T) {
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

	// Given: preferred DERP is 1, so we would send an update on change && lastStatus set ahead
	ahead := time.Date(2024, 1, 10, 8, 0o0, 1, 0, time.UTC)
	behind := time.Date(2024, 1, 10, 8, 0o0, 0, 0, time.UTC)
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.lastStatus = ahead
	uut.L.Unlock()

	// When: we set a status with endpoints {[fe80::1]:5678}, with AsOf set behind
	uut.setStatus(&wgengine.Status{
		LocalAddrs: []tailcfg.Endpoint{{Addr: netip.MustParseAddrPort("[fe80::1]:5678")}},
		AsOf:       behind,
	}, xerrors.New("test"))

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setAddresses_different(t *testing.T) {
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

	// When: we set addresses
	addrs := []netip.Prefix{netip.MustParsePrefix("192.168.0.200/32")}
	uut.setAddresses(addrs)

	// Then: we receive an update with the addresses
	node := testutil.RequireRecvCtx(ctx, t, nodeCh)
	require.Equal(t, nodeKey, node.Key)
	require.Equal(t, discoKey, node.DiscoKey)
	require.Equal(t, addrs, node.Addresses)
	require.Equal(t, addrs, node.AllowedIPs)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setAddresses_same(t *testing.T) {
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
	//        addrs already set
	addrs := []netip.Prefix{netip.MustParsePrefix("192.168.0.200/32")}
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.addresses = slices.Clone(addrs)
	uut.L.Unlock()

	// When: we set addrs
	uut.setAddresses(addrs)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setCallback(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	id := tailcfg.NodeID(1)
	nodeKey := key.NewNode().Public()
	discoKey := key.NewDisco().Public()
	uut := newNodeUpdater(
		logger,
		nil,
		id, nodeKey, discoKey,
	)
	defer uut.close()

	// Given: preferred DERP is 1
	addrs := []netip.Prefix{netip.MustParsePrefix("192.168.0.200/32")}
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.addresses = slices.Clone(addrs)
	uut.L.Unlock()

	// When: we set callback
	nodeCh := make(chan *Node)
	uut.setCallback(func(n *Node) {
		nodeCh <- n
	})

	// Then: we get a node update
	node := testutil.RequireRecvCtx(ctx, t, nodeCh)
	require.Equal(t, nodeKey, node.Key)
	require.Equal(t, discoKey, node.DiscoKey)
	require.Equal(t, 1, node.PreferredDERP)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setBlockEndpoints_different(t *testing.T) {
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

	// Given: preferred DERP is 1, so we'll send an update && some endpoints
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.endpoints = []string{"10.11.12.13:7890"}
	uut.L.Unlock()

	// When: we setBlockEndpoints
	uut.setBlockEndpoints(true)

	// Then: we receive an update without endpoints
	node := testutil.RequireRecvCtx(ctx, t, nodeCh)
	require.Equal(t, nodeKey, node.Key)
	require.Equal(t, discoKey, node.DiscoKey)
	require.Len(t, node.Endpoints, 0)

	// When: we unset BlockEndpoints
	uut.setBlockEndpoints(false)

	// Then: we receive an update with endpoints
	node = testutil.RequireRecvCtx(ctx, t, nodeCh)
	require.Equal(t, nodeKey, node.Key)
	require.Equal(t, discoKey, node.DiscoKey)
	require.Len(t, node.Endpoints, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestNodeUpdater_setBlockEndpoints_same(t *testing.T) {
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
	//        blockEndpoints already set
	uut.L.Lock()
	uut.preferredDERP = 1
	uut.blockEndpoints = true
	uut.L.Unlock()

	// When: we set block endpoints
	uut.setBlockEndpoints(true)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}
