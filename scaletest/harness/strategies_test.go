package harness_test
import (
	"errors"
	"context"
	"io"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/coder/coder/v2/scaletest/harness"
)
//nolint:paralleltest // this tests uses timings to determine if it's working
func Test_LinearExecutionStrategy(t *testing.T) {
	var (
		lastSeenI int64 = -1
		count     int64
	)
	runs, fns := strategyTestData(100, func(_ context.Context, i int, _ io.Writer) error {
		atomic.AddInt64(&count, 1)
		swapped := atomic.CompareAndSwapInt64(&lastSeenI, int64(i-1), int64(i))
		assert.True(t, swapped)
		time.Sleep(2 * time.Millisecond)
		if i%2 == 0 {
			return errors.New("error")
		}
		return nil
	})
	strategy := harness.LinearExecutionStrategy{}
	runErrs, err := strategy.Run(context.Background(), fns)
	require.NoError(t, err)
	require.Len(t, runErrs, 50)
	require.EqualValues(t, 100, atomic.LoadInt64(&count))
	lastStartTime := time.Time{}
	for _, run := range runs {
		startTime := run.Result().StartedAt
		require.True(t, startTime.After(lastStartTime))
		lastStartTime = startTime
	}
}
//nolint:paralleltest // this tests uses timings to determine if it's working
func Test_ConcurrentExecutionStrategy(t *testing.T) {
	runs, fns := strategyTestData(10, func(_ context.Context, i int, _ io.Writer) error {
		time.Sleep(1 * time.Second)
		if i%2 == 0 {
			return errors.New("error")
		}
		return nil
	})
	strategy := harness.ConcurrentExecutionStrategy{}
	startTime := time.Now()
	runErrs, err := strategy.Run(context.Background(), fns)
	require.NoError(t, err)
	require.Len(t, runErrs, 5)
	// Should've taken at least 900ms to run but less than 5 seconds.
	require.True(t, time.Since(startTime) > 900*time.Millisecond)
	require.True(t, time.Since(startTime) < 5*time.Second)
	// All tests should've started within 500 ms of the start time.
	endTime := startTime.Add(500 * time.Millisecond)
	for _, run := range runs {
		runStartTime := run.Result().StartedAt
		require.WithinRange(t, runStartTime, startTime, endTime)
	}
}
//nolint:paralleltest // this tests uses timings to determine if it's working
func Test_ParallelExecutionStrategy(t *testing.T) {
	runs, fns := strategyTestData(10, func(_ context.Context, i int, _ io.Writer) error {
		time.Sleep(1 * time.Second)
		if i%2 == 0 {
			return errors.New("error")
		}
		return nil
	})
	strategy := harness.ParallelExecutionStrategy{
		Limit: 5,
	}
	startTime := time.Now()
	time.Sleep(time.Millisecond)
	runErrs, err := strategy.Run(context.Background(), fns)
	require.NoError(t, err)
	require.Len(t, runErrs, 5)
	// Should've taken at least 1900ms to run but less than 8 seconds.
	require.True(t, time.Since(startTime) > 1900*time.Millisecond)
	require.True(t, time.Since(startTime) < 8*time.Second)
	// Any five of the tests should've started within 500 ms of the start time.
	endTime := startTime.Add(500 * time.Millisecond)
	withinRange := 0
	for _, run := range runs {
		runStartTime := run.Result().StartedAt
		if runStartTime.After(startTime) && runStartTime.Before(endTime) {
			withinRange++
		}
	}
	require.Equal(t, 5, withinRange)
	// The other 5 tests should've started between 900ms and 1.5s after the
	// start time.
	startTime = startTime.Add(900 * time.Millisecond)
	endTime = startTime.Add(600 * time.Millisecond)
	withinRange = 0
	for _, run := range runs {
		runStartTime := run.Result().StartedAt
		if runStartTime.After(startTime) && runStartTime.Before(endTime) {
			withinRange++
		}
	}
	require.Equal(t, 5, withinRange)
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
			return errors.New("context wasn't canceled")
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
		i := i
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
