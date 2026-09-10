package aibridged

import (
	"context"
	"net/http"
	"sync"
)

// inflightTracker admits requests until Shutdown begins, then waits for the
// admitted ones to finish. Each request runs under a context that Shutdown
// cancels once its own context expires, without waiting for handlers to
// observe cancellation. It mirrors [aibridge.RequestBridge]'s tracking for
// the proxy path, whose routers are swapped on reload and so cannot own it.
type inflightTracker struct {
	// mu orders wg.Add (Serve, read-held) before close(closed) (Shutdown,
	// write-held), so Add never races Wait.
	mu     sync.RWMutex
	wg     sync.WaitGroup
	closed chan struct{}

	// ctx is canceled by a forced shutdown to abort admitted requests.
	ctx    context.Context
	cancel context.CancelFunc

	shutdownOnce sync.Once
}

func newInflightTracker() *inflightTracker {
	ctx, cancel := context.WithCancel(context.Background())
	return &inflightTracker{closed: make(chan struct{}), ctx: ctx, cancel: cancel}
}

// Serve admits r and serves it with next, or refuses it with 503 once
// Shutdown has begun.
func (t *inflightTracker) Serve(rw http.ResponseWriter, r *http.Request, next http.Handler) {
	t.mu.RLock()
	select {
	case <-t.closed:
		t.mu.RUnlock()
		http.Error(rw, "AI Gateway is shutting down", http.StatusServiceUnavailable)
		return
	default:
	}
	t.wg.Add(1)
	t.mu.RUnlock()
	defer t.wg.Done()

	// Keep the request context, and cancel it too when shutdown is forced.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(t.ctx, cancel)
	defer stop()
	next.ServeHTTP(rw, r.WithContext(ctx))
}

// Shutdown stops admitting requests and waits for the admitted ones to finish.
// If ctx expires first, admitted requests are canceled and ctx's error is
// returned without waiting for handlers to exit.
func (t *inflightTracker) Shutdown(ctx context.Context) error {
	var err error
	t.shutdownOnce.Do(func() {
		t.mu.Lock()
		close(t.closed)
		t.mu.Unlock()

		done := make(chan struct{})
		// The waiter exits once all admitted handlers return, even if
		// Shutdown has already returned because its context expired.
		go func() {
			t.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			// Cancellation cannot interrupt handlers blocked on I/O or
			// otherwise ignoring their context. Do not wait for them.
			err = ctx.Err()
		}
		// Nothing can be admitted anymore; release the context.
		t.cancel()
	})
	return err
}
