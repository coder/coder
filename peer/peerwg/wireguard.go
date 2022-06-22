package peerwg

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tabbed/pqtype"
	"golang.org/x/xerrors"
	"inet.af/netaddr"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/dns"
	"tailscale.com/net/netns"
	"tailscale.com/net/tsdial"
	"tailscale.com/tailcfg"
	"tailscale.com/types/ipproto"
	"tailscale.com/types/key"
	tslogger "tailscale.com/types/logger"
	"tailscale.com/types/netmap"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/filter"
	"tailscale.com/wgengine/magicsock"
	"tailscale.com/wgengine/monitor"
	"tailscale.com/wgengine/netstack"
	"tailscale.com/wgengine/router"
	"tailscale.com/wgengine/wgcfg/nmcfg"

	"cdr.dev/slog"
)

func UUIDToInet(uid uuid.UUID) pqtype.Inet {
	uid = privateUUID(uid)

	return pqtype.Inet{
		Valid: true,
		IPNet: net.IPNet{
			IP:   uid[:],
			Mask: net.CIDRMask(128, 128),
		},
	}
}

func UUIDToNetaddr(uid uuid.UUID) netaddr.IP {
	return netaddr.IPFrom16(privateUUID(uid))
}

// privateUUID sets the uid to have the tailscale private ipv6 prefix.
func privateUUID(uid uuid.UUID) uuid.UUID {
	// fd7a:115c:a1e0
	uid[0] = 0xfd
	uid[1] = 0x7a
	uid[2] = 0x11
	uid[3] = 0x5c
	uid[4] = 0xa1
	uid[5] = 0xe0
	return uid
}

var logf tslogger.Logf = log.Printf

type WireguardNetwork struct {
	mu      sync.Mutex
	logger  slog.Logger
	Private key.NodePrivate
	Disco   key.DiscoPublic

	Engine   wgengine.Engine
	Netstack *netstack.Impl
	Magic    *magicsock.Conn

	netMap    *netmap.NetworkMap
	router    *router.Config
	listeners map[listenKey]*listener
}

func NewWireguardNetwork(_ context.Context, logger slog.Logger, addrs []netaddr.IPPrefix) (*WireguardNetwork, error) {
	var (
		private      = key.NewNode()
		public       = private.Public()
		id, stableID = nodeIDs(public)
	)

	netMap := &netmap.NetworkMap{
		NodeKey:    public,
		PrivateKey: private,
		Addresses:  addrs,
		PacketFilter: []filter.Match{{
			IPProto: []ipproto.Proto{ipproto.TCP, ipproto.UDP, ipproto.ICMPv4, ipproto.ICMPv6},
			Srcs: []netaddr.IPPrefix{
				netaddr.IPPrefixFrom(netaddr.IPv4(0, 0, 0, 0), 0),
				netaddr.IPPrefixFrom(netaddr.IPv6Unspecified(), 0),
			},
			Dsts: []filter.NetPortRange{
				{
					Net: netaddr.IPPrefixFrom(netaddr.IPv4(0, 0, 0, 0), 0),
					Ports: filter.PortRange{
						First: 0,
						Last:  65535,
					},
				},
				{
					Net: netaddr.IPPrefixFrom(netaddr.IPv6Unspecified(), 0),
					Ports: filter.PortRange{
						First: 0,
						Last:  65535,
					},
				},
			},
			Caps: []filter.CapMatch{},
		}},
	}
	netMap.SelfNode = &tailcfg.Node{
		ID:         id,
		StableID:   stableID,
		Key:        public,
		Addresses:  netMap.Addresses,
		AllowedIPs: append(netMap.Addresses, netaddr.MustParseIPPrefix("::/0")),
		Endpoints:  []string{},
		DERP:       DefaultDerpHome,
	}

	linkMon, err := monitor.New(logf)
	if err != nil {
		return nil, xerrors.Errorf("create link monitor: %w", err)
	}

	netns.SetEnabled(false)
	dialer := new(tsdial.Dialer)
	dialer.Logf = logf
	e, err := wgengine.NewUserspaceEngine(logf, wgengine.Config{
		LinkMonitor: linkMon,
		Dialer:      dialer,
	})
	if err != nil {
		return nil, xerrors.Errorf("create wgengine: %w", err)
	}

	ig, _ := e.(wgengine.InternalsGetter)
	tunDev, magicConn, dnsMgr, ok := ig.GetInternals()
	if !ok {
		return nil, xerrors.New("could not get wgengine internals")
	}

	// This can't error.
	_ = magicConn.SetPrivateKey(private)
	netMap.SelfNode.DiscoKey = magicConn.DiscoPublicKey()

	ns, err := netstack.Create(logf, tunDev, e, magicConn, dialer, dnsMgr)
	if err != nil {
		return nil, xerrors.Errorf("create netstack: %w", err)
	}

	ns.ProcessLocalIPs = true
	ns.ProcessSubnets = true
	dialer.UseNetstackForIP = func(ip netaddr.IP) bool {
		_, ok := e.PeerForIP(ip)
		return ok
	}
	dialer.NetstackDialTCP = func(ctx context.Context, dst netaddr.IPPort) (net.Conn, error) {
		return ns.DialContextTCP(ctx, dst)
	}

	err = ns.Start()
	if err != nil {
		return nil, xerrors.Errorf("start netstack: %w", err)
	}
	e = wgengine.NewWatchdog(e)

	cfg, err := nmcfg.WGCfg(netMap, logf, netmap.AllowSingleHosts|netmap.AllowSubnetRoutes, netMap.SelfNode.StableID)
	if err != nil {
		return nil, xerrors.Errorf("create wgcfg: %w", err)
	}

	rtr := &router.Config{
		LocalAddrs: cfg.Addresses,
	}

	err = e.Reconfig(cfg, rtr, &dns.Config{}, &tailcfg.Debug{})
	if err != nil {
		return nil, xerrors.Errorf("reconfig: %w", err)
	}

	e.SetDERPMap(DerpMap)
	e.SetNetworkMap(copyNetMap(netMap))

	ipb := netaddr.IPSetBuilder{}
	for _, addr := range netMap.Addresses {
		ipb.AddPrefix(addr)
	}
	ips, _ := ipb.IPSet()

	iplb := netaddr.IPSetBuilder{}
	ipl, _ := iplb.IPSet()
	e.SetFilter(filter.New(netMap.PacketFilter, ips, ipl, nil, logf))

	wn := &WireguardNetwork{
		logger:    logger,
		Private:   private,
		Disco:     magicConn.DiscoPublicKey(),
		Engine:    e,
		Netstack:  ns,
		Magic:     magicConn,
		netMap:    netMap,
		router:    rtr,
		listeners: map[listenKey]*listener{},
	}
	ns.ForwardTCPIn = wn.forwardTCP

	return wn, nil
}

// nodeIDs generates Tailscale node IDs for the provided public key.
func nodeIDs(public key.NodePublic) (tailcfg.NodeID, tailcfg.StableNodeID) {
	idhash := fnv.New64()
	pub, _ := public.MarshalText()
	_, _ = idhash.Write(pub)

	return tailcfg.NodeID(idhash.Sum64()), tailcfg.StableNodeID(pub)
}

// forwardTCP handles incoming TCP connections from wireguard.
func (wn *WireguardNetwork) forwardTCP(c net.Conn, port uint16) {
	wn.mu.Lock()
	ln, ok := wn.listeners[listenKey{"tcp", "", fmt.Sprint(port)}]
	wn.mu.Unlock()
	if !ok {
		// No listener added, forward to host.
		wn.forwardTCPLocal(c, port)
		return
	}

	t := time.NewTimer(time.Second)
	defer t.Stop()
	select {
	case ln.conn <- c:
	case <-t.C:
		_ = c.Close()
	}
}

// forwardTCPLocal forwards the provided net.Conn to the matching port on the
// host.
func (wn *WireguardNetwork) forwardTCPLocal(c net.Conn, port uint16) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer c.Close()

	dialAddrStr := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port)))
	var stdDialer net.Dialer
	server, err := stdDialer.DialContext(ctx, "tcp", dialAddrStr)
	if err != nil {
		wn.logger.Debug(ctx, "dial local port", slog.F("port", port), slog.Error(err))
		return
	}
	defer server.Close()

	connClosed := make(chan error, 2)
	go func() {
		_, err := io.Copy(server, c)
		connClosed <- err
	}()
	go func() {
		_, err := io.Copy(c, server)
		connClosed <- err
	}()
	err = <-connClosed
	if err != nil {
		wn.logger.Debug(ctx, "proxy connection closed with error", slog.Error(err))
	}
	wn.logger.Debug(ctx, "forwarded connection closed", slog.F("local_addr", dialAddrStr))
}

func (wn *WireguardNetwork) Close() error {
	_ = wn.Netstack.Close()
	wn.Engine.Close()

	return nil
}

// AddPeer adds a peer to the network from a WireguardPeerMessage. After adding
// a peer, they may connect to you.
func (wn *WireguardNetwork) AddPeer(peer WireguardPeerMessage) error {
	wn.mu.Lock()
	defer wn.mu.Unlock()

	// If the peer already exists in the network map, do nothing.
	for _, p := range wn.netMap.Peers {
		if p.Key == peer.Public {
			wn.logger.Debug(context.Background(), "peer already in netmap", slog.F("peer", peer.Public.ShortString()))
			return nil
		}
	}

	// The Tailscale engine owns this slice, so we need to copy to make
	// modifications.
	peers := append(([]*tailcfg.Node)(nil), wn.netMap.Peers...)

	id, stableID := nodeIDs(peer.Public)
	peers = append(peers, &tailcfg.Node{
		ID:         id,
		StableID:   stableID,
		Name:       peer.Public.String() + ".com",
		Key:        peer.Public,
		DiscoKey:   peer.Disco,
		Addresses:  []netaddr.IPPrefix{netaddr.IPPrefixFrom(peer.IPv6, 128)},
		AllowedIPs: []netaddr.IPPrefix{netaddr.IPPrefixFrom(peer.IPv6, 128)},
		DERP:       DefaultDerpHome,
		Endpoints:  []string{DefaultDerpHome},
	})

	wn.netMap.Peers = peers

	cfg, err := nmcfg.WGCfg(wn.netMap, logf, netmap.AllowSingleHosts|netmap.AllowSubnetRoutes, tailcfg.StableNodeID("nBBoJZ5CNTRL"))
	if err != nil {
		return xerrors.Errorf("create wgcfg: %w", err)
	}

	err = wn.Engine.Reconfig(cfg, wn.router, &dns.Config{}, &tailcfg.Debug{})
	if err != nil {
		return xerrors.Errorf("reconfig: %w", err)
	}

	// Always give the Tailscale engine a copy of our network map.
	wn.Engine.SetNetworkMap(copyNetMap(wn.netMap))
	return nil
}

func copyNetMap(nm *netmap.NetworkMap) *netmap.NetworkMap {
	nmCopy := *nm
	return &nmCopy
}

// Ping sends a discovery ping to the provided peer.
func (wn *WireguardNetwork) Ping(peer WireguardPeerMessage) *ipnstate.PingResult {
	ch := make(chan *ipnstate.PingResult)
	wn.Engine.Ping(peer.IPv6, tailcfg.PingDisco, func(pr *ipnstate.PingResult) {
		ch <- pr
	})

	return <-ch
}

// Listen returns a net.Listener that can be used to accept connections from the
// wireguard network at the specified address.
func (wn *WireguardNetwork) Listen(network, addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, xerrors.Errorf("split addr host port: %w", err)
	}

	lkey := listenKey{network, host, port}
	ln := &listener{
		wn:   wn,
		key:  lkey,
		addr: addr,

		conn: make(chan net.Conn, 1),
	}

	wn.mu.Lock()
	defer wn.mu.Unlock()

	if _, ok := wn.listeners[lkey]; ok {
		return nil, xerrors.Errorf("listener already open for %s, %s", network, addr)
	}
	wn.listeners[lkey] = ln

	return ln, nil
}

type listenKey struct {
	network string
	host    string
	port    string
}

type listener struct {
	wn   *WireguardNetwork
	key  listenKey
	addr string
	conn chan net.Conn
}

func (ln *listener) Accept() (net.Conn, error) {
	c, ok := <-ln.conn
	if !ok {
		return nil, xerrors.Errorf("tsnet: %w", net.ErrClosed)
	}
	return c, nil
}

func (ln *listener) Addr() net.Addr { return addr{ln} }
func (ln *listener) Close() error {
	ln.wn.mu.Lock()
	defer ln.wn.mu.Unlock()

	if v, ok := ln.wn.listeners[ln.key]; ok && v == ln {
		delete(ln.wn.listeners, ln.key)
		close(ln.conn)
	}

	return nil
}

type addr struct{ ln *listener }

func (a addr) Network() string { return a.ln.key.network }
func (a addr) String() string  { return a.ln.addr }
