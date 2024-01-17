package coderd

import (
	"bufio"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/wsconncache"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/site"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/retry"
)

var tailnetTransport *http.Transport

func init() {
	var valid bool
	tailnetTransport, valid = http.DefaultTransport.(*http.Transport)
	if !valid {
		panic("dev error: default transport is the wrong type")
	}
}

var _ workspaceapps.AgentProvider = (*ServerTailnet)(nil)

// NewServerTailnet creates a new tailnet intended for use by coderd. It
// automatically falls back to wsconncache if a legacy agent is encountered.
func NewServerTailnet(
	ctx context.Context,
	logger slog.Logger,
	derpServer *derp.Server,
	derpMapFn func() *tailcfg.DERPMap,
	derpForceWebSockets bool,
	getMultiAgent func(context.Context) (tailnet.MultiAgentConn, error),
	cache *wsconncache.Cache,
	traceProvider trace.TracerProvider,
) (*ServerTailnet, error) {
	logger = logger.Named("servertailnet")
	originalDerpMap := derpMapFn()
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:             originalDerpMap,
		DERPForceWebSockets: derpForceWebSockets,
		Logger:              logger,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet conn: %w", err)
	}

	serverCtx, cancel := context.WithCancel(ctx)
	derpMapUpdaterClosed := make(chan struct{})
	go func() {
		defer close(derpMapUpdaterClosed)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-serverCtx.Done():
				return
			case <-ticker.C:
			}

			newDerpMap := derpMapFn()
			if !tailnet.CompareDERPMaps(originalDerpMap, newDerpMap) {
				conn.SetDERPMap(newDerpMap)
				originalDerpMap = newDerpMap
			}
		}
	}()

	tn := &ServerTailnet{
		ctx:                  serverCtx,
		cancel:               cancel,
		derpMapUpdaterClosed: derpMapUpdaterClosed,
		logger:               logger,
		tracer:               traceProvider.Tracer(tracing.TracerName),
		conn:                 conn,
		getMultiAgent:        getMultiAgent,
		cache:                cache,
		agentConnectionTimes: map[uuid.UUID]time.Time{},
		agentTickets:         map[uuid.UUID]map[uuid.UUID]struct{}{},
		transport:            tailnetTransport.Clone(),
	}
	tn.transport.DialContext = tn.dialContext
	tn.transport.MaxIdleConnsPerHost = 10
	tn.transport.MaxIdleConns = 0
	// We intentionally don't verify the certificate chain here.
	// The connection to the workspace is already established and most
	// apps are already going to be accessed over plain HTTP, this config
	// simply allows apps being run over HTTPS to be accessed without error --
	// many of which may be using self-signed certs.
	tn.transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		//nolint:gosec
		InsecureSkipVerify: true,
	}

	agentConn, err := getMultiAgent(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get initial multi agent: %w", err)
	}
	tn.agentConn.Store(&agentConn)

	err = tn.getAgentConn().UpdateSelf(conn.Node())
	if err != nil {
		tn.logger.Warn(context.Background(), "server tailnet update self", slog.Error(err))
	}
	conn.SetNodeCallback(func(node *tailnet.Node) {
		err := tn.getAgentConn().UpdateSelf(node)
		if err != nil {
			tn.logger.Warn(context.Background(), "broadcast server node to agents", slog.Error(err))
		}
	})

	// This is set to allow local DERP traffic to be proxied through memory
	// instead of needing to hit the external access URL. Don't use the ctx
	// given in this callback, it's only valid while connecting.
	if derpServer != nil {
		conn.SetDERPRegionDialer(func(_ context.Context, region *tailcfg.DERPRegion) net.Conn {
			if !region.EmbeddedRelay {
				return nil
			}
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

	go tn.watchAgentUpdates()
	go tn.expireOldAgents()
	return tn, nil
}

func (s *ServerTailnet) expireOldAgents() {
	const (
		tick   = 5 * time.Minute
		cutoff = 30 * time.Minute
	)

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}

		s.doExpireOldAgents(cutoff)
	}
}

func (s *ServerTailnet) doExpireOldAgents(cutoff time.Duration) {
	// TODO: add some attrs to this.
	ctx, span := s.tracer.Start(s.ctx, tracing.FuncName())
	defer span.End()

	start := time.Now()
	deletedCount := 0

	s.nodesMu.Lock()
	s.logger.Debug(ctx, "pruning inactive agents", slog.F("agent_count", len(s.agentConnectionTimes)))
	agentConn := s.getAgentConn()
	for agentID, lastConnection := range s.agentConnectionTimes {
		// If no one has connected since the cutoff and there are no active
		// connections, remove the agent.
		if time.Since(lastConnection) > cutoff && len(s.agentTickets[agentID]) == 0 {
			deleted, err := s.conn.RemovePeer(tailnet.PeerSelector{
				ID: tailnet.NodeID(agentID),
				IP: netip.PrefixFrom(tailnet.IPFromUUID(agentID), 128),
			})
			if err != nil {
				s.logger.Warn(ctx, "failed to remove peer from server tailnet", slog.Error(err))
				continue
			}
			if !deleted {
				s.logger.Warn(ctx, "peer didn't exist in tailnet", slog.Error(err))
			}

			deletedCount++
			delete(s.agentConnectionTimes, agentID)
			err = agentConn.UnsubscribeAgent(agentID)
			if err != nil {
				s.logger.Error(ctx, "unsubscribe expired agent", slog.Error(err), slog.F("agent_id", agentID))
			}
		}
	}
	s.nodesMu.Unlock()
	s.logger.Debug(s.ctx, "successfully pruned inactive agents",
		slog.F("deleted", deletedCount),
		slog.F("took", time.Since(start)),
	)
}

func (s *ServerTailnet) watchAgentUpdates() {
	for {
		conn := s.getAgentConn()
		nodes, ok := conn.NextUpdate(s.ctx)
		if !ok {
			if conn.IsClosed() && s.ctx.Err() == nil {
				s.logger.Warn(s.ctx, "multiagent closed, reinitializing")
				s.reinitCoordinator()
				continue
			}
			return
		}

		err := s.conn.UpdateNodes(nodes, false)
		if err != nil {
			if xerrors.Is(err, tailnet.ErrConnClosed) {
				s.logger.Warn(context.Background(), "tailnet conn closed, exiting watchAgentUpdates", slog.Error(err))
				return
			}
			s.logger.Error(context.Background(), "update node in server tailnet", slog.Error(err))
			return
		}
	}
}

func (s *ServerTailnet) getAgentConn() tailnet.MultiAgentConn {
	return *s.agentConn.Load()
}

func (s *ServerTailnet) reinitCoordinator() {
	start := time.Now()
	for retrier := retry.New(25*time.Millisecond, 5*time.Second); retrier.Wait(s.ctx); {
		s.nodesMu.Lock()
		agentConn, err := s.getMultiAgent(s.ctx)
		if err != nil {
			s.nodesMu.Unlock()
			s.logger.Error(s.ctx, "reinit multi agent", slog.Error(err))
			continue
		}
		s.agentConn.Store(&agentConn)

		// Resubscribe to all of the agents we're tracking.
		for agentID := range s.agentConnectionTimes {
			err := agentConn.SubscribeAgent(agentID)
			if err != nil {
				s.logger.Warn(s.ctx, "resubscribe to agent", slog.Error(err), slog.F("agent_id", agentID))
			}
		}

		s.logger.Info(s.ctx, "successfully reinitialized multiagent",
			slog.F("agents", len(s.agentConnectionTimes)),
			slog.F("took", time.Since(start)),
		)
		s.nodesMu.Unlock()
		return
	}
}

type ServerTailnet struct {
	ctx                  context.Context
	cancel               func()
	derpMapUpdaterClosed chan struct{}

	logger        slog.Logger
	tracer        trace.Tracer
	conn          *tailnet.Conn
	getMultiAgent func(context.Context) (tailnet.MultiAgentConn, error)
	agentConn     atomic.Pointer[tailnet.MultiAgentConn]
	cache         *wsconncache.Cache
	nodesMu       sync.Mutex
	// agentConnectionTimes is a map of agent tailnetNodes the server wants to
	// keep a connection to. It contains the last time the agent was connected
	// to.
	agentConnectionTimes map[uuid.UUID]time.Time
	// agentTockets holds a map of all open connections to an agent.
	agentTickets map[uuid.UUID]map[uuid.UUID]struct{}

	transport *http.Transport
}

func (s *ServerTailnet) ReverseProxy(targetURL, dashboardURL *url.URL, agentID uuid.UUID) (_ *httputil.ReverseProxy, release func(), _ error) {
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		site.RenderStaticErrorPage(w, r, site.ErrorPageData{
			Status:       http.StatusBadGateway,
			Title:        "Bad Gateway",
			Description:  "Failed to proxy request to application: " + err.Error(),
			RetryEnabled: true,
			DashboardURL: dashboardURL.String(),
		})
	}
	proxy.Director = s.director(agentID, proxy.Director)
	proxy.Transport = s.transport

	return proxy, func() {}, nil
}

type agentIDKey struct{}

// director makes sure agentIDKey is set on the context in the reverse proxy.
// This allows the transport to correctly identify which agent to dial to.
func (*ServerTailnet) director(agentID uuid.UUID, prev func(req *http.Request)) func(req *http.Request) {
	return func(req *http.Request) {
		ctx := context.WithValue(req.Context(), agentIDKey{}, agentID)
		*req = *req.WithContext(ctx)
		prev(req)
	}
}

func (s *ServerTailnet) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	agentID, ok := ctx.Value(agentIDKey{}).(uuid.UUID)
	if !ok {
		return nil, xerrors.Errorf("no agent id attached")
	}

	return s.DialAgentNetConn(ctx, agentID, network, addr)
}

func (s *ServerTailnet) ensureAgent(agentID uuid.UUID) error {
	s.nodesMu.Lock()
	defer s.nodesMu.Unlock()

	_, ok := s.agentConnectionTimes[agentID]
	// If we don't have the node, subscribe.
	if !ok {
		s.logger.Debug(s.ctx, "subscribing to agent", slog.F("agent_id", agentID))
		err := s.getAgentConn().SubscribeAgent(agentID)
		if err != nil {
			return xerrors.Errorf("subscribe agent: %w", err)
		}
		s.agentTickets[agentID] = map[uuid.UUID]struct{}{}
	}

	s.agentConnectionTimes[agentID] = time.Now()
	return nil
}

func (s *ServerTailnet) acquireTicket(agentID uuid.UUID) (release func()) {
	id := uuid.New()
	s.nodesMu.Lock()
	s.agentTickets[agentID][id] = struct{}{}
	s.nodesMu.Unlock()

	return func() {
		s.nodesMu.Lock()
		delete(s.agentTickets[agentID], id)
		s.nodesMu.Unlock()
	}
}

func (s *ServerTailnet) AgentConn(ctx context.Context, agentID uuid.UUID) (*codersdk.WorkspaceAgentConn, func(), error) {
	var (
		conn *codersdk.WorkspaceAgentConn
		ret  func()
	)

	if s.getAgentConn().AgentIsLegacy(agentID) {
		s.logger.Debug(s.ctx, "acquiring legacy agent", slog.F("agent_id", agentID))
		cconn, release, err := s.cache.Acquire(agentID)
		if err != nil {
			return nil, nil, xerrors.Errorf("acquire legacy agent conn: %w", err)
		}

		conn = cconn.WorkspaceAgentConn
		ret = release
	} else {
		s.logger.Debug(s.ctx, "acquiring agent", slog.F("agent_id", agentID))
		err := s.ensureAgent(agentID)
		if err != nil {
			return nil, nil, xerrors.Errorf("ensure agent: %w", err)
		}
		ret = s.acquireTicket(agentID)

		conn = codersdk.NewWorkspaceAgentConn(s.conn, codersdk.WorkspaceAgentConnOptions{
			AgentID:   agentID,
			CloseFunc: func() error { return codersdk.ErrSkipClose },
		})
	}

	// Since we now have an open conn, be careful to close it if we error
	// without returning it to the user.

	reachable := conn.AwaitReachable(ctx)
	if !reachable {
		ret()
		return nil, nil, xerrors.New("agent is unreachable")
	}

	return conn, ret, nil
}

func (s *ServerTailnet) DialAgentNetConn(ctx context.Context, agentID uuid.UUID, network, addr string) (net.Conn, error) {
	conn, release, err := s.AgentConn(ctx, agentID)
	if err != nil {
		return nil, xerrors.Errorf("acquire agent conn: %w", err)
	}

	// Since we now have an open conn, be careful to close it if we error
	// without returning it to the user.

	nc, err := conn.DialContext(ctx, network, addr)
	if err != nil {
		release()
		return nil, xerrors.Errorf("dial context: %w", err)
	}

	return &netConnCloser{Conn: nc, close: func() {
		release()
	}}, err
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
	s.cancel()
	_ = s.cache.Close()
	_ = s.conn.Close()
	s.transport.CloseIdleConnections()
	<-s.derpMapUpdaterClosed
	return nil
}
