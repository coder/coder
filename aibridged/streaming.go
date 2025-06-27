package aibridged

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged/util"
)

// BasicSSESender was implemented to overcome httpapi.ServerSentEventSender's odd design choices. For example, it doesn't
// write "event: data" for every data event (it's unnecessary, and breaks some AI tools' parsing of the SSE stream).
func BasicSSESender(outerCtx context.Context, sessionID uuid.UUID, stream EventStreamer, logger slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Send initial flush to ensure connection is established.
		flush(w)

		for {
			select {
			case <-outerCtx.Done():
				return
			case <-ctx.Done():
				return
			case <-stream.Closed():
				return
			case payload, ok := <-stream.Events():
				if !ok {
					return
				}

				var buf bytes.Buffer

				buf.Write([]byte("data: "))
				buf.Write(payload)
				buf.Write([]byte("\n\n"))

				// TODO: use logger, make configurable.
				_, _ = fmt.Fprintf(os.Stderr, "[%s] 	%s", sessionID, buf.Bytes())

				_, err := w.Write(buf.Bytes())
				if err != nil {
					logger.Error(ctx, "failed to write SSE event", slog.Error(err))
					return
				}
				flush(w)
			}
		}
	}
}

func flush(w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	if flusher == nil {
		return
	}

	flusher.Flush()
}

type EventStreamer interface {
	TrySend(ctx context.Context, data any, exclusions ...string) error
	Events() <-chan []byte
	Close(ctx context.Context) error
	Closed() <-chan any
}

type openAIEventStream struct {
	eventsCh chan []byte

	closedOnce sync.Once
	closedCh   chan any
}

func newOpenAIEventStream() *openAIEventStream {
	return &openAIEventStream{
		eventsCh: make(chan []byte),
		closedCh: make(chan any),
	}
}

func (s *openAIEventStream) Events() <-chan []byte {
	return s.eventsCh
}

func (s *openAIEventStream) Closed() <-chan any {
	return s.closedCh
}

func (s *openAIEventStream) TrySend(ctx context.Context, data any, exclusions ...string) error {
	// Save an unnecessary marshaling if possible.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closedCh:
		return xerrors.New("closed")
	default:
	}

	payload, err := util.MarshalNoZero(data, exclusions...)
	if err != nil {
		return xerrors.Errorf("marshal payload: %w", err)
	}

	return s.send(ctx, payload)
}

func (s *openAIEventStream) send(ctx context.Context, payload []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closedCh:
		return xerrors.New("closed")
	case s.eventsCh <- payload:
		return nil
	}
}

func (s *openAIEventStream) Close(ctx context.Context) error {
	var out error
	s.closedOnce.Do(func() {
		err := s.send(ctx, []byte("[DONE]")) // TODO: OpenAI-specific?
		if err != nil {
			out = xerrors.Errorf("close stream: %w", err)
		}

		close(s.closedCh)
		close(s.eventsCh)
	})

	return out
}
