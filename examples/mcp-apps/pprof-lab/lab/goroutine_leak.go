package lab

import (
	"context"
	"sync/atomic"
)

// goroutineLeak parks a configurable number of goroutines on a channel that
// is never written to. They exit only when the run's context is canceled.
type goroutineLeak struct {
	runner
	count  atomic.Int64
	parked atomic.Int64
}

func newGoroutineLeak() *goroutineLeak {
	g := &goroutineLeak{}
	g.name = g.Name()
	return g
}

func (*goroutineLeak) Name() string { return "goroutine-leak" }

func (*goroutineLeak) Description() string {
	return "Parks N goroutines on a channel that is never written to. " +
		"The goroutine profile shows them stacked in parkedWorker."
}

func (*goroutineLeak) Knobs() []Knob {
	return []Knob{{Name: "count", Unit: "goroutines", Default: 500, Min: 1, Max: 20000}}
}

func (g *goroutineLeak) Start(ctx context.Context, knobs map[string]any) error {
	vals, err := parseKnobs(knobs, g.Knobs())
	if err != nil {
		return err
	}
	count := vals["count"]
	park := make(chan struct{})
	g.restart(ctx, count, func() {
		g.count.Store(int64(count))
		g.parked.Store(0)
	}, func(ctx context.Context, _ int) {
		g.parkedWorker(ctx, park)
	})
	return nil
}

func (g *goroutineLeak) Stop() {
	g.stop(func() { g.count.Store(0) })
}

func (g *goroutineLeak) State() map[string]any {
	return map[string]any{
		"goroutines": g.count.Load(),
		"parked":     g.parked.Load(),
	}
}

// parkedWorker blocks until park is closed or ctx is canceled. park is never
// closed, so the goroutine stays in this frame for the life of the run.
//
//go:noinline
func (g *goroutineLeak) parkedWorker(ctx context.Context, park <-chan struct{}) {
	g.parked.Add(1)
	defer g.parked.Add(-1)
	select {
	case <-park:
	case <-ctx.Done():
	}
}
