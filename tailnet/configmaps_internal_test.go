package tailnet

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/dns"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/netmap"
	"tailscale.com/wgengine/filter"
	"tailscale.com/wgengine/router"
	"tailscale.com/wgengine/wgcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestConfigMaps_setAddresses_different(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()

	addrs := []netip.Prefix{netip.MustParsePrefix("192.168.0.200/32")}
	uut.setAddresses(addrs)

	nm := testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	require.Equal(t, addrs, nm.Addresses)

	// here were in the middle of a reconfig, blocked on a channel write to fEng.reconfig
	locked := uut.L.(*sync.Mutex).TryLock()
	require.True(t, locked)
	require.Equal(t, configuring, uut.phase)
	uut.L.Unlock()
	// send in another update while blocked
	addrs2 := []netip.Prefix{
		netip.MustParsePrefix("192.168.0.200/32"),
		netip.MustParsePrefix("10.20.30.40/32"),
	}
	uut.setAddresses(addrs2)

	r := testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Equal(t, addrs, r.wg.Addresses)
	require.Equal(t, addrs, r.router.LocalAddrs)
	f := testutil.RequireRecvCtx(ctx, t, fEng.filter)
	fr := f.CheckTCP(netip.MustParseAddr("33.44.55.66"), netip.MustParseAddr("192.168.0.200"), 5555)
	require.Equal(t, filter.Accept, fr)
	fr = f.CheckTCP(netip.MustParseAddr("33.44.55.66"), netip.MustParseAddr("10.20.30.40"), 5555)
	require.Equal(t, filter.Drop, fr, "first addr config should not include 10.20.30.40")

	// we should get another round of configurations from the second set of addrs
	nm = testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	require.Equal(t, addrs2, nm.Addresses)
	r = testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Equal(t, addrs2, r.wg.Addresses)
	require.Equal(t, addrs2, r.router.LocalAddrs)
	f = testutil.RequireRecvCtx(ctx, t, fEng.filter)
	fr = f.CheckTCP(netip.MustParseAddr("33.44.55.66"), netip.MustParseAddr("192.168.0.200"), 5555)
	require.Equal(t, filter.Accept, fr)
	fr = f.CheckTCP(netip.MustParseAddr("33.44.55.66"), netip.MustParseAddr("10.20.30.40"), 5555)
	require.Equal(t, filter.Accept, fr)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_setAddresses_same(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	addrs := []netip.Prefix{netip.MustParsePrefix("192.168.0.200/32")}
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()

	// Given: addresses already set
	uut.L.Lock()
	uut.addresses = addrs
	uut.L.Unlock()

	// Then: it doesn't configure
	requireNeverConfigures(ctx, t, &uut.phased)

	// When: we set addresses
	uut.setAddresses(addrs)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_updatePeers_new(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()

	p1ID := uuid.UUID{1}
	p1Node := newTestNode(1)
	p1n, err := NodeToProto(p1Node)
	require.NoError(t, err)
	p2ID := uuid.UUID{2}
	p2Node := newTestNode(2)
	p2n, err := NodeToProto(p2Node)
	require.NoError(t, err)

	go func() {
		b := <-fEng.status
		b.AddPeer(p1Node.Key, &ipnstate.PeerStatus{
			PublicKey:     p1Node.Key,
			LastHandshake: time.Date(2024, 1, 7, 12, 13, 10, 0, time.UTC),
			Active:        true,
		})
		// peer 2 is missing, so it won't have KeepAlives set
		fEng.statusDone <- struct{}{}
	}()

	updates := []*proto.CoordinateResponse_PeerUpdate{
		{
			Id:   p1ID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: p1n,
		},
		{
			Id:   p2ID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: p2n,
		},
	}
	uut.updatePeers(updates)

	nm := testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	r := testutil.RequireRecvCtx(ctx, t, fEng.reconfig)

	require.Len(t, nm.Peers, 2)
	n1 := getNodeWithID(t, nm.Peers, 1)
	require.Equal(t, "127.3.3.40:1", n1.DERP)
	require.Equal(t, p1Node.Endpoints, n1.Endpoints)
	require.True(t, n1.KeepAlive)
	n2 := getNodeWithID(t, nm.Peers, 2)
	require.Equal(t, "127.3.3.40:2", n2.DERP)
	require.Equal(t, p2Node.Endpoints, n2.Endpoints)
	require.False(t, n2.KeepAlive)

	// we rely on nmcfg.WGCfg() to convert the netmap to wireguard config, so just
	// require the right number of peers.
	require.Len(t, r.wg.Peers, 2)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_updatePeers_same(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()

	// Then: we don't configure
	requireNeverConfigures(ctx, t, &uut.phased)

	p1ID := uuid.UUID{1}
	p1Node := newTestNode(1)
	p1n, err := NodeToProto(p1Node)
	require.NoError(t, err)
	p1tcn, err := uut.protoNodeToTailcfg(p1n)
	p1tcn.KeepAlive = true
	require.NoError(t, err)

	// Given: peer already exists
	uut.L.Lock()
	uut.peers[p1ID] = &peerLifecycle{
		peerID:        p1ID,
		node:          p1tcn,
		lastHandshake: time.Date(2024, 1, 7, 12, 0, 10, 0, time.UTC),
	}
	uut.L.Unlock()

	go func() {
		b := <-fEng.status
		b.AddPeer(p1Node.Key, &ipnstate.PeerStatus{
			PublicKey:     p1Node.Key,
			LastHandshake: time.Date(2024, 1, 7, 12, 13, 10, 0, time.UTC),
			Active:        true,
		})
		fEng.statusDone <- struct{}{}
	}()

	// When: update with no changes
	updates := []*proto.CoordinateResponse_PeerUpdate{
		{
			Id:   p1ID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: p1n,
		},
	}
	uut.updatePeers(updates)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_updatePeers_disconnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()

	p1ID := uuid.UUID{1}
	p1Node := newTestNode(1)
	p1n, err := NodeToProto(p1Node)
	require.NoError(t, err)
	p1tcn, err := uut.protoNodeToTailcfg(p1n)
	p1tcn.KeepAlive = true
	require.NoError(t, err)

	// set a timer, which should get canceled by the disconnect.
	timer := uut.clock.AfterFunc(testutil.WaitMedium, func() {
		t.Error("this should not be called!")
	})

	// Given: peer already exists
	uut.L.Lock()
	uut.peers[p1ID] = &peerLifecycle{
		peerID:        p1ID,
		node:          p1tcn,
		lastHandshake: time.Date(2024, 1, 7, 12, 0, 10, 0, time.UTC),
		timer:         timer,
	}
	uut.L.Unlock()

	go func() {
		b := <-fEng.status
		b.AddPeer(p1Node.Key, &ipnstate.PeerStatus{
			PublicKey:     p1Node.Key,
			LastHandshake: time.Date(2024, 1, 7, 12, 13, 10, 0, time.UTC),
			Active:        true,
		})
		fEng.statusDone <- struct{}{}
	}()

	// When: update DISCONNECTED
	updates := []*proto.CoordinateResponse_PeerUpdate{
		{
			Id:   p1ID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_DISCONNECTED,
		},
	}
	uut.updatePeers(updates)
	assert.False(t, timer.Stop(), "timer was not stopped")

	// Then, configure engine without the peer.
	nm := testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	r := testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Len(t, nm.Peers, 0)
	require.Len(t, r.wg.Peers, 0)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_updatePeers_lost(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()
	start := time.Date(2024, time.January, 1, 8, 0, 0, 0, time.UTC)
	mClock := clock.NewMock()
	mClock.Set(start)
	uut.clock = mClock

	p1ID := uuid.UUID{1}
	p1Node := newTestNode(1)
	p1n, err := NodeToProto(p1Node)
	require.NoError(t, err)

	s1 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, start)

	updates := []*proto.CoordinateResponse_PeerUpdate{
		{
			Id:   p1ID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: p1n,
		},
	}
	uut.updatePeers(updates)
	nm := testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	r := testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Len(t, nm.Peers, 1)
	require.Len(t, r.wg.Peers, 1)
	_ = testutil.RequireRecvCtx(ctx, t, s1)

	mClock.Add(5 * time.Second)

	s2 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, start)

	updates[0].Kind = proto.CoordinateResponse_PeerUpdate_LOST
	updates[0].Node = nil
	uut.updatePeers(updates)
	_ = testutil.RequireRecvCtx(ctx, t, s2)

	// No reprogramming yet, since we keep the peer around.
	select {
	case <-fEng.setNetworkMap:
		t.Fatal("should not reprogram")
	default:
		// OK!
	}

	// When we advance the clock, the timeout triggers.  However, the new
	// latest handshake has advanced by a minute, so we don't remove the peer.
	lh := start.Add(time.Minute)
	s3 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, lh)
	mClock.Add(lostTimeout)
	_ = testutil.RequireRecvCtx(ctx, t, s3)
	select {
	case <-fEng.setNetworkMap:
		t.Fatal("should not reprogram")
	default:
		// OK!
	}

	// Before we update the clock again, we need to be sure the timeout has
	// completed running. To do that, we check the new lastHandshake has been set
	require.Eventually(t, func() bool {
		uut.L.Lock()
		defer uut.L.Unlock()
		return uut.peers[p1ID].lastHandshake == lh
	}, testutil.WaitShort, testutil.IntervalFast)

	// Advance the clock again by a minute, which should trigger the reprogrammed
	// timeout.
	s4 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, lh)
	mClock.Add(time.Minute)

	nm = testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	r = testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Len(t, nm.Peers, 0)
	require.Len(t, r.wg.Peers, 0)
	_ = testutil.RequireRecvCtx(ctx, t, s4)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_updatePeers_lost_and_found(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()
	start := time.Date(2024, time.January, 1, 8, 0, 0, 0, time.UTC)
	mClock := clock.NewMock()
	mClock.Set(start)
	uut.clock = mClock

	p1ID := uuid.UUID{1}
	p1Node := newTestNode(1)
	p1n, err := NodeToProto(p1Node)
	require.NoError(t, err)

	s1 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, start)

	updates := []*proto.CoordinateResponse_PeerUpdate{
		{
			Id:   p1ID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: p1n,
		},
	}
	uut.updatePeers(updates)
	nm := testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	r := testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Len(t, nm.Peers, 1)
	require.Len(t, r.wg.Peers, 1)
	_ = testutil.RequireRecvCtx(ctx, t, s1)

	mClock.Add(5 * time.Second)

	s2 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, start)

	updates[0].Kind = proto.CoordinateResponse_PeerUpdate_LOST
	updates[0].Node = nil
	uut.updatePeers(updates)
	_ = testutil.RequireRecvCtx(ctx, t, s2)

	// No reprogramming yet, since we keep the peer around.
	select {
	case <-fEng.setNetworkMap:
		t.Fatal("should not reprogram")
	default:
		// OK!
	}

	mClock.Add(5 * time.Second)
	s3 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, start)

	updates[0].Kind = proto.CoordinateResponse_PeerUpdate_NODE
	updates[0].Node = p1n
	uut.updatePeers(updates)
	_ = testutil.RequireRecvCtx(ctx, t, s3)
	// This does not trigger reprogramming, because we never removed the node
	select {
	case <-fEng.setNetworkMap:
		t.Fatal("should not reprogram")
	default:
		// OK!
	}

	// When we advance the clock, nothing happens because the timeout was
	// canceled
	mClock.Add(lostTimeout)
	select {
	case <-fEng.setNetworkMap:
		t.Fatal("should not reprogram")
	default:
		// OK!
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_setAllPeersLost(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()
	start := time.Date(2024, time.January, 1, 8, 0, 0, 0, time.UTC)
	mClock := clock.NewMock()
	mClock.Set(start)
	uut.clock = mClock

	p1ID := uuid.UUID{1}
	p1Node := newTestNode(1)
	p1n, err := NodeToProto(p1Node)
	require.NoError(t, err)
	p2ID := uuid.UUID{2}
	p2Node := newTestNode(2)
	p2n, err := NodeToProto(p2Node)
	require.NoError(t, err)

	s1 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, start)

	updates := []*proto.CoordinateResponse_PeerUpdate{
		{
			Id:   p1ID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: p1n,
		},
		{
			Id:   p2ID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: p2n,
		},
	}
	uut.updatePeers(updates)
	nm := testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	r := testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Len(t, nm.Peers, 2)
	require.Len(t, r.wg.Peers, 2)
	_ = testutil.RequireRecvCtx(ctx, t, s1)

	mClock.Add(5 * time.Second)
	uut.setAllPeersLost()

	// No reprogramming yet, since we keep the peer around.
	select {
	case <-fEng.setNetworkMap:
		t.Fatal("should not reprogram")
	default:
		// OK!
	}

	// When we advance the clock, even by a few ms, the timeout for peer 2 pops
	// because our status only includes a handshake for peer 1
	s2 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, start)
	mClock.Add(time.Millisecond * 10)
	_ = testutil.RequireRecvCtx(ctx, t, s2)

	nm = testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	r = testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Len(t, nm.Peers, 1)
	require.Len(t, r.wg.Peers, 1)

	// Finally, advance the clock until after the timeout
	s3 := expectStatusWithHandshake(ctx, t, fEng, p1Node.Key, start)
	mClock.Add(lostTimeout)
	_ = testutil.RequireRecvCtx(ctx, t, s3)

	nm = testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	r = testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Len(t, nm.Peers, 0)
	require.Len(t, r.wg.Peers, 0)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_setBlockEndpoints_different(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()

	p1ID := uuid.MustParse("10000000-0000-0000-0000-000000000000")
	p1Node := newTestNode(1)
	p1n, err := NodeToProto(p1Node)
	require.NoError(t, err)
	p1tcn, err := uut.protoNodeToTailcfg(p1n)
	p1tcn.KeepAlive = true
	require.NoError(t, err)

	// Given: peer already exists
	uut.L.Lock()
	uut.peers[p1ID] = &peerLifecycle{
		peerID:        p1ID,
		node:          p1tcn,
		lastHandshake: time.Date(2024, 1, 7, 12, 0, 10, 0, time.UTC),
	}
	uut.L.Unlock()

	uut.setBlockEndpoints(true)

	nm := testutil.RequireRecvCtx(ctx, t, fEng.setNetworkMap)
	r := testutil.RequireRecvCtx(ctx, t, fEng.reconfig)
	require.Len(t, nm.Peers, 1)
	require.Len(t, nm.Peers[0].Endpoints, 0)
	require.Len(t, r.wg.Peers, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_setBlockEndpoints_same(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()

	p1ID := uuid.MustParse("10000000-0000-0000-0000-000000000000")
	p1Node := newTestNode(1)
	p1n, err := NodeToProto(p1Node)
	require.NoError(t, err)
	p1tcn, err := uut.protoNodeToTailcfg(p1n)
	p1tcn.KeepAlive = true
	require.NoError(t, err)

	// Given: peer already exists && blockEndpoints set to true
	uut.L.Lock()
	uut.peers[p1ID] = &peerLifecycle{
		peerID:        p1ID,
		node:          p1tcn,
		lastHandshake: time.Date(2024, 1, 7, 12, 0, 10, 0, time.UTC),
	}
	uut.blockEndpoints = true
	uut.L.Unlock()

	// Then: we don't configure
	requireNeverConfigures(ctx, t, &uut.phased)

	// When we set blockEndpoints to true
	uut.setBlockEndpoints(true)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_setDERPMap_different(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()

	derpMap := &tailcfg.DERPMap{
		HomeParams: &tailcfg.DERPHomeParams{RegionScore: map[int]float64{1: 0.025}},
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionCode: "AUH",
				Nodes: []*tailcfg.DERPNode{
					{Name: "AUH0"},
				},
			},
		},
	}
	uut.setDERPMap(derpMap)

	dm := testutil.RequireRecvCtx(ctx, t, fEng.setDERPMap)
	require.Len(t, dm.HomeParams.RegionScore, 1)
	require.Equal(t, dm.HomeParams.RegionScore[1], 0.025)
	require.Len(t, dm.Regions, 1)
	r1 := dm.Regions[1]
	require.Equal(t, "AUH", r1.RegionCode)
	require.Len(t, r1.Nodes, 1)
	require.Equal(t, "AUH0", r1.Nodes[0].Name)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func TestConfigMaps_setDERPMap_same(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
	defer uut.close()

	// Given: DERP Map already set
	derpMap := &tailcfg.DERPMap{
		HomeParams: &tailcfg.DERPHomeParams{RegionScore: map[int]float64{
			1:    0.025,
			1001: 0.111,
		}},
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionCode: "AUH",
				Nodes: []*tailcfg.DERPNode{
					{Name: "AUH0"},
				},
			},
			1001: {
				RegionCode: "DXB",
				Nodes: []*tailcfg.DERPNode{
					{Name: "DXB0"},
				},
			},
		},
	}
	uut.L.Lock()
	uut.derpMap = derpMap
	uut.L.Unlock()

	// Then: we don't configure
	requireNeverConfigures(ctx, t, &uut.phased)

	// When we set the equivalent DERP map, with different ordering
	uut.setDERPMap(&tailcfg.DERPMap{
		HomeParams: &tailcfg.DERPHomeParams{RegionScore: map[int]float64{
			1001: 0.111,
			1:    0.025,
		}},
		Regions: map[int]*tailcfg.DERPRegion{
			1001: {
				RegionCode: "DXB",
				Nodes: []*tailcfg.DERPNode{
					{Name: "DXB0"},
				},
			},
			1: {
				RegionCode: "AUH",
				Nodes: []*tailcfg.DERPNode{
					{Name: "AUH0"},
				},
			},
		},
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
}

func expectStatusWithHandshake(
	ctx context.Context, t testing.TB, fEng *fakeEngineConfigurable, k key.NodePublic, lastHandshake time.Time,
) <-chan struct{} {
	t.Helper()
	called := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			t.Error("timeout waiting for status")
			return
		case b := <-fEng.status:
			b.AddPeer(k, &ipnstate.PeerStatus{
				PublicKey:     k,
				LastHandshake: lastHandshake,
				Active:        true,
			})
			select {
			case <-ctx.Done():
				t.Error("timeout sending done")
			case fEng.statusDone <- struct{}{}:
				close(called)
				return
			}
		}
	}()
	return called
}

func TestConfigMaps_updatePeers_nonexist(t *testing.T) {
	t.Parallel()

	for _, k := range []proto.CoordinateResponse_PeerUpdate_Kind{
		proto.CoordinateResponse_PeerUpdate_DISCONNECTED,
		proto.CoordinateResponse_PeerUpdate_LOST,
	} {
		k := k
		t.Run(k.String(), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			fEng := newFakeEngineConfigurable()
			nodePrivateKey := key.NewNode()
			nodeID := tailcfg.NodeID(5)
			discoKey := key.NewDisco()
			uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public())
			defer uut.close()

			// Then: we don't configure
			requireNeverConfigures(ctx, t, &uut.phased)

			// Given: no known peers
			go func() {
				<-fEng.status
				fEng.statusDone <- struct{}{}
			}()

			// When: update with LOST/DISCONNECTED
			p1ID := uuid.UUID{1}
			updates := []*proto.CoordinateResponse_PeerUpdate{
				{
					Id:   p1ID[:],
					Kind: k,
				},
			}
			uut.updatePeers(updates)

			done := make(chan struct{})
			go func() {
				defer close(done)
				uut.close()
			}()
			_ = testutil.RequireRecvCtx(ctx, t, done)
		})
	}
}

func newTestNode(id int) *Node {
	return &Node{
		ID:            tailcfg.NodeID(id),
		AsOf:          time.Date(2024, 1, 7, 12, 13, 14, 15, time.UTC),
		Key:           key.NewNode().Public(),
		DiscoKey:      key.NewDisco().Public(),
		Endpoints:     []string{"192.168.0.55"},
		PreferredDERP: id,
	}
}

func getNodeWithID(t testing.TB, peers []*tailcfg.Node, id tailcfg.NodeID) *tailcfg.Node {
	t.Helper()
	for _, n := range peers {
		if n.ID == id {
			return n
		}
	}
	t.Fatal()
	return nil
}

func requireNeverConfigures(ctx context.Context, t *testing.T, uut *phased) {
	t.Helper()
	waiting := make(chan struct{})
	go func() {
		// ensure that we never configure, and go straight to closed
		uut.L.Lock()
		defer uut.L.Unlock()
		close(waiting)
		for uut.phase == idle {
			uut.Wait()
		}
		assert.Equal(t, closed, uut.phase)
	}()
	_ = testutil.RequireRecvCtx(ctx, t, waiting)
}

type reconfigCall struct {
	wg     *wgcfg.Config
	router *router.Config
}

var _ engineConfigurable = &fakeEngineConfigurable{}

type fakeEngineConfigurable struct {
	setNetworkMap chan *netmap.NetworkMap
	reconfig      chan reconfigCall
	filter        chan *filter.Filter
	setDERPMap    chan *tailcfg.DERPMap

	// To fake these fields the test should read from status, do stuff to the
	// StatusBuilder, then write to statusDone
	status     chan *ipnstate.StatusBuilder
	statusDone chan struct{}
}

func (f fakeEngineConfigurable) UpdateStatus(status *ipnstate.StatusBuilder) {
	f.status <- status
	<-f.statusDone
}

func newFakeEngineConfigurable() *fakeEngineConfigurable {
	return &fakeEngineConfigurable{
		setNetworkMap: make(chan *netmap.NetworkMap),
		reconfig:      make(chan reconfigCall),
		filter:        make(chan *filter.Filter),
		setDERPMap:    make(chan *tailcfg.DERPMap),
		status:        make(chan *ipnstate.StatusBuilder),
		statusDone:    make(chan struct{}),
	}
}

func (f fakeEngineConfigurable) SetNetworkMap(networkMap *netmap.NetworkMap) {
	f.setNetworkMap <- networkMap
}

func (f fakeEngineConfigurable) Reconfig(wg *wgcfg.Config, r *router.Config, _ *dns.Config, _ *tailcfg.Debug) error {
	f.reconfig <- reconfigCall{wg: wg, router: r}
	return nil
}

func (f fakeEngineConfigurable) SetDERPMap(d *tailcfg.DERPMap) {
	f.setDERPMap <- d
}

func (f fakeEngineConfigurable) SetFilter(flt *filter.Filter) {
	f.filter <- flt
}
