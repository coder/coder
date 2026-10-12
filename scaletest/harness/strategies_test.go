package harness_test

import (
	"context"
	"io"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/testutil"
)

//nolint:paralleltest // this tests uses timings to determine if it's working
func Test_LinearExecutionStrategy(t *testing.T) {
	var (
		lastSeenI atomic.Int64
		count     atomic.Int64
	)
	lastSeenI.Store(-1)
	runs, fns := strategyTestData(100, func(_ context.Context, i int, _ io.Writer) error {
		count.Add(1)
		swapped := lastSeenI.CompareAndSwap(int64(i-1), int64(i))
		assert.True(t, swapped)
		time.Sleep(2 * time.Millisecond)

		if i%2 == 0 {
			return xerrors.New("error")
		}
		return nil
	})

	strategy := harness.LinearExecutionStrategy{}
	runErrs, err := strategy.Run(context.Background(), fns)
	require.NoError(t, err)
	require.Len(t, runErrs, 50)
	require.EqualValues(t, 100, count.Load())

	lastStartTime := time.Time{}
	for _, run := range runs {
		startTime := run.Result().StartedAt
		require.True(t, startTime.After(lastStartTime))
		lastStartTime = startTime
	}
}

func Test_ConcurrentExecutionStrategy(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	// Every run blocks until all of them have started, which only a
	// strategy that runs them all at once can satisfy.
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	_, fns := strategyTestData(10, func(ctx context.Context, i int, _ io.Writer) error {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		if i%2 == 0 {
			return xerrors.New("error")
		}
		return nil
	})
	strategy := harness.ConcurrentExecutionStrategy{}

	type result struct {
		runErrs []error
		err     error
	}
	resultC := make(chan result, 1)
	go func() {
		runErrs, err := strategy.Run(ctx, fns)
		resultC <- result{runErrs, err}
	}()

	for range 10 {
		testutil.RequireReceive(ctx, t, started)
	}
	close(release)

	res := testutil.RequireReceive(ctx, t, resultC)
	require.NoError(t, res.err)
	require.Len(t, res.runErrs, 5)
}

func Test_ParallelExecutionStrategy(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	// Runs block until released, so the number in flight while blocked is
	// exactly the concurrency the strategy allows.
	var inFlight, maxInFlight atomic.Int64
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	_, fns := strategyTestData(10, func(ctx context.Context, i int, _ io.Writer) error {
		cur := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			prev := maxInFlight.Load()
			if cur <= prev || maxInFlight.CompareAndSwap(prev, cur) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		if i%2 == 0 {
			return xerrors.New("error")
		}
		return nil
	})
	strategy := harness.ParallelExecutionStrategy{
		Limit: 5,
	}

	type result struct {
		runErrs []error
		err     error
	}
	resultC := make(chan result, 1)
	go func() {
		runErrs, err := strategy.Run(ctx, fns)
		resultC <- result{runErrs, err}
	}()

	for range 5 {
		testutil.RequireReceive(ctx, t, started)
	}
	close(release)

	res := testutil.RequireReceive(ctx, t, resultC)
	require.NoError(t, res.err)
	require.Len(t, res.runErrs, 5)
	require.EqualValues(t, 5, maxInFlight.Load())
}

//nolint:paralleltest // this tests uses timings to determine if it's working
func Test_TimeoutExecutionStrategy(t *testing.T) {
	runs, fns := strategyTestData(1, func(ctx context.Context, _ int, _ io.Writer) error {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			return xerrors.New("context wasn't canceled")
		}
	})
	strategy := harness.TimeoutExecutionStrategyWrapper{
		Timeout: 100 * time.Millisecond,
		Inner:   harness.LinearExecutionStrategy{},
	}

	runErrs, err := strategy.Run(context.Background(), fns)
	require.NoError(t, err)
	require.Len(t, runErrs, 0)

	for _, run := range runs {
		require.NoError(t, run.Result().Error)
	}
}

//nolint:paralleltest // this tests uses timings to determine if it's working
func Test_ShuffleExecutionStrategyWrapper(t *testing.T) {
	runs, fns := strategyTestData(100000, func(_ context.Context, i int, _ io.Writer) error {
		// t.Logf("run %d", i)
		return nil
	})
	strategy := harness.ShuffleExecutionStrategyWrapper{
		Inner: harness.LinearExecutionStrategy{},
	}

	runErrs, err := strategy.Run(context.Background(), fns)
	require.NoError(t, err)
	require.Len(t, runErrs, 0)

	// Ensure not in order by sorting the start time of each run.
	unsortedTimes := make([]time.Time, len(runs))
	for i, run := range runs {
		unsortedTimes[i] = run.Result().StartedAt
	}

	sortedTimes := make([]time.Time, len(runs))
	copy(sortedTimes, unsortedTimes)
	sort.Slice(sortedTimes, func(i, j int) bool {
		return sortedTimes[i].Before(sortedTimes[j])
	})

	require.NotEqual(t, unsortedTimes, sortedTimes)
}

func strategyTestData(count int, runFn func(ctx context.Context, i int, logs io.Writer) error) ([]*harness.TestRun, []harness.TestFn) {
	var (
		runs = make([]*harness.TestRun, count)
		fns  = make([]harness.TestFn, count)
	)
	for i := 0; i < count; i++ {
		runs[i] = harness.NewTestRun("test", strconv.Itoa(i), testFns{
			RunFn: func(ctx context.Context, id string, logs io.Writer) error {
				if runFn != nil {
					return runFn(ctx, i, logs)
				}
				return nil
			},
		})
		fns[i] = runs[i].Run
	}

	return runs, fns
}
