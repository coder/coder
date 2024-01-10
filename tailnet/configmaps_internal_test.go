package tailnet

import (
	"net/netip"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/net/dns"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/netmap"
	"tailscale.com/wgengine/filter"
	"tailscale.com/wgengine/router"
	"tailscale.com/wgengine/wgcfg"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func TestConfigMaps_setAddresses_different(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public(), nil)
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
	logger := slogtest.Make(t, nil)
	fEng := newFakeEngineConfigurable()
	nodePrivateKey := key.NewNode()
	nodeID := tailcfg.NodeID(5)
	discoKey := key.NewDisco()
	addrs := []netip.Prefix{netip.MustParsePrefix("192.168.0.200/32")}
	uut := newConfigMaps(logger, fEng, nodeID, nodePrivateKey, discoKey.Public(), addrs)
	defer uut.close()

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

	uut.setAddresses(addrs)

	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.close()
	}()
	_ = testutil.RequireRecvCtx(ctx, t, done)
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
}

func newFakeEngineConfigurable() *fakeEngineConfigurable {
	return &fakeEngineConfigurable{
		setNetworkMap: make(chan *netmap.NetworkMap),
		reconfig:      make(chan reconfigCall),
		filter:        make(chan *filter.Filter),
	}
}

func (f fakeEngineConfigurable) SetNetworkMap(networkMap *netmap.NetworkMap) {
	f.setNetworkMap <- networkMap
}

func (f fakeEngineConfigurable) Reconfig(wg *wgcfg.Config, r *router.Config, _ *dns.Config, _ *tailcfg.Debug) error {
	f.reconfig <- reconfigCall{wg: wg, router: r}
	return nil
}

func (fakeEngineConfigurable) SetDERPMap(*tailcfg.DERPMap) {
	// TODO implement me
	panic("implement me")
}

func (f fakeEngineConfigurable) SetFilter(flt *filter.Filter) {
	f.filter <- flt
}
