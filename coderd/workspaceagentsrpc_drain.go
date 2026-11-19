package coderd

import (
	"context"
	"maps"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

const (
	// agentDrainWindow is the longest time that disconnects are spread over.
	// Agents that redial on a fixed schedule do so at most every 10 seconds,
	// so spreading disconnects over that long spreads their retries across
	// the whole interval.
	agentDrainWindow = 10 * time.Second
	// agentDrainInterval is the average time between two disconnects. It
	// keeps the drain short when few agents are connected.
	agentDrainInterval = 10 * time.Millisecond
)

// agentRPCConns tracks the open agent RPC connections so that they can be
// closed a few at a time.
type agentRPCConns struct {
	mu      sync.Mutex
	nextID  uint64
	closers map[uint64]func()
}

// add tracks a connection that closeConn closes. The returned function stops
// tracking it and must be called when the connection is gone.
func (c *agentRPCConns) add(closeConn func()) (remove func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closers == nil {
		c.closers = make(map[uint64]func())
	}
	id := c.nextID
	c.nextID++
	c.closers[id] = closeConn
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.closers, id)
	}
}

// drain closes the tracked connections at random moments spread over at most
// window. It returns the number of connections it set out to close and how
// long it spread them over. The error is non-nil if ctx ended first.
func (c *agentRPCConns) drain(ctx context.Context, clock quartz.Clock, window time.Duration) (int, time.Duration, error) {
	c.mu.Lock()
	closers := slices.Collect(maps.Values(c.closers))
	c.mu.Unlock()

	spread := min(window, time.Duration(len(closers))*agentDrainInterval)
	if spread <= 0 {
		for _, closeConn := range closers {
			closeConn()
		}
		return len(closers), 0, nil
	}

	delays := make([]time.Duration, len(closers))
	for i := range delays {
		// #nosec G404 - spreading out disconnects does not need a cryptographic source.
		delays[i] = rand.N(spread)
	}
	slices.Sort(delays)

	start := clock.Now()
	for i, closeConn := range closers {
		if wait := delays[i] - clock.Since(start); wait > 0 {
			timer := clock.NewTimer(wait, "agentRPCConns", "drain")
			select {
			case <-ctx.Done():
				timer.Stop()
				return len(closers), spread, ctx.Err()
			case <-timer.C:
			}
		}
		closeConn()
	}
	return len(closers), spread, nil
}

// DrainAgentConnections closes the open workspace agent connections a few at
// a time instead of all at once. It returns when they are closed, or with an
// error if ctx ends first. An agent redials on a schedule that starts when it
// loses the server, so agents that are cut off together also come back
// together. Call this while shutting down, after the server has stopped
// accepting connections.
func (api *API) DrainAgentConnections(ctx context.Context) error {
	count, spread, err := api.agentRPCConns.drain(ctx, api.Clock, agentDrainWindow)
	if count > 0 {
		api.Logger.Info(ctx, "closed workspace agent connections",
			slog.F("count", count), slog.F("spread", spread), slog.Error(err))
	}
	return err
}
