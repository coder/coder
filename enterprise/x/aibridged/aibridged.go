package aibridged

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

// Server provides the AI Bridge functionality.
// It is responsible for:
//   - receiving requests on /api/experimental/aibridged/* // TODO: update endpoint once out of experimental
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

func New(ctx context.Context, pool Pooler, rpcDialer Dialer, logger slog.Logger) (*Server, error) {
	if rpcDialer == nil {
		return nil, xerrors.Errorf("nil rpcDialer given")
	}

	ctx, cancel := context.WithCancel(ctx)
	daemon := &Server{
		logger:            logger,
		clientDialer:      rpcDialer,
		requestBridgePool: pool,
		clientCh:          make(chan DRPCClient),
		lifecycleCtx:      ctx,
		cancelFn:          cancel,
		initConnectionCh:  make(chan struct{}),
	}

	daemon.wg.Add(1)
	go daemon.connect()

	return daemon, nil
}

// Connect establishes a connection to coderd.
func (d *Server) connect() {
	defer d.logger.Debug(d.lifecycleCtx, "connect loop exited")
	defer d.wg.Done()

	logConnect := d.logger.With(slog.F("context", "aibridged.server")).Debug
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
connectLoop:
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(d.lifecycleCtx); {
		// It's possible for the aibridge daemon to be shut down
		// before the wait is complete!
		if d.isShutdown() {
			return
		}
		d.logger.Debug(d.lifecycleCtx, "dialing coderd")
		client, err := d.clientDialer(d.lifecycleCtx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			var sdkErr *codersdk.Error
			// If something is wrong with our auth, stop trying to connect.
			if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusForbidden {
				d.logger.Error(d.lifecycleCtx, "not authorized to dial coderd", slog.Error(err))
				return
			}
			if d.isShutdown() {
				return
			}
			d.logger.Warn(d.lifecycleCtx, "coderd client failed to dial", slog.Error(err))
			continue
		}

		// TODO: log this with INFO level when we implement external aibridge daemons.
		logConnect(d.lifecycleCtx, "successfully connected to coderd")
		retrier.Reset()
		d.initConnectionOnce.Do(func() {
			close(d.initConnectionCh)
		})

		// Serve the client until we are closed or it disconnects.
		for {
			select {
			case <-d.lifecycleCtx.Done():
				client.DRPCConn().Close()
				return
			case <-client.DRPCConn().Closed():
				logConnect(d.lifecycleCtx, "connection to coderd closed")
				continue connectLoop
			case d.clientCh <- client:
				continue
			}
		}
	}
}

func (d *Server) Client() (DRPCClient, error) {
	select {
	case <-d.lifecycleCtx.Done():
		return nil, xerrors.New("context closed")
	case client := <-d.clientCh:
		return client, nil
	}
}

// GetRequestHandler retrieves a (possibly reused) [*aibridge.RequestBridge] from the pool, for the given user.
func (d *Server) GetRequestHandler(ctx context.Context, req Request) (http.Handler, error) {
	if d.requestBridgePool == nil {
		return nil, xerrors.New("nil requestBridgePool")
	}

	reqBridge, err := d.requestBridgePool.Acquire(ctx, req, d.Client)
	if err != nil {
		return nil, xerrors.Errorf("acquire request bridge: %w", err)
	}

	return reqBridge, nil
}

// isShutdown returns whether the Server is shutdown or not.
func (d *Server) isShutdown() bool {
	select {
	case <-d.lifecycleCtx.Done():
		return true
	default:
		return false
	}
}

// Shutdown waits for all exiting in-flight requests to complete, or the context to expire, whichever comes first.
func (d *Server) Shutdown(ctx context.Context) error {
	var err error
	d.shutdownOnce.Do(func() {
		d.cancelFn()

		// Wait for any outstanding connections to terminate.
		d.wg.Wait()

		select {
		case <-ctx.Done():
			d.logger.Warn(ctx, "graceful shutdown failed", slog.Error(ctx.Err()))
			err = ctx.Err()
			return
		default:
		}

		d.logger.Info(ctx, "shutting down request pool")
		if err = d.requestBridgePool.Shutdown(ctx); err != nil {
			d.logger.Error(ctx, "request pool shutdown failed with error", slog.Error(err))
		}

		d.logger.Info(ctx, "gracefully shutdown")
	})
	return err
}
