package lab

import (
	"context"
	"sync/atomic"
	"time"
)

// blockWait makes N goroutines block on an unbuffered channel that a single
// feeder goroutine writes to in bursts every 1 to 10 ms. Each waiter is
// unblocked once per burst, so it accumulates block events whose duration
// is well above the block profile's 100 microsecond sampling rate.
type blockWait struct {
	runner
	goroutines atomic.Int64
	wakeups    atomic.Int64
}

func newBlockWait() *blockWait {
	b := &blockWait{}
	b.name = b.Name()
	return b
}

func (*blockWait) Name() string { return "block-wait" }

func (*blockWait) Description() string {
	return "N goroutines block on a channel that a feeder writes to every " +
		"1 to 10 ms. The block profile shows the wait time in waitForWake."
}

func (*blockWait) Knobs() []Knob {
	return []Knob{{Name: "goroutines", Unit: "goroutines", Default: 64, Min: 1, Max: 1024}}
}

func (b *blockWait) Start(ctx context.Context, knobs map[string]any) error {
	vals, err := parseKnobs(knobs, b.Knobs())
	if err != nil {
		return err
	}
	n := vals["goroutines"]
	wake := make(chan struct{})
	// Goroutine 0 is the feeder; the remaining n are waiters.
	b.restart(ctx, n+1, func() {
		b.goroutines.Store(int64(n))
		b.wakeups.Store(0)
	}, func(ctx context.Context, i int) {
		if i == 0 {
			b.feed(ctx, wake, n)
			return
		}
		b.waitForWake(ctx, wake)
	})
	return nil
}

func (b *blockWait) Stop() {
	b.stop(func() { b.goroutines.Store(0) })
}

func (b *blockWait) State() map[string]any {
	return map[string]any{
		"goroutines": b.goroutines.Load(),
		"wakeups":    b.wakeups.Load(),
	}
}

// feed sends n wakeups on wake after each pause until ctx is canceled. The
// pause cycles through 1, 2, ..., 10 ms so wait durations are spread across
// that range. Each send hands off to exactly one blocked waiter.
func (b *blockWait) feed(ctx context.Context, wake chan<- struct{}, n int) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for round := 0; ; round++ {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		for range n {
			select {
			case wake <- struct{}{}:
				b.wakeups.Add(1)
			case <-ctx.Done():
				return
			}
		}
		timer.Reset(time.Duration(1+round%10) * time.Millisecond)
	}
}

// waitForWake blocks on wake in a loop until ctx is canceled. Every receive
// ends a blocking wait, which is when the runtime records a block event
// against this frame.
//
//go:noinline
func (*blockWait) waitForWake(ctx context.Context, wake <-chan struct{}) {
	for {
		select {
		case <-wake:
		case <-ctx.Done():
			return
		}
	}
}
