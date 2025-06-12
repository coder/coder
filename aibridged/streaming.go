package aibridged

import (
	"context"
	"net/http"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type SSEStream[T any] struct {
	logger     slog.Logger
	eventsChan <-chan T
}

var ErrDone = xerrors.New("done")

func NewSSEStream[T any](eventsChan <-chan T, logger slog.Logger) *SSEStream[T] {
	return &SSEStream[T]{eventsChan: eventsChan, logger: logger}
}

func (s *SSEStream[T]) transmit(ctx context.Context, done chan struct{}, rw http.ResponseWriter, r *http.Request) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	sseSendEvent, sseSenderClosed, err := httpapi.ServerSentEventSender(rw, r)
	if err != nil {
		return xerrors.Errorf("failed to create sse transmitter: %w", err)
	}

	defer func() {
		// Block returning until the ServerSentEventSender is closed
		// to avoid a race condition where we might write or flush to rw after the handler returns.
		select {
		case <-sseSenderClosed:
		case <-done:
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return ErrDone
		case <-sseSenderClosed:
			return xerrors.New("SSE target closed")
		case event, ok := <-s.eventsChan:
			if !ok {
				return xerrors.New("SSE source closed")
			}

			err = sseSendEvent(codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeData,
				Data: event,
			})
			if err != nil {
				// TODO: handle error.
				continue
			}
		}
	}
}
