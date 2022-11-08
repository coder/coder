package harness

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"io"
	"math/rand"
	"sync"
	"time"
)

// LinearExecutionStrategy executes all test runs in a linear fashion, one after
// the other.
type LinearExecutionStrategy struct{}

var _ ExecutionStrategy = LinearExecutionStrategy{}

// Execute implements ExecutionStrategy.
func (LinearExecutionStrategy) Execute(ctx context.Context, runs []*TestRun) error {
	for _, run := range runs {
		_ = run.Run(ctx)
	}

	return nil
}

// ConcurrentExecutionStrategy executes all test runs concurrently without any
// regard for parallelism.
type ConcurrentExecutionStrategy struct{}

var _ ExecutionStrategy = ConcurrentExecutionStrategy{}

// Execute implements ExecutionStrategy.
func (ConcurrentExecutionStrategy) Execute(ctx context.Context, runs []*TestRun) error {
	var wg sync.WaitGroup
	for _, run := range runs {
		run := run

		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = run.Run(ctx)
		}()
	}

	wg.Wait()
	return nil
}

// ParallelExecutionStrategy executes all test runs concurrently, but limits the
// number of concurrent runs to the given limit.
type ParallelExecutionStrategy struct {
	Limit int
}

var _ ExecutionStrategy = ParallelExecutionStrategy{}

// Execute implements ExecutionStrategy.
func (p ParallelExecutionStrategy) Execute(ctx context.Context, runs []*TestRun) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, p.Limit)
	defer close(sem)

	for _, run := range runs {
		run := run

		wg.Add(1)
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()
			sem <- struct{}{}
			_ = run.Run(ctx)
		}()
	}

	wg.Wait()
	return nil
}

// TimeoutExecutionStrategyWrapper is an ExecutionStrategy that wraps another
// ExecutionStrategy and applies a timeout to each test run's context.
type TimeoutExecutionStrategyWrapper struct {
	Timeout time.Duration
	Inner   ExecutionStrategy
}

var _ ExecutionStrategy = TimeoutExecutionStrategyWrapper{}

type timeoutRunnerWrapper struct {
	timeout time.Duration
	inner   Runnable
}

var _ Runnable = timeoutRunnerWrapper{}

func (t timeoutRunnerWrapper) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	return t.inner.Run(ctx, id, logs)
}

// Execute implements ExecutionStrategy.
func (t TimeoutExecutionStrategyWrapper) Execute(ctx context.Context, runs []*TestRun) error {
	for _, run := range runs {
		oldRunner := run.runner
		run.runner = timeoutRunnerWrapper{
			timeout: t.Timeout,
			inner:   oldRunner,
		}
	}

	return t.Inner.Execute(ctx, runs)
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

// Execute implements ExecutionStrategy.
func (s ShuffleExecutionStrategyWrapper) Execute(ctx context.Context, runs []*TestRun) error {
	shuffledRuns := make([]*TestRun, len(runs))
	copy(shuffledRuns, runs)

	//nolint:gosec // gosec thinks we're using an insecure RNG, but we're not.
	src := rand.New(cryptoRandSource{})
	for i := range shuffledRuns {
		j := src.Intn(i + 1)
		shuffledRuns[i], shuffledRuns[j] = shuffledRuns[j], shuffledRuns[i]
	}

	return s.Inner.Execute(ctx, shuffledRuns)
}
