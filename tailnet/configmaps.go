package tailnet

import (
	"context"
	"errors"
	"net/netip"
	"sync"

	"github.com/google/uuid"
	"go4.org/netipx"
	"tailscale.com/net/dns"
	"tailscale.com/tailcfg"
	"tailscale.com/types/ipproto"
	"tailscale.com/types/key"
	"tailscale.com/types/netmap"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/filter"
	"tailscale.com/wgengine/router"
	"tailscale.com/wgengine/wgcfg"
	"tailscale.com/wgengine/wgcfg/nmcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet/proto"
)

// engineConfigurable is the subset of wgengine.Engine that we use for configuration.
//
// This allows us to test configuration code without faking the whole interface.
type engineConfigurable interface {
	SetNetworkMap(*netmap.NetworkMap)
	Reconfig(*wgcfg.Config, *router.Config, *dns.Config, *tailcfg.Debug) error
	SetDERPMap(*tailcfg.DERPMap)
	SetFilter(*filter.Filter)
}

type phase int

const (
	idle phase = iota
	configuring
	closed
)

type configMaps struct {
	sync.Cond
	netmapDirty  bool
	derpMapDirty bool
	filterDirty  bool
	closing      bool
	phase        phase

	engine    engineConfigurable
	static    netmap.NetworkMap
	peers     map[uuid.UUID]*peerLifecycle
	addresses []netip.Prefix
	derpMap   *proto.DERPMap
	logger    slog.Logger
}

func newConfigMaps(logger slog.Logger, engine engineConfigurable, nodeID tailcfg.NodeID, nodeKey key.NodePrivate, discoKey key.DiscoPublic, addresses []netip.Prefix) *configMaps {
	pubKey := nodeKey.Public()
	c := &configMaps{
		Cond:   *(sync.NewCond(&sync.Mutex{})),
		logger: logger,
		engine: engine,
		static: netmap.NetworkMap{
			SelfNode: &tailcfg.Node{
				ID:       nodeID,
				Key:      pubKey,
				DiscoKey: discoKey,
			},
			NodeKey:    pubKey,
			PrivateKey: nodeKey,
			PacketFilter: []filter.Match{{
				// Allow any protocol!
				IPProto: []ipproto.Proto{ipproto.TCP, ipproto.UDP, ipproto.ICMPv4, ipproto.ICMPv6, ipproto.SCTP},
				// Allow traffic sourced from anywhere.
				Srcs: []netip.Prefix{
					netip.PrefixFrom(netip.AddrFrom4([4]byte{}), 0),
					netip.PrefixFrom(netip.AddrFrom16([16]byte{}), 0),
				},
				// Allow traffic to route anywhere.
				Dsts: []filter.NetPortRange{
					{
						Net: netip.PrefixFrom(netip.AddrFrom4([4]byte{}), 0),
						Ports: filter.PortRange{
							First: 0,
							Last:  65535,
						},
					},
					{
						Net: netip.PrefixFrom(netip.AddrFrom16([16]byte{}), 0),
						Ports: filter.PortRange{
							First: 0,
							Last:  65535,
						},
					},
				},
				Caps: []filter.CapMatch{},
			}},
		},
		peers:     make(map[uuid.UUID]*peerLifecycle),
		addresses: addresses,
	}
	go c.configLoop()
	return c
}

// configLoop waits for the config to be dirty, then reconfigures the engine.
// It is internal to configMaps
func (c *configMaps) configLoop() {
	c.L.Lock()
	defer c.L.Unlock()
	defer func() {
		c.phase = closed
		c.Broadcast()
	}()
	for {
		for !(c.closing || c.netmapDirty || c.filterDirty || c.derpMapDirty) {
			c.phase = idle
			c.Wait()
		}
		if c.closing {
			return
		}
		// queue up the reconfiguration actions we will take while we have
		// the configMaps locked. We will execute them while unlocked to avoid
		// blocking during reconfig.
		actions := make([]func(), 0, 3)
		if c.derpMapDirty {
			derpMap := c.derpMapLocked()
			actions = append(actions, func() {
				c.engine.SetDERPMap(derpMap)
			})
		}
		if c.netmapDirty {
			nm := c.netMapLocked()
			actions = append(actions, func() {
				c.engine.SetNetworkMap(nm)
				c.reconfig(nm)
			})
		}
		if c.filterDirty {
			f := c.filterLocked()
			actions = append(actions, func() {
				c.engine.SetFilter(f)
			})
		}

		c.netmapDirty = false
		c.filterDirty = false
		c.derpMapDirty = false
		c.phase = configuring
		c.Broadcast()

		c.L.Unlock()
		for _, a := range actions {
			a()
		}
		c.L.Lock()
	}
}

// close closes the configMaps and stops it configuring the engine
func (c *configMaps) close() {
	c.L.Lock()
	defer c.L.Unlock()
	c.closing = true
	c.Broadcast()
	for c.phase != closed {
		c.Wait()
	}
}

// netMapLocked returns the current NetworkMap as determined by the config we
// have. c.L must be held.
func (c *configMaps) netMapLocked() *netmap.NetworkMap {
	nm := new(netmap.NetworkMap)
	*nm = c.static

	nm.Addresses = make([]netip.Prefix, len(c.addresses))
	copy(nm.Addresses, c.addresses)

	nm.DERPMap = DERPMapFromProto(c.derpMap)
	nm.Peers = c.peerConfigLocked()
	nm.SelfNode.Addresses = nm.Addresses
	nm.SelfNode.AllowedIPs = nm.Addresses
	return nm
}

// peerConfigLocked returns the set of peer nodes we have.  c.L must be held.
func (c *configMaps) peerConfigLocked() []*tailcfg.Node {
	out := make([]*tailcfg.Node, 0, len(c.peers))
	for _, p := range c.peers {
		out = append(out, p.node.Clone())
	}
	return out
}

// setAddresses sets the addresses belonging to this node to the given slice. It
// triggers configuration of the engine if the addresses have changed.
// c.L MUST NOT be held.
func (c *configMaps) setAddresses(ips []netip.Prefix) {
	c.L.Lock()
	defer c.L.Unlock()
	if d := prefixesDifferent(c.addresses, ips); !d {
		return
	}
	c.addresses = make([]netip.Prefix, len(ips))
	copy(c.addresses, ips)
	c.netmapDirty = true
	c.filterDirty = true
	c.Broadcast()
}

// derMapLocked returns the current DERPMap.  c.L must be held
func (c *configMaps) derpMapLocked() *tailcfg.DERPMap {
	m := DERPMapFromProto(c.derpMap)
	return m
}

// reconfig computes the correct wireguard config and calls the engine.Reconfig
// with the config we have.  It is not intended for this to be called outside of
// the updateLoop()
func (c *configMaps) reconfig(nm *netmap.NetworkMap) {
	cfg, err := nmcfg.WGCfg(nm, Logger(c.logger.Named("net.wgconfig")), netmap.AllowSingleHosts, "")
	if err != nil {
		// WGCfg never returns an error at the time this code was written.  If it starts, returning
		// errors if/when we upgrade tailscale, we'll need to deal.
		c.logger.Critical(context.Background(), "update wireguard config failed", slog.Error(err))
		return
	}

	rc := &router.Config{LocalAddrs: nm.Addresses}
	err = c.engine.Reconfig(cfg, rc, &dns.Config{}, &tailcfg.Debug{})
	if err != nil {
		if errors.Is(err, wgengine.ErrNoChanges) {
			return
		}
		c.logger.Error(context.Background(), "failed to reconfigure wireguard engine", slog.Error(err))
	}
}

// filterLocked returns the current filter, based on our local addresses.  c.L
// must be held.
func (c *configMaps) filterLocked() *filter.Filter {
	localIPSet := netipx.IPSetBuilder{}
	for _, addr := range c.addresses {
		localIPSet.AddPrefix(addr)
	}
	localIPs, _ := localIPSet.IPSet()
	logIPSet := netipx.IPSetBuilder{}
	logIPs, _ := logIPSet.IPSet()
	return filter.New(
		c.static.PacketFilter,
		localIPs,
		logIPs,
		nil,
		Logger(c.logger.Named("net.packet-filter")),
	)
}

type peerLifecycle struct {
	node *tailcfg.Node
	// TODO: implement timers to track lost peers
	// lastHandshake time.Time
	// timer         time.Timer
}

// prefixesDifferent returns true if the two slices contain different prefixes
// where order doesn't matter.
func prefixesDifferent(a, b []netip.Prefix) bool {
	if len(a) != len(b) {
		return true
	}
	as := make(map[string]bool)
	for _, p := range a {
		as[p.String()] = true
	}
	for _, p := range b {
		if !as[p.String()] {
			return true
		}
	}
	return false
}
