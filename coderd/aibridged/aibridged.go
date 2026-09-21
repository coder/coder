package aibridged

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

// mcpConfigTimeout bounds the startup MCP discovery call. The connect loop
// retries on failure, so exceeding it only delays mode selection.
const mcpConfigTimeout = 10 * time.Second

var (
	_ io.Closer = &Server{}

	ErrShutdown = xerrors.New("aibridged server shutdown")
)

// Server manages the AI Gateway's connection to coderd and request handlers
// for embedded and standalone deployments. It selects interception or proxy
// mode at startup, applies provider snapshots, and coordinates shutdown.
type Server struct {
	clientDialer Dialer
	clientCh     chan DRPCClient

	// backend holds the current request handler. Mode is fixed at startup.
	// Provider reloads update the pool in interception mode or atomically replace
	// the handler in proxy mode. In-flight requests retain their original router.
	backend atomic.Pointer[backend]
	// backendMu serializes mode selection and provider replacement construction.
	backendMu sync.Mutex
	// backendPublishMu serializes only the final publication step with shutdown.
	backendPublishMu sync.Mutex

	// inflight tracks request acquisition and serving across backend snapshots.
	// Shutdown waits for completion or cancels contexts when its deadline expires.
	inflight *aibridge.InflightGate

	// reverseProxyExp is the experiment flag. Proxy mode also requires no MCP configs.
	reverseProxyExp bool
	metrics         *aibridge.Metrics
	// recorder is shared only by proxy-mode router snapshots.
	recorder recorder.Recorder

	logger slog.Logger
	tracer trace.Tracer
	wg     sync.WaitGroup

	// connected tracks whether the DRPC connection to coderd is currently active.
	connected atomic.Bool
	// shuttingDown stops backend publication and request admission before the
	// connection lifecycle is canceled, allowing admitted requests to record
	// their completion during graceful shutdown.
	shuttingDown atomic.Bool

	// lifecycleCtx is canceled when we start closing or when the
	// connection loop exits permanently.
	lifecycleCtx context.Context
	// cancelFn closes the lifecycleCtx with the reason it closed.
	cancelFn context.CancelCauseFunc

	shutdownOnce sync.Once
}

// backend holds either an interception pool or a proxy router.
// Its fields are fixed after publication. Both pool and proxyRouter are nil when
// proxy mode is selected but providers are not yet loaded.
type backend struct {
	pool        Pooler                 // RequestBridge pool.
	proxyRouter http.Handler           // Proxy router for this snapshot.
	keyPools    func() []*keypool.Pool // Current key pools for metric scrapes.
	cleanup     func()                 // Releases resources owned by this snapshot.
}

// New starts a gateway server. Creates a request pool only when interception
// mode is selected. Mode is fixed at startup and requires a restart to change.
func New(ctx context.Context, rpcDialer Dialer, logger slog.Logger, tracer trace.Tracer, experiments codersdk.Experiments, metrics *aibridge.Metrics) (*Server, error) {
	if rpcDialer == nil {
		return nil, xerrors.Errorf("nil rpcDialer given")
	}

	ctx, cancel := context.WithCancelCause(ctx)
	daemon := &Server{
		logger:       logger,
		tracer:       tracer,
		clientDialer: rpcDialer,
		clientCh:     make(chan DRPCClient),
		lifecycleCtx: ctx,
		cancelFn:     cancel,

		reverseProxyExp: experiments.Enabled(codersdk.ExperimentAIGatewayReverseProxy),
		metrics:         metrics,
		inflight:        aibridge.NewInflightGate(logger),
	}

	daemon.recorder = aibridge.NewRecorder(
		logger.Named("recorder"),
		tracer,
		func(recordCtx context.Context) (aibridge.Recorder, error) {
			apiKeyID, ok := authenticatedAPIKeyIDFromContext(recordCtx)
			if !ok {
				return nil, xerrors.New("authenticated API key ID missing from recorder context")
			}
			client, err := daemon.Client(recordCtx)
			if err != nil {
				return nil, xerrors.Errorf("acquire recorder client: %w", err)
			}
			return recorder.NewDRPCRecorder(apiKeyID, client), nil
		},
	)

	if !daemon.reverseProxyExp {
		if err := daemon.initializeInterception(); err != nil {
			cancel(err)
			return nil, err
		}
	}

	daemon.wg.Add(1)
	go daemon.connect()

	return daemon, nil
}

// Connect establishes a connection to coderd.
func (s *Server) connect() {
	defer s.logger.Debug(s.lifecycleCtx, "connect loop exited")
	defer s.wg.Done()
	defer func() {
		if s.lifecycleCtx.Err() == nil && !s.shuttingDown.Load() {
			s.cancelFn(xerrors.New("connect loop exited"))
		}
	}()

	logConnect := s.logger.With(slog.F("context", "aibridged.server")).Debug
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
connectLoop:
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(s.lifecycleCtx); {
		// It's possible for the aibridge daemon to be shut down
		// before the wait is complete!
		if s.isShutdown() {
			return
		}
		s.logger.Debug(s.lifecycleCtx, "dialing coderd")
		client, err := s.clientDialer(s.lifecycleCtx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				if s.lifecycleCtx.Err() == nil {
					s.cancelFn(err)
				}
				return
			}
			var sdkErr *codersdk.Error
			// If something is wrong with configuration, stop trying to connect.
			if errors.As(err, &sdkErr) {
				switch sdkErr.StatusCode() {
				// These statuses are terminal failures from the /api/v2/ai-gateway/serve
				// handshake: wrong gateway key, incompatible API version, or entitlement failure.
				//
				// 404 means this coderd does not expose the AI Gateway serve
				// endpoint (older version); retrying cannot succeed. The URL is
				// expected to point directly at coderd, so a 404 from an
				// intermediary is not distinguished.
				case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
					err = xerrors.Errorf("dial coderd: %w", err)
					s.logger.Error(s.lifecycleCtx, "fatal error dialing coderd",
						slog.Error(err),
						slog.F("status_code", sdkErr.StatusCode()),
						slog.F("url", sdkErr.URL()),
					)
					s.cancelFn(err)
					return
				default:
					err = xerrors.Errorf("unexpected HTTP response dialing coderd: %w", err)
				}
			}
			if s.isShutdown() {
				return
			}
			s.logger.Warn(s.lifecycleCtx, "coderd client failed to dial", slog.Error(err))
			continue
		}

		if err := s.initializeBackend(s.lifecycleCtx, client); err != nil {
			_ = client.DRPCConn().Close()
			s.logger.Warn(s.lifecycleCtx, "failed to initialize gateway request handler", slog.Error(err))
			continue
		}

		// Logged at info so operators of standalone (external) gateways
		// can see initial connection and reconnection after a dial
		// failure (paired with the warning logged above).
		s.logger.Info(s.lifecycleCtx, "successfully connected to coderd")
		retrier.Reset()
		s.connected.Store(true)

		// Serve the client until we are closed or it disconnects.
		for {
			select {
			case <-s.lifecycleCtx.Done():
				s.connected.Store(false)
				client.DRPCConn().Close()
				return
			case <-client.DRPCConn().Closed():
				s.connected.Store(false)
				logConnect(s.lifecycleCtx, "connection to coderd closed")
				continue connectLoop
			case s.clientCh <- client:
				continue
			}
		}
	}
}

// initializeBackend selects the backend on the server's first connection to coderd.
// A failed or malformed MCP discovery is retried on the next connection, never read as an
// empty configuration.
func (s *Server) initializeBackend(ctx context.Context, client DRPCClient) error {
	s.backendMu.Lock()
	defer s.backendMu.Unlock()
	if s.backend.Load() != nil {
		return nil
	}
	if s.shuttingDown.Load() {
		return ErrShutdown
	}
	if err := s.lifecycleCtx.Err(); err != nil {
		return s.Err()
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

	// MCP configuration requires interception mode.
	if len(configs.GetExternalAuthMcpConfigs()) != 0 || configs.GetCoderMcpConfig() != nil {
		s.logger.Info(ctx, "mcp configuration requires interception; restart to change gateway mode")
		return s.initializeInterception()
	}

	// Otherwise, use proxy mode.
	if _, err := s.publishBackend(&backend{}); err != nil {
		return err
	}
	s.logger.Warn(ctx, "selected experimental reverse proxy routing; records interception lifecycle only; token/prompt/tool/model accounting and spend accrual are incomplete; Bedrock routing and actor-header injection are unsupported")
	return nil
}

// initializeInterception creates and publishes the server-owned request pool.
func (s *Server) initializeInterception() error {
	pool, err := NewCachedBridgePool(DefaultPoolOptions, nil, s.logger.Named("pool"), s.metrics, s.tracer)
	if err != nil {
		return xerrors.Errorf("create request pool: %w", err)
	}
	if _, err := s.publishBackend(&backend{pool: pool, keyPools: pool.KeyPools}); err != nil {
		_ = pool.Shutdown(context.Background())
		return err
	}
	return nil
}

// publishBackend linearizes backend publication with shutdown. Expensive
// candidate construction happens before this method so shutdown never waits on
// provider initialization or router construction.
func (s *Server) publishBackend(next *backend) (*backend, error) {
	s.backendPublishMu.Lock()
	defer s.backendPublishMu.Unlock()
	if s.shuttingDown.Load() {
		return nil, ErrShutdown
	}
	if err := s.lifecycleCtx.Err(); err != nil {
		return nil, s.Err()
	}
	return s.backend.Swap(next), nil
}

// Done returns a channel that is closed when the server lifecycle ends.
// It closes on explicit shutdown and on fatal connection-loop exit.
func (s *Server) Done() <-chan struct{} {
	return s.lifecycleCtx.Done()
}

// Err returns the reason the server lifecycle ended.
func (s *Server) Err() error {
	if cause := context.Cause(s.lifecycleCtx); cause != nil {
		return cause
	}
	return s.lifecycleCtx.Err()
}

// Client acquires a [DRPCClient], blocking until the daemon is connected to
// coderd, the server lifecycle ends, or ctx is canceled.
func (s *Server) Client(ctx context.Context) (DRPCClient, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.Done():
		if err := s.Err(); err != nil {
			return nil, err
		}
		return nil, xerrors.New("context closed")
	case client := <-s.clientCh:
		return client, nil
	}
}

// GetRequestHandler retrieves the selected gateway handler for the request.
func (s *Server) GetRequestHandler(ctx context.Context, req Request) (http.Handler, error) {
	if s.shuttingDown.Load() && s.lifecycleCtx.Err() == nil {
		return http.HandlerFunc(shuttingDownHandler), nil
	}
	current := s.backend.Load()
	if current == nil {
		return http.HandlerFunc(notReadyHandler), nil
	}
	if current.pool != nil {
		// Once lifecycle shutdown completes, preserve the pool's existing
		// post-shutdown error semantics instead of replacing them with a handler.
		if s.lifecycleCtx.Err() != nil {
			reqBridge, err := current.pool.Acquire(ctx, req, s.Client, NewMCPProxyFactory(s.logger, s.tracer, s.Client))
			if err != nil {
				return nil, xerrors.Errorf("acquire request bridge: %w", err)
			}
			return reqBridge, nil
		}
		// Admit before potentially expensive legacy bridge acquisition so
		// shutdown prevents new MCP bridge construction and can cancel an
		// acquisition that is still running when its deadline expires.
		release, ok := s.inflight.Admit()
		if !ok {
			return http.HandlerFunc(shuttingDownHandler), nil
		}
		acquireCtx, cleanup := s.inflight.RequestContext(ctx)
		reqBridge, err := current.pool.Acquire(acquireCtx, req, s.Client, NewMCPProxyFactory(s.logger, s.tracer, s.Client))
		cleanup()
		release()
		if err != nil {
			return nil, xerrors.Errorf("acquire request bridge: %w", err)
		}
		return s.inflight.Middleware(reqBridge), nil
	}
	// Proxy mode serves every request from one handler, which does not exist
	// until the first provider snapshot arrives.
	if current.proxyRouter == nil {
		return http.HandlerFunc(notReadyHandler), nil
	}
	return current.proxyRouter, nil
}

// Ready reports whether the server currently has an active DRPC connection to coderd.
func (s *Server) Ready() bool {
	return s.connected.Load()
}

// ReplaceProviders publishes a provider snapshot to the selected backend.
// Proxy requests retain their router across reloads, and a failed construction
// keeps the previous router serving. Returns an error if mode selection has
// not completed.
//
// A replaced router is not drained: in-flight requests hold it by reference
// and finish on the snapshot they started with. Shutdown drains them through
// the server's in-flight tracking, whichever snapshot they run on.
func (s *Server) ReplaceProviders(ctx context.Context, providers []aibridge.Provider) error {
	s.backendMu.Lock()
	defer s.backendMu.Unlock()
	if s.shuttingDown.Load() {
		return ErrShutdown
	}
	if err := s.lifecycleCtx.Err(); err != nil {
		return s.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	current := s.backend.Load()
	if current == nil {
		return xerrors.New("gateway mode not selected")
	}
	if current.pool != nil {
		s.backendPublishMu.Lock()
		if s.shuttingDown.Load() {
			s.backendPublishMu.Unlock()
			return ErrShutdown
		}
		s.backendPublishMu.Unlock()
		current.pool.ReplaceProviders(providers)
		return nil
	}
	router, err := proxy.NewRouter(providers, s.recorder, s.logger, s.metrics, s.tracer)
	if err != nil {
		return xerrors.Errorf("create proxy router: %w", err)
	}
	next := &backend{
		proxyRouter: s.inflight.Middleware(router),
		keyPools:    router.KeyPools,
		cleanup:     router.CloseIdleConnections,
	}
	previous, err := s.publishBackend(next)
	if err != nil {
		next.cleanup()
		return err
	}
	if previous != nil && previous.cleanup != nil {
		previous.cleanup()
	}
	return nil
}

// KeyPoolStateCollector reports key states from the current provider snapshot.
func (s *Server) KeyPoolStateCollector() prometheus.Collector {
	return keypool.NewStateCollector(s.keyPools)
}

func (s *Server) keyPools() []*keypool.Pool {
	current := s.backend.Load()
	if current == nil || current.keyPools == nil {
		return nil
	}
	return current.keyPools()
}

// isShutdown returns whether the Server is shutdown or not.
func (s *Server) isShutdown() bool {
	if s.shuttingDown.Load() {
		return true
	}
	select {
	case <-s.lifecycleCtx.Done():
		return true
	default:
		return false
	}
}

// Shutdown stops new requests and provider reloads, then waits for admitted
// requests to complete while the recorder connection remains available. If ctx
// expires, admitted request contexts are canceled before the connection closes.
func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	s.shutdownOnce.Do(func() {
		s.backendPublishMu.Lock()
		s.shuttingDown.Store(true)
		current := s.backend.Load()
		s.backendPublishMu.Unlock()

		drainErr := s.inflight.Shutdown(ctx)
		s.inflight.Close()
		if drainErr != nil {
			s.logger.Debug(ctx, "shutdown deadline passed, canceled in-flight requests", slog.Error(drainErr))
		}

		// Provider publication is already stopped and the current snapshot was
		// captured atomically with the shutdown flag. Request drainage never
		// waits for backendMu, which may be held by slow provider construction.

		var poolErr error
		if current != nil {
			if current.pool != nil {
				s.logger.Info(ctx, "shutting down request pool")
				poolErr = current.pool.Shutdown(ctx)
				if poolErr != nil {
					s.logger.Error(ctx, "request pool shutdown failed with error", slog.Error(poolErr))
				}
			}
			if current.cleanup != nil {
				current.cleanup()
			}
		}

		// Keep the DRPC client available through request drainage and resource
		// cleanup so end records can complete before terminating the connection.
		s.cancelFn(ErrShutdown)
		connectionDone := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(connectionDone)
		}()
		select {
		case <-connectionDone:
		case <-ctx.Done():
		}

		switch {
		case drainErr != nil:
			err = drainErr
		case ctx.Err() != nil:
			err = ctx.Err()
		case poolErr != nil:
			err = poolErr
		default:
			s.logger.Info(ctx, "gracefully shutdown")
		}
		if err != nil {
			s.logger.Warn(ctx, "graceful shutdown failed", slog.Error(err))
		}
	})
	return err
}

// Close shuts down the server with a timeout of 5s.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return s.Shutdown(ctx)
}

func notReadyHandler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "AI Gateway is starting up, retry shortly", http.StatusServiceUnavailable)
}

func shuttingDownHandler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "AI Gateway is shutting down", http.StatusServiceUnavailable)
}
