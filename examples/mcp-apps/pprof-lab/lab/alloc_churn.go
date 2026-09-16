package lab

import (
	"context"
	"sync/atomic"
)

// allocChurn runs workers that allocate short-lived byte slices as fast as
// they can and drop them immediately. Nothing is retained, so the pressure
// shows up in the allocs profile (alloc_space) and as GC time in the CPU
// profile rather than in inuse_space.
type allocChurn struct {
	runner
	workers atomic.Int64
	allocs  atomic.Int64
	bytes   atomic.Int64
}

func newAllocChurn() *allocChurn {
	a := &allocChurn{}
	a.name = a.Name()
	return a
}

func (*allocChurn) Name() string { return "alloc-churn" }

func (*allocChurn) Description() string {
	return "N workers allocate and immediately drop 1 to 64 KiB slices. " +
		"The allocs profile (alloc_space) attributes them to allocateAndDrop; " +
		"the CPU profile shows GC work."
}

func (*allocChurn) Knobs() []Knob {
	return []Knob{{Name: "workers", Unit: "workers", Default: 4, Min: 1, Max: 32}}
}

func (a *allocChurn) Start(ctx context.Context, knobs map[string]any) error {
	vals, err := parseKnobs(knobs, a.Knobs())
	if err != nil {
		return err
	}
	workers := vals["workers"]
	a.restart(ctx, workers, func() {
		a.workers.Store(int64(workers))
		a.allocs.Store(0)
		a.bytes.Store(0)
	}, func(ctx context.Context, _ int) {
		a.churn(ctx)
	})
	return nil
}

func (a *allocChurn) Stop() {
	a.stop(func() { a.workers.Store(0) })
}

func (a *allocChurn) State() map[string]any {
	return map[string]any{
		"workers":       a.workers.Load(),
		"allocations":   a.allocs.Load(),
		"allocated_mib": bytesToMiB(a.bytes.Load()),
	}
}

// churn allocates in a loop until ctx is canceled, cycling the request
// size from 1 KiB to 64 KiB so allocations land in many size classes. The
// context is checked every iteration so Stop returns promptly.
func (a *allocChurn) churn(ctx context.Context) {
	for n := 0; ctx.Err() == nil; n++ {
		size := (1 + n%64) << 10
		a.bytes.Add(int64(a.allocateAndDrop(size)))
		a.allocs.Add(1)
	}
}

// allocateAndDrop allocates a size-byte slice, touches it so the allocation
// cannot be elided, and returns its length. The slice becomes garbage as
// soon as the function returns.
//
//go:noinline
func (*allocChurn) allocateAndDrop(size int) int {
	buf := make([]byte, size)
	buf[0] = 1
	buf[len(buf)-1] = 1
	return len(buf)
}
