package aibridge

import (
	"context"
	"sync"
)

// InflightGate provides the shared request admission and drain machinery used
// by the interception bridge and the reverse proxy tracker. It admits
// requests until shutdown begins, counts the admitted ones, and lets callers
// drain or forcibly cancel them. Mode-specific concerns, such as the refusal
// status code, request counting, and whether a shutdown keeps waiting after
// cancellation, stay with the caller.
//
// InflightGate is safe for concurrent use.
type InflightGate struct {
	// mu orders Admit's wg.Add (read-held) before BeginShutdown's
	// close(closed) (write-held), so Add never races the drain waiter.
	mu     sync.RWMutex
	wg     sync.WaitGroup
	closed chan struct{}

	// ctx is canceled by CancelInflight to abort admitted requests.
	ctx    context.Context
	cancel context.CancelFunc

	// shutdownOnce guards BeginShutdown so the drain waiter starts once,
	// after admission is closed. drained closes when the waiter finishes.
	shutdownOnce sync.Once
	drained      chan struct{}
}

// NewInflightGate returns a gate ready to admit requests.
func NewInflightGate() *InflightGate {
	ctx, cancel := context.WithCancel(context.Background())
	return &InflightGate{closed: make(chan struct{}), ctx: ctx, cancel: cancel}
}

// Admit registers an in-flight request unless shutdown has begun. When ok is
// true, onAdmit (if non-nil) runs while the admission lock is held, after the
// shutdown check and before the request is counted, and the caller must call
// release exactly once when the request finishes. When ok is false the request
// must be refused and release is nil.
//
// Running onAdmit inside the admission lock lets callers observe a consistent
// point between the shutdown check and the count, which the interception
// bridge uses for a deterministic race trap.
func (g *InflightGate) Admit(onAdmit func()) (release func(), ok bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	select {
	case <-g.closed:
		return nil, false
	default:
	}
	if onAdmit != nil {
		onAdmit()
	}
	g.wg.Add(1)
	return g.wg.Done, true
}

// Track derives a request context from parent that is also canceled when a
// forced shutdown aborts in-flight requests. The returned stop releases the
// link and must be called when the request finishes.
func (g *InflightGate) Track(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(g.ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

// BeginShutdown stops admitting new requests and returns a channel that closes
// once every admitted request has finished. It is idempotent: repeated calls
// return the same channel and only the first has any effect.
//
// The drain waiter is started here, after admission is closed under the write
// lock, so no in-flight Admit can wg.Add concurrently with the waiter's
// wg.Wait (see mu). Combining the close and the waiter into one call keeps that
// ordering an invariant of the type rather than a caller obligation. The waiter
// runs to completion even if the caller stops waiting, so callers may keep
// waiting on the channel after a forced cancellation.
func (g *InflightGate) BeginShutdown() <-chan struct{} {
	g.shutdownOnce.Do(func() {
		g.mu.Lock()
		close(g.closed)
		g.mu.Unlock()

		g.drained = make(chan struct{})
		go func() {
			g.wg.Wait()
			close(g.drained)
		}()
	})
	return g.drained
}

// CancelInflight cancels the context returned to admitted requests through
// Track, aborting those that observe cancellation.
func (g *InflightGate) CancelInflight() {
	g.cancel()
}
