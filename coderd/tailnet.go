package coderd

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)

var _ workspaceapps.AgentProvider = (*ServerTailnet)(nil)

// NewServerTailnet creates a new tailnet intended for use by coderd.
func NewServerTailnet(
	ctx context.Context,
	logger slog.Logger,
	derpServer *derp.Server,
	dialer tailnet.ControlProtocolDialer,
	derpForceWebSockets bool,
	blockEndpoints bool,
	traceProvider trace.TracerProvider,
) (*ServerTailnet, error) {
	logger = logger.Named("servertailnet")
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:           []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
		DERPForceWebSockets: derpForceWebSockets,
		Logger:              logger,
		BlockEndpoints:      blockEndpoints,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet conn: %w", err)
	}
	serverCtx, cancel := context.WithCancel(ctx)

	// This is set to allow local DERP traffic to be proxied through memory
	// instead of needing to hit the external access URL. Don't use the ctx
	// given in this callback, it's only valid while connecting.
	if derpServer != nil {
		conn.SetDERPRegionDialer(func(_ context.Context, region *tailcfg.DERPRegion) net.Conn {
			// Don't set up the embedded relay if we're shutting down
			if !region.EmbeddedRelay || ctx.Err() != nil {
				return nil
			}
			logger.Debug(ctx, "connecting to embedded DERP via in-memory pipe")
			left, right := net.Pipe()
			go func() {
				defer left.Close()
				defer right.Close()
				brw := bufio.NewReadWriter(bufio.NewReader(right), bufio.NewWriter(right))
				derpServer.Accept(ctx, right, brw, "internal")
			}()
			return left
		})
	}

	tracer := traceProvider.Tracer(tracing.TracerName)

	controller := tailnet.NewController(logger, dialer)
	// it's important to set the DERPRegionDialer above _before_ we set the DERP map so that if
	// there is an embedded relay, we use the local in-memory dialer.
	controller.DERPCtrl = tailnet.NewBasicDERPController(logger, nil, conn)
	transports := &agentTransports{agents: make(map[uuid.UUID]agentTransportPair)}
	coordCtrl := NewMultiAgentController(serverCtx, logger, tracer, conn, transports.forget)
	controller.CoordCtrl = coordCtrl
	// TODO: support controller.TelemetryCtrl

	tn := &ServerTailnet{
		ctx:         serverCtx,
		cancel:      cancel,
		logger:      logger,
		tracer:      tracer,
		conn:        conn,
		coordinatee: conn,
		controller:  controller,
		coordCtrl:   coordCtrl,
		transports:  transports,
		connsPerAgent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "servertailnet",
			Name:      "open_connections",
			Help:      "Total number of TCP connections currently open to workspace agents.",
		}, []string{"network"}),
		totalConns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "servertailnet",
			Name:      "connections_total",
			Help:      "Total number of TCP connections made to workspace agents.",
		}, []string{"network"}),
	}

	tn.controller.Run(tn.ctx)
	return tn, nil
}

// Conn is used to access the underlying tailnet conn of the ServerTailnet. It
// should only be used for read-only purposes.
func (s *ServerTailnet) Conn() *tailnet.Conn {
	return s.conn
}

func (s *ServerTailnet) Describe(descs chan<- *prometheus.Desc) {
	s.connsPerAgent.Describe(descs)
	s.totalConns.Describe(descs)
}

func (s *ServerTailnet) Collect(metrics chan<- prometheus.Metric) {
	s.connsPerAgent.Collect(metrics)
	s.totalConns.Collect(metrics)
}

type ServerTailnet struct {
	ctx    context.Context
	cancel func()

	logger slog.Logger
	tracer trace.Tracer

	// in prod, these are the same, but coordinatee is a subset of Conn's
	// methods which makes some tests easier.
	conn        *tailnet.Conn
	coordinatee tailnet.Coordinatee

	controller *tailnet.Controller
	coordCtrl  *MultiAgentController

	transports *agentTransports

	connsPerAgent *prometheus.GaugeVec
	totalConns    *prometheus.CounterVec
}

// AppTransport returns the transport for HTTP requests to agentID's apps.
func (s *ServerTailnet) AppTransport(agentID uuid.UUID) *workspacesdk.AgentAppTransport {
	return s.transportsFor(agentID).apps
}

func (s *ServerTailnet) AgentConn(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	var (
		conn workspacesdk.AgentConn
		ret  func()
	)

	s.logger.Debug(s.ctx, "acquiring agent", slog.F("agent_id", agentID))
	err := s.coordCtrl.ensureAgent(agentID)
	if err != nil {
		return nil, nil, xerrors.Errorf("ensure agent: %w", err)
	}
	ret = s.coordCtrl.acquireTicket(agentID)

	conn = workspacesdk.NewAgentConn(s.conn, workspacesdk.AgentConnOptions{
		AgentID:      agentID,
		CloseFunc:    func() error { return workspacesdk.ErrSkipClose },
		APITransport: s.transportsFor(agentID).api,
	})

	// Since we now have an open conn, be careful to close it if we error
	// without returning it to the user.

	reachable := conn.AwaitReachable(ctx)
	if !reachable {
		ret()
		return nil, nil, xerrors.New("agent is unreachable")
	}

	return conn, ret, nil
}

// agentTransports holds each agent's HTTP transports, so a pooled
// connection only serves requests for the agent it was dialed to.
type agentTransports struct {
	mu     sync.Mutex
	agents map[uuid.UUID]agentTransportPair
}

type agentTransportPair struct {
	api  *workspacesdk.AgentAPITransport
	apps *workspacesdk.AgentAppTransport
}

func (p agentTransportPair) closeIdleConnections() {
	p.api.CloseIdleConnections()
	p.apps.CloseIdleConnections()
}

// forget removes agentID's transports and closes their idle connections.
func (t *agentTransports) forget(agentID uuid.UUID) {
	t.mu.Lock()
	p, ok := t.agents[agentID]
	delete(t.agents, agentID)
	t.mu.Unlock()
	if ok {
		p.closeIdleConnections()
	}
}

func (t *agentTransports) closeIdleConnections() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, p := range t.agents {
		p.closeIdleConnections()
	}
}

func (s *ServerTailnet) transportsFor(agentID uuid.UUID) agentTransportPair {
	s.transports.mu.Lock()
	defer s.transports.mu.Unlock()
	p, ok := s.transports.agents[agentID]
	if !ok {
		dial := s.agentDialer(agentID)
		p = agentTransportPair{
			api:  workspacesdk.NewAgentAPITransport(agentID, dial, s.logger),
			apps: workspacesdk.NewAgentAppTransport(agentID, dial, s.logger),
		}
		s.transports.agents[agentID] = p
	}
	return p
}

// agentDialer returns a dialer for agentID. Each connection holds a tunnel
// ticket until it closes, so the agent does not expire while a connection
// is open or pooled.
func (s *ServerTailnet) agentDialer(agentID uuid.UUID) workspacesdk.AgentDialer {
	addr := tailnet.TailscaleServicePrefix.AddrFromUUID(agentID)
	return func(ctx context.Context, port uint16) (net.Conn, error) {
		if err := s.coordCtrl.ensureAgent(agentID); err != nil {
			return nil, xerrors.Errorf("ensure agent: %w", err)
		}
		release := s.coordCtrl.acquireTicket(agentID)
		if !s.conn.AwaitReachable(ctx, addr) {
			release()
			return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
		}
		nc, err := s.conn.DialContextTCP(ctx, netip.AddrPortFrom(addr, port))
		if err != nil {
			release()
			return nil, xerrors.Errorf("dial agent: %w", err)
		}

		s.connsPerAgent.WithLabelValues("tcp").Inc()
		s.totalConns.WithLabelValues("tcp").Inc()
		return &instrumentedConn{
			Conn:          &netConnCloser{Conn: nc, close: release},
			agentID:       agentID,
			connsPerAgent: s.connsPerAgent,
		}, nil
	}
}

func (s *ServerTailnet) ServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	s.conn.MagicsockServeHTTPDebug(w, r)
}

type netConnCloser struct {
	net.Conn
	close func()
}

func (c *netConnCloser) Close() error {
	c.close()
	return c.Conn.Close()
}

func (s *ServerTailnet) Close() error {
	s.logger.Info(s.ctx, "closing server tailnet")
	defer s.logger.Debug(s.ctx, "server tailnet close complete")
	s.cancel()
	// Close idle connections while the tailnet can still deliver the close to
	// agents, which otherwise hold them until their idle timeout.
	s.transports.closeIdleConnections()
	_ = s.conn.Close()
	s.coordCtrl.Close()
	<-s.controller.Closed()
	return nil
}

type instrumentedConn struct {
	net.Conn

	agentID       uuid.UUID
	closeOnce     sync.Once
	connsPerAgent *prometheus.GaugeVec
}

func (c *instrumentedConn) Close() error {
	c.closeOnce.Do(func() {
		c.connsPerAgent.WithLabelValues("tcp").Dec()
	})
	return c.Conn.Close()
}

// MultiAgentController is a tailnet.CoordinationController for connecting to multiple workspace
// agents.  It keeps track of connection times to the agents, and removes them on a timer if they
// have no active connections and haven't been used in a while.
type MultiAgentController struct {
	*tailnet.BasicCoordinationController

	logger slog.Logger
	tracer trace.Tracer

	mu sync.Mutex
	// connectionTimes is a map of agents the server wants to keep a connection to. It
	// contains the last time the agent was connected to.
	connectionTimes map[uuid.UUID]time.Time
	// tickets is a map of destinations to a set of connection tickets, representing open
	// connections to the destination
	tickets      map[uuid.UUID]map[uuid.UUID]struct{}
	coordination *tailnet.BasicCoordination
	// onExpire runs, without mu held, for each agent removed by expiry.
	onExpire func(agentID uuid.UUID)

	cancel              context.CancelFunc
	expireOldAgentsDone chan struct{}
}

func (m *MultiAgentController) New(client tailnet.CoordinatorClient) tailnet.CloserWaiter {
	b := m.NewCoordination(client)
	// resync all destinations
	m.mu.Lock()
	defer m.mu.Unlock()
	m.coordination = b
	for agentID := range m.connectionTimes {
		err := b.SendRequest(&proto.CoordinateRequest{
			AddTunnel: &proto.CoordinateRequest_Tunnel{Id: agentID[:]},
		})
		if err != nil {
			m.logger.Error(context.Background(), "failed to re-add tunnel", slog.F("agent_id", agentID),
				slog.Error(err))
			b.SendErr(err)
			_ = client.Close()
			m.coordination = nil
			break
		}
	}
	return b
}

func (m *MultiAgentController) ensureAgent(agentID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.connectionTimes[agentID]
	// If we don't have the agent, subscribe.
	if !ok {
		m.logger.Debug(context.Background(),
			"subscribing to agent", slog.F("agent_id", agentID))
		if m.coordination != nil {
			err := m.coordination.SendRequest(&proto.CoordinateRequest{
				AddTunnel: &proto.CoordinateRequest_Tunnel{Id: agentID[:]},
			})
			if err != nil {
				err = xerrors.Errorf("subscribe agent: %w", err)
				m.coordination.SendErr(err)
				_ = m.coordination.CloseClient()
				m.coordination = nil
				return err
			}
		}
		m.tickets[agentID] = map[uuid.UUID]struct{}{}
	}
	m.connectionTimes[agentID] = time.Now()
	return nil
}

func (m *MultiAgentController) acquireTicket(agentID uuid.UUID) (release func()) {
	id := uuid.New()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets[agentID][id] = struct{}{}

	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.tickets[agentID], id)
	}
}

func (m *MultiAgentController) expireOldAgents(ctx context.Context) {
	defer close(m.expireOldAgentsDone)
	defer m.logger.Debug(context.Background(), "stopped expiring old agents")
	const (
		tick   = 5 * time.Minute
		cutoff = 30 * time.Minute
	)

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		m.doExpireOldAgents(ctx, cutoff)
	}
}

func (m *MultiAgentController) doExpireOldAgents(ctx context.Context, cutoff time.Duration) {
	// TODO: add some attrs to this.
	ctx, span := m.tracer.Start(ctx, tracing.FuncName())
	defer span.End()

	start := time.Now()
	var expired []uuid.UUID

	m.mu.Lock()
	m.logger.Debug(ctx, "pruning inactive agents", slog.F("agent_count", len(m.connectionTimes)))
	for agentID, lastConnection := range m.connectionTimes {
		// If no one has connected since the cutoff and there are no active
		// connections, remove the agent.
		if time.Since(lastConnection) > cutoff && len(m.tickets[agentID]) == 0 {
			if m.coordination != nil {
				err := m.coordination.SendRequest(&proto.CoordinateRequest{
					RemoveTunnel: &proto.CoordinateRequest_Tunnel{Id: agentID[:]},
				})
				if err != nil {
					m.logger.Debug(ctx, "unsubscribe expired agent", slog.Error(err), slog.F("agent_id", agentID))
					m.coordination.SendErr(xerrors.Errorf("unsubscribe expired agent: %w", err))
					// close the client because we do not want to do a graceful disconnect by
					// closing the coordination.
					_ = m.coordination.CloseClient()
					m.coordination = nil
					// Here we continue deleting any inactive agents: there is no point in
					// re-establishing tunnels to expired agents when we eventually reconnect.
				}
			}
			expired = append(expired, agentID)
			delete(m.connectionTimes, agentID)
		}
	}
	m.mu.Unlock()
	for _, agentID := range expired {
		m.onExpire(agentID)
	}
	m.logger.Debug(ctx, "pruned inactive agents",
		slog.F("deleted", len(expired)),
		slog.F("took", time.Since(start)),
	)
}

func (m *MultiAgentController) Close() {
	m.cancel()
	<-m.expireOldAgentsDone
}

func NewMultiAgentController(ctx context.Context, logger slog.Logger, tracer trace.Tracer, coordinatee tailnet.Coordinatee, onExpire func(agentID uuid.UUID)) *MultiAgentController {
	m := &MultiAgentController{
		BasicCoordinationController: &tailnet.BasicCoordinationController{
			Logger:      logger,
			Coordinatee: coordinatee,
			SendAcks:    false, // we are a client, connecting to multiple agents
			Initiator:   codersdk.DisconnectInitiatorServer,
			Direction:   codersdk.ConnectionDirectionServerToAgent,
		},
		logger:              logger,
		tracer:              tracer,
		connectionTimes:     make(map[uuid.UUID]time.Time),
		tickets:             make(map[uuid.UUID]map[uuid.UUID]struct{}),
		onExpire:            onExpire,
		expireOldAgentsDone: make(chan struct{}),
	}
	ctx, m.cancel = context.WithCancel(ctx)
	go m.expireOldAgents(ctx)
	return m
}

type Pinger interface {
	Ping(context.Context) (time.Duration, error)
}

// InmemTailnetDialer is a tailnet.ControlProtocolDialer that connects to a Coordinator and DERPMap
// service running in the same memory space.
type InmemTailnetDialer struct {
	CoordPtr *atomic.Pointer[tailnet.Coordinator]
	DERPFn   func() *tailcfg.DERPMap
	Logger   slog.Logger
	ClientID uuid.UUID
	// DatabaseHealthCheck is used to validate that the store is reachable.
	DatabaseHealthCheck Pinger
	// Clock is optional and defaults to the real clock.
	Clock quartz.Clock
}

func (a *InmemTailnetDialer) Dial(ctx context.Context, _ tailnet.ResumeTokenController) (tailnet.ControlProtocolClients, error) {
	if a.DatabaseHealthCheck != nil {
		if _, err := a.DatabaseHealthCheck.Ping(ctx); err != nil {
			return tailnet.ControlProtocolClients{}, xerrors.Errorf("%w: %v", codersdk.ErrDatabaseNotReachable, err)
		}
	}

	coord := a.CoordPtr.Load()
	if coord == nil {
		return tailnet.ControlProtocolClients{}, xerrors.Errorf("tailnet coordinator not initialized")
	}
	coordClient := tailnet.NewInMemoryCoordinatorClient(
		a.Logger, a.ClientID, tailnet.SingleTailnetCoordinateeAuth{}, *coord)
	clock := a.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	derpClient := newPollingDERPClient(a.DERPFn, a.Logger, clock)
	return tailnet.ControlProtocolClients{
		Closer:      closeAll{coord: coordClient, derp: derpClient},
		Coordinator: coordClient,
		DERP:        derpClient,
	}, nil
}

func newPollingDERPClient(derpFn func() *tailcfg.DERPMap, logger slog.Logger, clock quartz.Clock) tailnet.DERPClient {
	ctx, cancel := context.WithCancel(context.Background())
	a := &pollingDERPClient{
		fn:       derpFn,
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger,
		clock:    clock,
		ch:       make(chan *tailcfg.DERPMap),
		loopDone: make(chan struct{}),
	}
	go a.pollDERP()
	return a
}

// pollingDERPClient is a DERP client that just calls a function on a polling
// interval
type pollingDERPClient struct {
	fn          func() *tailcfg.DERPMap
	logger      slog.Logger
	clock       quartz.Clock
	ctx         context.Context
	cancel      context.CancelFunc
	loopDone    chan struct{}
	lastDERPMap *tailcfg.DERPMap
	ch          chan *tailcfg.DERPMap
}

// Close the DERP client
func (a *pollingDERPClient) Close() error {
	a.cancel()
	<-a.loopDone
	return nil
}

func (a *pollingDERPClient) Recv() (*tailcfg.DERPMap, error) {
	select {
	case <-a.ctx.Done():
		return nil, a.ctx.Err()
	case dm := <-a.ch:
		return dm, nil
	}
}

func (a *pollingDERPClient) pollDERP() {
	defer close(a.loopDone)
	defer a.logger.Debug(a.ctx, "polling DERPMap exited")

	// Fetch immediately so the tailnet can publish its node without waiting
	// for the first tick.
	ticker := a.clock.NewTicker(5*time.Second, "pollDERP")
	defer ticker.Stop()

	for {
		newDerpMap := a.fn()
		if !tailnet.CompareDERPMaps(a.lastDERPMap, newDerpMap) {
			select {
			case <-a.ctx.Done():
				return
			case a.ch <- newDerpMap:
			}
		}

		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type closeAll struct {
	coord tailnet.CoordinatorClient
	derp  tailnet.DERPClient
}

func (c closeAll) Close() error {
	cErr := c.coord.Close()
	dErr := c.derp.Close()
	if cErr != nil {
		return cErr
	}
	return dErr
}
