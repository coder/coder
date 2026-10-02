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
	coordCtrl := NewMultiAgentController(serverCtx, logger, tracer, conn)
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

	connsPerAgent *prometheus.GaugeVec
	totalConns    *prometheus.CounterVec
}

// agentAPIPool keeps one idle API connection per agent. An idle connection
// holds about 65 KB in coderd and 95 KB in the agent. One serves sequential
// tool calls; concurrent requests beyond it dial.
var agentAPIPool = workspacesdk.AgentAPIPool{
	MaxIdleConns:    1,
	IdleConnTimeout: 10 * time.Minute,
}

// AppTransport returns the transport for HTTP requests to agentID's apps.
// The caller must call release when its request ends.
func (s *ServerTailnet) AppTransport(agentID uuid.UUID) (_ *workspaceapps.AgentAppTransport, release func(), _ error) {
	var t *workspaceapps.AgentAppTransport
	release, err := s.coordCtrl.acquire(agentID, func(a *agentState) error {
		if a.apps == nil {
			a.apps = workspaceapps.NewAgentAppTransport(agentID, s.agentDialer(agentID), s.logger)
		}
		t = a.apps
		return nil
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("ensure agent: %w", err)
	}
	return t, release, nil
}

// acquireAPITransport returns agentID's API transport. The caller must call
// release when it stops using the transport.
func (s *ServerTailnet) acquireAPITransport(agentID uuid.UUID) (_ *workspacesdk.AgentAPITransport, release func(), _ error) {
	var t *workspacesdk.AgentAPITransport
	release, err := s.coordCtrl.acquire(agentID, func(a *agentState) error {
		if a.api == nil {
			api, err := workspacesdk.NewAgentAPITransport(agentID, s.agentDialer(agentID), s.logger, agentAPIPool)
			if err != nil {
				return xerrors.Errorf("agent API transport: %w", err)
			}
			a.api = api
		}
		t = a.api
		return nil
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("ensure agent: %w", err)
	}
	return t, release, nil
}

func (s *ServerTailnet) AgentConn(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	s.logger.Debug(s.ctx, "acquiring agent", slog.F("agent_id", agentID))
	apiTransport, ret, err := s.acquireAPITransport(agentID)
	if err != nil {
		return nil, nil, err
	}

	conn := workspacesdk.NewAgentConn(s.conn, workspacesdk.AgentConnOptions{
		AgentID:      agentID,
		CloseFunc:    func() error { return workspacesdk.ErrSkipClose },
		Logger:       s.logger,
		APITransport: apiTransport,
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

// agentDialer returns a dialer for agentID. Each connection holds a ticket
// until it closes, so the agent does not expire while a connection is open
// or pooled.
func (s *ServerTailnet) agentDialer(agentID uuid.UUID) workspacesdk.AgentDialer {
	addr := tailnet.TailscaleServicePrefix.AddrFromUUID(agentID)
	return func(ctx context.Context, port uint16) (net.Conn, error) {
		release, err := s.coordCtrl.acquire(agentID, nil)
		if err != nil {
			return nil, xerrors.Errorf("ensure agent: %w", err)
		}
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
	s.coordCtrl.closeIdleConnections()
	s.coordCtrl.Close()
	// A coordination can still apply peer updates until the controller closes,
	// and fails if the tailnet is closed first.
	<-s.controller.Closed()
	_ = s.conn.Close()
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
// agents. It keeps the state coderd holds for each agent, and removes an agent on a timer if it
// has no open connections and hasn't been used in a while.
type MultiAgentController struct {
	*tailnet.BasicCoordinationController

	logger slog.Logger
	tracer trace.Tracer

	mu sync.Mutex
	// agents holds every agent the server wants a tunnel to.
	agents       map[uuid.UUID]*agentState
	coordination *tailnet.BasicCoordination

	cancel              context.CancelFunc
	expireOldAgentsDone chan struct{}
}

// agentState is what the server keeps for one agent until the agent expires.
// It does not expire while it has tickets, and its transports are handed out
// only with a ticket. A transport used after its ticket is released can dial
// after the state expired; the dial then creates a new state without it.
type agentState struct {
	lastUsed time.Time
	tickets  map[uuid.UUID]struct{}           // One per open connection, AgentConn or app request.
	api      *workspacesdk.AgentAPITransport  // Built on first use.
	apps     *workspaceapps.AgentAppTransport // Built on first use.
}

func (m *MultiAgentController) New(client tailnet.CoordinatorClient) tailnet.CloserWaiter {
	b := m.NewCoordination(client)
	// resync all destinations
	m.mu.Lock()
	defer m.mu.Unlock()
	m.coordination = b
	for agentID := range m.agents {
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

// acquire subscribes to agentID if needed and holds a ticket on its state
// until release is called. use, if not nil, runs with m.mu held and may
// build the state's transports; if it fails, no ticket is held.
func (m *MultiAgentController) acquire(agentID uuid.UUID, use func(*agentState) error) (release func(), _ error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.agents[agentID]
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
				return nil, err
			}
		}
		a = &agentState{tickets: make(map[uuid.UUID]struct{})}
		m.agents[agentID] = a
	}
	a.lastUsed = time.Now()
	if use != nil {
		if err := use(a); err != nil {
			return nil, err
		}
	}
	id := uuid.New()
	a.tickets[id] = struct{}{}
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(a.tickets, id)
	}, nil
}

// closeIdleConnections closes every agent's idle connections.
func (m *MultiAgentController) closeIdleConnections() {
	var transports []interface{ CloseIdleConnections() }
	m.mu.Lock()
	for _, a := range m.agents {
		if a.api != nil {
			transports = append(transports, a.api)
		}
		if a.apps != nil {
			transports = append(transports, a.apps)
		}
	}
	m.mu.Unlock()
	// Closing a connection releases its ticket, which takes m.mu.
	for _, t := range transports {
		t.CloseIdleConnections()
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
	deletedCount := 0

	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger.Debug(ctx, "pruning inactive agents", slog.F("agent_count", len(m.agents)))
	for agentID, a := range m.agents {
		// If no one has connected since the cutoff and there are no active
		// connections, remove the agent. Without tickets, its transports are
		// unused and have no connections to close.
		if time.Since(a.lastUsed) > cutoff && len(a.tickets) == 0 {
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
			deletedCount++
			delete(m.agents, agentID)
		}
	}
	m.logger.Debug(ctx, "pruned inactive agents",
		slog.F("deleted", deletedCount),
		slog.F("took", time.Since(start)),
	)
}

func (m *MultiAgentController) Close() {
	m.cancel()
	<-m.expireOldAgentsDone
}

func NewMultiAgentController(ctx context.Context, logger slog.Logger, tracer trace.Tracer, coordinatee tailnet.Coordinatee) *MultiAgentController {
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
		agents:              make(map[uuid.UUID]*agentState),
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
