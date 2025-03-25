package tailnet

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
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
	"tailscale.com/types/key"
	tslogger "tailscale.com/types/logger"
	"tailscale.com/types/netlogtype"
	"tailscale.com/types/netmap"
	"tailscale.com/util/dnsname"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/capture"
	"tailscale.com/wgengine/magicsock"
	"tailscale.com/wgengine/netstack"
	"tailscale.com/wgengine/router"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/tailnet/proto"
)

var ErrConnClosed = xerrors.New("connection closed")

const (
	WorkspaceAgentSSHPort             = 1
	WorkspaceAgentReconnectingPTYPort = 2
	WorkspaceAgentSpeedtestPort       = 3
	WorkspaceAgentStandardSSHPort     = 22
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
	// CaptureHook is a callback that captures Disco packets and packets sent
	// into the tailnet tunnel.
	CaptureHook capture.Callback
	// ForceNetworkUp forces the network to be considered up. magicsock will not
	// do anything if it thinks it can't reach the internet.
	ForceNetworkUp bool
	// Network Telemetry Client Type: CLI | Agent | coderd
	ClientType proto.TelemetryEvent_ClientType
	// TelemetrySink is optional.
	TelemetrySink TelemetrySink
	// DNSConfigurator is optional, and is passed to the underlying wireguard
	// engine.
	DNSConfigurator dns.OSConfigurator
	// Router is optional, and is passed to the underlying wireguard engine.
	Router router.Router
	// TUNDev is optional, and is passed to the underlying wireguard engine.
	TUNDev tun.Device
	// WireguardMonitor is optional, and is passed to the underlying wireguard
	// engine.
	WireguardMonitor *netmon.Monitor
}

// TelemetrySink allows tailnet.Conn to send network telemetry to the Coder
// server.
type TelemetrySink interface {
	// SendTelemetryEvent sends a telemetry event to some external sink.
	SendTelemetryEvent(event *proto.TelemetryEvent)
}

// NodeID creates a Tailscale NodeID from the last 8 bytes of a UUID. It ensures
// the returned NodeID is always positive.
func NodeID(uid uuid.UUID) tailcfg.NodeID {
	// This may overflow, but we handle that by ensuring the result is positive below
	id := int64(binary.BigEndian.Uint64(uid[8:])) // #nosec G115 -- potential overflow is handled below

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

	netns.SetEnabled(options.TUNDev != nil)

	var telemetryStore *TelemetryStore
	if options.TelemetrySink != nil {
		var err error
		telemetryStore, err = newTelemetryStore()
		if err != nil {
			return nil, xerrors.Errorf("create telemetry store: %w", err)
		}
	}

	nodePrivateKey := key.NewNode()
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

	if options.WireguardMonitor == nil {
		options.WireguardMonitor, err = netmon.New(Logger(options.Logger.Named("net.wgmonitor")))
		if err != nil {
			return nil, xerrors.Errorf("create wireguard link monitor: %w", err)
		}
	}
	defer func() {
		if err != nil {
			options.WireguardMonitor.Close()
		}
	}()

	dialer := &tsdial.Dialer{
		Logf: Logger(options.Logger.Named("net.tsdial")),
	}
	sys := new(tsd.System)
	wireguardEngine, err := wgengine.NewUserspaceEngine(Logger(options.Logger.Named("net.wgengine")), wgengine.Config{
		NetMon:       options.WireguardMonitor,
		Dialer:       dialer,
		ListenPort:   options.ListenPort,
		SetSubsystem: sys.Set,
		DNS:          options.DNSConfigurator,
		Router:       options.Router,
		Tun:          options.TUNDev,
	})
	if err != nil {
		return nil, xerrors.Errorf("create wgengine: %w", err)
	}
	defer func() {
		if err != nil {
			wireguardEngine.Close()
		}
	}()
	wireguardEngine.InstallCaptureHook(options.CaptureHook)
	if options.TUNDev == nil {
		dialer.UseNetstackForIP = func(ip netip.Addr) bool {
			_, ok := wireguardEngine.PeerForIP(ip)
			return ok
		}
	}

	wireguardEngine = wgengine.NewWatchdog(wireguardEngine)
	sys.Set(wireguardEngine)

	magicConn := sys.MagicSock.Get()
	magicConn.SetDERPForceWebsockets(options.DERPForceWebSockets)
	magicConn.SetBlockEndpoints(options.BlockEndpoints)
	if options.DERPHeader != nil {
		magicConn.SetDERPHeader(options.DERPHeader.Clone())
	}
	if options.ForceNetworkUp {
		magicConn.SetNetworkUp(true)
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

	if options.TUNDev == nil {
		dialer.NetstackDialTCP = func(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
			return netStack.DialContextTCP(ctx, dst)
		}
		netStack.ProcessLocalIPs = true
	}

	cfgMaps := newConfigMaps(
		options.Logger,
		wireguardEngine,
		nodeID,
		nodePrivateKey,
		magicConn.DiscoPublicKey(),
	)
	cfgMaps.setAddresses(options.Addresses)
	if options.DERPMap != nil {
		cfgMaps.setDERPMap(options.DERPMap)
	}
	cfgMaps.setBlockEndpoints(options.BlockEndpoints)

	nodeUp := newNodeUpdater(
		options.Logger,
		nil,
		nodeID,
		nodePrivateKey.Public(),
		magicConn.DiscoPublicKey(),
	)
	nodeUp.setAddresses(options.Addresses)
	nodeUp.setBlockEndpoints(options.BlockEndpoints)

	ctx, ctxCancel := context.WithCancel(context.Background())
	server := &Conn{
		id:               uuid.New(),
		closed:           make(chan struct{}),
		logger:           options.Logger,
		magicConn:        magicConn,
		dialer:           dialer,
		listeners:        map[listenKey]*listener{},
		tunDevice:        sys.Tun.Get(),
		netStack:         netStack,
		wireguardMonitor: options.WireguardMonitor,
		wireguardRouter: &router.Config{
			LocalAddrs: options.Addresses,
		},
		wireguardEngine: wireguardEngine,
		configMaps:      cfgMaps,
		nodeUpdater:     nodeUp,
		telemetrySink:   options.TelemetrySink,
		dnsConfigurator: options.DNSConfigurator,
		telemetryStore:  telemetryStore,
		createdAt:       time.Now(),
		watchCtx:        ctx,
		watchCancel:     ctxCancel,
	}
	defer func() {
		if err != nil {
			_ = server.Close()
		}
	}()
	if server.telemetryStore != nil {
		server.wireguardEngine.SetNetInfoCallback(func(ni *tailcfg.NetInfo) {
			server.mutex.Lock()
			server.lastNetInfo = ni.Clone()
			server.mutex.Unlock()
			server.telemetryStore.setNetInfo(ni)
			nodeUp.setNetInfo(ni)
			server.telemetryStore.pingPeer(server)
		})
		server.wireguardEngine.AddNetworkMapCallback(func(nm *netmap.NetworkMap) {
			server.telemetryStore.updateNetworkMap(nm)
			server.telemetryStore.pingPeer(server)
		})
		go server.watchConnChange()
	} else {
		server.wireguardEngine.SetNetInfoCallback(func(ni *tailcfg.NetInfo) {
			server.mutex.Lock()
			server.lastNetInfo = ni.Clone()
			server.mutex.Unlock()
			nodeUp.setNetInfo(ni)
		})
	}
	server.wireguardEngine.SetStatusCallback(nodeUp.setStatus)
	server.magicConn.SetDERPForcedWebsocketCallback(nodeUp.setDERPForcedWebsocket)

	netStack.GetTCPHandlerForFlow = server.forwardTCP

	err = netStack.Start(nil)
	if err != nil {
		return nil, xerrors.Errorf("start netstack: %w", err)
	}

	return server, nil
}

type ServicePrefix [6]byte

var (
	// TailscaleServicePrefix is the IPv6 prefix for all tailnet nodes since it was first added to
	// Coder.  It is identical to the service prefix Tailscale.com uses. With the introduction of
	// CoderVPN, we would like to stop using the Tailscale prefix so that we don't conflict with
	// Tailscale if both are installed at the same time. However, there are a large number of agents
	// and clients using this prefix, so we need to carefully manage deprecation and eventual
	// removal.
	// fd7a:115c:a1e0:://48
	TailscaleServicePrefix ServicePrefix = [6]byte{0xfd, 0x7a, 0x11, 0x5c, 0xa1, 0xe0}
	// CoderServicePrefix is the Coder-specific IPv6 prefix for tailnet nodes, which we are in the
	// process of migrating to. It allows Coder to run alongside Tailscale without conflicts even
	// if both are set up as TUN interfaces into the OS (e.g. CoderVPN).
	// fd60:627a:a42b::/48
	CoderServicePrefix ServicePrefix = [6]byte{0xfd, 0x60, 0x62, 0x7a, 0xa4, 0x2b}
)

// maskUUID returns a new UUID with the first 6 bytes changed to the ServicePrefix
func (p ServicePrefix) maskUUID(uid uuid.UUID) uuid.UUID {
	copy(uid[:], p[:])
	return uid
}

// RandomAddr returns a random IP address in the service prefix.
func (p ServicePrefix) RandomAddr() netip.Addr {
	return netip.AddrFrom16(p.maskUUID(uuid.New()))
}

// AddrFromUUID returns an IPv6 address corresponding to the given UUID in the service prefix.
func (p ServicePrefix) AddrFromUUID(uid uuid.UUID) netip.Addr {
	return netip.AddrFrom16(p.maskUUID(uid))
}

// PrefixFromUUID returns a single IPv6 /128 prefix corresponding to the given UUID.
func (p ServicePrefix) PrefixFromUUID(uid uuid.UUID) netip.Prefix {
	return netip.PrefixFrom(p.AddrFromUUID(uid), 128)
}

// RandomPrefix returns a single IPv6 /128 prefix within the service prefix.
func (p ServicePrefix) RandomPrefix() netip.Prefix {
	return netip.PrefixFrom(p.RandomAddr(), 128)
}

func (p ServicePrefix) AsNetip() netip.Prefix {
	out := [16]byte{}
	copy(out[:], p[:])
	return netip.PrefixFrom(netip.AddrFrom16(out), 48)
}

// Conn is an actively listening Wireguard connection.
type Conn struct {
	// Unique ID used for telemetry.
	id     uuid.UUID
	mutex  sync.Mutex
	closed chan struct{}
	logger slog.Logger

	dialer           *tsdial.Dialer
	tunDevice        *tstun.Wrapper
	configMaps       *configMaps
	nodeUpdater      *nodeUpdater
	netStack         *netstack.Impl
	magicConn        *magicsock.Conn
	wireguardMonitor *netmon.Monitor
	wireguardRouter  *router.Config
	wireguardEngine  wgengine.Engine
	dnsConfigurator  dns.OSConfigurator
	listeners        map[listenKey]*listener
	clientType       proto.TelemetryEvent_ClientType
	createdAt        time.Time

	telemetrySink TelemetrySink
	// telemetryStore will be nil if telemetrySink is nil.
	telemetryStore *TelemetryStore
	telemetryWg    sync.WaitGroup

	watchCtx    context.Context
	watchCancel func()

	trafficStats *connstats.Statistics
	lastNetInfo  *tailcfg.NetInfo
}

func (c *Conn) GetNetInfo() *tailcfg.NetInfo {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.lastNetInfo.Clone()
}

func (c *Conn) SetTunnelDestination(id uuid.UUID) {
	c.configMaps.setTunnelDestination(id)
}

func (c *Conn) GetBlockEndpoints() bool {
	return c.configMaps.getBlockEndpoints() && c.nodeUpdater.getBlockEndpoints()
}

func (c *Conn) InstallCaptureHook(f capture.Callback) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.wireguardEngine.InstallCaptureHook(f)
}

func (c *Conn) MagicsockSetDebugLoggingEnabled(enabled bool) {
	c.magicConn.SetDebugLoggingEnabled(enabled)
}

func (c *Conn) SetAddresses(ips []netip.Prefix) error {
	c.configMaps.setAddresses(ips)
	c.nodeUpdater.setAddresses(ips)
	return nil
}

// SetDNSHosts replaces the map of DNS hosts for the connection.
func (c *Conn) SetDNSHosts(hosts map[dnsname.FQDN][]netip.Addr) error {
	if c.dnsConfigurator == nil {
		return xerrors.New("no DNSConfigurator set")
	}
	c.configMaps.setHosts(hosts)
	return nil
}

func (c *Conn) SetNodeCallback(callback func(node *Node)) {
	c.nodeUpdater.setCallback(callback)
}

// SetDERPMap updates the DERPMap of a connection.
func (c *Conn) SetDERPMap(derpMap *tailcfg.DERPMap) {
	if c.configMaps.setDERPMap(derpMap) && c.telemetryStore != nil {
		c.telemetryStore.updateDerpMap(derpMap)
	}
}

func (c *Conn) SetDERPForceWebSockets(v bool) {
	c.logger.Info(context.Background(), "setting DERP Force Websockets", slog.F("force_derp_websockets", v))
	c.magicConn.SetDERPForceWebsockets(v)
}

// SetBlockEndpoints sets whether to block P2P endpoints. This setting
// will only apply to new peers.
func (c *Conn) SetBlockEndpoints(blockEndpoints bool) {
	c.configMaps.setBlockEndpoints(blockEndpoints)
	c.nodeUpdater.setBlockEndpoints(blockEndpoints)
	c.magicConn.SetBlockEndpoints(blockEndpoints)
}

// SetDERPRegionDialer updates the dialer to use for connecting to DERP regions.
func (c *Conn) SetDERPRegionDialer(dialer func(ctx context.Context, region *tailcfg.DERPRegion) net.Conn) {
	c.magicConn.SetDERPRegionDialer(dialer)
}

// UpdatePeers connects with a set of peers. This can be constantly updated,
// and peers will continually be reconnected as necessary.
func (c *Conn) UpdatePeers(updates []*proto.CoordinateResponse_PeerUpdate) error {
	if c.isClosed() {
		return ErrConnClosed
	}
	c.configMaps.updatePeers(updates)
	return nil
}

// SetAllPeersLost marks all peers lost; typically used when we disconnect from a coordinator.
func (c *Conn) SetAllPeersLost() {
	c.configMaps.setAllPeersLost()
}

// NodeAddresses returns the addresses of a node from the NetworkMap.
func (c *Conn) NodeAddresses(publicKey key.NodePublic) ([]netip.Prefix, bool) {
	return c.configMaps.nodeAddresses(publicKey)
}

// Status returns the current ipnstate of a connection.
func (c *Conn) Status() *ipnstate.Status {
	return c.configMaps.status()
}

// Ping sends a ping to the Wireguard engine.
// The bool returned is true if the ping was performed P2P.
func (c *Conn) Ping(ctx context.Context, ip netip.Addr) (time.Duration, bool, *ipnstate.PingResult, error) {
	dur, p2p, pr, err := c.pingWithType(ctx, ip, tailcfg.PingDisco)
	if err == nil {
		c.sendPingTelemetry(pr)
	}
	return dur, p2p, pr, err
}

func (c *Conn) pingWithType(ctx context.Context, ip netip.Addr, pt tailcfg.PingType) (time.Duration, bool, *ipnstate.PingResult, error) {
	errCh := make(chan error, 1)
	prChan := make(chan *ipnstate.PingResult, 1)
	go c.wireguardEngine.Ping(ip, pt, func(pr *ipnstate.PingResult) {
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
	c.configMaps.L.Lock()
	defer c.configMaps.L.Unlock()
	return c.configMaps.derpMapLocked()
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

		// For reachability, we use TSMP ping, which pings at the IP layer, and
		// therefore requires that wireguard and the netstack are up.  If we
		// don't wait for wireguard to be up, we could miss a handshake, and it
		// might take 5 seconds for the handshake to be retried. A 5s initial
		// round trip can set us up for poor TCP performance, since the initial
		// round-trip-time sets the initial retransmit timeout.
		_, _, _, err := c.pingWithType(ctx, ip, tailcfg.PingTSMP)
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
	c.logger.Info(context.Background(), "closing tailnet Conn")
	c.watchCancel()
	c.configMaps.close()
	c.nodeUpdater.close()
	c.mutex.Lock()
	select {
	case <-c.closed:
		c.mutex.Unlock()
		return nil
	default:
	}
	close(c.closed)
	c.telemetryWg.Wait()
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
	c.logger.Debug(context.Background(), "closed netstack")
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

// Node returns the last node that was sent to the node callback.
func (c *Conn) Node() *Node {
	c.nodeUpdater.L.Lock()
	defer c.nodeUpdater.L.Unlock()
	return c.nodeUpdater.nodeLocked()
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
	if dst.Port() == WorkspaceAgentSSHPort || dst.Port() == WorkspaceAgentStandardSSHPort {
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

// SendConnectedTelemetry should be called when connection to a peer with the given IP is established.
func (c *Conn) SendConnectedTelemetry(ip netip.Addr, application string) {
	if c.telemetrySink == nil {
		return
	}
	c.telemetryStore.markConnected(&ip, application)
	e := c.newTelemetryEvent()
	e.Status = proto.TelemetryEvent_CONNECTED
	e.ConnectionSetup = durationpb.New(time.Since(c.createdAt))
	c.sendTelemetryBackground(e)
}

func (c *Conn) SendDisconnectedTelemetry() {
	if c.telemetrySink == nil {
		return
	}
	e := c.newTelemetryEvent()
	e.Status = proto.TelemetryEvent_DISCONNECTED
	c.sendTelemetryBackground(e)
}

func (c *Conn) SendSpeedtestTelemetry(throughputMbits float64) {
	if c.telemetrySink == nil {
		return
	}
	e := c.newTelemetryEvent()
	e.ThroughputMbits = wrapperspb.Float(float32(throughputMbits))
	e.Status = proto.TelemetryEvent_CONNECTED
	c.sendTelemetryBackground(e)
}

// nolint:revive
func (c *Conn) sendPingTelemetry(pr *ipnstate.PingResult) {
	if c.telemetrySink == nil {
		return
	}
	e := c.newTelemetryEvent()

	latency := durationpb.New(time.Duration(pr.LatencySeconds * float64(time.Second)))
	if pr.Endpoint != "" {
		e.P2PLatency = latency
		e.P2PEndpoint = c.telemetryStore.toEndpoint(pr.Endpoint)
	} else {
		e.DerpLatency = latency
	}
	e.Status = proto.TelemetryEvent_CONNECTED
	c.sendTelemetryBackground(e)
}

// The returned telemetry event will not have it's status set.
func (c *Conn) newTelemetryEvent() *proto.TelemetryEvent {
	event := c.telemetryStore.newEvent()
	event.ClientType = c.clientType
	event.Id = c.id[:]
	event.ConnectionAge = durationpb.New(time.Since(c.createdAt))
	return event
}

func (c *Conn) sendTelemetryBackground(e *proto.TelemetryEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	select {
	case <-c.closed:
		return
	default:
	}
	c.telemetryWg.Add(1)
	go func() {
		defer c.telemetryWg.Done()
		c.telemetrySink.SendTelemetryEvent(e)
	}()
}

// Watch for changes in the connection type (P2P<->DERP) and send telemetry events.
func (c *Conn) watchConnChange() {
	ticker := time.NewTicker(time.Millisecond * 50)
	defer ticker.Stop()
	for {
		select {
		case <-c.watchCtx.Done():
			return
		case <-ticker.C:
		}
		status := c.Status()
		peers := status.Peers()
		if len(peers) > 1 {
			// Not a CLI<->agent connection, stop watching
			return
		} else if len(peers) == 0 {
			continue
		}
		peer := status.Peer[peers[0]]
		// If the connection type has changed, send a telemetry event with the latest ping stats
		if c.telemetryStore.changedConntype(peer.CurAddr) {
			c.telemetryStore.pingPeer(c)
		}
	}
}

// PeerDiagnostics is a checklist of human-readable conditions necessary to establish an encrypted
// tunnel to a peer via a Conn
type PeerDiagnostics struct {
	// PreferredDERP is 0 if we are not connected to a DERP region. If non-zero, we are connected to
	// the given region as our home or "preferred" DERP.
	PreferredDERP   int
	DERPRegionNames map[int]string
	// SentNode is true if we have successfully transmitted our local Node via the most recently set
	// NodeCallback.
	SentNode bool
	// ReceivedNode is the last Node we received for the peer, or nil if we haven't received the node.
	ReceivedNode *tailcfg.Node
	// LastWireguardHandshake is the last time we completed a wireguard handshake
	LastWireguardHandshake time.Time
	// TODO: surface Discovery (disco) protocol problems
}

func (c *Conn) GetPeerDiagnostics(peerID uuid.UUID) PeerDiagnostics {
	d := PeerDiagnostics{DERPRegionNames: make(map[int]string)}
	c.nodeUpdater.fillPeerDiagnostics(&d)
	c.configMaps.fillPeerDiagnostics(&d, peerID)
	return d
}

func (c *Conn) GetKnownPeerIDs() []uuid.UUID {
	return c.configMaps.knownPeerIDs()
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

// Logger converts the Tailscale logging function to use a slog-compatible
// logger.
func Logger(logger interface {
	Debug(ctx context.Context, str string, args ...any)
},
) tslogger.Logf {
	return tslogger.Logf(func(format string, args ...any) {
		slog.Helper()
		logger.Debug(context.Background(), fmt.Sprintf(format, args...))
	})
}
