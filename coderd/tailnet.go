package coderd

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/wsconncache"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/site"
	"github.com/coder/coder/tailnet"
)

var defaultTransport *http.Transport

func init() {
	var valid bool
	defaultTransport, valid = http.DefaultTransport.(*http.Transport)
	if !valid {
		panic("dev error: default transport is the wrong type")
	}
}

// TODO: ServerTailnet does not currently remove stale peers.

// NewServerTailnet creates a new tailnet intended for use by coderd. It
// automatically falls back to wsconncache if a legacy agent is encountered.
func NewServerTailnet(
	ctx context.Context,
	logger slog.Logger,
	derpServer *derp.Server,
	derpMap *tailcfg.DERPMap,
	coord *atomic.Pointer[tailnet.Coordinator],
	cache *wsconncache.Cache,
) (*ServerTailnet, error) {
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:   derpMap,
		Logger:    logger.Named("tailnet"),
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet conn: %w", err)
	}

	tn := &ServerTailnet{
		logger:      logger,
		conn:        conn,
		coordinator: coord,
		cache:       cache,
		agentNodes:  map[uuid.UUID]*tailnetNode{},
		transport:   defaultTransport.Clone(),
	}
	tn.transport.DialContext = tn.dialContext
	tn.transport.MaxIdleConnsPerHost = 10
	tn.transport.MaxIdleConns = 0

	conn.SetNodeCallback(func(node *tailnet.Node) {
		tn.nodesMu.Lock()
		ids := make([]uuid.UUID, 0, len(tn.agentNodes))
		for id := range tn.agentNodes {
			ids = append(ids, id)
		}
		tn.nodesMu.Unlock()

		err := (*tn.coordinator.Load()).BroadcastToAgents(ids, node)
		if err != nil {
			tn.logger.Error(context.Background(), "broadcast server node to agents", slog.Error(err))
		}
	})

	// This is set to allow local DERP traffic to be proxied through memory
	// instead of needing to hit the external access URL. Don't use the ctx
	// given in this callback, it's only valid while connecting.
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

	return tn, nil
}

type tailnetNode struct {
	node           *tailnet.Node
	lastConnection time.Time
	stop           func()
}

type ServerTailnet struct {
	logger      slog.Logger
	conn        *tailnet.Conn
	coordinator *atomic.Pointer[tailnet.Coordinator]
	cache       *wsconncache.Cache
	nodesMu     sync.Mutex
	// agentNodes is a map of agent tailnetNodes the server wants to keep a
	// connection to.
	agentNodes map[uuid.UUID]*tailnetNode

	transport *http.Transport
}

func (s *ServerTailnet) updateNode(id uuid.UUID, node *tailnet.Node) {
	s.nodesMu.Lock()
	cached, ok := s.agentNodes[id]
	if ok {
		cached.node = node
	}
	s.nodesMu.Unlock()

	if ok {
		err := s.conn.UpdateNodes([]*tailnet.Node{node}, false)
		if err != nil {
			s.logger.Error(context.Background(), "update node in server tailnet", slog.Error(err))
			return
		}
	}
}

func (s *ServerTailnet) ReverseProxy(targetURL, dashboardURL *url.URL, agentID uuid.UUID) (*httputil.ReverseProxy, func(), error) {
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

func (s *ServerTailnet) getNode(agentID uuid.UUID) (*tailnet.Node, error) {
	s.nodesMu.Lock()
	tnode, ok := s.agentNodes[agentID]
	// If we don't have the node, fetch it from the coordinator.
	if !ok {
		coord := *s.coordinator.Load()
		node := coord.Node(agentID)
		// The coordinator doesn't have the node either. Nothing we can do here.
		if node == nil {
			s.nodesMu.Unlock()
			return nil, xerrors.Errorf("node %q not found", agentID.String())
		}
		stop := coord.SubscribeAgent(agentID, s.updateNode)
		tnode = &tailnetNode{
			node:           node,
			lastConnection: time.Now(),
			stop:           stop,
		}
		s.agentNodes[agentID] = tnode

		err := coord.BroadcastToAgents([]uuid.UUID{agentID}, s.conn.Node())
		if err != nil {
			s.logger.Debug(context.Background(), "broadcast server node to agents", slog.Error(err))
		}
	}
	s.nodesMu.Unlock()

	if len(tnode.node.Addresses) == 0 {
		return nil, xerrors.New("agent has no reachable addresses")
	}

	// if we didn't already have the node locally, add it to our tailnet.
	if !ok {
		err := s.conn.UpdateNodes([]*tailnet.Node{tnode.node}, false)
		if err != nil {
			return nil, xerrors.Errorf("update nodes: %w", err)
		}
	}

	return tnode.node, nil
}

func (s *ServerTailnet) awaitNodeExists(ctx context.Context, id uuid.UUID, timeout time.Duration) (*tailnet.Node, error) {
	// Short circuit, if the node already exists, don't spend time setting up
	// the ticker and loop.
	if node, err := s.getNode(id); err == nil {
		return node, nil
	}

	var (
		ticker = time.NewTicker(10 * time.Millisecond)

		tries int
		node  *tailnet.Node
		err   error
	)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			// return the last error we got from getNode.
			return nil, xerrors.Errorf("tries %d, last error: %w", tries, err)
		case <-ticker.C:
		}

		tries++
		node, err = s.getNode(id)
		if err == nil {
			return node, nil
		}
	}
}

func (*ServerTailnet) nodeIsLegacy(node *tailnet.Node) bool {
	return node.Addresses[0].Addr() == codersdk.WorkspaceAgentIP
}

func (s *ServerTailnet) AgentConn(ctx context.Context, agentID uuid.UUID) (*codersdk.WorkspaceAgentConn, func(), error) {
	node, err := s.awaitNodeExists(ctx, agentID, 5*time.Second)
	if err != nil {
		return nil, nil, xerrors.Errorf("get agent node: %w", err)
	}

	var (
		conn *codersdk.WorkspaceAgentConn
		ret  = func() {}
	)

	if s.nodeIsLegacy(node) {
		cconn, release, err := s.cache.Acquire(agentID)
		if err != nil {
			return nil, nil, xerrors.Errorf("acquire legacy agent conn: %w", err)
		}

		conn = cconn.WorkspaceAgentConn
		ret = release
	} else {
		conn = codersdk.NewWorkspaceAgentConn(s.conn, codersdk.WorkspaceAgentConnOptions{
			AgentID:   agentID,
			GetNode:   s.getNode,
			CloseFunc: func() error { return codersdk.ErrSkipClose },
		})
	}

	reachable := conn.AwaitReachable(ctx)
	if !reachable {
		return nil, nil, xerrors.New("agent is unreachable")
	}

	return conn, ret, nil
}

func (s *ServerTailnet) DialAgentNetConn(ctx context.Context, agentID uuid.UUID, network, addr string) (net.Conn, error) {
	conn, release, err := s.AgentConn(ctx, agentID)
	if err != nil {
		return nil, xerrors.Errorf("acquire agent conn: %w", err)
	}
	defer release()

	reachable := conn.AwaitReachable(ctx)
	if !reachable {
		return nil, xerrors.New("agent is unreachable")
	}

	node, err := s.getNode(agentID)
	if err != nil {
		return nil, xerrors.New("get agent node")
	}

	_, rawPort, _ := net.SplitHostPort(addr)
	port, _ := strconv.ParseUint(rawPort, 10, 16)
	ipp := netip.AddrPortFrom(node.Addresses[0].Addr(), uint16(port))

	if network == "tcp" {
		return conn.DialContextTCP(ctx, ipp)
	} else if network == "udp" {
		return conn.DialContextUDP(ctx, ipp)
	} else {
		return nil, xerrors.Errorf("unknown network %q", network)
	}
}

func (s *ServerTailnet) Close() error {
	_ = s.cache.Close()
	_ = s.conn.Close()
	s.transport.CloseIdleConnections()
	return nil
}
