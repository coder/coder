package tailnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
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

	"github.com/coder/coder/coderd/database"
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

	// BlockEndpoints specifies whether P2P endpoints are blocked.
	// If so, only DERPs can establish connections.
	BlockEndpoints bool
	Logger         slog.Logger
}

// NewConn constructs a new Wireguard server that will accept connections from the addresses provided.
func NewConn(options *Options) (*Conn, error) {
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
	dialContext, dialCancel := context.WithCancel(context.Background())
	server := &Conn{
		blockEndpoints:   options.BlockEndpoints,
		dialContext:      dialContext,
		dialCancel:       dialCancel,
		closed:           make(chan struct{}),
		logger:           options.Logger,
		magicConn:        magicConn,
		dialer:           dialer,
		listeners:        map[listenKey]*listener{},
		peerMap:          map[tailcfg.NodeID]*tailcfg.Node{},
		tunDevice:        tunDevice,
		netMap:           netMap,
		netStack:         netStack,
		wireguardMonitor: wireguardMonitor,
		wireguardRouter: &router.Config{
			LocalAddrs: netMap.Addresses,
		},
		wireguardEngine: wireguardEngine,
	}
	wireguardEngine.SetStatusCallback(func(s *wgengine.Status, err error) {
		server.logger.Debug(context.Background(), "wireguard status", slog.F("status", s), slog.F("err", err))
		if err != nil {
			return
		}
		server.lastMutex.Lock()
		if s.AsOf.Before(server.lastStatus) {
			// Don't process outdated status!
			server.lastMutex.Unlock()
			return
		}
		server.lastStatus = s.AsOf
		server.lastEndpoints = make([]string, 0, len(s.LocalAddrs))
		for _, addr := range s.LocalAddrs {
			server.lastEndpoints = append(server.lastEndpoints, addr.Addr.String())
		}
		server.lastMutex.Unlock()
		server.sendNode()
	})
	wireguardEngine.SetNetInfoCallback(func(ni *tailcfg.NetInfo) {
		server.logger.Debug(context.Background(), "netinfo callback", slog.F("netinfo", ni))
		// If the lastMutex is blocked, it's possible that
		// multiple NetInfo callbacks occur at the same time.
		//
		// We need to ensure only the latest is sent!
		asOf := time.Now()
		server.lastMutex.Lock()
		if asOf.Before(server.lastNetInfo) {
			server.lastMutex.Unlock()
			return
		}
		server.lastNetInfo = asOf
		server.lastPreferredDERP = ni.PreferredDERP
		server.lastDERPLatency = ni.DERPLatency
		server.lastMutex.Unlock()
		server.sendNode()
	})
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

// Conn is an actively listening Wireguard connection.
type Conn struct {
	dialContext    context.Context
	dialCancel     context.CancelFunc
	mutex          sync.Mutex
	closed         chan struct{}
	logger         slog.Logger
	blockEndpoints bool

	dialer             *tsdial.Dialer
	tunDevice          *tstun.Wrapper
	peerMap            map[tailcfg.NodeID]*tailcfg.Node
	netMap             *netmap.NetworkMap
	netStack           *netstack.Impl
	magicConn          *magicsock.Conn
	wireguardMonitor   *monitor.Mon
	wireguardRouter    *router.Config
	wireguardEngine    wgengine.Engine
	listeners          map[listenKey]*listener
	forwardTCPCallback func(conn net.Conn, listenerExists bool) net.Conn

	lastMutex   sync.Mutex
	nodeSending bool
	nodeChanged bool
	// It's only possible to store these values via status functions,
	// so the values must be stored for retrieval later on.
	lastStatus        time.Time
	lastNetInfo       time.Time
	lastEndpoints     []string
	lastPreferredDERP int
	lastDERPLatency   map[string]float64
	nodeCallback      func(node *Node)
}

// SetForwardTCPCallback is called every time a TCP connection is initiated inbound.
// listenerExists is true if a listener is registered for the target port. If there
// isn't one, traffic is forwarded to the local listening port.
//
// This allows wrapping a Conn to track reads and writes.
func (c *Conn) SetForwardTCPCallback(callback func(conn net.Conn, listenerExists bool) net.Conn) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.forwardTCPCallback = callback
}

func (c *Conn) SetNodeCallback(callback func(node *Node)) {
	c.lastMutex.Lock()
	c.nodeCallback = callback
	c.lastMutex.Unlock()
	c.sendNode()
}

// SetDERPMap updates the DERPMap of a connection.
func (c *Conn) SetDERPMap(derpMap *tailcfg.DERPMap) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.logger.Debug(context.Background(), "updating derp map", slog.F("derp_map", derpMap))
	c.netMap.DERPMap = derpMap
	c.wireguardEngine.SetNetworkMap(c.netMap)
	c.wireguardEngine.SetDERPMap(derpMap)
}

// UpdateNodes connects with a set of peers. This can be constantly updated,
// and peers will continually be reconnected as necessary.
func (c *Conn) UpdateNodes(nodes []*Node) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	status := c.Status()
	for _, peer := range c.netMap.Peers {
		peerStatus, ok := status.Peer[peer.Key]
		if !ok {
			continue
		}
		// If this peer was added in the last 5 minutes, assume it
		// could still be active.
		if time.Since(peer.Created) < 5*time.Minute {
			continue
		}
		// We double-check that it's safe to remove by ensuring no
		// handshake has been sent in the past 5 minutes as well. Connections that
		// are actively exchanging IP traffic will handshake every 2 minutes.
		if time.Since(peerStatus.LastHandshake) < 5*time.Minute {
			continue
		}
		delete(c.peerMap, peer.ID)
	}
	for _, node := range nodes {
		c.logger.Debug(context.Background(), "adding node", slog.F("node", node))

		peerStatus, ok := status.Peer[node.Key]
		peerNode := &tailcfg.Node{
			ID:         node.ID,
			Created:    time.Now(),
			Key:        node.Key,
			DiscoKey:   node.DiscoKey,
			Addresses:  node.Addresses,
			AllowedIPs: node.AllowedIPs,
			Endpoints:  node.Endpoints,
			DERP:       fmt.Sprintf("%s:%d", tailcfg.DerpMagicIP, node.PreferredDERP),
			Hostinfo:   hostinfo.New().View(),
			// Starting KeepAlive messages at the initialization
			// of a connection cause it to hang for an unknown
			// reason. TODO: @kylecarbs debug this!
			KeepAlive: ok && peerStatus.Active,
		}
		// If no preferred DERP is provided, don't set an IP!
		if node.PreferredDERP == 0 {
			peerNode.DERP = ""
		}
		if c.blockEndpoints {
			peerNode.Endpoints = nil
		}
		c.peerMap[node.ID] = peerNode
	}
	c.netMap.Peers = make([]*tailcfg.Node, 0, len(c.peerMap))
	for _, peer := range c.peerMap {
		c.netMap.Peers = append(c.netMap.Peers, peer.Clone())
	}
	netMapCopy := *c.netMap
	c.wireguardEngine.SetNetworkMap(&netMapCopy)
	cfg, err := nmcfg.WGCfg(c.netMap, Logger(c.logger.Named("wgconfig")), netmap.AllowSingleHosts, "")
	if err != nil {
		return xerrors.Errorf("update wireguard config: %w", err)
	}
	err = c.wireguardEngine.Reconfig(cfg, c.wireguardRouter, &dns.Config{}, &tailcfg.Debug{})
	if err != nil {
		if c.isClosed() {
			return nil
		}
		if errors.Is(err, wgengine.ErrNoChanges) {
			return nil
		}
		return xerrors.Errorf("reconfig: %w", err)
	}
	return nil
}

// Status returns the current ipnstate of a connection.
func (c *Conn) Status() *ipnstate.Status {
	sb := &ipnstate.StatusBuilder{}
	c.wireguardEngine.UpdateStatus(sb)
	return sb.Status()
}

// Ping sends a Disco ping to the Wireguard engine.
func (c *Conn) Ping(ctx context.Context, ip netip.Addr) (time.Duration, error) {
	errCh := make(chan error, 1)
	durCh := make(chan time.Duration, 1)
	go c.wireguardEngine.Ping(ip, tailcfg.PingDisco, func(pr *ipnstate.PingResult) {
		if pr.Err != "" {
			errCh <- xerrors.New(pr.Err)
			return
		}
		durCh <- time.Duration(pr.LatencySeconds * float64(time.Second))
	})
	select {
	case err := <-errCh:
		return 0, err
	case <-ctx.Done():
		return 0, ctx.Err()
	case dur := <-durCh:
		return dur, nil
	}
}

// AwaitReachable pings the provided IP continually until the
// address is reachable. It's the callers responsibility to provide
// a timeout, otherwise this function will block forever.
func (c *Conn) AwaitReachable(ctx context.Context, ip netip.Addr) bool {
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	completedCtx, completed := context.WithCancel(ctx)
	run := func() {
		ctx, cancelFunc := context.WithTimeout(completedCtx, time.Second)
		defer cancelFunc()
		_, err := c.Ping(ctx, ip)
		if err == nil {
			completed()
		}
	}
	go run()
	defer completed()
	for {
		select {
		case <-completedCtx.Done():
			return true
		case <-ticker.C:
			// Pings can take a while, so we can run multiple
			// in parallel to return ASAP.
			go run()
		case <-ctx.Done():
			return false
		}
	}
}

// Closed is a channel that ends when the connection has
// been closed.
func (c *Conn) Closed() <-chan struct{} {
	return c.closed
}

// Close shuts down the Wireguard connection.
func (c *Conn) Close() error {
	c.mutex.Lock()
	select {
	case <-c.closed:
		c.mutex.Unlock()
		return nil
	default:
	}
	close(c.closed)
	for _, l := range c.listeners {
		_ = l.closeNoLock()
	}
	c.mutex.Unlock()
	c.dialCancel()
	_ = c.dialer.Close()
	_ = c.magicConn.Close()
	_ = c.netStack.Close()
	_ = c.wireguardMonitor.Close()
	_ = c.tunDevice.Close()
	c.wireguardEngine.Close()
	return nil
}

func (c *Conn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *Conn) sendNode() {
	c.lastMutex.Lock()
	defer c.lastMutex.Unlock()
	if c.nodeSending {
		c.nodeChanged = true
		return
	}
	node := &Node{
		ID:            c.netMap.SelfNode.ID,
		AsOf:          database.Now(),
		Key:           c.netMap.SelfNode.Key,
		Addresses:     c.netMap.SelfNode.Addresses,
		AllowedIPs:    c.netMap.SelfNode.AllowedIPs,
		DiscoKey:      c.magicConn.DiscoPublicKey(),
		Endpoints:     c.lastEndpoints,
		PreferredDERP: c.lastPreferredDERP,
		DERPLatency:   c.lastDERPLatency,
	}
	if c.blockEndpoints {
		node.Endpoints = nil
	}
	nodeCallback := c.nodeCallback
	if nodeCallback == nil {
		return
	}
	c.nodeSending = true
	go func() {
		c.logger.Debug(context.Background(), "sending node", slog.F("node", node))
		nodeCallback(node)
		c.lastMutex.Lock()
		c.nodeSending = false
		if c.nodeChanged {
			c.nodeChanged = false
			c.lastMutex.Unlock()
			c.sendNode()
			return
		}
		c.lastMutex.Unlock()
	}()
}

// This and below is taken _mostly_ verbatim from Tailscale:
// https://github.com/tailscale/tailscale/blob/c88bd53b1b7b2fcf7ba302f2e53dd1ce8c32dad4/tsnet/tsnet.go#L459-L494

// Listen announces only on the Tailscale network.
// It will start the server if it has not been started yet.
func (c *Conn) Listen(network, addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, xerrors.Errorf("wgnet: %w", err)
	}
	lk := listenKey{network, host, port}
	ln := &listener{
		s:    c,
		key:  lk,
		addr: addr,

		closed: make(chan struct{}),
		conn:   make(chan net.Conn),
	}
	c.mutex.Lock()
	if c.isClosed() {
		c.mutex.Unlock()
		return nil, xerrors.New("closed")
	}
	if c.listeners == nil {
		c.listeners = map[listenKey]*listener{}
	}
	if _, ok := c.listeners[lk]; ok {
		c.mutex.Unlock()
		return nil, xerrors.Errorf("wgnet: listener already open for %s, %s", network, addr)
	}
	c.listeners[lk] = ln
	c.mutex.Unlock()
	return ln, nil
}

func (c *Conn) DialContextTCP(ctx context.Context, ipp netip.AddrPort) (*gonet.TCPConn, error) {
	return c.netStack.DialContextTCP(ctx, ipp)
}

func (c *Conn) DialContextUDP(ctx context.Context, ipp netip.AddrPort) (*gonet.UDPConn, error) {
	return c.netStack.DialContextUDP(ctx, ipp)
}

func (c *Conn) forwardTCP(conn net.Conn, port uint16) {
	c.mutex.Lock()
	ln, ok := c.listeners[listenKey{"tcp", "", fmt.Sprint(port)}]
	if c.forwardTCPCallback != nil {
		conn = c.forwardTCPCallback(conn, ok)
	}
	c.mutex.Unlock()
	if !ok {
		c.forwardTCPToLocal(conn, port)
		return
	}
	t := time.NewTimer(time.Second)
	defer t.Stop()
	select {
	case ln.conn <- conn:
		return
	case <-ln.closed:
	case <-c.closed:
	case <-t.C:
	}
	_ = conn.Close()
}

func (c *Conn) forwardTCPToLocal(conn net.Conn, port uint16) {
	defer conn.Close()
	dialAddrStr := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port)))
	var stdDialer net.Dialer
	server, err := stdDialer.DialContext(c.dialContext, "tcp", dialAddrStr)
	if err != nil {
		c.logger.Debug(c.dialContext, "dial local port", slog.F("port", port), slog.Error(err))
		return
	}
	defer server.Close()

	connClosed := make(chan error, 2)
	go func() {
		_, err := io.Copy(server, conn)
		connClosed <- err
	}()
	go func() {
		_, err := io.Copy(conn, server)
		connClosed <- err
	}()
	select {
	case err = <-connClosed:
	case <-c.closed:
		return
	}
	if err != nil {
		c.logger.Debug(c.dialContext, "proxy connection closed with error", slog.Error(err))
	}
	c.logger.Debug(c.dialContext, "forwarded connection closed", slog.F("local_addr", dialAddrStr))
}

type listenKey struct {
	network string
	host    string
	port    string
}

type listener struct {
	s      *Conn
	key    listenKey
	addr   string
	conn   chan net.Conn
	closed chan struct{}
}

func (ln *listener) Accept() (net.Conn, error) {
	var c net.Conn
	select {
	case c = <-ln.conn:
	case <-ln.closed:
		return nil, xerrors.Errorf("wgnet: %w", net.ErrClosed)
	}
	return c, nil
}

func (ln *listener) Addr() net.Addr { return addr{ln} }
func (ln *listener) Close() error {
	ln.s.mutex.Lock()
	defer ln.s.mutex.Unlock()
	return ln.closeNoLock()
}

func (ln *listener) closeNoLock() error {
	if v, ok := ln.s.listeners[ln.key]; ok && v == ln {
		delete(ln.s.listeners, ln.key)
		close(ln.closed)
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
