package harness
import (
	"fmt"
	"errors"
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"sync"
	"time"
)
// TestFn is a function that can be run by an ExecutionStrategy.
type TestFn func(ctx context.Context) error
// ExecutionStrategy defines how a TestHarness should execute a set of runs. It
// essentially defines the concurrency model for a given testing session.
type ExecutionStrategy interface {
	// Execute calls each function in whatever way the strategy wants. All
	// errors returned from the function should be wrapped and returned, but all
	// given functions must be executed.
	Run(ctx context.Context, fns []TestFn) ([]error, error)
}
// LinearExecutionStrategy executes all test runs in a linear fashion, one after
// the other.
type LinearExecutionStrategy struct{}
var _ ExecutionStrategy = LinearExecutionStrategy{}
// Run implements ExecutionStrategy.
func (LinearExecutionStrategy) Run(ctx context.Context, fns []TestFn) ([]error, error) {
	var errs []error
	for i, fn := range fns {
		err := fn(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %d: %w", i, err))
		}
	}
	return errs, nil
}
// ConcurrentExecutionStrategy executes all test runs concurrently without any
// regard for parallelism.
type ConcurrentExecutionStrategy struct{}
var _ ExecutionStrategy = ConcurrentExecutionStrategy{}
// Run implements ExecutionStrategy.
func (ConcurrentExecutionStrategy) Run(ctx context.Context, fns []TestFn) ([]error, error) {
	var (
		wg   sync.WaitGroup
		errs = newErrorsList()
	)
	for i, fn := range fns {
		i, fn := i, fn
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := fn(ctx)
			if err != nil {
				errs.add(fmt.Errorf("run %d: %w", i, err))
			}
		}()
	}
	wg.Wait()
	return errs.errs, nil
}
// ParallelExecutionStrategy executes all test runs concurrently, but limits the
// number of concurrent runs to the given limit.
type ParallelExecutionStrategy struct {
	Limit int
}
var _ ExecutionStrategy = ParallelExecutionStrategy{}
// Run implements ExecutionStrategy.
func (p ParallelExecutionStrategy) Run(ctx context.Context, fns []TestFn) ([]error, error) {
	var (
		wg   sync.WaitGroup
		errs = newErrorsList()
		sem  = make(chan struct{}, p.Limit)
	)
	defer close(sem)
	for i, fn := range fns {
		i, fn := i, fn
		wg.Add(1)
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()
			sem <- struct{}{}
			err := fn(ctx)
			if err != nil {
				errs.add(fmt.Errorf("run %d: %w", i, err))
			}
		}()
	}
	wg.Wait()
	return errs.errs, nil
}
// TimeoutExecutionStrategyWrapper is an ExecutionStrategy that wraps another
// ExecutionStrategy and applies a timeout to each test run's context.
type TimeoutExecutionStrategyWrapper struct {
	Timeout time.Duration
	Inner   ExecutionStrategy
}
var _ ExecutionStrategy = TimeoutExecutionStrategyWrapper{}
// Run implements ExecutionStrategy.
func (t TimeoutExecutionStrategyWrapper) Run(ctx context.Context, fns []TestFn) ([]error, error) {
	newFns := make([]TestFn, len(fns))
	for i, fn := range fns {
		fn := fn
		newFns[i] = func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, t.Timeout)
			defer cancel()
			return fn(ctx)
		}
	}
	return t.Inner.Run(ctx, newFns)
}
// ShuffleExecutionStrategyWrapper is an ExecutionStrategy that wraps another
// ExecutionStrategy and shuffles the order of the test runs before executing.
type ShuffleExecutionStrategyWrapper struct {
	Inner ExecutionStrategy
}
var _ ExecutionStrategy = ShuffleExecutionStrategyWrapper{}
type cryptoRandSource struct{}
var _ rand.Source = cryptoRandSource{}
func (cryptoRandSource) Int63() int64 {
	var b [8]byte
	_, err := cryptorand.Read(b[:])
	if err != nil {
		panic(err)
	}
	// mask off sign bit to ensure positive number
	return int64(binary.LittleEndian.Uint64(b[:]) & (1<<63 - 1))
}
func (cryptoRandSource) Seed(_ int64) {}
// Run implements ExecutionStrategy.
func (s ShuffleExecutionStrategyWrapper) Run(ctx context.Context, fns []TestFn) ([]error, error) {
	shuffledFns := make([]TestFn, len(fns))
	copy(shuffledFns, fns)
	//nolint:gosec // gosec thinks we're using an insecure RNG, but we're not.
	src := rand.New(cryptoRandSource{})
	for i := range shuffledFns {
		j := src.Intn(i + 1)
		shuffledFns[i], shuffledFns[j] = shuffledFns[j], shuffledFns[i]
	}
	return s.Inner.Run(ctx, shuffledFns)
}
type errorsList struct {
	mut  *sync.Mutex
	errs []error
}
func newErrorsList() *errorsList {
	return &errorsList{
		mut:  &sync.Mutex{},
		errs: []error{},
	}
}
func (l *errorsList) add(err error) {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.errs = append(l.errs, err)
}
