package aibridged

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
	"github.com/prometheus/client_golang/prometheus"
)

var _ io.Closer = &Server{}

// Server provides the AI Bridge functionality.
// It is responsible for:
//   - receiving requests on /api/v2/aibridged/*
//   - manipulating the requests
//   - relaying requests to upstream AI services and relaying responses to caller
//
// It requires a [Dialer] to provide a [DRPCClient] implementation to
// communicate with a [DRPCServer] implementation, to persist state and perform other functions.
type Server struct {
	clientDialer Dialer
	clientCh     chan DRPCClient

	// A pool of [aibridge.RequestBridge] instances, which service incoming requests.
	requestBridgePool Pooler
	metrics           *aibridge.Metrics

	logger slog.Logger
	wg     sync.WaitGroup

	// initConnectionCh will receive when the daemon connects to coderd for the
	// first time.
	initConnectionCh   chan struct{}
	initConnectionOnce sync.Once

	// lifecycleCtx is canceled when we start closing.
	lifecycleCtx context.Context
	// cancelFn closes the lifecycleCtx.
	cancelFn func()

	shutdownOnce sync.Once
}

func New(ctx context.Context, pool Pooler, rpcDialer Dialer, reg prometheus.Registerer, logger slog.Logger) (*Server, error) {
	if rpcDialer == nil {
		return nil, xerrors.Errorf("nil rpcDialer given")
	}

	ctx, cancel := context.WithCancel(ctx)
	daemon := &Server{
		logger:           logger,
		clientDialer:     rpcDialer,
		clientCh:         make(chan DRPCClient),
		lifecycleCtx:     ctx,
		cancelFn:         cancel,
		initConnectionCh: make(chan struct{}),

		requestBridgePool: pool,
		metrics:           aibridge.NewMetrics(reg),
	}

	daemon.wg.Add(1)
	go daemon.connect()

	return daemon, nil
}

// Connect establishes a connection to coderd.
func (s *Server) connect() {
	defer s.logger.Debug(s.lifecycleCtx, "connect loop exited")
	defer s.wg.Done()

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
				return
			}
			var sdkErr *codersdk.Error
			// If something is wrong with our auth, stop trying to connect.
			if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusForbidden {
				s.logger.Error(s.lifecycleCtx, "not authorized to dial coderd", slog.Error(err))
				return
			}
			if s.isShutdown() {
				return
			}
			s.logger.Warn(s.lifecycleCtx, "coderd client failed to dial", slog.Error(err))
			continue
		}

		// TODO: log this with INFO level when we implement external aibridge daemons.
		logConnect(s.lifecycleCtx, "successfully connected to coderd")
		retrier.Reset()
		s.initConnectionOnce.Do(func() {
			close(s.initConnectionCh)
		})

		// Serve the client until we are closed or it disconnects.
		for {
			select {
			case <-s.lifecycleCtx.Done():
				client.DRPCConn().Close()
				return
			case <-client.DRPCConn().Closed():
				logConnect(s.lifecycleCtx, "connection to coderd closed")
				continue connectLoop
			case s.clientCh <- client:
				continue
			}
		}
	}
}

func (s *Server) Client() (DRPCClient, error) {
	select {
	case <-s.lifecycleCtx.Done():
		return nil, xerrors.New("context closed")
	case client := <-s.clientCh:
		return client, nil
	}
}

// GetRequestHandler retrieves a (possibly reused) [*aibridge.RequestBridge] from the pool, for the given user.
func (s *Server) GetRequestHandler(ctx context.Context, req Request) (http.Handler, error) {
	if s.requestBridgePool == nil {
		return nil, xerrors.New("nil requestBridgePool")
	}

	reqBridge, err := s.requestBridgePool.Acquire(ctx, req, s.Client, NewMCPProxyFactory(s.logger, s.Client), s.metrics)
	if err != nil {
		return nil, xerrors.Errorf("acquire request bridge: %w", err)
	}

	return reqBridge, nil
}

// isShutdown returns whether the Server is shutdown or not.
func (s *Server) isShutdown() bool {
	select {
	case <-s.lifecycleCtx.Done():
		return true
	default:
		return false
	}
}

// Shutdown waits for all exiting in-flight requests to complete, or the context to expire, whichever comes first.
func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	s.shutdownOnce.Do(func() {
		s.cancelFn()

		// Wait for any outstanding connections to terminate.
		s.wg.Wait()

		select {
		case <-ctx.Done():
			s.logger.Warn(ctx, "graceful shutdown failed", slog.Error(ctx.Err()))
			err = ctx.Err()
			return
		default:
		}

		s.logger.Info(ctx, "shutting down request pool")
		if err = s.requestBridgePool.Shutdown(ctx); err != nil {
			s.logger.Error(ctx, "request pool shutdown failed with error", slog.Error(err))
		}

		s.logger.Info(ctx, "gracefully shutdown")
	})
	return err
}

// Close shuts down the server with a timeout of 5s.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return s.Shutdown(ctx)
}
