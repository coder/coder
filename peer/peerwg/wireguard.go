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

var logf tslogger.Logf = log.Printf

func init() {
	// Globally disable network namespacing.
	// All networking happens in userspace.
	netns.SetEnabled(false)
}

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

type Network struct {
	mu     sync.Mutex
	logger slog.Logger

	listeners map[listenKey]*listener
	magicSock *magicsock.Conn
	netMap    *netmap.NetworkMap
	router    *router.Config
	wgEngine  wgengine.Engine

	DiscoPublicKey key.DiscoPublic
	Netstack       *netstack.Impl
	NodePrivateKey key.NodePrivate
}

// New constructs a Wireguard network that filters traffic
// to destinations matching the addresses provided.
func New(logger slog.Logger, addresses []netaddr.IPPrefix) (*Network, error) {
	nodePrivateKey := key.NewNode()
	nodePublicKey := nodePrivateKey.Public()
	id, stableID := nodeIDs(nodePublicKey)

	netMap := &netmap.NetworkMap{
		NodeKey:    nodePublicKey,
		PrivateKey: nodePrivateKey,
		Addresses:  addresses,
		PacketFilter: []filter.Match{{
			// Allow any protocol!
			IPProto: []ipproto.Proto{ipproto.TCP, ipproto.UDP, ipproto.ICMPv4, ipproto.ICMPv6, ipproto.SCTP},
			// Allow traffic sourced from anywhere.
			Srcs: []netaddr.IPPrefix{
				netaddr.IPPrefixFrom(netaddr.IPv4(0, 0, 0, 0), 0),
				netaddr.IPPrefixFrom(netaddr.IPv6Unspecified(), 0),
			},
			// Allow traffic to route anywhere.
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
	// Identify itself as a node on the network with the addresses provided.
	netMap.SelfNode = &tailcfg.Node{
		ID:         id,
		StableID:   stableID,
		Key:        nodePublicKey,
		Addresses:  netMap.Addresses,
		AllowedIPs: append(netMap.Addresses, netaddr.MustParseIPPrefix("::/0")),
		Endpoints:  []string{},
		DERP:       DefaultDerpHome,
	}

	wgMonitor, err := monitor.New(logf)
	if err != nil {
		return nil, xerrors.Errorf("create link monitor: %w", err)
	}

	dialer := new(tsdial.Dialer)
	dialer.Logf = logf
	// Create a wireguard engine in userspace.
	engine, err := wgengine.NewUserspaceEngine(logf, wgengine.Config{
		LinkMonitor: wgMonitor,
		Dialer:      dialer,
	})
	if err != nil {
		return nil, xerrors.Errorf("create wgengine: %w", err)
	}

	// This is taken from Tailscale:
	// https://github.com/tailscale/tailscale/blob/0f05b2c13ff0c305aa7a1655fa9c17ed969d65be/tsnet/tsnet.go#L247-L255
	// nolint
	tunDev, magicConn, dnsManager, ok := engine.(wgengine.InternalsGetter).GetInternals()
	if !ok {
		return nil, xerrors.New("could not get wgengine internals")
	}

	// Update the keys for the magic connection!
	err = magicConn.SetPrivateKey(nodePrivateKey)
	if err != nil {
		return nil, xerrors.Errorf("set node private key: %w", err)
	}
	netMap.SelfNode.DiscoKey = magicConn.DiscoPublicKey()

	// Create the networking stack.
	// This is called to route connections.
	netStack, err := netstack.Create(logf, tunDev, engine, magicConn, dialer, dnsManager)
	if err != nil {
		return nil, xerrors.Errorf("create netstack: %w", err)
	}
	netStack.ProcessLocalIPs = true
	netStack.ProcessSubnets = true
	dialer.UseNetstackForIP = func(ip netaddr.IP) bool {
		_, ok := engine.PeerForIP(ip)
		return ok
	}
	dialer.NetstackDialTCP = func(ctx context.Context, dst netaddr.IPPort) (net.Conn, error) {
		return netStack.DialContextTCP(ctx, dst)
	}
	err = netStack.Start()
	if err != nil {
		return nil, xerrors.Errorf("start netstack: %w", err)
	}
	engine = wgengine.NewWatchdog(engine)

	// Update the wireguard configuration to allow traffic to flow.
	cfg, err := nmcfg.WGCfg(netMap, logf, netmap.AllowSingleHosts|netmap.AllowSubnetRoutes, netMap.SelfNode.StableID)
	if err != nil {
		return nil, xerrors.Errorf("create wgcfg: %w", err)
	}

	rtr := &router.Config{
		LocalAddrs: cfg.Addresses,
	}
	err = engine.Reconfig(cfg, rtr, &dns.Config{}, &tailcfg.Debug{})
	if err != nil {
		return nil, xerrors.Errorf("reconfig: %w", err)
	}

	engine.SetDERPMap(DerpMap)
	engine.SetNetworkMap(copyNetMap(netMap))

	ipb := netaddr.IPSetBuilder{}
	for _, addr := range netMap.Addresses {
		ipb.AddPrefix(addr)
	}
	ips, _ := ipb.IPSet()

	iplb := netaddr.IPSetBuilder{}
	ipl, _ := iplb.IPSet()
	engine.SetFilter(filter.New(netMap.PacketFilter, ips, ipl, nil, logf))

	wn := &Network{
		logger:         logger,
		NodePrivateKey: nodePrivateKey,
		DiscoPublicKey: magicConn.DiscoPublicKey(),
		wgEngine:       engine,
		Netstack:       netStack,
		magicSock:      magicConn,
		netMap:         netMap,
		router:         rtr,
		listeners:      map[listenKey]*listener{},
	}
	netStack.ForwardTCPIn = wn.forwardTCP

	return wn, nil
}

// forwardTCP handles incoming connections from Wireguard in userspace.
func (n *Network) forwardTCP(conn net.Conn, port uint16) {
	n.mu.Lock()
	listener, ok := n.listeners[listenKey{"tcp", "", fmt.Sprint(port)}]
	n.mu.Unlock()
	if !ok {
		// No listener added, forward to host.
		n.forwardTCPToLocalHandler(conn, port)
		return
	}

	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case listener.conn <- conn:
	case <-timer.C:
		_ = conn.Close()
	}
}

// forwardTCPToLocalHandler forwards the provided net.Conn to the
// matching port bound to localhost.
func (n *Network) forwardTCPToLocalHandler(c net.Conn, port uint16) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer c.Close()

	dialAddrStr := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port)))
	var stdDialer net.Dialer
	server, err := stdDialer.DialContext(ctx, "tcp", dialAddrStr)
	if err != nil {
		n.logger.Debug(ctx, "dial local port", slog.F("port", port), slog.Error(err))
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
		n.logger.Debug(ctx, "proxy connection closed with error", slog.Error(err))
	}
	n.logger.Debug(ctx, "forwarded connection closed", slog.F("local_addr", dialAddrStr))
}

// AddPeer allows connections from another Wireguard instance with the
// handshake credentials.
func (n *Network) AddPeer(handshake Handshake) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// If the peer already exists in the network map, do nothing.
	for _, p := range n.netMap.Peers {
		if p.Key == handshake.NodePublicKey {
			n.logger.Debug(context.Background(), "peer already in netmap", slog.F("peer", handshake.NodePublicKey.ShortString()))
			return nil
		}
	}

	// The Tailscale engine owns this slice, so we need to copy to make
	// modifications.
	peers := append(([]*tailcfg.Node)(nil), n.netMap.Peers...)

	id, stableID := nodeIDs(handshake.NodePublicKey)
	peers = append(peers, &tailcfg.Node{
		ID:         id,
		StableID:   stableID,
		Name:       handshake.NodePublicKey.String() + ".com",
		Key:        handshake.NodePublicKey,
		DiscoKey:   handshake.DiscoPublicKey,
		Addresses:  []netaddr.IPPrefix{netaddr.IPPrefixFrom(handshake.IPv6, 128)},
		AllowedIPs: []netaddr.IPPrefix{netaddr.IPPrefixFrom(handshake.IPv6, 128)},
		DERP:       DefaultDerpHome,
		Endpoints:  []string{DefaultDerpHome},
	})

	n.netMap.Peers = peers

	cfg, err := nmcfg.WGCfg(n.netMap, logf, netmap.AllowSingleHosts|netmap.AllowSubnetRoutes, tailcfg.StableNodeID("nBBoJZ5CNTRL"))
	if err != nil {
		return xerrors.Errorf("create wgcfg: %w", err)
	}

	err = n.wgEngine.Reconfig(cfg, n.router, &dns.Config{}, &tailcfg.Debug{})
	if err != nil {
		return xerrors.Errorf("reconfig: %w", err)
	}

	// Always give the Tailscale engine a copy of our network map.
	n.wgEngine.SetNetworkMap(copyNetMap(n.netMap))
	return nil
}

// Ping sends a discovery ping to the provided peer.
// The peer address must be connected before a successful ping will work.
func (n *Network) Ping(ip netaddr.IP) *ipnstate.PingResult {
	ch := make(chan *ipnstate.PingResult)
	n.wgEngine.Ping(ip, tailcfg.PingDisco, func(pr *ipnstate.PingResult) {
		ch <- pr
	})
	return <-ch
}

// Listener returns a net.Listener in userspace that can be used to accept
// connections from the Wireguard network to the specified address.
func (n *Network) Listen(network, addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, xerrors.Errorf("split addr host port: %w", err)
	}

	lkey := listenKey{network, host, port}
	ln := &listener{
		wn:   n,
		key:  lkey,
		addr: addr,

		conn: make(chan net.Conn, 1),
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.listeners[lkey]; ok {
		return nil, xerrors.Errorf("listener already open for %s, %s", network, addr)
	}
	n.listeners[lkey] = ln

	return ln, nil
}

func (n *Network) Close() error {
	_ = n.Netstack.Close()
	n.wgEngine.Close()

	return nil
}

type listenKey struct {
	network string
	host    string
	port    string
}

type listener struct {
	wn   *Network
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

// nodeIDs generates Tailscale node IDs for the provided public key.
func nodeIDs(public key.NodePublic) (tailcfg.NodeID, tailcfg.StableNodeID) {
	idhash := fnv.New64()
	pub, _ := public.MarshalText()
	_, _ = idhash.Write(pub)

	return tailcfg.NodeID(idhash.Sum64()), tailcfg.StableNodeID(pub)
}

func copyNetMap(nm *netmap.NetworkMap) *netmap.NetworkMap {
	nmCopy := *nm
	return &nmCopy
}
