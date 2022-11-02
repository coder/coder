package harness

import (
	"context"
	"sync"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
)

// ExecutionStrategy defines how a TestHarness should execute a set of runs. It
// essentially defines the concurrency model for a given testing session.
type ExecutionStrategy interface {
	// Execute runs the given runs in whatever way the strategy wants. An error
	// may only be returned if the strategy has a failure itself, not if any of
	// the runs fail.
	Execute(ctx context.Context, runs []*TestRun) error
}

// TestHarness runs a bunch of registered test runs using the given
// ExecutionStrategy.
type TestHarness struct {
	strategy ExecutionStrategy

	mut     *sync.Mutex
	runIDs  map[string]struct{}
	runs    []*TestRun
	started bool
	done    chan struct{}
}

// NewTestHarness creates a new TestHarness with the given ExecutionStrategy.
func NewTestHarness(strategy ExecutionStrategy) *TestHarness {
	return &TestHarness{
		strategy: strategy,
		mut:      new(sync.Mutex),
		runIDs:   map[string]struct{}{},
		runs:     []*TestRun{},
		done:     make(chan struct{}),
	}
}

// Run runs the registered tests using the given ExecutionStrategy. The provided
// context can be used to cancel or set a deadline for the test run. Blocks
// until the tests have finished and returns the test execution error (not
// individual run errors).
//
// Panics if called more than once.
func (h *TestHarness) Run(ctx context.Context) (err error) {
	h.mut.Lock()
	if h.started {
		h.mut.Unlock()
		panic("harness is already started")
	}
	h.started = true
	h.mut.Unlock()

	defer close(h.done)
	defer func() {
		e := recover()
		if e != nil {
			err = xerrors.Errorf("execution strategy panicked: %w", e)
		}
	}()

	err = h.strategy.Execute(ctx, h.runs)
	//nolint:revive // we use named returns because we mutate it in a defer
	return
}

// Cleanup should be called after the test run has finished and results have
// been collected.
func (h *TestHarness) Cleanup(ctx context.Context) (err error) {
	h.mut.Lock()
	defer h.mut.Unlock()
	if !h.started {
		panic("harness has not started")
	}
	select {
	case <-h.done:
	default:
		panic("harness has not finished")
	}

	defer func() {
		e := recover()
		if e != nil {
			err = multierror.Append(err, xerrors.Errorf("panic in cleanup: %w", e))
		}
	}()

	for _, run := range h.runs {
		e := run.Cleanup(ctx)
		if e != nil {
			err = multierror.Append(err, xerrors.Errorf("cleanup for %s failed: %w", run.FullID(), e))
		}
	}

	//nolint:revive // we use named returns because we mutate it in a defer
	return
}
