package lab

import (
	"context"
	"sync"
	"time"
	"unsafe"
)

// Record is the unit of retained memory. It is exactly 1 KiB so a MiB of
// growth shows up as 1024 objects allocated in retainBlob.
type Record struct {
	ID      uint64
	Payload [1016]byte
}

const recordsPerMiB = (1 << 20) / int(unsafe.Sizeof(Record{}))

// heapGrowth appends *Record values to a slice that is only dropped by Stop.
// One goroutine adds mib_per_second MiB per second until cap_mib is retained,
// then exits while the slice stays alive so the heap profile can be
// captured.
type heapGrowth struct {
	runner

	mu       sync.Mutex
	retained []*Record
	rate     int
	capMiB   int
	capped   bool
}

func newHeapGrowth() *heapGrowth {
	h := &heapGrowth{}
	h.name = h.Name()
	return h
}

func (*heapGrowth) Name() string { return "heap-growth" }

func (*heapGrowth) Description() string {
	return "Retains 1 KiB Record structs at a steady rate until cap_mib is " +
		"reached. The heap profile (inuse_space, inuse_objects) attributes " +
		"the bytes to retainBlob."
}

func (*heapGrowth) Knobs() []Knob {
	return []Knob{
		{Name: "mib_per_second", Unit: "MiB/s", Default: 4, Min: 1, Max: 64},
		{Name: "cap_mib", Unit: "MiB", Default: 512, Min: 1, Max: 1024},
	}
}

func (h *heapGrowth) Start(ctx context.Context, knobs map[string]any) error {
	vals, err := parseKnobs(knobs, h.Knobs())
	if err != nil {
		return err
	}
	rate, capMiB := vals["mib_per_second"], vals["cap_mib"]

	// The previous grower is stopped before its records are dropped, so it
	// cannot append to the slice after release.
	h.restart(ctx, 1, func() {
		h.release()
		h.mu.Lock()
		h.rate, h.capMiB = rate, capMiB
		h.mu.Unlock()
	}, func(ctx context.Context, _ int) {
		h.grow(ctx, rate, capMiB)
	})
	return nil
}

func (h *heapGrowth) Stop() {
	h.stop(h.release)
}

// Running is true while the grower goroutine is alive or while records are
// still retained after the cap was reached.
func (h *heapGrowth) Running() bool {
	if h.runner.Running() {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.retained) > 0
}

func (h *heapGrowth) State() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return map[string]any{
		"retained_mib":   len(h.retained) / recordsPerMiB,
		"records":        len(h.retained),
		"mib_per_second": h.rate,
		"cap_mib":        h.capMiB,
		"capped":         h.capped,
	}
}

// grow adds one MiB of records per tick until capMiB is retained or ctx is
// canceled. The tick interval is derived from rate so growth stays close to
// rate MiB per second regardless of allocation speed.
func (h *heapGrowth) grow(ctx context.Context, rate, capMiB int) {
	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if h.retainBlob(recordsPerMiB) >= capMiB {
			h.mu.Lock()
			h.capped = true
			h.mu.Unlock()
			return
		}
	}
}

// retainBlob allocates n Record values and appends them to the retained
// slice. It returns the retained size in MiB after the append.
//
//go:noinline
func (h *heapGrowth) retainBlob(n int) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	for range n {
		rec := &Record{ID: uint64(len(h.retained))}
		h.retained = append(h.retained, rec)
	}
	return len(h.retained) / recordsPerMiB
}

// release drops the retained records and clears the run's knobs. The
// memory is returned to the heap on the next GC cycle.
func (h *heapGrowth) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retained = nil
	h.capped = false
	h.rate, h.capMiB = 0, 0
}
