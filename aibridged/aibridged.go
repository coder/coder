package aibridged

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/retry"

	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/aibridge"
)

type Handler interface {
	http.Handler

	Shutdown(context.Context) error
	Close() error
}

// Server is the implementation which fulfills the DRPCServer interface.
// It is responsible for communication with the
type Server struct {
	clientDialer Dialer
	clientCh     chan DRPCClient

	requestBridgePool pooler

	logger slog.Logger
	wg     sync.WaitGroup

	// initConnectionCh will receive when the daemon connects to coderd for the
	// first time.
	initConnectionCh   chan struct{}
	initConnectionOnce sync.Once

	// closeContext is canceled when we start closing.
	closeContext context.Context
	closeCancel  context.CancelFunc
	closeOnce    sync.Once
	// closeError stores the error when closing to return to subsequent callers
	closeError error
	// closingB is set to true when we start closing
	closing      atomic.Bool
	shutdownOnce sync.Once
	// shuttingDownCh will receive when we start graceful shutdown
	shuttingDownCh chan struct{}
}

func New(rpcDialer Dialer, cfg aibridge.Config, logger slog.Logger) (*Server, error) {
	if rpcDialer == nil {
		return nil, xerrors.Errorf("nil rpcDialer given")
	}

	pool, err := NewCachedBridgePool(cfg, logger.Named("aibridge-pool")) // TODO: configurable size.
	if err != nil {
		return nil, xerrors.Errorf("create aibridge pool: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	daemon := &Server{
		logger:            logger,
		clientDialer:      rpcDialer,
		requestBridgePool: pool,
		clientCh:          make(chan DRPCClient),
		closeContext:      ctx,
		closeCancel:       cancel,
		initConnectionCh:  make(chan struct{}),
		shuttingDownCh:    make(chan struct{}),
	}

	daemon.wg.Add(1)
	go daemon.connect()

	return daemon, nil
}

// Connect establishes a connection to coderd.
func (s *Server) connect() {
	defer s.logger.Debug(s.closeContext, "connect loop exited")
	defer s.wg.Done()
	logConnect := s.logger.With(slog.F("context", "aibridged.server")).Debug
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
connectLoop:
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(s.closeContext); {
		// It's possible for the aibridge daemon to be shut down
		// before the wait is complete!
		if s.isClosed() {
			return
		}
		s.logger.Debug(s.closeContext, "dialing coderd")
		client, err := s.clientDialer(s.closeContext)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			var sdkErr *codersdk.Error
			// If something is wrong with our auth, stop trying to connect.
			if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusForbidden {
				s.logger.Error(s.closeContext, "not authorized to dial coderd", slog.Error(err))
				return
			}
			if s.isClosed() {
				return
			}
			s.logger.Warn(s.closeContext, "coderd client failed to dial", slog.Error(err))
			continue
		}

		// TODO: log this with INFO level when we implement external aibridge daemons.
		logConnect(s.closeContext, "successfully connected to coderd")
		retrier.Reset()
		s.initConnectionOnce.Do(func() {
			close(s.initConnectionCh)
		})

		// serve the client until we are closed or it disconnects
		for {
			select {
			case <-s.closeContext.Done():
				client.DRPCConn().Close()
				return
			case <-client.DRPCConn().Closed():
				logConnect(s.closeContext, "connection to coderd closed")
				continue connectLoop
			case s.clientCh <- client:
				continue
			}
		}
	}
}

func (s *Server) Client() (DRPCClient, error) {
	select {
	case <-s.closeContext.Done():
		return nil, xerrors.New("context closed")
	case <-s.shuttingDownCh:
		// Shutting down should return a nil client and unblock
		return nil, xerrors.New("shutting down")
	case client := <-s.clientCh:
		return client, nil
	}
}

// GetRequestHandler retrieves a (possibly reused) *aibridge.RequestBridge from the pool, for the given user.
func (s *Server) GetRequestHandler(ctx context.Context, req Request) (http.Handler, error) {
	if s.requestBridgePool == nil {
		return nil, xerrors.New("nil requestBridgePool")
	}

	reqBridge, err := s.requestBridgePool.Acquire(ctx, req, s.Client)
	if err != nil {
		return nil, xerrors.Errorf("acquire request bridge: %w", err)
	}

	return reqBridge, nil
}

// isClosed returns whether the API is closed or not.
func (s *Server) isClosed() bool {
	select {
	case <-s.closeContext.Done():
		return true
	default:
		return false
	}
}

// closeWithError closes aibridged once; subsequent calls will return the error err.
func (s *Server) closeWithError(err error) error {
	s.closing.Store(true)
	s.closeOnce.Do(func() {
		s.closeCancel()
		s.logger.Debug(context.Background(), "waiting for goroutines to exit")
		s.wg.Wait()
		s.logger.Debug(context.Background(), "closing server with error", slog.Error(err))
		s.closeError = err
	})

	return s.closeError
}

// Close ends the aibridge daemon.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}

	s.logger.Info(s.closeContext, "closing aibridged")
	return s.closeWithError(nil)
}

// Shutdown waits for all exiting in-flight requests to complete, or the context to expire, whichever comes first.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}

	var err error
	s.shutdownOnce.Do(func() {
		close(s.shuttingDownCh)

		select {
		case <-ctx.Done():
			s.logger.Warn(ctx, "graceful shutdown failed", slog.Error(ctx.Err()))
			err = ctx.Err()
			return
		default:
		}

		s.logger.Info(ctx, "shutting down aibridged pool")
		if err = s.requestBridgePool.Shutdown(ctx); err != nil && errors.Is(err, http.ErrServerClosed) {
			s.logger.Error(ctx, "shutdown failed with error", slog.Error(err))
			return
		}

		s.logger.Info(ctx, "gracefully shutdown")
	})
	return err
}

var DefaultHandler Handler = &NoopHandler{}

var _ Handler = &NoopHandler{}

type NoopHandler struct{}

func (*NoopHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "no aibridged server", http.StatusBadGateway)
}

func (*NoopHandler) Shutdown(context.Context) error {
	return nil
}

func (*NoopHandler) Close() error {
	return nil
}
