package clock_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/coder/coder/v2/clock"
)

type exampleTickCounter struct {
	ctx   context.Context
	mu    sync.Mutex
	ticks int
	clock clock.Clock
}

func (c *exampleTickCounter) Ticks() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ticks
}

func (c *exampleTickCounter) count() {
	_ = c.clock.TickerFunc(c.ctx, time.Hour, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.ticks++
		return nil
	}, "mytag")
}

func newExampleTickCounter(ctx context.Context, clk clock.Clock) *exampleTickCounter {
	tc := &exampleTickCounter{ctx: ctx, clock: clk}
	go tc.count()
	return tc
}

// TestExampleTickerFunc demonstrates how to test the use of TickerFunc.
func TestExampleTickerFunc(t *testing.T) {
	t.Parallel()
	// nolint:gocritic // trying to avoid Coder-specific stuff with an eye toward spinning this out
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mClock := clock.NewMock()

	// Because the ticker is started on a goroutine, we can't immediately start
	// advancing the clock, or we will race with the start of the ticker. If we
	// win that race, the clock gets advanced _before_ the ticker starts, and
	// our ticker will not get a tick.
	//
	// To handle this, we set a trap for the call to TickerFunc(), so that we
	// can assert it has been called before advancing the clock.
	trap := mClock.Trap().TickerFunc("mytag")
	defer trap.Close()

	tc := newExampleTickCounter(ctx, mClock)

	// Here, we wait for our trap to be triggered.
	call, err := trap.Wait(ctx)
	if err != nil {
		t.Fatal("ticker never started")
	}
	// it's good practice to release calls before any possible t.Fatal() calls
	// so that we don't leave dangling goroutines waiting for the call to be
	// released.
	call.Release()
	if call.Duration != time.Hour {
		t.Fatal("unexpected duration")
	}

	if tks := tc.Ticks(); tks != 0 {
		t.Fatalf("expected 0 got %d ticks", tks)
	}

	// Now that we know the ticker is started, we can advance the time.
	mClock.Advance(time.Hour).MustWait(ctx, t)

	if tks := tc.Ticks(); tks != 1 {
		t.Fatalf("expected 1 got %d ticks", tks)
	}
}
