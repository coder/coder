package coderd

import (
	"context"
	"net/http"
	"sync"

	"nhooyr.io/websocket"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// ActiveWebsockets is a helper struct that can be used to track active
// websocket connections.
type ActiveWebsockets struct {
	ctx    context.Context
	cancel func()

	wg sync.WaitGroup
}

func NewActiveWebsockets(ctx context.Context, cancel func()) *ActiveWebsockets {
	return &ActiveWebsockets{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Accept accepts a websocket connection and calls f with the connection.
// The function will be tracked by the ActiveWebsockets struct and will be
// closed when the parent context is canceled.
func (a *ActiveWebsockets) Accept(rw http.ResponseWriter, r *http.Request, options *websocket.AcceptOptions, f func(conn *websocket.Conn)) {
	// Ensure we are still accepting websocket connections, and not shutting down.
	if err := a.ctx.Err(); err != nil {
		httpapi.Write(context.Background(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "No longer accepting websocket requests.",
			Detail:  err.Error(),
		})
		return
	}
	// Ensure we decrement the wait group when we are done.
	a.wg.Add(1)
	defer a.wg.Done()

	// Accept the websocket connection
	conn, err := websocket.Accept(rw, r, options)
	if err != nil {
		httpapi.Write(context.Background(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	// Always track the connection before allowing the caller to handle it.
	// This ensures the connection is closed when the parent context is canceled.
	// This new context will end if the parent context is cancelled or if
	// the connection is closed.
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	a.track(ctx, conn)

	// Handle the websocket connection
	f(conn)
}

// Track runs a go routine that will close a given websocket connection when
// the parent context is canceled.
func (a *ActiveWebsockets) track(ctx context.Context, conn *websocket.Conn) {
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "")
		}
	}()
}

func (a *ActiveWebsockets) Close() {
	a.cancel()
	a.wg.Wait()
}
