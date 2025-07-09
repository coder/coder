package aibridged

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged/util"
)

// isConnectionError checks if an error is related to client disconnection
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, io.EOF) {
		return true
	}

	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}

	errStr := err.Error()
	return strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "use of closed network connection")
}

// BasicSSESender was implemented to overcome httpapi.ServerSentEventSender's odd design choices. For example, it doesn't
// write "event: data" for every data event (it's unnecessary, and breaks some AI tools' parsing of the SSE stream).
func BasicSSESender(outerCtx context.Context, sessionID uuid.UUID, model string, stream EventStreamer, logger slog.Logger) http.HandlerFunc {
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
				fmt.Printf("request done for model %s, reason: %q\n", model, ctx.Err())
				return
			case <-stream.Closed():
				return
			case ev, ok := <-stream.Events():
				if !ok {
					return
				}


				// TODO: https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/implement-tool-use#example-of-successful-tool-result see "Important formatting requirements"




				// TODO: use logger, make configurable.
				//_, _ = fmt.Fprintf(os.Stderr, "[%s] 	%s", sessionID, payload)
				_, _ = os.Stderr.Write([]byte(fmt.Sprintf("[orig] %s\n[zero] %s\n[out] %s", ev.orig, ev.zero, ev.payload)))

				_, err := w.Write(ev.payload)
				if err != nil {
					if isConnectionError(err) {
						logger.Debug(ctx, "client disconnected during SSE write", slog.Error(err))
					} else {
						logger.Error(ctx, "failed to write SSE event", slog.Error(err))
					}
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
	TrySend(ctx context.Context, data any, input string, exclusions ...string) error
	Events() <-chan event
	Close(ctx context.Context) error
	Closed() <-chan any
}

type event struct {
	payload []byte
	zero    []byte // Marshaling with zero-value elements omitted.
	orig    string
}

type eventStream struct {
	eventsCh chan event
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
		eventsCh: make(chan event),
		closedCh: make(chan any),
	}
}

func (s *eventStream) Events() <-chan event {
	return s.eventsCh
}

func (s *eventStream) Closed() <-chan any {
	return s.closedCh
}

func (s *eventStream) TrySend(ctx context.Context, data any, input string, exclusions ...string) error {
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
		payload, err = json.Marshal(data)
	}

	zero, _ := util.MarshalNoZero(data, exclusions...)

	if err != nil {
		return xerrors.Errorf("marshal payload: %w", err)
	}

	return s.send(ctx, payload, zero, input)
}

func (s *eventStream) send(ctx context.Context, payload, zero []byte, input string) error {
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
	case s.eventsCh <- event{payload: payload, orig: input, zero: zero}:
		return nil
	}
}

func (s *eventStream) Close(ctx context.Context) error {
	var out error
	s.closedOnce.Do(func() {
		switch s.kind {
		case openAIEventStream:
			err := s.send(ctx, []byte("[DONE]"), nil, "")
			if err != nil {
				out = xerrors.Errorf("close stream: %w", err)
			}
		}

		close(s.closedCh)
		close(s.eventsCh)
	})

	return out
}
