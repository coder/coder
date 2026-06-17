package httpapi

import (
	"context"
	"errors"
	"net"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

const HeartbeatInterval time.Duration = 15 * time.Second

// ProbeResult classifies the outcome of a single WebSocket liveness
// probe so that callers (typically a Prometheus recorder) can track
// successes and the various failure modes independently.
type ProbeResult string

const (
	ProbeOK         ProbeResult = "ok"
	ProbeTimeout    ProbeResult = "timeout"
	ProbePeerClosed ProbeResult = "peer_closed"
	ProbeCanceled   ProbeResult = "canceled"
	ProbeError      ProbeResult = "error"
)

// ProbeRecorder is called once per liveness probe with its outcome.
// It may be nil, in which case probes are still run but not recorded.
type ProbeRecorder func(ctx context.Context, result ProbeResult)

// PingCloser is the minimal interface for WebSocket liveness probing.
// *websocket.Conn satisfies this interface.
type PingCloser interface {
	Ping(ctx context.Context) error
	Close(code websocket.StatusCode, reason string) error
}

// WSWatcher supervises WebSocket connections for liveness by
// periodically sending ping frames. On probe failure, the watcher
// closes the connection with StatusGoingAway and cancels the
// returned context; the caller owns closing the connection on
// normal teardown.
type WSWatcher struct {
	rec      ProbeRecorder
	clk      quartz.Clock
	interval time.Duration
}

// NewWSWatcher creates a WSWatcher. Pass nil for rec when no
// recording is needed (e.g. agent-side code without a Prometheus
// registry).
func NewWSWatcher(clk quartz.Clock, rec ProbeRecorder) *WSWatcher {
	return &WSWatcher{
		rec:      rec,
		clk:      clk,
		interval: HeartbeatInterval,
	}
}

// Watch supervises conn for liveness. The returned context is
// canceled when parent is canceled or when conn fails a probe.
// Watch closes conn on probe failure with StatusGoingAway; the
// caller owns close on normal teardown.
func (w *WSWatcher) Watch(parent context.Context, log slog.Logger, conn PingCloser) context.Context {
	if w == nil {
		panic("developer error: WSWatcher is nil")
	}
	ctx, cancel := context.WithCancel(parent)
	go func() {
		defer cancel()
		w.supervise(ctx, log, conn)
	}()
	return ctx
}

func (w *WSWatcher) supervise(ctx context.Context, log slog.Logger, conn PingCloser) {
	ticker := w.clk.NewTicker(w.interval, "WSWatcher")
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		result, err := probe(ctx, conn, w.interval)
		if w.rec != nil {
			w.rec(ctx, result)
		}
		if result == ProbeOK {
			continue
		}
		if result == ProbeError {
			log.Error(ctx, "websocket probe failed", slog.Error(err))
		} else {
			log.Debug(ctx, "websocket probe stopped",
				slog.F("result", string(result)), slog.Error(err))
		}
		_ = conn.Close(websocket.StatusGoingAway, "liveness probe failed")
		return
	}
}

func probe(ctx context.Context, conn PingCloser, timeout time.Duration) (ProbeResult, error) {
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := conn.Ping(pingCtx)
	switch {
	case err == nil:
		return ProbeOK, nil
	case errors.Is(err, context.Canceled):
		return ProbeCanceled, err
	case errors.Is(err, context.DeadlineExceeded):
		return ProbeTimeout, err
	case errors.Is(err, net.ErrClosed) || websocket.CloseStatus(err) != -1:
		return ProbePeerClosed, err
	default:
		return ProbeError, xerrors.Errorf("ping: %w", err)
	}
}

// HeartbeatClose is a legacy helper that pings conn in a loop and
// calls exit on failure. Callers that need metric recording should
// use WSWatcher directly.
func HeartbeatClose(ctx context.Context, logger slog.Logger, exit func(), conn *websocket.Conn) {
	w := NewWSWatcher(quartz.NewReal(), nil)
	watchCtx := w.Watch(ctx, logger, conn)
	<-watchCtx.Done()
	// Only call exit when the probe failed; if the parent context was
	// canceled the caller is already shutting down.
	if ctx.Err() == nil {
		exit()
	}
}
