package tailnet

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/uuid"
	"go4.org/netipx"
	"tailscale.com/ipn/ipnstate"
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

const lostTimeout = 15 * time.Minute

// engineConfigurable is the subset of wgengine.Engine that we use for configuration.
//
// This allows us to test configuration code without faking the whole interface.
type engineConfigurable interface {
	UpdateStatus(*ipnstate.StatusBuilder)
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

type phased struct {
	sync.Cond
	phase phase
}

type configMaps struct {
	phased
	netmapDirty  bool
	derpMapDirty bool
	filterDirty  bool
	closing      bool

	engine         engineConfigurable
	static         netmap.NetworkMap
	peers          map[uuid.UUID]*peerLifecycle
	addresses      []netip.Prefix
	derpMap        *proto.DERPMap
	logger         slog.Logger
	blockEndpoints bool

	// for testing
	clock clock.Clock
}

func newConfigMaps(logger slog.Logger, engine engineConfigurable, nodeID tailcfg.NodeID, nodeKey key.NodePrivate, discoKey key.DiscoPublic, addresses []netip.Prefix) *configMaps {
	pubKey := nodeKey.Public()
	c := &configMaps{
		phased: phased{Cond: *(sync.NewCond(&sync.Mutex{}))},
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
		clock:     clock.New(),
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
			c.logger.Debug(context.Background(), "closing configMaps configLoop")
			return
		}
		// queue up the reconfiguration actions we will take while we have
		// the configMaps locked. We will execute them while unlocked to avoid
		// blocking during reconfig.
		actions := make([]func(), 0, 3)
		if c.derpMapDirty {
			derpMap := c.derpMapLocked()
			actions = append(actions, func() {
				c.logger.Debug(context.Background(), "updating engine DERP map", slog.F("derp_map", derpMap))
				c.engine.SetDERPMap(derpMap)
			})
		}
		if c.netmapDirty {
			nm := c.netMapLocked()
			actions = append(actions, func() {
				c.logger.Debug(context.Background(), "updating engine network map", slog.F("network_map", nm))
				c.engine.SetNetworkMap(nm)
				c.reconfig(nm)
			})
		}
		if c.filterDirty {
			f := c.filterLocked()
			actions = append(actions, func() {
				c.logger.Debug(context.Background(), "updating engine filter", slog.F("filter", f))
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
	for _, lc := range c.peers {
		lc.resetTimer()
	}
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
		n := p.node.Clone()
		if c.blockEndpoints {
			n.Endpoints = nil
		}
		out = append(out, n)
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

// setBlockEndpoints sets whether we should block configuring endpoints we learn
// from peers.  It triggers a configuration of the engine if the value changes.
// nolint: revive
func (c *configMaps) setBlockEndpoints(blockEndpoints bool) {
	c.L.Lock()
	defer c.L.Unlock()
	if c.blockEndpoints != blockEndpoints {
		c.netmapDirty = true
	}
	c.blockEndpoints = blockEndpoints
	c.Broadcast()
}

// setDERPMap sets the DERP map, triggering a configuration of the engine if it has changed.
// c.L MUST NOT be held.
func (c *configMaps) setDERPMap(derpMap *proto.DERPMap) {
	c.L.Lock()
	defer c.L.Unlock()
	eq, err := c.derpMap.Equal(derpMap)
	if err != nil {
		c.logger.Critical(context.Background(), "failed to compare DERP maps", slog.Error(err))
		return
	}
	if eq {
		return
	}
	c.derpMap = derpMap
	c.derpMapDirty = true
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

// updatePeers handles protocol updates about peers from the coordinator.  c.L MUST NOT be held.
func (c *configMaps) updatePeers(updates []*proto.CoordinateResponse_PeerUpdate) {
	status := c.status()
	c.L.Lock()
	defer c.L.Unlock()

	// Update all the lastHandshake values here. That way we don't have to
	// worry about them being up-to-date when handling updates below, and it covers
	// all peers, not just the ones we got updates about.
	for _, lc := range c.peers {
		if peerStatus, ok := status.Peer[lc.node.Key]; ok {
			lc.lastHandshake = peerStatus.LastHandshake
		}
	}

	for _, update := range updates {
		if dirty := c.updatePeerLocked(update, status); dirty {
			c.netmapDirty = true
		}
	}
	if c.netmapDirty {
		c.Broadcast()
	}
}

// status requests a status update from the engine.
func (c *configMaps) status() *ipnstate.Status {
	sb := &ipnstate.StatusBuilder{WantPeers: true}
	c.engine.UpdateStatus(sb)
	return sb.Status()
}

// updatePeerLocked processes a single update for a single peer. It is intended
// as internal function since it returns whether or not the config is dirtied by
// the update (instead of handling it directly like updatePeers).  c.L must be held.
func (c *configMaps) updatePeerLocked(update *proto.CoordinateResponse_PeerUpdate, status *ipnstate.Status) (dirty bool) {
	id, err := uuid.FromBytes(update.Id)
	if err != nil {
		c.logger.Critical(context.Background(), "received update with bad id", slog.F("id", update.Id))
		return false
	}
	logger := c.logger.With(slog.F("peer_id", id))
	lc, ok := c.peers[id]
	var node *tailcfg.Node
	if update.Kind == proto.CoordinateResponse_PeerUpdate_NODE {
		// If no preferred DERP is provided, we can't reach the node.
		if update.Node.PreferredDerp == 0 {
			logger.Warn(context.Background(), "no preferred DERP, peer update", slog.F("node_proto", update.Node))
			return false
		}
		node, err = c.protoNodeToTailcfg(update.Node)
		if err != nil {
			logger.Critical(context.Background(), "failed to convert proto node to tailcfg", slog.F("node_proto", update.Node))
			return false
		}
		logger = logger.With(slog.F("key_id", node.Key.ShortString()), slog.F("node", node))
		peerStatus, ok := status.Peer[node.Key]
		// Starting KeepAlive messages at the initialization of a connection
		// causes a race condition. If we send the handshake before the peer has
		// our node, we'll have to wait for 5 seconds before trying again.
		// Ideally, the first handshake starts when the user first initiates a
		// connection to the peer. After a successful connection we enable
		// keep alives to persist the connection and keep it from becoming idle.
		// SSH connections don't send packets while idle, so we use keep alives
		// to avoid random hangs while we set up the connection again after
		// inactivity.
		node.KeepAlive = ok && peerStatus.Active
	}
	switch {
	case !ok && update.Kind == proto.CoordinateResponse_PeerUpdate_NODE:
		// new!
		var lastHandshake time.Time
		if ps, ok := status.Peer[node.Key]; ok {
			lastHandshake = ps.LastHandshake
		}
		c.peers[id] = &peerLifecycle{
			peerID:        id,
			node:          node,
			lastHandshake: lastHandshake,
			lost:          false,
		}
		logger.Debug(context.Background(), "adding new peer")
		return true
	case ok && update.Kind == proto.CoordinateResponse_PeerUpdate_NODE:
		// update
		node.Created = lc.node.Created
		dirty = !lc.node.Equal(node)
		lc.node = node
		lc.lost = false
		lc.resetTimer()
		logger.Debug(context.Background(), "node update to existing peer", slog.F("dirty", dirty))
		return dirty
	case !ok:
		// disconnected or lost, but we don't have the node. No op
		logger.Debug(context.Background(), "skipping update for peer we don't recognize")
		return false
	case update.Kind == proto.CoordinateResponse_PeerUpdate_DISCONNECTED:
		lc.resetTimer()
		delete(c.peers, id)
		logger.Debug(context.Background(), "disconnected peer")
		return true
	case update.Kind == proto.CoordinateResponse_PeerUpdate_LOST:
		lc.lost = true
		lc.setLostTimer(c)
		logger.Debug(context.Background(), "marked peer lost")
		// marking a node lost doesn't change anything right now, so dirty=false
		return false
	default:
		logger.Warn(context.Background(), "unknown peer update", slog.F("kind", update.Kind))
		return false
	}
}

// peerLostTimeout is the callback that peerLifecycle uses when a peer is lost the timeout to
// receive a handshake fires.
func (c *configMaps) peerLostTimeout(id uuid.UUID) {
	logger := c.logger.With(slog.F("peer_id", id))
	logger.Debug(context.Background(),
		"peer lost timeout")

	// First do a status update to see if the peer did a handshake while we were
	// waiting
	status := c.status()
	c.L.Lock()
	defer c.L.Unlock()

	lc, ok := c.peers[id]
	if !ok {
		logger.Debug(context.Background(),
			"timeout triggered for peer that is removed from the map")
		return
	}
	if peerStatus, ok := status.Peer[lc.node.Key]; ok {
		lc.lastHandshake = peerStatus.LastHandshake
	}
	logger = logger.With(slog.F("key_id", lc.node.Key.ShortString()))
	if !lc.lost {
		logger.Debug(context.Background(),
			"timeout triggered for peer that is no longer lost")
		return
	}
	since := c.clock.Since(lc.lastHandshake)
	if since >= lostTimeout {
		logger.Info(
			context.Background(), "removing lost peer")
		delete(c.peers, id)
		c.netmapDirty = true
		c.Broadcast()
		return
	}
	logger.Debug(context.Background(),
		"timeout triggered for peer but it had handshake in meantime")
	lc.setLostTimer(c)
}

func (c *configMaps) protoNodeToTailcfg(p *proto.Node) (*tailcfg.Node, error) {
	node, err := ProtoToNode(p)
	if err != nil {
		return nil, err
	}
	return &tailcfg.Node{
		ID:         tailcfg.NodeID(p.GetId()),
		Created:    c.clock.Now(),
		Key:        node.Key,
		DiscoKey:   node.DiscoKey,
		Addresses:  node.Addresses,
		AllowedIPs: node.AllowedIPs,
		Endpoints:  node.Endpoints,
		DERP:       fmt.Sprintf("%s:%d", tailcfg.DerpMagicIP, node.PreferredDERP),
		Hostinfo:   (&tailcfg.Hostinfo{}).View(),
	}, nil
}

type peerLifecycle struct {
	peerID        uuid.UUID
	node          *tailcfg.Node
	lost          bool
	lastHandshake time.Time
	timer         *clock.Timer
}

func (l *peerLifecycle) resetTimer() {
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
}

func (l *peerLifecycle) setLostTimer(c *configMaps) {
	if l.timer != nil {
		l.timer.Stop()
	}
	ttl := lostTimeout - c.clock.Since(l.lastHandshake)
	if ttl <= 0 {
		ttl = time.Nanosecond
	}
	l.timer = c.clock.AfterFunc(ttl, func() {
		c.peerLostTimeout(l.peerID)
	})
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
