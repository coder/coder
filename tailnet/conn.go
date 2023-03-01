package tailnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"go4.org/netipx"
	"golang.org/x/xerrors"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"tailscale.com/hostinfo"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/connstats"
	"tailscale.com/net/dns"
	"tailscale.com/net/netns"
	"tailscale.com/net/tsdial"
	"tailscale.com/net/tstun"
	"tailscale.com/tailcfg"
	"tailscale.com/types/ipproto"
	"tailscale.com/types/key"
	tslogger "tailscale.com/types/logger"
	"tailscale.com/types/netlogtype"
	"tailscale.com/types/netmap"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/filter"
	"tailscale.com/wgengine/magicsock"
	"tailscale.com/wgengine/monitor"
	"tailscale.com/wgengine/netstack"
	"tailscale.com/wgengine/router"
	"tailscale.com/wgengine/wgcfg/nmcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/cryptorand"
)

func init() {
	// Globally disable network namespacing. All networking happens in
	// userspace.
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
func NewConn(options *Options) (conn *Conn, err error) {
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
		DERPMap:    options.DERPMap,
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
	defer func() {
		if err != nil {
			wireguardMonitor.Close()
		}
	}()

	dialer := &tsdial.Dialer{
		Logf: Logger(options.Logger.Named("tsdial")),
	}
	wireguardEngine, err := wgengine.NewUserspaceEngine(Logger(options.Logger.Named("wgengine")), wgengine.Config{
		LinkMonitor: wireguardMonitor,
		Dialer:      dialer,
	})
	if err != nil {
		return nil, xerrors.Errorf("create wgengine: %w", err)
	}
	defer func() {
		if err != nil {
			wireguardEngine.Close()
		}
	}()
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
		return nil, xerrors.New("get wireguard internals")
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
	wireguardEngine = wgengine.NewWatchdog(wireguardEngine)
	wireguardEngine.SetDERPMap(options.DERPMap)
	netMapCopy := *netMap
	options.Logger.Debug(context.Background(), "updating network map")
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
		blockEndpoints:           options.BlockEndpoints,
		dialContext:              dialContext,
		dialCancel:               dialCancel,
		closed:                   make(chan struct{}),
		logger:                   options.Logger,
		magicConn:                magicConn,
		dialer:                   dialer,
		listeners:                map[listenKey]*listener{},
		peerMap:                  map[tailcfg.NodeID]*tailcfg.Node{},
		lastDERPForcedWebsockets: map[int]string{},
		tunDevice:                tunDevice,
		netMap:                   netMap,
		netStack:                 netStack,
		wireguardMonitor:         wireguardMonitor,
		wireguardRouter: &router.Config{
			LocalAddrs: netMap.Addresses,
		},
		wireguardEngine: wireguardEngine,
	}
	defer func() {
		if err != nil {
			_ = server.Close()
		}
	}()
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
		if endpointsEqual(s.LocalAddrs, server.lastEndpoints) {
			// No need to update the node if nothing changed!
			server.lastMutex.Unlock()
			return
		}
		server.lastEndpoints = append([]tailcfg.Endpoint{}, s.LocalAddrs...)
		server.lastMutex.Unlock()
		server.sendNode()
	})
	wireguardEngine.SetNetInfoCallback(func(ni *tailcfg.NetInfo) {
		server.logger.Debug(context.Background(), "netinfo callback", slog.F("netinfo", ni))
		server.lastMutex.Lock()
		if reflect.DeepEqual(server.lastNetInfo, ni) {
			server.lastMutex.Unlock()
			return
		}
		server.lastNetInfo = ni.Clone()
		server.lastMutex.Unlock()
		server.sendNode()
	})
	magicConn.SetDERPForcedWebsocketCallback(func(region int, reason string) {
		server.logger.Debug(context.Background(), "derp forced websocket", slog.F("region", region), slog.F("reason", reason))
		server.lastMutex.Lock()
		if server.lastDERPForcedWebsockets[region] == reason {
			server.lastMutex.Unlock()
			return
		}
		server.lastDERPForcedWebsockets[region] = reason
		server.lastMutex.Unlock()
		server.sendNode()
	})
	netStack.ForwardTCPIn = server.forwardTCP

	err = netStack.Start(nil)
	if err != nil {
		return nil, xerrors.Errorf("start netstack: %w", err)
	}

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
	lastStatus               time.Time
	lastEndpoints            []tailcfg.Endpoint
	lastDERPForcedWebsockets map[int]string
	lastNetInfo              *tailcfg.NetInfo
	nodeCallback             func(node *Node)

	trafficStats *connstats.Statistics
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
	c.wireguardEngine.SetDERPMap(derpMap)
	c.netMap.DERPMap = derpMap
	netMapCopy := *c.netMap
	c.logger.Debug(context.Background(), "updating network map")
	c.wireguardEngine.SetNetworkMap(&netMapCopy)
}

func (c *Conn) RemoveAllPeers() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.netMap.Peers = []*tailcfg.Node{}
	c.peerMap = map[tailcfg.NodeID]*tailcfg.Node{}
	netMapCopy := *c.netMap
	c.logger.Debug(context.Background(), "updating network map")
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

// UpdateNodes connects with a set of peers. This can be constantly updated,
// and peers will continually be reconnected as necessary. If replacePeers is
// true, all peers will be removed before adding the new ones.
//
//nolint:revive // Complains about replacePeers.
func (c *Conn) UpdateNodes(nodes []*Node, replacePeers bool) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	status := c.Status()
	if replacePeers {
		c.netMap.Peers = []*tailcfg.Node{}
		c.peerMap = map[tailcfg.NodeID]*tailcfg.Node{}
	}
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
		// If no preferred DERP is provided, we can't reach the node.
		if node.PreferredDERP == 0 {
			c.logger.Debug(context.Background(), "no preferred DERP, skipping node", slog.F("node", node))
			continue
		}
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
	c.logger.Debug(context.Background(), "updating network map")
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
	sb := &ipnstate.StatusBuilder{WantPeers: true}
	c.wireguardEngine.UpdateStatus(sb)
	return sb.Status()
}

// Ping sends a Disco ping to the Wireguard engine.
// The bool returned is true if the ping was performed P2P.
func (c *Conn) Ping(ctx context.Context, ip netip.Addr) (time.Duration, bool, *ipnstate.PingResult, error) {
	errCh := make(chan error, 1)
	prChan := make(chan *ipnstate.PingResult, 1)
	go c.wireguardEngine.Ping(ip, tailcfg.PingDisco, func(pr *ipnstate.PingResult) {
		if pr.Err != "" {
			errCh <- xerrors.New(pr.Err)
			return
		}
		prChan <- pr
	})
	select {
	case err := <-errCh:
		return 0, false, nil, err
	case <-ctx.Done():
		return 0, false, nil, ctx.Err()
	case pr := <-prChan:
		return time.Duration(pr.LatencySeconds * float64(time.Second)), pr.Endpoint != "", pr, nil
	}
}

// DERPMap returns the currently set DERP mapping.
func (c *Conn) DERPMap() *tailcfg.DERPMap {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.netMap.DERPMap
}

// AwaitReachable pings the provided IP continually until the
// address is reachable. It's the callers responsibility to provide
// a timeout, otherwise this function will block forever.
func (c *Conn) AwaitReachable(ctx context.Context, ip netip.Addr) bool {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // Cancel all pending pings on exit.

	completedCtx, completed := context.WithCancel(context.Background())
	defer completed()

	run := func() {
		// Safety timeout, initially we'll have around 10-20 goroutines
		// running in parallel. The exponential backoff will converge
		// around ~1 ping / 30s, this means we'll have around 10-20
		// goroutines pending towards the end as well.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		_, _, _, err := c.Ping(ctx, ip)
		if err == nil {
			completed()
		}
	}

	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0
	eb.InitialInterval = 50 * time.Millisecond
	eb.MaxInterval = 30 * time.Second
	// Consume the first interval since
	// we'll fire off a ping immediately.
	_ = eb.NextBackOff()

	t := backoff.NewTicker(eb)
	defer t.Stop()

	go run()
	for {
		select {
		case <-completedCtx.Done():
			return true
		case <-t.C:
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
	c.mutex.Unlock()

	var wg sync.WaitGroup
	defer wg.Wait()

	if c.trafficStats != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.trafficStats.Shutdown(ctx)
		}()
	}

	_ = c.netStack.Close()
	c.dialCancel()
	_ = c.wireguardMonitor.Close()
	_ = c.dialer.Close()
	// Stops internals, e.g. tunDevice, magicConn and dnsManager.
	c.wireguardEngine.Close()

	c.mutex.Lock()
	for _, l := range c.listeners {
		_ = l.closeNoLock()
	}
	c.listeners = nil
	c.mutex.Unlock()

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
	node := c.selfNode()
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

// Node returns the last node that was sent to the node callback.
func (c *Conn) Node() *Node {
	c.lastMutex.Lock()
	defer c.lastMutex.Unlock()
	return c.selfNode()
}

func (c *Conn) selfNode() *Node {
	endpoints := make([]string, 0, len(c.lastEndpoints))
	for _, addr := range c.lastEndpoints {
		endpoints = append(endpoints, addr.Addr.String())
	}
	var preferredDERP int
	var derpLatency map[string]float64
	var derpForcedWebsocket map[int]string
	if c.lastNetInfo != nil {
		preferredDERP = c.lastNetInfo.PreferredDERP
		derpLatency = c.lastNetInfo.DERPLatency
		derpForcedWebsocket = c.lastDERPForcedWebsockets
	}

	node := &Node{
		ID:                  c.netMap.SelfNode.ID,
		AsOf:                database.Now(),
		Key:                 c.netMap.SelfNode.Key,
		Addresses:           c.netMap.SelfNode.Addresses,
		AllowedIPs:          c.netMap.SelfNode.AllowedIPs,
		DiscoKey:            c.magicConn.DiscoPublicKey(),
		Endpoints:           endpoints,
		PreferredDERP:       preferredDERP,
		DERPLatency:         derpLatency,
		DERPForcedWebsocket: derpForcedWebsocket,
	}
	if c.blockEndpoints {
		node.Endpoints = nil
	}
	return node
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

// SetConnStatsCallback sets a callback to be called after maxPeriod or
// maxConns, whichever comes first. Multiple calls overwrites the callback.
func (c *Conn) SetConnStatsCallback(maxPeriod time.Duration, maxConns int, dump func(start, end time.Time, virtual, physical map[netlogtype.Connection]netlogtype.Counts)) {
	connStats := connstats.NewStatistics(maxPeriod, maxConns, dump)

	shutdown := func(s *connstats.Statistics) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}

	c.mutex.Lock()
	if c.isClosed() {
		c.mutex.Unlock()
		shutdown(connStats)
		return
	}
	old := c.trafficStats
	c.trafficStats = connStats
	c.mutex.Unlock()

	// Make sure to shutdown the old callback.
	if old != nil {
		shutdown(old)
	}

	c.tunDevice.SetStatistics(connStats)
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
		slog.Helper()
		logger.Debug(context.Background(), fmt.Sprintf(format, args...))
	})
}

func endpointsEqual(x, y []tailcfg.Endpoint) bool {
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
