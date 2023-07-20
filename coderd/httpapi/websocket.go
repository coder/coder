package httpapi

import (
	"context"
	"time"

	"nhooyr.io/websocket"
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

// Heartbeat loops to ping a WebSocket to keep it alive. It kills the connection
// on ping failure.
func HeartbeatClose(ctx context.Context, exit func(), conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		err := conn.Ping(ctx)
		if err != nil {
			_ = conn.Close(websocket.StatusGoingAway, "Ping failed")
			exit()
			return
		}
	}
}
