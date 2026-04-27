package eventstream

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

var ErrEventStreamClosed = xerrors.New("event stream closed")

const (
	pingInterval = time.Second * 10
	// SlowFlushThreshold is the duration after which a flush to the client is
	// considered slow and a warning is logged.
	SlowFlushThreshold = time.Millisecond * 500
)

type event []byte

type EventStream struct {
	ctx    context.Context
	logger slog.Logger
	clk    quartz.Clock

	pingPayload []byte

	initiated    atomic.Bool
	initiateOnce sync.Once

	shutdownOnce sync.Once
	eventsCh     chan event

	// doneCh is closed when the start loop exits.
	doneCh chan struct{}

	// tick sends periodic pings to keep the connection alive.
	tick *time.Ticker
}

// NewEventStream creates a new SSE stream, with an optional payload which is used to send pings every [pingInterval].
func NewEventStream(ctx context.Context, logger slog.Logger, pingPayload []byte, clk quartz.Clock) *EventStream {
	// Send periodic pings to keep connections alive.
	// The upstream provider may also send their own pings, but we can't rely on this.
	tick := time.NewTicker(time.Nanosecond)
	tick.Stop() // Ticker will start after stream initiation.

	return &EventStream{
		ctx:    ctx,
		logger: logger,
		clk:    clk,

		pingPayload: pingPayload,

		eventsCh: make(chan event, 128), // Small buffer to unblock senders; once full, senders will block.
		doneCh:   make(chan struct{}),
		tick:     tick,
	}
}

// InitiateStream initiates the SSE stream by sending headers and starting the
// ping ticker. This is safe to call multiple times as only the first call has
// any effect.
func (s *EventStream) InitiateStream(w http.ResponseWriter) {
	s.initiateOnce.Do(func() {
		s.initiated.Store(true)
		s.logger.Debug(s.ctx, "stream initiated")

		// Send headers for Server-Sent Event stream.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Send initial flush to ensure connection is established.
		if err := flush(w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Start ping ticker.
		s.tick.Reset(pingInterval)
	})
}

// Start handles sending Server-Sent Event to the client.
func (s *EventStream) Start(w http.ResponseWriter, r *http.Request) {
	// Signal completion on exit so senders don't block indefinitely after closure.
	defer close(s.doneCh)

	ctx := r.Context()

	defer s.tick.Stop()

	for {
		var (
			ev   event
			open bool
		)

		select {
		case <-s.ctx.Done():
			return
		case <-ctx.Done():
			s.logger.Debug(ctx, "request context canceled", slog.Error(ctx.Err()))
			return
		case ev, open = <-s.eventsCh: // Once closed, the buffered channel will drain all buffered values before showing as closed.
			if !open {
				s.logger.Debug(ctx, "events channel closed")
				return
			}

			// Initiate the stream on first event (if not already initiated).
			s.InitiateStream(w)
		case <-s.tick.C:
			ev = s.pingPayload
			if ev == nil {
				continue
			}
		}

		_, err := w.Write(ev)
		if err != nil {
			if IsConnError(err) {
				s.logger.Debug(ctx, "client disconnected during SSE write", slog.Error(err))
			} else {
				s.logger.Warn(ctx, "failed to write SSE event", slog.Error(err))
			}
			return
		}
		flushStart := s.clk.Now()
		if err := flush(w); err != nil {
			s.logger.Warn(ctx, "failed to flush event stream", slog.Error(err))
			return
		}
		if d := s.clk.Since(flushStart); d > SlowFlushThreshold {
			clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
			s.logger.Warn(ctx, "slow client detected",
				slog.F("flush_duration", d),
				slog.F("client_ip", clientIP),
				slog.F("user_agent", r.Header.Get("User-Agent")),
				slog.F("payload_size", len(ev)),
			)
		}

		// Reset the timer once we've flushed some data to the stream, since it's already fresh.
		// No need to ping in that case.
		s.tick.Reset(pingInterval)
	}
}

// Send enqueues an event in a non-blocking fashion, but if the channel is full
// then it will block.
func (s *EventStream) Send(ctx context.Context, payload []byte) error {
	// Save an unnecessary marshaling if possible.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-s.doneCh:
		return ErrEventStreamClosed
	default:
	}

	return s.SendRaw(ctx, payload)
}

func (s *EventStream) SendRaw(ctx context.Context, payload []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-s.doneCh:
		return ErrEventStreamClosed
	case s.eventsCh <- payload:
		return nil
	}
}

// Shutdown gracefully shuts down the stream, sending any supplementary events downstream if required.
// ONLY call this once all events have been submitted.
func (s *EventStream) Shutdown(shutdownCtx context.Context) error {
	s.shutdownOnce.Do(func() {
		s.logger.Debug(shutdownCtx, "shutdown initiated", slog.F("outstanding_events", len(s.eventsCh)))

		// Now it is safe to close the events channel; the Start() loop will exit
		// after draining remaining events and receivers will stop ranging.
		close(s.eventsCh)
	})

	var err error
	select {
	case <-shutdownCtx.Done():
		// If shutdownCtx completes, shutdown likely exceeded its timeout.
		err = xerrors.Errorf("shutdown ended prematurely with %d outstanding events: %w", len(s.eventsCh), shutdownCtx.Err())
	case <-s.ctx.Done():
		err = xerrors.Errorf("shutdown ended prematurely with %d outstanding events: %w", len(s.eventsCh), s.ctx.Err())
	case <-s.doneCh:
		return nil
	}

	// Even if the context is canceled, we need to wait for Start() to complete.
	<-s.doneCh
	return err
}

// IsStreaming checks if the stream has been initiated, or
// when events are buffered which - when processed - will initiate the stream.
func (s *EventStream) IsStreaming() bool {
	return s.initiated.Load() || len(s.eventsCh) > 0
}

// IsConnError checks if an error is related to client disconnection or context cancellation.
func IsConnError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, io.EOF) {
		return true
	}

	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) || errors.Is(err, net.ErrClosed) {
		return true
	}

	errStr := err.Error()
	return strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection reset by peer")
}

func IsUnrecoverableError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}

	return IsConnError(err)
}

func flush(w http.ResponseWriter) (err error) {
	flusher, ok := w.(http.Flusher)
	if !ok || flusher == nil {
		return xerrors.New("SSE not supported")
	}

	defer func() {
		if r := recover(); r != nil { //nolint:revive,staticcheck // Intentionally swallowed; likely a broken connection.
		}
	}()

	flusher.Flush()
	return nil
}
