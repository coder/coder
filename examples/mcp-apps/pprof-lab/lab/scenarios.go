// Package lab implements the pathological workloads that pprof-lab exposes
// through its HTTP API. Each scenario is a stoppable workload whose hot
// functions carry descriptive names so they are easy to spot in pprof
// output.
package lab

import (
	"context"
	"runtime/pprof"
	"sync"
)

// Scenario is a stoppable workload with a fixed set of integer knobs.
//
// Start on a running scenario stops the current run first and starts a new
// one with the given knobs. Stop blocks until every goroutine the scenario
// launched has exited and any retained memory has been dropped.
type Scenario interface {
	Name() string
	Description() string
	Knobs() []Knob
	Start(ctx context.Context, knobs map[string]any) error
	Stop()
	Running() bool
	State() map[string]any
}

// runner owns the goroutines of one scenario run. It serializes Start and
// Stop, cancels the run's context on Stop, and exposes a channel that
// closes when every launched goroutine has returned.
type runner struct {
	name string

	// opMu serializes launch and stop so two concurrent Start calls cannot
	// both install a run and orphan one of them.
	opMu sync.Mutex

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// restart stops any previous run, calls reset, then starts n goroutines
// running fn. The op mutex is held throughout, so a concurrent Stop can
// neither run between the counter reset and the new run being installed
// nor observe the new run before reset has completed. Every goroutine
// carries the pprof label scenario=<name> so goroutine and CPU samples can
// be attributed to the scenario.
func (r *runner) restart(ctx context.Context, n int, reset func(), fn func(ctx context.Context, i int)) {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	r.stopLocked()
	if reset != nil {
		reset()
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var wg sync.WaitGroup
	labels := pprof.Labels("scenario", r.name)
	for i := range n {
		wg.Go(func() {
			pprof.Do(ctx, labels, func(ctx context.Context) {
				fn(ctx, i)
			})
		})
	}
	go func() {
		wg.Wait()
		close(done)
	}()

	r.mu.Lock()
	r.cancel, r.done = cancel, done
	r.mu.Unlock()
}

// stop cancels the current run, waits for its goroutines to exit, and then
// calls after (if non-nil) while still holding the op mutex.
func (r *runner) stop(after func()) {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	r.stopLocked()
	if after != nil {
		after()
	}
}

func (r *runner) stopLocked() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.cancel, r.done = nil, nil
	r.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

// Running reports whether a run has been launched and at least one of its
// goroutines is still alive.
func (r *runner) Running() bool {
	done := r.doneChan()
	if done == nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}

// doneChan returns the current run's completion channel, or nil when no
// run is installed.
func (r *runner) doneChan() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done == nil {
		return nil
	}
	return r.done
}

// Registry holds every scenario by name in a stable display order.
type Registry struct {
	order  []Scenario
	byName map[string]Scenario
}

// NewRegistry returns a registry populated with every scenario.
func NewRegistry() *Registry {
	r := &Registry{byName: make(map[string]Scenario)}
	for _, s := range []Scenario{
		newGoroutineLeak(),
		newHeapGrowth(),
		newAllocChurn(),
		newCPUBurn(),
		newMutexContention(),
		newBlockWait(),
	} {
		r.order = append(r.order, s)
		r.byName[s.Name()] = s
	}
	return r
}

// Get returns the scenario registered under name.
func (r *Registry) Get(name string) (Scenario, bool) {
	s, ok := r.byName[name]
	return s, ok
}

// All returns the scenarios in registration order.
func (r *Registry) All() []Scenario {
	return r.order
}

// StopAll stops every scenario and returns once all of them have released
// their goroutines and retained memory.
func (r *Registry) StopAll() {
	for _, s := range r.order {
		s.Stop()
	}
}

// bytesToMiB converts a byte count to whole MiB, rounding down.
func bytesToMiB(n int64) int64 {
	return n / (1 << 20)
}
