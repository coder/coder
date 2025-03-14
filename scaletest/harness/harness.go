package harness
import (
	"fmt"
	"errors"
	"context"
	"sync"
	"time"
	"github.com/hashicorp/go-multierror"
	"github.com/coder/coder/v2/coderd/tracing"
)
// TestHarness runs a bunch of registered test runs using the given execution
// strategies.
type TestHarness struct {
	runStrategy     ExecutionStrategy
	cleanupStrategy ExecutionStrategy
	mut     *sync.Mutex
	runIDs  map[string]struct{}
	runs    []*TestRun
	started bool
	done    chan struct{}
	elapsed time.Duration
}
// NewTestHarness creates a new TestHarness with the given execution strategies.
func NewTestHarness(runStrategy, cleanupStrategy ExecutionStrategy) *TestHarness {
	return &TestHarness{
		runStrategy:     runStrategy,
		cleanupStrategy: cleanupStrategy,
		mut:             new(sync.Mutex),
		runIDs:          map[string]struct{}{},
		runs:            []*TestRun{},
		done:            make(chan struct{}),
	}
}
// Run runs the registered tests using the given ExecutionStrategy. The provided
// context can be used to cancel or set a deadline for the test run. Blocks
// until the tests have finished and returns the test execution error (not
// individual run errors).
//
// Panics if called more than once.
func (h *TestHarness) Run(ctx context.Context) (err error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	h.mut.Lock()
	if h.started {
		h.mut.Unlock()
		panic("harness is already started")
	}
	h.started = true
	h.mut.Unlock()
	runFns := make([]TestFn, len(h.runs))
	for i, run := range h.runs {
		runFns[i] = run.Run
	}
	defer close(h.done)
	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("panic in harness.Run: %+v", e)
		}
	}()
	start := time.Now()
	defer func() {
		h.mut.Lock()
		defer h.mut.Unlock()
		h.elapsed = time.Since(start)
	}()
	// We don't care about test failures here since they already get recorded
	// by the *TestRun.
	_, err = h.runStrategy.Run(ctx, runFns)
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
	cleanupFns := make([]TestFn, len(h.runs))
	for i, run := range h.runs {
		cleanupFns[i] = run.Cleanup
	}
	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("panic in harness.Cleanup: %+v", e)
		}
	}()
	var cleanupErrs []error
	cleanupErrs, err = h.cleanupStrategy.Run(ctx, cleanupFns)
	if err != nil {
		err = fmt.Errorf("cleanup strategy error: %w", err)
		//nolint:revive // we use named returns because we mutate it in a defer
		return
	}
	var merr error
	for _, cleanupErr := range cleanupErrs {
		if cleanupErr != nil {
			merr = multierror.Append(merr, cleanupErr)
		}
	}
	err = merr
	//nolint:revive // we use named returns because we mutate it in a defer
	return
}
