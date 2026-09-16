package lab

import (
	"context"
	"runtime"
	"sync/atomic"
)

// cpuBurn spins N goroutines in a pure computation loop. The CPU profile
// attributes nearly all samples to hotLoop.
type cpuBurn struct {
	runner
	workers    atomic.Int64
	iterations atomic.Int64
}

func newCPUBurn() *cpuBurn {
	c := &cpuBurn{}
	c.name = c.Name()
	return c
}

func (*cpuBurn) Name() string { return "cpu-burn" }

func (*cpuBurn) Description() string {
	return "N goroutines spin in a mixing loop with no blocking and no " +
		"allocation. The CPU profile shows hotLoop at the top."
}

func (*cpuBurn) Knobs() []Knob {
	return []Knob{{Name: "workers", Unit: "workers", Default: min(runtime.GOMAXPROCS(0), 64), Min: 1, Max: 64}}
}

func (c *cpuBurn) Start(ctx context.Context, knobs map[string]any) error {
	vals, err := parseKnobs(knobs, c.Knobs())
	if err != nil {
		return err
	}
	workers := vals["workers"]
	c.restart(ctx, workers, func() {
		c.workers.Store(int64(workers))
		c.iterations.Store(0)
	}, func(ctx context.Context, _ int) {
		c.burn(ctx)
	})
	return nil
}

func (c *cpuBurn) Stop() {
	c.stop(func() { c.workers.Store(0) })
}

func (c *cpuBurn) State() map[string]any {
	return map[string]any{
		"workers":    c.workers.Load(),
		"iterations": c.iterations.Load(),
	}
}

// hotLoopRounds is the number of mixing steps per hotLoop call. It is large
// enough that the loop dominates the profile and small enough that Stop is
// observed within a fraction of a millisecond.
const hotLoopRounds = 1 << 14

// burn calls hotLoop until ctx is canceled, feeding each result back in as
// the next seed so the compiler cannot drop the computation.
func (c *cpuBurn) burn(ctx context.Context) {
	seed := uint64(1)
	for ctx.Err() == nil {
		seed = c.hotLoop(seed)
		c.iterations.Add(hotLoopRounds)
	}
}

// hotLoop runs hotLoopRounds steps of a splitmix64-style mixer over seed and
// returns the result.
//
//go:noinline
func (*cpuBurn) hotLoop(seed uint64) uint64 {
	x := seed
	for range hotLoopRounds {
		x += 0x9e3779b97f4a7c15
		z := x
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		x ^= z >> 31
	}
	return x
}
