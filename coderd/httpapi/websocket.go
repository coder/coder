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

// HeartbeatClose loops to ping a WebSocket to keep it alive.
// It calls `exit` on ping failure.
func HeartbeatClose(ctx context.Context, logger slog.Logger, exit func(), conn *websocket.Conn) {
	heartbeatCloseWith(ctx, logger, exit, conn, quartz.NewReal(), HeartbeatInterval)
}

func heartbeatCloseWith(ctx context.Context, logger slog.Logger, exit func(), conn *websocket.Conn, clk quartz.Clock, interval time.Duration) {
	ticker := clk.NewTicker(interval, "HeartbeatClose")
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		err := pingWithTimeout(ctx, conn, interval)
		if err != nil {
			// These errors are all expected during normal connection
			// teardown and should not be logged at error level:
			//   - context.DeadlineExceeded: client disconnected
			//     without sending a close frame.
			//   - context.Canceled: request context was canceled.
			//   - net.ErrClosed: connection was already closed by
			//     another goroutine (e.g. handler returned).
			//   - websocket.CloseError: a close frame was
			//     received or sent.
			if errors.Is(err, context.DeadlineExceeded) ||
				errors.Is(err, context.Canceled) ||
				errors.Is(err, net.ErrClosed) ||
				websocket.CloseStatus(err) != -1 {
				logger.Debug(ctx, "heartbeat ping stopped", slog.Error(err))
			} else {
				logger.Error(ctx, "failed to heartbeat ping", slog.Error(err))
			}
			_ = conn.Close(websocket.StatusGoingAway, "Ping failed")
			exit()
			return
		}
	}
}

func pingWithTimeout(ctx context.Context, conn *websocket.Conn, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := conn.Ping(ctx)
	if err != nil {
		return xerrors.Errorf("failed to ping: %w", err)
	}

	return nil
}
