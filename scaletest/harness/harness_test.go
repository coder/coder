package harness_test
import (
	"errors"
	"context"
	"io"
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/coder/coder/v2/scaletest/harness"
)
const testPanicMessage = "expected test panic"
type panickingExecutionStrategy struct{}
var _ harness.ExecutionStrategy = panickingExecutionStrategy{}
func (panickingExecutionStrategy) Run(_ context.Context, _ []harness.TestFn) ([]error, error) {
	panic(testPanicMessage)
}
type erroringExecutionStrategy struct {
	err error
}
var _ harness.ExecutionStrategy = erroringExecutionStrategy{}
func (e erroringExecutionStrategy) Run(_ context.Context, _ []harness.TestFn) ([]error, error) {
	return []error{}, e.err
}
func Test_TestHarness(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("expected error")
		h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
		r1 := h.AddRun("test", "1", fakeTestFns(nil, nil))
		r2 := h.AddRun("test", "2", fakeTestFns(expectedErr, nil))
		err := h.Run(context.Background())
		require.NoError(t, err)
		res := h.Results()
		require.Equal(t, 2, res.TotalRuns)
		require.Equal(t, 1, res.TotalPass)
		require.Equal(t, 1, res.TotalFail)
		require.Equal(t, map[string]harness.RunResult{
			r1.FullID(): r1.Result(),
			r2.FullID(): r2.Result(),
		}, res.Runs)
		err = h.Cleanup(context.Background())
		require.NoError(t, err)
	})
	t.Run("CatchesExecutionError", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("expected error")
		h := harness.NewTestHarness(erroringExecutionStrategy{err: expectedErr}, harness.LinearExecutionStrategy{})
		_ = h.AddRun("test", "1", fakeTestFns(nil, nil))
		err := h.Run(context.Background())
		require.Error(t, err)
		require.ErrorIs(t, err, expectedErr)
	})
	t.Run("CatchesExecutionPanic", func(t *testing.T) {
		t.Parallel()
		h := harness.NewTestHarness(panickingExecutionStrategy{}, harness.LinearExecutionStrategy{})
		_ = h.AddRun("test", "1", fakeTestFns(nil, nil))
		err := h.Run(context.Background())
		require.Error(t, err)
		require.ErrorContains(t, err, "panic")
		require.ErrorContains(t, err, testPanicMessage)
	})
	t.Run("Cleanup", func(t *testing.T) {
		t.Parallel()
		t.Run("Error", func(t *testing.T) {
			t.Parallel()
			expectedErr := errors.New("expected error")
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
			_ = h.AddRun("test", "1", fakeTestFns(nil, expectedErr))
			err := h.Run(context.Background())
			require.NoError(t, err)
			err = h.Cleanup(context.Background())
			require.Error(t, err)
			require.ErrorContains(t, err, expectedErr.Error())
		})
		t.Run("Panic", func(t *testing.T) {
			t.Parallel()
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
			_ = h.AddRun("test", "1", testFns{
				RunFn: func(_ context.Context, _ string, _ io.Writer) error {
					return nil
				},
				CleanupFn: func(_ context.Context, _ string, _ io.Writer) error {
					panic(testPanicMessage)
				},
			})
			err := h.Run(context.Background())
			require.NoError(t, err)
			err = h.Cleanup(context.Background())
			require.Error(t, err)
			require.ErrorContains(t, err, "panic")
			require.ErrorContains(t, err, testPanicMessage)
		})
		t.Run("CatchesExecutionError", func(t *testing.T) {
			t.Parallel()
			expectedErr := errors.New("expected error")
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, erroringExecutionStrategy{err: expectedErr})
			_ = h.AddRun("test", "1", fakeTestFns(nil, nil))
			err := h.Run(context.Background())
			require.NoError(t, err)
			err = h.Cleanup(context.Background())
			require.Error(t, err)
			require.ErrorIs(t, err, expectedErr)
		})
		t.Run("CatchesExecutionPanic", func(t *testing.T) {
			t.Parallel()
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, panickingExecutionStrategy{})
			_ = h.AddRun("test", "1", testFns{
				RunFn: func(_ context.Context, _ string, _ io.Writer) error {
					return nil
				},
				CleanupFn: func(_ context.Context, _ string, _ io.Writer) error {
					return nil
				},
			})
			err := h.Run(context.Background())
			require.NoError(t, err)
			err = h.Cleanup(context.Background())
			require.Error(t, err)
			require.ErrorContains(t, err, "panic")
			require.ErrorContains(t, err, testPanicMessage)
		})
	})
	t.Run("Panics", func(t *testing.T) {
		t.Parallel()
		t.Run("RegisterAfterStart", func(t *testing.T) {
			t.Parallel()
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
			_ = h.Run(context.Background())
			require.Panics(t, func() {
				_ = h.AddRun("test", "1", fakeTestFns(nil, nil))
			})
		})
		t.Run("DuplicateTestID", func(t *testing.T) {
			t.Parallel()
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
			name, id := "test", "1"
			_ = h.AddRun(name, id, fakeTestFns(nil, nil))
			require.Panics(t, func() {
				_ = h.AddRun(name, id, fakeTestFns(nil, nil))
			})
		})
		t.Run("StartedTwice", func(t *testing.T) {
			t.Parallel()
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
			h.Run(context.Background())
			require.Panics(t, func() {
				h.Run(context.Background())
			})
		})
		t.Run("ResultsBeforeStart", func(t *testing.T) {
			t.Parallel()
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
			require.Panics(t, func() {
				h.Results()
			})
		})
		t.Run("ResultsBeforeFinish", func(t *testing.T) {
			t.Parallel()
			var (
				started    = make(chan struct{})
				endRun     = make(chan struct{})
				testsEnded = make(chan struct{})
			)
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
			_ = h.AddRun("test", "1", testFns{
				RunFn: func(_ context.Context, _ string, _ io.Writer) error {
					close(started)
					<-endRun
					return nil
				},
			})
			go func() {
				defer close(testsEnded)
				err := h.Run(context.Background())
				assert.NoError(t, err)
			}()
			<-started
			require.Panics(t, func() {
				h.Results()
			})
			close(endRun)
			<-testsEnded
			_ = h.Results()
		})
		t.Run("CleanupBeforeStart", func(t *testing.T) {
			t.Parallel()
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
			require.Panics(t, func() {
				h.Cleanup(context.Background())
			})
		})
		t.Run("CleanupBeforeFinish", func(t *testing.T) {
			t.Parallel()
			var (
				started    = make(chan struct{})
				endRun     = make(chan struct{})
				testsEnded = make(chan struct{})
			)
			h := harness.NewTestHarness(harness.LinearExecutionStrategy{}, harness.LinearExecutionStrategy{})
			_ = h.AddRun("test", "1", testFns{
				RunFn: func(_ context.Context, _ string, _ io.Writer) error {
					close(started)
					<-endRun
					return nil
				},
			})
			go func() {
				defer close(testsEnded)
				err := h.Run(context.Background())
				assert.NoError(t, err)
			}()
			<-started
			require.Panics(t, func() {
				h.Cleanup(context.Background())
			})
			close(endRun)
			<-testsEnded
			err := h.Cleanup(context.Background())
			require.NoError(t, err)
		})
	})
}
func fakeTestFns(err, cleanupErr error) testFns {
	return testFns{
		RunFn: func(_ context.Context, _ string, _ io.Writer) error {
			return err
		},
		CleanupFn: func(_ context.Context, _ string, _ io.Writer) error {
			return cleanupErr
		},
	}
}
