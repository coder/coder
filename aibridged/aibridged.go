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

	bridge *Bridge
}

var _ proto.DRPCAIBridgeDaemonServer = &Server{}

func New(rpcDialer Dialer, httpAddr string, logger slog.Logger, bridgeCfg codersdk.AIBridgeConfig) (*Server, error) {
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

	// TODO: improve error handling here; if this fails it prevents the whole server from starting up!

	bridge, err := NewBridge(bridgeCfg, httpAddr, logger.Named("ai_bridge"), daemon.client)
	if err != nil {
		return nil, xerrors.Errorf("create new bridge server: %w", err)
	}

	daemon.bridge = bridge

	daemon.wg.Add(1)
	go daemon.connect()
	go func() {
		err := bridge.Serve()
		// TODO: better error handling.
		// TODO: close on shutdown.
		logger.Error(ctx, "bridge server stopped", slog.Error(err))
	}()

	return daemon, nil
} // Connect establishes a connection to coderd.
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

func (s *Server) client() (proto.DRPCAIBridgeDaemonClient, bool) {
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

func (s *Server) AuditPrompt(ctx context.Context, in *proto.AuditPromptRequest) (*proto.AuditPromptResponse, error) {
	out, err := clientDoWithRetries(ctx, s.client, func(ctx context.Context, client proto.DRPCAIBridgeDaemonClient) (*proto.AuditPromptResponse, error) {
		return client.AuditPrompt(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) TrackTokenUsage(ctx context.Context, in *proto.TrackTokenUsageRequest) (*proto.TrackTokenUsageResponse, error) {
	out, err := clientDoWithRetries(ctx, s.client, func(ctx context.Context, client proto.DRPCAIBridgeDaemonClient) (*proto.TrackTokenUsageResponse, error) {
		return client.TrackTokenUsage(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) TrackUserPrompts(ctx context.Context, in *proto.TrackUserPromptsRequest) (*proto.TrackUserPromptsResponse, error) {
	out, err := clientDoWithRetries(ctx, s.client, func(ctx context.Context, client proto.DRPCAIBridgeDaemonClient) (*proto.TrackUserPromptsResponse, error) {
		return client.TrackUserPrompts(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) TrackToolUse(ctx context.Context, in *proto.TrackToolUseRequest) (*proto.TrackToolUseResponse, error) {
	out, err := clientDoWithRetries(ctx, s.client, func(ctx context.Context, client proto.DRPCAIBridgeDaemonClient) (*proto.TrackToolUseResponse, error) {
		return client.TrackToolUse(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// func (s *Server) ChatCompletions(payload *proto.JSONPayload, stream proto.DRPCOpenAIService_ChatCompletionsStream) error {
//	// TODO: call OpenAI API.
//
//	select {
//	case <-stream.Context().Done():
//		return nil
//	default:
//	}
//
//	err := stream.Send(&proto.JSONPayload{
//		Content: `
//{
//  "id": "chatcmpl-B9MBs8CjcvOU2jLn4n570S5qMJKcT",
//  "object": "chat.completion",
//  "created": 1741569952,
//  "model": "gpt-4.1-2025-04-14",
//  "choices": [
//    {
//      "index": 0,
//      "message": {
//        "role": "assistant",
//        "content": "Hello! How can I assist you today?",
//        "refusal": null,
//        "annotations": []
//      },
//      "logprobs": null,
//      "finish_reason": "stop"
//    }
//  ],
//  "usage": {
//    "prompt_tokens": 19,
//    "completion_tokens": 10,
//    "total_tokens": 29,
//    "prompt_tokens_details": {
//      "cached_tokens": 0,
//      "audio_tokens": 0
//    },
//    "completion_tokens_details": {
//      "reasoning_tokens": 0,
//      "audio_tokens": 0,
//      "accepted_prediction_tokens": 0,
//      "rejected_prediction_tokens": 0
//    }
//  },
//  "service_tier": "default"
//}
// `})
//	if err != nil {
//		return xerrors.Errorf("stream chat completion response: %w", err)
//	}
//	return nil
//}

func (s *Server) BridgeAddr() string {
	return s.bridge.Addr()
}

func (s *Server) BridgeErr() error {
	return s.bridge.lastErr
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
