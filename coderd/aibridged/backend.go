package aibridged

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
)

// mcpConfigTimeout bounds the startup MCP discovery call. The connect loop
// retries on failure, so exceeding it only delays mode selection.
const mcpConfigTimeout = 10 * time.Second

// ServerOption configures a gateway server before it connects to coderd.
type ServerOption func(*Server)

// WithExperiments enables the gateway's experiments, which select the request
// backend at startup. The selection is fixed for the lifetime of the server.
func WithExperiments(experiments codersdk.Experiments) ServerOption {
	return func(s *Server) {
		s.reverseProxy = experiments.Enabled(codersdk.ExperimentAIGatewayReverseProxy)
	}
}

// WithMetrics supplies metrics for a server-owned request pool.
func WithMetrics(metrics *aibridge.Metrics) ServerOption {
	return func(s *Server) { s.metrics = metrics }
}

// requestBackend is the request handler selected at startup, immutable after
// publication. Interception and proxy mode each set their own field.
type requestBackend struct {
	// pool serves interception mode. It is nil in proxy mode, which is what
	// keeps request serving and provider reloads from using interception.
	pool Pooler
	// proxy serves proxy mode. It is nil until the first provider snapshot
	// arrives, because a router with no providers cannot serve anything.
	proxy *proxyHandler
}

// KeyPools returns the pools of the published provider snapshot, skipping
// providers which have none.
func (b *requestBackend) KeyPools() []*keypool.Pool {
	if b.proxy != nil {
		return b.proxy.router.KeyPools()
	}
	if pool, ok := b.pool.(interface{ KeyPools() []*keypool.Pool }); ok {
		return pool.KeyPools()
	}
	return nil
}

// proxyHandler serves one router snapshot through the server's in-flight
// tracking. There is one per snapshot, so callers see the same handler
// between reloads and a request finishes on the snapshot it started with.
type proxyHandler struct {
	inflight *inflightTracker
	router   *aibridge.ProxyRouter
}

func (h *proxyHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.inflight.Serve(rw, r, h.router)
}

// KeyPools feeds [keypool.NewStateCollector]. It returns nil before mode
// selection and, in proxy mode, before the first provider snapshot.
func (s *Server) KeyPools() []*keypool.Pool {
	if backend := s.backend.Load(); backend != nil {
		return backend.KeyPools()
	}
	return nil
}

// initializeInterception publishes the interception backend. The pool is
// server-owned unless the caller supplied one.
func (s *Server) initializeInterception() error {
	pool := s.initialPool
	if pool == nil {
		var err error
		pool, err = NewCachedBridgePool(DefaultPoolOptions, nil, s.logger.Named("pool"), s.metrics, s.tracer)
		if err != nil {
			return xerrors.Errorf("create request pool: %w", err)
		}
	}
	s.backend.Store(&requestBackend{pool: pool})
	return nil
}

// initializeBackend selects the request backend on the server's first
// connection to coderd, so it runs at most once per server. A failed or
// malformed MCP discovery is retried on the next connection, never read as an
// empty configuration.
func (s *Server) initializeBackend(ctx context.Context, client DRPCClient) error {
	s.backendMu.Lock()
	defer s.backendMu.Unlock()
	if s.backend.Load() != nil {
		return nil
	}
	configCtx, cancel := context.WithTimeout(ctx, mcpConfigTimeout)
	defer cancel()
	configs, err := client.GetMCPServerConfigs(configCtx, &proto.GetMCPServerConfigsRequest{})
	if err != nil {
		return xerrors.Errorf("get MCP server configs: %w", err)
	}
	if configs == nil {
		return xerrors.New("nil MCP server configs response")
	}
	if configs.GetCoderMcpConfig() != nil || len(configs.GetExternalAuthMcpConfigs()) != 0 {
		s.logger.Info(ctx, "mcp configuration requires interception; restart to change gateway mode")
		return s.initializeInterception()
	}
	s.backend.Store(&requestBackend{})
	s.logger.Info(ctx, "using reverse proxy routing; restart to change gateway mode")
	return nil
}

// ReplaceProviders publishes a provider snapshot to the selected backend.
// Proxy requests retain their router across reloads, and a failed construction
// keeps the previous router serving. Callers must have completed mode selection
// first; reloaders block on the client the connect loop offers afterwards.
//
// A replaced router is not drained: in-flight requests hold it by reference
// and finish on the snapshot they started with. Shutdown drains them through
// the server's in-flight tracking, whichever snapshot they run on.
func (s *Server) ReplaceProviders(ctx context.Context, providers []aibridge.Provider) error {
	s.backendMu.Lock()
	defer s.backendMu.Unlock()
	if s.isShutdown() {
		return s.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	backend := s.backend.Load()
	if backend == nil {
		return xerrors.New("gateway mode not selected")
	}
	if backend.pool != nil {
		backend.pool.ReplaceProviders(providers)
		return nil
	}
	router, err := aibridge.NewProxyRouter(providers, s.logger)
	if err != nil {
		return xerrors.Errorf("create proxy router: %w", err)
	}
	s.backend.Store(&requestBackend{proxy: &proxyHandler{inflight: s.inflight, router: router}})
	return nil
}

func notReadyHandler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "AI Gateway is not ready", http.StatusServiceUnavailable)
}
