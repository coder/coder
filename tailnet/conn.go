package tailnet

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"go4.org/netipx"
	"golang.org/x/xerrors"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"tailscale.com/envknob"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/connstats"
	"tailscale.com/net/dns"
	"tailscale.com/net/netmon"
	"tailscale.com/net/netns"
	"tailscale.com/net/tsdial"
	"tailscale.com/net/tstun"
	"tailscale.com/tailcfg"
	"tailscale.com/tsd"
	"tailscale.com/types/ipproto"
	"tailscale.com/types/key"
	tslogger "tailscale.com/types/logger"
	"tailscale.com/types/netlogtype"
	"tailscale.com/types/netmap"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/filter"
	"tailscale.com/wgengine/magicsock"
	"tailscale.com/wgengine/netstack"
	"tailscale.com/wgengine/router"
	"tailscale.com/wgengine/wgcfg/nmcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/cryptorand"
)

var ErrConnClosed = xerrors.New("connection closed")

const (
	WorkspaceAgentSSHPort             = 1
	WorkspaceAgentReconnectingPTYPort = 2
	WorkspaceAgentSpeedtestPort       = 3
)

// EnvMagicsockDebugLogging enables super-verbose logging for the magicsock
// internals. A logger must be supplied to the connection with the debug level
// enabled.
//
// With this disabled, you still get a lot of output if you have a valid logger
// with the debug level enabled.
const EnvMagicsockDebugLogging = "CODER_MAGICSOCK_DEBUG_LOGGING"

func init() {
	// Globally disable network namespacing. All networking happens in
	// userspace.
	netns.SetEnabled(false)
	// Tailscale, by default, "trims" the set of peers down to ones that we are
	// "actively" communicating with in an effort to save memory. Since
	// Tailscale removed keep-alives, it seems like open but idle connections
	// (SSH, port-forward, etc) can get trimmed fairly easily, causing hangs for
	// a few seconds while the connection is setup again.
	//
	// Note that Tailscale.com's use case is very different from ours: in their
	// use case, users create one persistent tailnet per device, and it allows
	// connections to every other thing in Tailscale that belongs to them.  The
	// tailnet stays up as long as your laptop or phone is turned on.
	//
	// Our use case is different: for clients, it's a point-to-point connection
	// to a single workspace, and lasts only as long as the connection.  For
	// agents, it's connections to a small number of clients (CLI or Coderd)
	// that are being actively used by the end user.
	envknob.Setenv("TS_DEBUG_TRIM_WIREGUARD", "false")
}

type Options struct {
	ID         uuid.UUID
	Addresses  []netip.Prefix
	DERPMap    *tailcfg.DERPMap
	DERPHeader *http.Header
	// DERPForceWebSockets determines whether websockets is always used for DERP
	// connections, rather than trying `Upgrade: derp` first and potentially
	// falling back. This is useful for misbehaving proxies that prevent
	// fallback due to odd behavior, like Azure App Proxy.
	DERPForceWebSockets bool

	// BlockEndpoints specifies whether P2P endpoints are blocked.
	// If so, only DERPs can establish connections.
	BlockEndpoints bool
	Logger         slog.Logger
	ListenPort     uint16
}

// NodeID creates a Tailscale NodeID from the last 8 bytes of a UUID. It ensures
// the returned NodeID is always positive.
func NodeID(uid uuid.UUID) tailcfg.NodeID {
	id := int64(binary.BigEndian.Uint64(uid[8:]))

	// ensure id is positive
	y := id >> 63
	id = (id ^ y) - y

	return tailcfg.NodeID(id)
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

	var nodeID tailcfg.NodeID

	// If we're provided with a UUID, use it to populate our node ID.
	if options.ID != uuid.Nil {
		nodeID = NodeID(options.ID)
	} else {
		uid, err := cryptorand.Int63()
		if err != nil {
			return nil, xerrors.Errorf("generate node id: %w", err)
		}
		nodeID = tailcfg.NodeID(uid)
	}

	// This is used by functions below to identify the node via key
	netMap.SelfNode = &tailcfg.Node{
		ID:         nodeID,
		Key:        nodePublicKey,
		Addresses:  options.Addresses,
		AllowedIPs: options.Addresses,
	}

	wireguardMonitor, err := netmon.New(Logger(options.Logger.Named("net.wgmonitor")))
	if err != nil {
		return nil, xerrors.Errorf("create wireguard link monitor: %w", err)
	}
	defer func() {
		if err != nil {
			wireguardMonitor.Close()
		}
	}()

	dialer := &tsdial.Dialer{
		Logf: Logger(options.Logger.Named("net.tsdial")),
	}
	sys := new(tsd.System)
	wireguardEngine, err := wgengine.NewUserspaceEngine(Logger(options.Logger.Named("net.wgengine")), wgengine.Config{
		NetMon:       wireguardMonitor,
		Dialer:       dialer,
		ListenPort:   options.ListenPort,
		SetSubsystem: sys.Set,
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

	sys.Set(wireguardEngine)

	magicConn := sys.MagicSock.Get()
	magicConn.SetDERPForceWebsockets(options.DERPForceWebSockets)
	if options.DERPHeader != nil {
		magicConn.SetDERPHeader(options.DERPHeader.Clone())
	}

	if v, ok := os.LookupEnv(EnvMagicsockDebugLogging); ok {
		vBool, err := strconv.ParseBool(v)
		if err != nil {
			options.Logger.Debug(context.Background(), fmt.Sprintf("magicsock debug logging disabled due to invalid value %s=%q, use true or false", EnvMagicsockDebugLogging, v))
		} else {
			magicConn.SetDebugLoggingEnabled(vBool)
			options.Logger.Debug(context.Background(), fmt.Sprintf("magicsock debug logging set by %s=%t", EnvMagicsockDebugLogging, vBool))
		}
	} else {
		options.Logger.Debug(context.Background(), fmt.Sprintf("magicsock debug logging disabled, use %s=true to enable", EnvMagicsockDebugLogging))
	}

	// Update the keys for the magic connection!
	err = magicConn.SetPrivateKey(nodePrivateKey)
	if err != nil {
		return nil, xerrors.Errorf("set node private key: %w", err)
	}
	netMap.SelfNode.DiscoKey = magicConn.DiscoPublicKey()

	netStack, err := netstack.Create(
		Logger(options.Logger.Named("net.netstack")),
		sys.Tun.Get(),
		wireguardEngine,
		magicConn,
		dialer,
		sys.DNSManager.Get(),
	)
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
	wireguardEngine.SetFilter(filter.New(
		netMap.PacketFilter,
		localIPs,
		logIPs,
		nil,
		Logger(options.Logger.Named("net.packet-filter")),
	))

	dialContext, dialCancel := context.WithCancel(context.Background())
	server := &Conn{
		blockEndpoints:           options.BlockEndpoints,
		derpForceWebSockets:      options.DERPForceWebSockets,
		dialContext:              dialContext,
		dialCancel:               dialCancel,
		closed:                   make(chan struct{}),
		logger:                   options.Logger,
		magicConn:                magicConn,
		dialer:                   dialer,
		listeners:                map[listenKey]*listener{},
		peerMap:                  map[tailcfg.NodeID]*tailcfg.Node{},
		lastDERPForcedWebSockets: map[int]string{},
		tunDevice:                sys.Tun.Get(),
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
		server.logger.Debug(context.Background(), "wireguard status", slog.F("status", s), slog.Error(err))
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
		if server.lastDERPForcedWebSockets[region] == reason {
			server.lastMutex.Unlock()
			return
		}
		server.lastDERPForcedWebSockets[region] = reason
		server.lastMutex.Unlock()
		server.sendNode()
	})

	netStack.GetTCPHandlerForFlow = server.forwardTCP

	err = netStack.Start(nil)
	if err != nil {
		return nil, xerrors.Errorf("start netstack: %w", err)
	}

	return server, nil
}

func maskUUID(uid uuid.UUID) uuid.UUID {
	// This is Tailscale's ephemeral service prefix. This can be changed easily
	// later-on, because all of our nodes are ephemeral.
	// fd7a:115c:a1e0
	uid[0] = 0xfd
	uid[1] = 0x7a
	uid[2] = 0x11
	uid[3] = 0x5c
	uid[4] = 0xa1
	uid[5] = 0xe0
	return uid
}

// IP generates a random IP with a static service prefix.
func IP() netip.Addr {
	uid := maskUUID(uuid.New())
	return netip.AddrFrom16(uid)
}

// IP generates a new IP from a UUID.
func IPFromUUID(uid uuid.UUID) netip.Addr {
	return netip.AddrFrom16(maskUUID(uid))
}

// Conn is an actively listening Wireguard connection.
type Conn struct {
	dialContext         context.Context
	dialCancel          context.CancelFunc
	mutex               sync.Mutex
	closed              chan struct{}
	logger              slog.Logger
	blockEndpoints      bool
	derpForceWebSockets bool

	dialer           *tsdial.Dialer
	tunDevice        *tstun.Wrapper
	peerMap          map[tailcfg.NodeID]*tailcfg.Node
	netMap           *netmap.NetworkMap
	netStack         *netstack.Impl
	magicConn        *magicsock.Conn
	wireguardMonitor *netmon.Monitor
	wireguardRouter  *router.Config
	wireguardEngine  wgengine.Engine
	listeners        map[listenKey]*listener

	lastMutex   sync.Mutex
	nodeSending bool
	nodeChanged bool
	// It's only possible to store these values via status functions,
	// so the values must be stored for retrieval later on.
	lastStatus               time.Time
	lastEndpoints            []tailcfg.Endpoint
	lastDERPForcedWebSockets map[int]string
	lastNetInfo              *tailcfg.NetInfo
	nodeCallback             func(node *Node)

	trafficStats *connstats.Statistics
}

func (c *Conn) MagicsockSetDebugLoggingEnabled(enabled bool) {
	c.magicConn.SetDebugLoggingEnabled(enabled)
}

func (c *Conn) SetAddresses(ips []netip.Prefix) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.netMap.Addresses = ips

	netMapCopy := *c.netMap
	c.logger.Debug(context.Background(), "updating network map")
	c.wireguardEngine.SetNetworkMap(&netMapCopy)
	err := c.reconfig()
	if err != nil {
		return xerrors.Errorf("reconfig: %w", err)
	}

	return nil
}

func (c *Conn) Addresses() []netip.Prefix {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.netMap.Addresses
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

func (c *Conn) SetDERPForceWebSockets(v bool) {
	c.magicConn.SetDERPForceWebsockets(v)
}

// SetBlockEndpoints sets whether or not to block P2P endpoints. This setting
// will only apply to new peers.
func (c *Conn) SetBlockEndpoints(blockEndpoints bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.blockEndpoints = blockEndpoints
}

// SetDERPRegionDialer updates the dialer to use for connecting to DERP regions.
func (c *Conn) SetDERPRegionDialer(dialer func(ctx context.Context, region *tailcfg.DERPRegion) net.Conn) {
	c.magicConn.SetDERPRegionDialer(dialer)
}

// UpdateNodes connects with a set of peers. This can be constantly updated,
// and peers will continually be reconnected as necessary. If replacePeers is
// true, all peers will be removed before adding the new ones.
//
//nolint:revive // Complains about replacePeers.
func (c *Conn) UpdateNodes(nodes []*Node, replacePeers bool) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.isClosed() {
		return ErrConnClosed
	}

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

		c.logger.Debug(context.Background(), "removing peer, last handshake >5m ago",
			slog.F("peer", peer.Key), slog.F("last_handshake", peerStatus.LastHandshake),
		)
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
			Hostinfo:   (&tailcfg.Hostinfo{}).View(),
			// Starting KeepAlive messages at the initialization of a connection
			// causes a race condition. If we handshake before the peer has our
			// node, we'll have wait for 5 seconds before trying again. Ideally,
			// the first handshake starts when the user first initiates a
			// connection to the peer. After a successful connection we enable
			// keep alives to persist the connection and keep it from becoming
			// idle. SSH connections don't send send packets while idle, so we
			// use keep alives to avoid random hangs while we set up the
			// connection again after inactivity.
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
	err := c.reconfig()
	if err != nil {
		return xerrors.Errorf("reconfig: %w", err)
	}

	return nil
}

// PeerSelector is used to select a peer from within a Tailnet.
type PeerSelector struct {
	ID tailcfg.NodeID
	IP netip.Prefix
}

func (c *Conn) RemovePeer(selector PeerSelector) (deleted bool, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.isClosed() {
		return false, ErrConnClosed
	}

	deleted = false
	for _, peer := range c.peerMap {
		if peer.ID == selector.ID {
			delete(c.peerMap, peer.ID)
			deleted = true
			break
		}

		for _, peerIP := range peer.Addresses {
			if peerIP.Bits() == selector.IP.Bits() && peerIP.Addr().Compare(selector.IP.Addr()) == 0 {
				delete(c.peerMap, peer.ID)
				deleted = true
				break
			}
		}
	}
	if !deleted {
		return false, nil
	}

	c.netMap.Peers = make([]*tailcfg.Node, 0, len(c.peerMap))
	for _, peer := range c.peerMap {
		c.netMap.Peers = append(c.netMap.Peers, peer.Clone())
	}

	netMapCopy := *c.netMap
	c.logger.Debug(context.Background(), "updating network map")
	c.wireguardEngine.SetNetworkMap(&netMapCopy)
	err = c.reconfig()
	if err != nil {
		return false, xerrors.Errorf("reconfig: %w", err)
	}

	return true, nil
}

func (c *Conn) reconfig() error {
	cfg, err := nmcfg.WGCfg(c.netMap, Logger(c.logger.Named("net.wgconfig")), netmap.AllowSingleHosts, "")
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

// NodeAddresses returns the addresses of a node from the NetworkMap.
func (c *Conn) NodeAddresses(publicKey key.NodePublic) ([]netip.Prefix, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, node := range c.netMap.Peers {
		if node.Key == publicKey {
			return node.Addresses, true
		}
	}
	return nil, false
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

// BlockEndpoints returns whether or not P2P is blocked.
func (c *Conn) BlockEndpoints() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.blockEndpoints
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
	// Conn.UpdateNodes will skip any nodes that don't have the PreferredDERP
	// set to non-zero, since we cannot reach nodes without DERP for discovery.
	// Therefore, there is no point in sending the node without this, and we can
	// save ourselves from churn in the tailscale/wireguard layer.
	if node.PreferredDERP == 0 {
		c.logger.Debug(context.Background(), "skipped sending node; no PreferredDERP", slog.F("node", node))
		return
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
	derpForcedWebsocket := make(map[int]string, 0)
	if c.lastNetInfo != nil {
		preferredDERP = c.lastNetInfo.PreferredDERP
		derpLatency = c.lastNetInfo.DERPLatency

		if c.derpForceWebSockets {
			// We only need to store this for a single region, since this is
			// mostly used for debugging purposes and doesn't actually have a
			// code purpose.
			derpForcedWebsocket[preferredDERP] = "DERP is configured to always fallback to WebSockets"
		} else {
			for k, v := range c.lastDERPForcedWebSockets {
				derpForcedWebsocket[k] = v
			}
		}
	}

	node := &Node{
		ID:                  c.netMap.SelfNode.ID,
		AsOf:                dbtime.Now(),
		Key:                 c.netMap.SelfNode.Key,
		Addresses:           c.netMap.SelfNode.Addresses,
		AllowedIPs:          c.netMap.SelfNode.AllowedIPs,
		DiscoKey:            c.magicConn.DiscoPublicKey(),
		Endpoints:           endpoints,
		PreferredDERP:       preferredDERP,
		DERPLatency:         derpLatency,
		DERPForcedWebsocket: derpForcedWebsocket,
	}
	c.mutex.Lock()
	if c.blockEndpoints {
		node.Endpoints = nil
	}
	c.mutex.Unlock()
	return node
}

// This and below is taken _mostly_ verbatim from Tailscale:
// https://github.com/tailscale/tailscale/blob/c88bd53b1b7b2fcf7ba302f2e53dd1ce8c32dad4/tsnet/tsnet.go#L459-L494

// Listen listens for connections only on the Tailscale network.
func (c *Conn) Listen(network, addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, xerrors.Errorf("tailnet: split host port for listen: %w", err)
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
		return nil, ErrConnClosed
	}
	if c.listeners == nil {
		c.listeners = map[listenKey]*listener{}
	}
	if _, ok := c.listeners[lk]; ok {
		c.mutex.Unlock()
		return nil, xerrors.Errorf("tailnet: listener already open for %s, %s", network, addr)
	}
	c.listeners[lk] = ln
	c.mutex.Unlock()
	return ln, nil
}

func (c *Conn) DialContextTCP(ctx context.Context, ipp netip.AddrPort) (*gonet.TCPConn, error) {
	c.logger.Debug(ctx, "dial tcp", slog.F("addr_port", ipp))
	return c.netStack.DialContextTCP(ctx, ipp)
}

func (c *Conn) DialContextUDP(ctx context.Context, ipp netip.AddrPort) (*gonet.UDPConn, error) {
	c.logger.Debug(ctx, "dial udp", slog.F("addr_port", ipp))
	return c.netStack.DialContextUDP(ctx, ipp)
}

func (c *Conn) forwardTCP(src, dst netip.AddrPort) (handler func(net.Conn), opts []tcpip.SettableSocketOption, intercept bool) {
	logger := c.logger.Named("tcp").With(slog.F("src", src.String()), slog.F("dst", dst.String()))
	c.mutex.Lock()
	ln, ok := c.listeners[listenKey{"tcp", "", fmt.Sprint(dst.Port())}]
	c.mutex.Unlock()
	if !ok {
		return nil, nil, false
	}
	// See: https://github.com/tailscale/tailscale/blob/c7cea825aea39a00aca71ea02bab7266afc03e7c/wgengine/netstack/netstack.go#L888
	if dst.Port() == WorkspaceAgentSSHPort || dst.Port() == 22 {
		opt := tcpip.KeepaliveIdleOption(72 * time.Hour)
		opts = append(opts, &opt)
	}

	return func(conn net.Conn) {
		t := time.NewTimer(time.Second)
		defer t.Stop()
		select {
		case ln.conn <- conn:
			logger.Info(context.Background(), "accepted connection")
			return
		case <-ln.closed:
			logger.Info(context.Background(), "listener closed; closing connection")
		case <-c.closed:
			logger.Info(context.Background(), "tailnet closed; closing connection")
		case <-t.C:
			logger.Info(context.Background(), "listener timed out accepting; closing connection")
		}
		_ = conn.Close()
	}, opts, true
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

func (c *Conn) MagicsockServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	c.magicConn.ServeHTTPDebug(w, r)
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
		return nil, xerrors.Errorf("tailnet: %w", net.ErrClosed)
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
