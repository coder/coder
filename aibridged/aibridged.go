package aibridged

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/valyala/fasthttp/fasthttputil"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/retry"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
)

type Dialer func(ctx context.Context) (proto.DRPCAIBridgeDaemonClient, error)

type Server struct {
	clientDialer Dialer
	clientCh     chan proto.DRPCAIBridgeDaemonClient

	logger slog.Logger
	wg     sync.WaitGroup

	// initConnectionCh will receive when the daemon connects to coderd for the
	// first time.
	initConnectionCh   chan struct{}
	initConnectionOnce sync.Once

	// mutex protects all subsequent fields
	mutex sync.Mutex
	// closeContext is canceled when we start closing.
	closeContext context.Context
	closeCancel  context.CancelFunc
	// closeError stores the error when closing to return to subsequent callers
	closeError error
	// closingB is set to true when we start closing
	closingB bool
	// closedCh will receive when we complete closing
	closedCh chan struct{}
	// shuttingDownB is set to true when we start graceful shutdown
	shuttingDownB bool
	// shuttingDownCh will receive when we start graceful shutdown
	shuttingDownCh chan struct{}
}

var _ proto.DRPCAIBridgeDaemonServer = &Server{}

func New(rpcDialer Dialer, httpAddr string, logger slog.Logger) (*Server, error) {
	if rpcDialer == nil {
		return nil, xerrors.Errorf("nil rpcDialer given")
	}

	ctx, cancel := context.WithCancel(context.Background())
	daemon := &Server{
		logger:           logger,
		clientDialer:     rpcDialer,
		clientCh:         make(chan proto.DRPCAIBridgeDaemonClient),
		closeContext:     ctx,
		closeCancel:      cancel,
		closedCh:         make(chan struct{}),
		shuttingDownCh:   make(chan struct{}),
		initConnectionCh: make(chan struct{}),
	}

	daemon.wg.Add(1)
	go daemon.connect()

	return daemon, nil
}

// Connect establishes a connection to coderd.
func (s *Server) connect() {
	defer s.logger.Debug(s.closeContext, "connect loop exited")
	defer s.wg.Done()
	logConnect := s.logger.Debug
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
connectLoop:
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(s.closeContext); {
		// TODO(dannyk): handle premature close.
		//// It's possible for the provisioner daemon to be shut down
		//// before the wait is complete!
		// if s.isClosed() {
		//	return
		//}

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

		// This log is useful to verify that an external provisioner daemon is
		// successfully connecting to coderd. It doesn't add much value if the
		// daemon is built-in, so we only log it on the info level if p.externalProvisioner
		// is true. This log message is mentioned in the docs:
		// https://github.com/coder/coder/blob/5bd86cb1c06561d1d3e90ce689da220467e525c0/docs/admin/provisioners.md#L346
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

func (s *Server) Client() (proto.DRPCAIBridgeDaemonClient, bool) {
	select {
	case <-s.closeContext.Done():
		return nil, false
	case <-s.shuttingDownCh:
		// Shutting down should return a nil client and unblock
		return nil, false
	case client := <-s.clientCh:
		return client, true
	}
}

func (s *Server) TrackTokenUsage(ctx context.Context, in *proto.TrackTokenUsageRequest) (*proto.TrackTokenUsageResponse, error) {
	out, err := clientDoWithRetries(ctx, s.Client, func(ctx context.Context, client proto.DRPCAIBridgeDaemonClient) (*proto.TrackTokenUsageResponse, error) {
		return client.TrackTokenUsage(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) TrackUserPrompt(ctx context.Context, in *proto.TrackUserPromptRequest) (*proto.TrackUserPromptResponse, error) {
	out, err := clientDoWithRetries(ctx, s.Client, func(ctx context.Context, client proto.DRPCAIBridgeDaemonClient) (*proto.TrackUserPromptResponse, error) {
		return client.TrackUserPrompt(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) TrackToolUsage(ctx context.Context, in *proto.TrackToolUsageRequest) (*proto.TrackToolUsageResponse, error) {
	out, err := clientDoWithRetries(ctx, s.Client, func(ctx context.Context, client proto.DRPCAIBridgeDaemonClient) (*proto.TrackToolUsageResponse, error) {
		return client.TrackToolUsage(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TODO: direct copy/paste from provisionerd, abstract into common util.
func retryable(err error) bool {
	return xerrors.Is(err, yamux.ErrSessionShutdown) || xerrors.Is(err, io.EOF) || xerrors.Is(err, fasthttputil.ErrInmemoryListenerClosed) ||
		// annoyingly, dRPC sometimes returns context.Canceled if the transport was closed, even if the context for
		// the RPC *is not canceled*.  Retrying is fine if the RPC context is not canceled.
		xerrors.Is(err, context.Canceled)
}

// clientDoWithRetries runs the function f with a client, and retries with
// backoff until either the error returned is not retryable() or the context
// expires.
// TODO: direct copy/paste from provisionerd, abstract into common util.
func clientDoWithRetries[T any](ctx context.Context,
	getClient func() (proto.DRPCAIBridgeDaemonClient, bool),
	f func(context.Context, proto.DRPCAIBridgeDaemonClient) (T, error),
) (ret T, _ error) {
	for retrier := retry.New(25*time.Millisecond, 5*time.Second); retrier.Wait(ctx); {
		client, ok := getClient()
		if !ok {
			continue
		}
		resp, err := f(ctx, client)
		if retryable(err) {
			continue
		}
		return resp, err
	}
	return ret, ctx.Err()
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

// closeWithError closes the provisioner; subsequent reads/writes will return the error err.
func (s *Server) closeWithError(err error) error {
	s.mutex.Lock()
	first := false
	if !s.closingB {
		first = true
		s.closingB = true
	}
	// don't hold the mutex while doing I/O.
	s.mutex.Unlock()

	if first {
		s.closeCancel()
		s.logger.Debug(context.Background(), "waiting for goroutines to exit")
		s.wg.Wait()
		s.logger.Debug(context.Background(), "closing server with error", slog.Error(err))
		s.closeError = err
		close(s.closedCh)
		return err
	}
	s.logger.Debug(s.closeContext, "waiting for first closer to complete")
	<-s.closedCh
	s.logger.Debug(s.closeContext, "first closer completed")
	return s.closeError
}

// Close ends the aibridge daemon.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}

	s.logger.Info(s.closeContext, "closing aibridged")
	// TODO: invalidate all running requests (canceling context should be enough?).
	errMsg := "aibridged closed gracefully"
	err := s.closeWithError(nil)
	if err != nil {
		errMsg = err.Error()
	}
	s.logger.Warn(s.closeContext, errMsg)

	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	// TODO: implement or remove.
	return nil
}
