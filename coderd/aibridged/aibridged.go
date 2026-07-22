package aibridged

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

var (
	_ io.Closer = &Server{}

	ErrShutdown = xerrors.New("aibridged server shutdown")
)

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

	logger slog.Logger
	tracer trace.Tracer
	wg     sync.WaitGroup

	// connected tracks whether the DRPC connection to coderd is currently active.
	connected atomic.Bool

	// lifecycleCtx is canceled when we start closing or when the
	// connection loop exits permanently.
	lifecycleCtx context.Context
	// cancelFn closes the lifecycleCtx with the reason it closed.
	cancelFn context.CancelCauseFunc

	shutdownOnce sync.Once
}

func New(ctx context.Context, pool Pooler, rpcDialer Dialer, logger slog.Logger, tracer trace.Tracer) (*Server, error) {
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

		requestBridgePool: pool,
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
		if s.lifecycleCtx.Err() == nil {
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
				case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
					err = xerrors.Errorf("dial coderd: %w", err)
					s.logger.Error(s.lifecycleCtx, "fatal error dialing coderd", slog.Error(err))
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

func (s *Server) Client() (DRPCClient, error) {
	return s.ClientContext(context.Background())
}

func (s *Server) ClientContext(ctx context.Context) (DRPCClient, error) {
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

// GetRequestHandler retrieves a (possibly reused) [*aibridge.RequestBridge] from the pool, for the given user.
func (s *Server) GetRequestHandler(ctx context.Context, req Request) (http.Handler, error) {
	if s.requestBridgePool == nil {
		return nil, xerrors.New("nil requestBridgePool")
	}

	reqBridge, err := s.requestBridgePool.Acquire(ctx, req, s.Client, NewMCPProxyFactory(s.logger, s.tracer, s.Client))
	if err != nil {
		return nil, xerrors.Errorf("acquire request bridge: %w", err)
	}

	return reqBridge, nil
}

// Ready reports whether the server currently has an active DRPC connection to coderd.
func (s *Server) Ready() bool {
	return s.connected.Load()
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
		s.cancelFn(ErrShutdown)

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
