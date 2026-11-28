package chattest

import (
	"context"
	"sync"
)

// Gate holds test HTTP handlers until the test opens it, so a test can act
// while a request is in flight, for example call another endpoint while an
// OAuth token request is outstanding. Open it on every path, for example
// with defer, so held handlers return before their server closes.
type Gate struct {
	mu       sync.Mutex
	arrivals int
	waiters  []gateWaiter
	open     chan struct{}
	openOnce sync.Once
}

type gateWaiter struct {
	n       int
	arrived chan struct{}
}

// NewGate returns a gate that holds every caller until Open.
func NewGate() *Gate {
	return &Gate{open: make(chan struct{})}
}

// Wait records an arrival, then blocks until the gate opens or ctx ends.
// After the gate opens, Wait still records arrivals but returns at once.
func (g *Gate) Wait(ctx context.Context) error {
	g.mu.Lock()
	g.arrivals++
	pending := g.waiters[:0]
	for _, w := range g.waiters {
		if g.arrivals >= w.n {
			close(w.arrived)
			continue
		}
		pending = append(pending, w)
	}
	g.waiters = pending
	g.mu.Unlock()

	select {
	case <-g.open:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Arrived returns a channel that is closed once n callers have reached
// Wait.
func (g *Gate) Arrived(n int) <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	arrived := make(chan struct{})
	if g.arrivals >= n {
		close(arrived)
		return arrived
	}
	g.waiters = append(g.waiters, gateWaiter{n: n, arrived: arrived})
	return arrived
}

// Arrivals reports how many callers have reached Wait.
func (g *Gate) Arrivals() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.arrivals
}

// Open releases every held caller and every later one. It is safe to
// call more than once.
func (g *Gate) Open() {
	g.openOnce.Do(func() { close(g.open) })
}
