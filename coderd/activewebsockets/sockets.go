package activewebsockets

import (
	"context"
	"net/http"
	"runtime/pprof"
	"sync"

	"nhooyr.io/websocket"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// Active is a helper struct that can be used to track active
// websocket connections. All connections will be closed when the parent
// context is canceled.
type Active struct {
	ctx    context.Context
	cancel func()

	wg sync.WaitGroup
}

func New(ctx context.Context) *Active {
	ctx, cancel := context.WithCancel(ctx)
	return &Active{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Accept accepts a websocket connection and calls f with the connection.
// The function will be tracked by the Active struct and will be
// closed when the parent context is canceled.
// Steps:
//  1. Ensure we are still accepting websocket connections, and not shutting down.
//  2. Add 1 to the wait group.
//  3. Ensure we decrement the wait group when we are done (defer).
//  4. Accept the websocket connection.
//     4a. If there is an error, write the error to the response writer and return.
//  5. Launch go routine to kill websocket if the parent context is canceled.
//  6. Call 'f' with the websocket connection.
func (a *Active) Accept(rw http.ResponseWriter, r *http.Request, options *websocket.AcceptOptions, f func(conn *websocket.Conn)) {
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
	closeConnOnContext(ctx, conn)

	// Handle the websocket connection
	f(conn)
}

// closeConnOnContext launches a go routine that will watch a given context
// and close a websocket connection if that context is canceled.
func closeConnOnContext(ctx context.Context, conn *websocket.Conn) {
	// Labeling the go routine for goroutine dumps/debugging.
	go pprof.Do(ctx, pprof.Labels("service", "ActiveWebsockets"), func(ctx context.Context) {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "")
		}
	})
}

// Close will close all active websocket connections and wait for them to
// finish.
func (a *Active) Close() {
	a.cancel()
	a.wg.Wait()
}
