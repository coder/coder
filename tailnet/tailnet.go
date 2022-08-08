package tailnet

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/google/uuid"
	"go4.org/netipx"
	"golang.org/x/xerrors"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"tailscale.com/hostinfo"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/dns"
	"tailscale.com/net/netns"
	"tailscale.com/net/tsdial"
	"tailscale.com/net/tstun"
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

	"github.com/coder/coder/cryptorand"

	"cdr.dev/slog"
)

func init() {
	// Globally disable network namespacing.
	// All networking happens in userspace.
	netns.SetEnabled(false)
}

type Options struct {
	Addresses []netip.Prefix
	DERPMap   *tailcfg.DERPMap

	Logger slog.Logger
}

// New constructs a new Wireguard server that will accept connections from the addresses provided.
func New(options *Options) (*Server, error) {
	if options == nil {
		options = &Options{}
	}
	if len(options.Addresses) == 0 {
		return nil, xerrors.New("At least one IP range must be provided")
	}
	if options.DERPMap == nil {
		return nil, xerrors.New("DERPMap must be provided")
	}
	nodePrivateKey := key.NewNode()
	nodePublicKey := nodePrivateKey.Public()

	netMap := &netmap.NetworkMap{
		NodeKey:    nodePublicKey,
		PrivateKey: nodePrivateKey,
		Addresses:  options.Addresses,
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
	}
	nodeID, err := cryptorand.Int63()
	if err != nil {
		return nil, xerrors.Errorf("generate node id: %w", err)
	}
	// This is used by functions below to identify the node via key
	netMap.SelfNode = &tailcfg.Node{
		ID:         tailcfg.NodeID(nodeID),
		Key:        nodePublicKey,
		Addresses:  options.Addresses,
		AllowedIPs: options.Addresses,
	}

	wireguardMonitor, err := monitor.New(Logger(options.Logger.Named("wgmonitor")))
	if err != nil {
		return nil, xerrors.Errorf("create wireguard link monitor: %w", err)
	}

	dialer := &tsdial.Dialer{
		Logf: Logger(options.Logger),
	}
	wireguardEngine, err := wgengine.NewUserspaceEngine(Logger(options.Logger.Named("wgengine")), wgengine.Config{
		LinkMonitor: wireguardMonitor,
		Dialer:      dialer,
	})
	if err != nil {
		return nil, xerrors.Errorf("create wgengine: %w", err)
	}
	dialer.UseNetstackForIP = func(ip netip.Addr) bool {
		_, ok := wireguardEngine.PeerForIP(ip)
		return ok
	}

	// This is taken from Tailscale:
	// https://github.com/tailscale/tailscale/blob/0f05b2c13ff0c305aa7a1655fa9c17ed969d65be/tsnet/tsnet.go#L247-L255
	wireguardInternals, ok := wireguardEngine.(wgengine.InternalsGetter)
	if !ok {
		return nil, xerrors.Errorf("wireguard engine isn't the correct type %T", wireguardEngine)
	}
	tunDevice, magicConn, dnsManager, ok := wireguardInternals.GetInternals()
	if !ok {
		return nil, xerrors.New("failed to get wireguard internals")
	}

	// Update the keys for the magic connection!
	err = magicConn.SetPrivateKey(nodePrivateKey)
	if err != nil {
		return nil, xerrors.Errorf("set node private key: %w", err)
	}
	netMap.SelfNode.DiscoKey = magicConn.DiscoPublicKey()

	netStack, err := netstack.Create(
		Logger(options.Logger.Named("netstack")), tunDevice, wireguardEngine, magicConn, dialer, dnsManager)
	if err != nil {
		return nil, xerrors.Errorf("create netstack: %w", err)
	}
	dialer.NetstackDialTCP = func(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
		return netStack.DialContextTCP(ctx, dst)
	}
	netStack.ProcessLocalIPs = true
	err = netStack.Start()
	if err != nil {
		return nil, xerrors.Errorf("start netstack: %w", err)
	}
	wireguardEngine = wgengine.NewWatchdog(wireguardEngine)

	// Update the wireguard configuration to allow traffic to flow.
	wireguardConfig, err := nmcfg.WGCfg(netMap, Logger(options.Logger.Named("wgconfig")), netmap.AllowSingleHosts, "")
	if err != nil {
		return nil, xerrors.Errorf("create wgcfg: %w", err)
	}

	wireguardRouter := &router.Config{
		LocalAddrs: wireguardConfig.Addresses,
	}
	err = wireguardEngine.Reconfig(wireguardConfig, wireguardRouter, &dns.Config{}, &tailcfg.Debug{})
	if err != nil {
		return nil, xerrors.Errorf("reconfig: %w", err)
	}

	wireguardEngine.SetDERPMap(options.DERPMap)
	netMapCopy := *netMap
	wireguardEngine.SetNetworkMap(&netMapCopy)

	localIPSet := netipx.IPSetBuilder{}
	for _, addr := range netMap.Addresses {
		localIPSet.AddPrefix(addr)
	}
	localIPs, _ := localIPSet.IPSet()
	logIPSet := netipx.IPSetBuilder{}
	logIPs, _ := logIPSet.IPSet()
	wireguardEngine.SetFilter(filter.New(netMap.PacketFilter, localIPs, logIPs, nil, Logger(options.Logger.Named("packet-filter"))))
	server := &Server{
		logger:           options.Logger,
		magicConn:        magicConn,
		dialer:           dialer,
		listeners:        map[listenKey]*listener{},
		tunDevice:        tunDevice,
		netMap:           netMap,
		netStack:         netStack,
		wireguardMonitor: wireguardMonitor,
		wireguardRouter:  wireguardRouter,
		wireguardEngine:  wireguardEngine,
	}
	netStack.ForwardTCPIn = server.forwardTCP
	return server, nil
}

// IP generates a new IP with a static service prefix.
func IP() netip.Addr {
	// This is Tailscale's ephemeral service prefix.
	// This can be changed easily later-on, because
	// all of our nodes are ephemeral.
	// fd7a:115c:a1e0
	uid := uuid.New()
	uid[0] = 0xfd
	uid[1] = 0x7a
	uid[2] = 0x11
	uid[3] = 0x5c
	uid[4] = 0xa1
	uid[5] = 0xe0
	return netip.AddrFrom16(uid)
}

// Server is an actively listening Wireguard connection.
type Server struct {
	mutex  sync.Mutex
	logger slog.Logger

	dialer           *tsdial.Dialer
	tunDevice        *tstun.Wrapper
	netMap           *netmap.NetworkMap
	netStack         *netstack.Impl
	magicConn        *magicsock.Conn
	wireguardMonitor *monitor.Mon
	wireguardRouter  *router.Config
	wireguardEngine  wgengine.Engine
	listeners        map[listenKey]*listener
}

// SetNodeCallback is triggered when a network change occurs and peer
// renegotiation may be required. Clients should constantly be emitting
// node changes.
func (s *Server) SetNodeCallback(callback func(node *Node)) {
	s.magicConn.SetNetInfoCallback(func(ni *tailcfg.NetInfo) {
		s.logger.Info(context.Background(), "latency", slog.F("latency", ni.DERPLatency))
		callback(&Node{
			ID:            s.netMap.SelfNode.ID,
			Key:           s.netMap.SelfNode.Key,
			Addresses:     s.netMap.SelfNode.Addresses,
			AllowedIPs:    s.netMap.SelfNode.AllowedIPs,
			DiscoKey:      s.magicConn.DiscoPublicKey(),
			PreferredDERP: ni.PreferredDERP,
			DERPLatency:   ni.DERPLatency,
		})
	})
}

// UpdateNodes connects with a set of peers. This can be constantly updated,
// and peers will continually be reconnected as necessary.
func (s *Server) UpdateNodes(nodes []*Node) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	peerMap := map[tailcfg.NodeID]*tailcfg.Node{}
	for _, peer := range s.netMap.Peers {
		peerMap[peer.ID] = peer
	}
	for _, node := range nodes {
		peerMap[node.ID] = &tailcfg.Node{
			ID:         node.ID,
			Key:        node.Key,
			DiscoKey:   node.DiscoKey,
			Addresses:  node.Addresses,
			AllowedIPs: node.AllowedIPs,
			DERP:       fmt.Sprintf("%s:%d", tailcfg.DerpMagicIP, node.PreferredDERP),
			Hostinfo:   hostinfo.New().View(),
		}
	}
	s.netMap.Peers = make([]*tailcfg.Node, 0, len(peerMap))
	for _, peer := range peerMap {
		s.netMap.Peers = append(s.netMap.Peers, peer)
	}
	cfg, err := nmcfg.WGCfg(s.netMap, Logger(s.logger.Named("wgconfig")), netmap.AllowSingleHosts, "")
	if err != nil {
		return xerrors.Errorf("update wireguard config: %w", err)
	}
	err = s.wireguardEngine.Reconfig(cfg, s.wireguardRouter, &dns.Config{}, &tailcfg.Debug{})
	if err != nil {
		return xerrors.Errorf("reconfig: %w", err)
	}
	netMapCopy := *s.netMap
	s.wireguardEngine.SetNetworkMap(&netMapCopy)
	return nil
}

// Ping sends a ping to the Wireguard engine.
func (s *Server) Ping(ip netip.Addr, pingType tailcfg.PingType, cb func(*ipnstate.PingResult)) {
	s.wireguardEngine.Ping(ip, pingType, cb)
}

// Close shuts down the Wireguard connection.
func (s *Server) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, l := range s.listeners {
		_ = l.Close()
	}
	_ = s.dialer.Close()
	_ = s.magicConn.Close()
	_ = s.netStack.Close()
	_ = s.wireguardMonitor.Close()
	_ = s.tunDevice.Close()
	s.wireguardEngine.Close()
	return nil
}

// Node represents a node in the network.
type Node struct {
	ID            tailcfg.NodeID     `json:"id"`
	Key           key.NodePublic     `json:"key"`
	DiscoKey      key.DiscoPublic    `json:"disco"`
	PreferredDERP int                `json:"preferred_derp"`
	DERPLatency   map[string]float64 `json:"derp_latency"`
	Addresses     []netip.Prefix     `json:"addresses"`
	AllowedIPs    []netip.Prefix     `json:"allowed_ips"`
}

// This and below is taken _mostly_ verbatim from Tailscale:
// https://github.com/tailscale/tailscale/blob/c88bd53b1b7b2fcf7ba302f2e53dd1ce8c32dad4/tsnet/tsnet.go#L459-L494

// Listen announces only on the Tailscale network.
// It will start the server if it has not been started yet.
func (s *Server) Listen(network, addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, xerrors.Errorf("wgnet: %w", err)
	}
	lk := listenKey{network, host, port}
	ln := &listener{
		s:    s,
		key:  lk,
		addr: addr,

		conn: make(chan net.Conn),
	}
	s.mutex.Lock()
	if s.listeners == nil {
		s.listeners = map[listenKey]*listener{}
	}
	if _, ok := s.listeners[lk]; ok {
		s.mutex.Unlock()
		return nil, xerrors.Errorf("wgnet: listener already open for %s, %s", network, addr)
	}
	s.listeners[lk] = ln
	s.mutex.Unlock()
	return ln, nil
}

func (s *Server) DialContextTCP(ctx context.Context, ipp netip.AddrPort) (*gonet.TCPConn, error) {
	return s.netStack.DialContextTCP(ctx, ipp)
}

func (s *Server) DialContextUDP(ctx context.Context, ipp netip.AddrPort) (*gonet.UDPConn, error) {
	return s.netStack.DialContextUDP(ctx, ipp)
}

func (s *Server) forwardTCP(c net.Conn, port uint16) {
	s.mutex.Lock()
	ln, ok := s.listeners[listenKey{"tcp", "", fmt.Sprint(port)}]
	s.mutex.Unlock()
	if !ok {
		_ = c.Close()
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

type listenKey struct {
	network string
	host    string
	port    string
}

type listener struct {
	s    *Server
	key  listenKey
	addr string
	conn chan net.Conn
}

func (ln *listener) Accept() (net.Conn, error) {
	c, ok := <-ln.conn
	if !ok {
		return nil, xerrors.Errorf("wgnet: %w", net.ErrClosed)
	}
	return c, nil
}

func (ln *listener) Addr() net.Addr { return addr{ln} }
func (ln *listener) Close() error {
	ln.s.mutex.Lock()
	defer ln.s.mutex.Unlock()
	if v, ok := ln.s.listeners[ln.key]; ok && v == ln {
		delete(ln.s.listeners, ln.key)
		close(ln.conn)
	}
	return nil
}

type addr struct{ ln *listener }

func (a addr) Network() string { return a.ln.key.network }
func (a addr) String() string  { return a.ln.addr }

// Logger converts the Tailscale logging function to use slog.
func Logger(logger slog.Logger) tslogger.Logf {
	return tslogger.Logf(func(format string, args ...any) {
		logger.Debug(context.Background(), fmt.Sprintf(format, args...))
	})
}

// The exchanger is entirely in-memory and works based on connected nodes.
// It uses a PubSub system to dynamically add/remove nodes from the network
// and build a netmap based on connection ID.
//
// Each node is allocated it's own internal connection ID.
//
// The connecting node *just* requires information about the other node.
// The other node needs connection information of all the others.
