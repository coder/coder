package aibridged

import (
	"bytes"
	"context"
	"encoding/json"
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

				// TODO: use logger, make configurable.
				//_, _ = fmt.Fprintf(os.Stderr, "[%s] 	%s", sessionID, payload)
				_, _ = os.Stderr.Write(payload)

				_, err := w.Write(payload)
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

type eventStream struct {
	eventsCh chan []byte
	kind     eventStreamProvider

	closedOnce sync.Once
	closedCh   chan any
}

type eventStreamProvider string

const (
	openAIEventStream    eventStreamProvider = "openai"
	anthropicEventStream eventStreamProvider = "anthropic"
)

func newEventStream(kind eventStreamProvider) *eventStream {
	return &eventStream{
		kind:     kind,
		eventsCh: make(chan []byte),
		closedCh: make(chan any),
	}
}

func (s *eventStream) Events() <-chan []byte {
	return s.eventsCh
}

func (s *eventStream) Closed() <-chan any {
	return s.closedCh
}

func (s *eventStream) TrySend(ctx context.Context, data any, exclusions ...string) error {
	// Save an unnecessary marshaling if possible.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closedCh:
		return xerrors.New("closed")
	default:
	}

	var (
		payload []byte
		err     error
	)
	switch s.kind {
	case openAIEventStream:
		// https://github.com/openai/openai-go#request-fields
		// I noticed that Cursor would bork if it received streaming response payloads which had zero values.
		// I'm not sure if this is a Cursor-specific issue or more widespread, but I've vibed a marshaler which will filter
		// out all the zero value objects in the response, with optional exclusions.
		payload, err = util.MarshalNoZero(data, exclusions...)
	default:
		zero, _ := util.MarshalNoZero(data, exclusions...)
		fmt.Printf("[zero] %s\n", zero)

		payload, err = json.Marshal(data)
	}

	if err != nil {
		return xerrors.Errorf("marshal payload: %w", err)
	}

	return s.send(ctx, payload)
}

func (s *eventStream) send(ctx context.Context, payload []byte) error {
	switch s.kind {
	case openAIEventStream:
		var buf bytes.Buffer
		buf.WriteString("data: ")
		buf.Write(payload)
		buf.WriteString("\n\n")
		payload = buf.Bytes()
	case anthropicEventStream:
		// TODO: improve this approach.
		type msgType struct {
			Val string `json:"type"`
		}
		var typ msgType
		if err := json.NewDecoder(bytes.NewBuffer(payload)).Decode(&typ); err != nil {
			return xerrors.Errorf("failed to determine anthropic event type for %q: %w", payload, err)
		}

		var buf bytes.Buffer
		buf.WriteString("event: ")
		buf.WriteString(typ.Val)
		buf.WriteString("\n")
		buf.WriteString("data: ")
		buf.Write(payload)
		buf.WriteString("\n\n")
		payload = buf.Bytes()
	default:
		return xerrors.Errorf("unknown stream kind: %q", s.kind)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closedCh:
		return xerrors.New("closed")
	case s.eventsCh <- payload:
		return nil
	}
}

func (s *eventStream) Close(ctx context.Context) error {
	var out error
	s.closedOnce.Do(func() {
		switch s.kind {
		case openAIEventStream:
			err := s.send(ctx, []byte("[DONE]"))
			if err != nil {
				out = xerrors.Errorf("close stream: %w", err)
			}
		}

		close(s.closedCh)
		close(s.eventsCh)
	})

	return out
}
