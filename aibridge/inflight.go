package aibridge

import (
	"context"
	"net/http"
	"sync"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

// ErrGatewayShutdown is the cancellation cause of request contexts canceled by
// a forced gateway shutdown.
var ErrGatewayShutdown = xerrors.New("AI Gateway is shutting down")

// InflightGate counts admitted requests and links their contexts to Close.
// It is safe for concurrent use. Must be created with NewInflightGate.
type InflightGate struct {
	logger slog.Logger
	clock  quartz.Clock

	// mu guards active, closed, and closure of drained.
	// Reads and writes of active and closed must hold mu.
	mu      sync.Mutex
	active  int
	closed  bool
	drained chan struct{}

	// ctx is canceled by Close to signal admitted requests.
	ctx    context.Context
	cancel context.CancelFunc
}

// NewInflightGate returns a gate ready to admit requests.
func NewInflightGate(logger slog.Logger) *InflightGate {
	ctx, cancel := context.WithCancel(context.Background())
	return &InflightGate{logger: logger, clock: quartz.NewReal(), drained: make(chan struct{}), ctx: ctx, cancel: cancel}
}

// Middleware admits requests and links their contexts to Close, or returns 503
// after shutdown begins.
func (g *InflightGate) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release, ok := g.Admit()
		if !ok {
			http.Error(w, "AI Gateway is shutting down", http.StatusServiceUnavailable)
			return
		}
		defer release()
		ctx, cleanup := g.RequestContext(r.Context())
		defer cleanup()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Admit returns nil, false after shutdown begins. Otherwise, release must be
// called exactly once when the handler returns.
func (g *InflightGate) Admit() (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, false
	}
	_ = g.clock.Now("serve_admission") // Trap point for deterministic race tests.
	g.active++
	return g.release, true
}

func (g *InflightGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active == 0 {
		g.logger.Error(g.ctx, "released an unregistered request")
		return
	}
	g.active--
	if g.closed && g.active == 0 {
		close(g.drained)
	}
}

// RequestContext derives a context canceled by either the parent or forced
// shutdown. Forced shutdown sets [ErrGatewayShutdown] as the cancellation
// cause so it can be told apart from a client abort. The caller must invoke
// cleanup when the handler returns to unregister the shutdown callback and
// cancel the derived context.
func (g *InflightGate) RequestContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	stop := context.AfterFunc(g.ctx, func() { cancel(ErrGatewayShutdown) })
	return ctx, func() {
		stop()
		cancel(nil)
	}
}

// Shutdown stops admission and waits for all admitted requests to finish.
// If ctx expires, it returns ctx.Err() without canceling request contexts.
func (g *InflightGate) Shutdown(ctx context.Context) error {
	done := g.stopAdmission()
	// Prefer completed drainage, including an idle gate with an expired ctx.
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// stopAdmission returns the channel closed by the last admitted release.
func (g *InflightGate) stopAdmission() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closed {
		g.closed = true
		if g.active == 0 {
			close(g.drained)
		}
	}
	return g.drained
}

// Close stops admission and cancels request contexts without waiting.
// It does not close network connections or force handlers to return.
func (g *InflightGate) Close() {
	g.stopAdmission()
	g.cancel()
}
