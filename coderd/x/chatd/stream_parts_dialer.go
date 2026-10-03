package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
)

// LocalStreamPartsDialerConfig configures an in-process stream parts dialer.
type LocalStreamPartsDialerConfig struct {
	Buffer *messagepartbuffer.Buffer
	Logger slog.Logger
}

// NewLocalStreamPartsDialer returns a dialer that streams message parts through
// in-process channels while using the same stream serving loop as WebSockets.
func NewLocalStreamPartsDialer(cfg LocalStreamPartsDialerConfig) StreamPartsDialer {
	return func(ctx context.Context, input StreamPartsDialInput) (StreamPartsSession, error) {
		if cfg.Buffer == nil {
			return nil, xerrors.New("message part buffer is not configured")
		}
		serverTransport, clientTransport := newStreamPartsChannelTransportPair()
		logger := cfg.Logger.Named("chat_stream_parts").With(slog.F("chat_id", input.ChatID))
		endpoint := streamPartsEndpoint{
			chatID: input.ChatID,
			buffer: cfg.Buffer,
			logger: logger,
		}
		serveCtx, cancel := context.WithCancel(ctx)
		go func() {
			defer cancel()
			defer func() {
				_ = serverTransport.Close()
			}()
			if err := endpoint.serve(serveCtx, serverTransport); err != nil && !streamPartsExpectedTransportClose(err) {
				logger.Debug(serveCtx, "chat stream parts closed", slog.Error(err))
			}
		}()
		return newStreamPartsTransportSession(serveCtx, clientTransport), nil
	}
}

func streamPartsDialerForServer(workerID uuid.UUID, local StreamPartsDialer, remote StreamPartsDialer) StreamPartsDialer {
	return func(ctx context.Context, input StreamPartsDialInput) (StreamPartsSession, error) {
		if local == nil && remote == nil {
			return nil, xerrors.New("stream parts dialer is not configured")
		}
		if remote == nil || input.WorkerID == uuid.Nil || input.WorkerID == workerID {
			if local == nil {
				return nil, xerrors.New("local stream parts dialer is not configured")
			}
			return local(ctx, input)
		}
		return remote(ctx, input)
	}
}
