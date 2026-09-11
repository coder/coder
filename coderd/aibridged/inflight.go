package aibridged

import (
	"context"
	"net/http"
	"sync"

	"github.com/coder/coder/v2/aibridge"
)

// inflightTracker admits requests until Shutdown begins, then drains the
// admitted ones. Each request runs under a context that Shutdown cancels once
// its own context expires. Unlike [aibridge.RequestBridge], whose Shutdown
// waits for the drain even after cancellation, the proxy tracker returns as
// soon as the shutdown context expires and does not wait for handlers to
// observe cancellation. The proxy path needs its own tracker because its
// routers are swapped on reload and so cannot own it.
//
// It builds on [aibridge.InflightGate] for the shared admission and drain
// machinery; the proxy-specific 503 refusal and deadline-without-waiting
// shutdown live here.
type inflightTracker struct {
	gate         *aibridge.InflightGate
	shutdownOnce sync.Once
}

func newInflightTracker() *inflightTracker {
	return &inflightTracker{gate: aibridge.NewInflightGate()}
}

// Serve admits r and serves it with next, or refuses it with 503 once
// Shutdown has begun.
func (t *inflightTracker) Serve(rw http.ResponseWriter, r *http.Request, next http.Handler) {
	release, ok := t.gate.Admit(nil)
	if !ok {
		http.Error(rw, "AI Gateway is shutting down", http.StatusServiceUnavailable)
		return
	}
	defer release()

	// Keep the request context, and cancel it too when shutdown is forced.
	ctx, stop := t.gate.Track(r.Context())
	defer stop()
	next.ServeHTTP(rw, r.WithContext(ctx))
}

// Shutdown stops admitting requests and waits for the admitted ones to finish.
// If ctx expires first, admitted requests are canceled and ctx's error is
// returned without waiting for handlers to exit.
func (t *inflightTracker) Shutdown(ctx context.Context) error {
	var err error
	t.shutdownOnce.Do(func() {
		done := t.gate.BeginShutdown()
		select {
		case <-done:
		case <-ctx.Done():
			// Cancellation cannot interrupt handlers blocked on I/O or
			// otherwise ignoring their context. Do not wait for them.
			err = ctx.Err()
		}
		// Nothing can be admitted anymore; release the context.
		t.gate.CancelInflight()
	})
	return err
}
