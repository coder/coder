// Package exitnode implements the exit node: a tailnet peer that terminates
// workspace egress via HTTP CONNECT, enforces a policy, and reports every
// flow to coderd.
package exitnode

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/exitnode/exitnodesdk"
	"github.com/coder/coder/v2/tailnet"
)

// Options configures a Server.
type Options struct {
	// Client talks to coderd on behalf of this exit node. Required.
	Client *exitnodesdk.Client
	// ExitNodeID is the ID half of the token. It determines the node's
	// deterministic tailnet address. Required.
	ExitNodeID uuid.UUID
	// Policy decides each flow. Required.
	Policy PolicyEvaluator
	// ListenPort is the CONNECT port inside the tailnet. Defaults to
	// codersdk.ExitNodeTailnetPort.
	ListenPort int
	// PrometheusRegistry receives the exit node collectors when non-nil.
	PrometheusRegistry prometheus.Registerer
	// Version is reported to coderd on registration.
	Version string
	// Hostname is reported to coderd on registration.
	Hostname string
	// WireguardEndpointsAdvertised are public ip:port pairs coderd relays to
	// agents so they can exempt them from egress enforcement.
	WireguardEndpointsAdvertised []string
	// WireguardListenPort fixes the local WireGuard UDP port so an advertised
	// endpoint stays valid. Zero picks a random port.
	WireguardListenPort uint16
	// BlockEndpoints forces all traffic through DERP.
	BlockEndpoints bool
	// DialTimeout bounds upstream connects. Defaults to DefaultDialTimeout.
	DialTimeout time.Duration
	// SniffTimeout bounds host sniffing. Defaults to DefaultSniffTimeout.
	SniffTimeout time.Duration
}

// Server is a running exit node.
type Server struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger slog.Logger

	id      uuid.UUID
	addr    netip.Addr
	policy  PolicyEvaluator
	metrics *Metrics

	registerLoop *exitnodesdk.RegisterLoop
	conn         *tailnet.Conn
	controller   *tailnet.Controller
	coordCtrl    *tailnet.TunnelSrcCoordController
	agents       *AgentTable
	flows        *FlowReporter
	listener     net.Listener
	proxy        *ConnectProxy

	// registerErr receives a fatal registration loop error.
	registerErr chan error
	serveDone   chan error

	// mu guards the fields below. Re-registration callbacks can arrive while
	// New is still building the tailnet, so agent updates are parked until
	// the coordination controller exists.
	mu            sync.Mutex
	tailnetReady  bool
	agentsUpdated bool
	latestAgents  []uuid.UUID

	closeOnce sync.Once
	closeErr  error
}

// New registers with coderd, joins the tailnet, and starts accepting CONNECT
// requests. Call Close to shut everything down.
func New(ctx context.Context, logger slog.Logger, opts Options) (*Server, error) {
	if opts.Client == nil {
		return nil, xerrors.New("client is required")
	}
	if opts.ExitNodeID == uuid.Nil {
		return nil, xerrors.New("exit node id is required")
	}
	if opts.Policy == nil {
		return nil, xerrors.New("policy is required")
	}
	if opts.ListenPort == 0 {
		opts.ListenPort = codersdk.ExitNodeTailnetPort
	}
	if opts.ListenPort < 1 || opts.ListenPort > 65535 {
		return nil, xerrors.Errorf("listen port %d out of range", opts.ListenPort)
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &Server{
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
		id:          opts.ExitNodeID,
		addr:        TailnetAddrForID(opts.ExitNodeID),
		policy:      opts.Policy,
		metrics:     NewMetrics(opts.PrometheusRegistry),
		agents:      NewAgentTable(),
		registerErr: make(chan error, 1),
		serveDone:   make(chan error, 1),
	}
	var err error
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()

	// Registration first: it yields the DERP map the tailnet needs and the
	// agents to tunnel to.
	var regResp codersdk.RegisterExitNodeResponse
	s.registerLoop, regResp, err = opts.Client.RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{
		Logger: logger.Named("register"),
		Request: codersdk.RegisterExitNodeRequest{
			Version:            opts.Version,
			Hostname:           opts.Hostname,
			WireguardEndpoints: opts.WireguardEndpointsAdvertised,
		},
		CallbackFn: s.handleRegister,
		FailureFn: func(err error) {
			select {
			case s.registerErr <- err:
			default:
			}
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("register exit node: %w", err)
	}
	if regResp.DERPMap == nil {
		err = xerrors.New("registration response did not include a DERP map")
		return nil, err
	}
	s.mu.Lock()
	if !s.agentsUpdated {
		s.latestAgents = regResp.AgentIDs
	}
	s.mu.Unlock()

	s.conn, err = tailnet.NewConn(&tailnet.Options{
		ID:                  opts.ExitNodeID,
		Addresses:           []netip.Prefix{netip.PrefixFrom(s.addr, 128)},
		DERPMap:             regResp.DERPMap,
		DERPForceWebSockets: regResp.DERPForceWebSockets,
		BlockEndpoints:      opts.BlockEndpoints,
		ListenPort:          opts.WireguardListenPort,
		Logger:              logger.Named("net.tailnet"),
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet conn: %w", err)
	}

	dialer, err := opts.Client.TailnetDialer()
	if err != nil {
		return nil, xerrors.Errorf("create tailnet dialer: %w", err)
	}
	s.controller = tailnet.NewController(logger.Named("tailnet_controller"), dialer)
	s.controller.DERPCtrl = tailnet.NewBasicDERPController(logger, nil, s.conn)
	s.coordCtrl = tailnet.NewTunnelSrcCoordController(logger, s.conn)
	s.controller.CoordCtrl = s.coordCtrl
	s.mu.Lock()
	s.tailnetReady = true
	s.applyAgentsLocked(s.latestAgents)
	s.mu.Unlock()
	s.controller.Run(ctx)

	s.flows = NewFlowReporter(ctx, FlowReporterOptions{
		Logger:  logger.Named("flows"),
		Client:  opts.Client,
		Metrics: s.metrics,
	})

	s.proxy = NewConnectProxy(ConnectProxyOptions{
		Logger:       logger.Named("connect"),
		Policy:       opts.Policy,
		Agents:       s.agents,
		Flows:        s.flows,
		Metrics:      s.metrics,
		DialTimeout:  opts.DialTimeout,
		SniffTimeout: opts.SniffTimeout,
	})

	s.listener, err = s.conn.Listen("tcp", fmt.Sprintf(":%d", opts.ListenPort))
	if err != nil {
		return nil, xerrors.Errorf("listen on tailnet port %d: %w", opts.ListenPort, err)
	}
	go func() {
		s.serveDone <- s.proxy.Serve(ctx, s.listener)
	}()

	logger.Info(ctx, "exit node started",
		slog.F("exit_node_id", s.id),
		slog.F("tailnet_addr", s.addr),
		slog.F("listen_port", opts.ListenPort),
		slog.F("agents", len(regResp.AgentIDs)),
	)
	return s, nil
}

// TailnetAddr is the deterministic address agents dial to reach this node.
func (s *Server) TailnetAddr() netip.Addr {
	return s.addr
}

// TailnetAddrForID returns the deterministic tailnet address of the exit node
// with the given ID. Agents dial it without discovery.
func TailnetAddrForID(id uuid.UUID) netip.Addr {
	return tailnet.TailscaleServicePrefix.AddrFromUUID(id)
}

// Metrics exposes the node's collectors, for example to a test.
func (s *Server) Metrics() *Metrics {
	return s.metrics
}

// ReloadPolicy re-reads the policy when the evaluator supports it. It is
// wired to SIGHUP by the CLI.
func (s *Server) ReloadPolicy() error {
	reloader, ok := s.policy.(Reloader)
	if !ok {
		return xerrors.New("policy does not support reloading")
	}
	if err := reloader.Reload(); err != nil {
		s.metrics.PolicyReloadTotal.WithLabelValues("error").Inc()
		return xerrors.Errorf("reload policy: %w", err)
	}
	s.metrics.PolicyReloadTotal.WithLabelValues("success").Inc()
	s.logger.Info(s.ctx, "policy reloaded from file")
	return nil
}

// Wait blocks until the server stops on its own, returning the cause. It
// returns nil after Close.
func (s *Server) Wait() error {
	select {
	case err := <-s.registerErr:
		return xerrors.Errorf("registration loop: %w", err)
	case err := <-s.serveDone:
		if err != nil {
			return xerrors.Errorf("serve: %w", err)
		}
		return nil
	case <-s.ctx.Done():
		return nil
	}
}

// handleRegister is invoked on every periodic re-registration.
func (s *Server) handleRegister(res codersdk.RegisterExitNodeResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latestAgents = res.AgentIDs
	s.agentsUpdated = true
	if !s.tailnetReady {
		return nil
	}
	s.applyAgentsLocked(res.AgentIDs)
	if res.DERPMap != nil {
		s.conn.SetDERPMap(res.DERPMap)
	}
	return nil
}

// applyAgentsLocked makes the given agents, and only those, reachable: it
// updates the source identity table and syncs coordinator tunnels to match.
// s.mu must be held.
func (s *Server) applyAgentsLocked(agentIDs []uuid.UUID) {
	s.agents.Set(agentIDs)
	s.coordCtrl.SyncDestinations(agentIDs)
}

// Close stops the server. It is safe to call more than once.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		if s.listener != nil {
			_ = s.listener.Close()
		}
		if s.proxy != nil {
			s.proxy.Wait()
		}
		if s.registerLoop != nil {
			s.registerLoop.Close()
		}
		if s.controller != nil {
			<-s.controller.Closed()
		}
		if s.conn != nil {
			if err := s.conn.Close(); err != nil {
				s.closeErr = xerrors.Errorf("close tailnet: %w", err)
			}
		}
		if s.flows != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.flows.Close(ctx); err != nil {
				s.logger.Warn(ctx, "final flow flush failed", slog.Error(err))
			}
		}
	})
	return s.closeErr
}
