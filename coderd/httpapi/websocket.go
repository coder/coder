package httpapi

import (
	"fmt"
	"context"
	"errors"
	"time"

	"cdr.dev/slog"
	"github.com/coder/websocket"

)
// Heartbeat loops to ping a WebSocket to keep it alive.
// Default idle connection timeouts are typically 60 seconds.
// See: https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html#connection-idle-timeout

func Heartbeat(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		err := conn.Ping(ctx)
		if err != nil {
			return
		}
	}
}
// Heartbeat loops to ping a WebSocket to keep it alive. It calls `exit` on ping
// failure.
func HeartbeatClose(ctx context.Context, logger slog.Logger, exit func(), conn *websocket.Conn) {
	interval := 15 * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:

		}
		err := pingWithTimeout(ctx, conn, interval)
		if err != nil {
			// context.DeadlineExceeded is expected when the client disconnects without sending a close frame
			if !errors.Is(err, context.DeadlineExceeded) {
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
		return fmt.Errorf("failed to ping: %w", err)

	}
	return nil
}
