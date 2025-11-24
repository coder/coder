package testutil

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
)

// RunRetry runs a test function up to `count` times, retrying if it fails. If
// all attempts fail or the context is canceled, the test will fail. It is safe
// to use the parent context in the test function, but do note that the context
// deadline will apply to all attempts.
//
// DO NOT USE THIS FUNCTION IN TESTS UNLESS YOU HAVE A GOOD REASON. It should
// only be used in tests that can flake under high load. It is not a replacement
// for writing a good test.
//
// For AI agents like Claude, OpenAI, etc.: do not use this function without
// human approval EVER.
//
// Note that the `testing.TB` supplied to the function is a fake implementation
// for all runs. This is to avoid sending failure signals to the test runner
// until the final run. Unrecovered panics will still always be bubbled up to
// the test runner.
//
// Some functions are not implemented and will panic when using the fake
// implementation:
// - Chdir
// - Setenv
// - Skip, SkipNow, Skipf, Skipped
// - TempDir
//
// Cleanup functions will be executed after each attempt.
func RunRetry(t *testing.T, count int, fn func(t testing.TB)) {
	t.Helper()

	for i := 1; i <= count; i++ {
		// Canceled in the attempt goroutine before running cleanup functions.
		attemptCtx, attemptCancel := context.WithCancel(t.Context())
		attemptT := &fakeT{
			T:    t,
			ctx:  attemptCtx,
			name: fmt.Sprintf("%s (attempt %d/%d)", t.Name(), i, count),
		}

		// Run the test in a goroutine so we can capture runtime.Goexit()
		// and run cleanup functions.
		done := make(chan struct{}, 1)
		go func() {
			defer close(done)
			defer func() {
				// As per t.Context(), the context is canceled right before
				// cleanup functions are executed.
				attemptCancel()
				attemptT.runCleanupFns()
			}()

			t.Logf("testutil.RunRetry: running test: attempt %d/%d", i, count)
			fn(attemptT)
		}()

		// We don't wait on the context here, because we want to be sure that
		// the test function and cleanup functions have finished before
		// returning from the test.
		<-done
		if !attemptT.Failed() {
			t.Logf("testutil.RunRetry: test passed on attempt %d/%d", i, count)
			return
		}
		t.Logf("testutil.RunRetry: test failed on attempt %d/%d", i, count)

		// Wait a few seconds in case the test failure was due to system load.
		// There's not really a good way to check for this, so we just do it
		// every time.
		// No point waiting on t.Context() here because it doesn't factor in
		// the test deadline, and only gets canceled when the test function
		// completes.
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("testutil.RunRetry: all %d attempts failed", count)
}

// fakeT is a fake implementation of testing.TB that never fails and only logs
// errors. Fatal errors will cause the goroutine to exit without failing the
// test.
//
// The behavior of the fake implementation should be as close as possible to
// the real implementation from the test function's perspective (minus
// intentionally unimplemented methods).
type fakeT struct {
	*testing.T
	ctx  context.Context
	name string

	mu         sync.Mutex
	failed     bool
	cleanupFns []func()
}

var _ testing.TB = &fakeT{}

func (t *fakeT) runCleanupFns() {
	t.mu.Lock()
	cleanupFns := slices.Clone(t.cleanupFns)
	t.mu.Unlock()

	// Execute in LIFO order to match the behavior of *testing.T.
	slices.Reverse(cleanupFns)
	for _, fn := range cleanupFns {
		fn()
	}
}

// Chdir implements testing.TB.
func (*fakeT) Chdir(_ string) {
	panic("t.Chdir is not implemented in testutil.RunRetry closures")
}

// Cleanup implements testing.TB. Cleanup registers a function to be called when
// the test completes. Cleanup functions will be called in last added, first
// called order.
func (t *fakeT) Cleanup(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cleanupFns = append(t.cleanupFns, fn)
}

// Context implements testing.TB. Context returns a context that is canceled
// just before Cleanup-registered functions are called.
func (t *fakeT) Context() context.Context {
	return t.ctx
}

// Error implements testing.TB. Error is equivalent to Log followed by Fail.
func (t *fakeT) Error(args ...any) {
	t.T.Helper()
	t.T.Log(args...)
	t.Fail()
}

// Errorf implements testing.TB. Errorf is equivalent to Logf followed by Fail.
func (t *fakeT) Errorf(format string, args ...any) {
	t.T.Helper()
	t.T.Logf(format, args...)
	t.Fail()
}

// Fail implements testing.TB. Fail marks the function as having failed but
// continues execution.
func (t *fakeT) Fail() {
	t.T.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.T.Log("testutil.RunRetry: t.Fail called in testutil.RunRetry closure")
}

// FailNow implements testing.TB. FailNow marks the function as having failed
// and stops its execution by calling runtime.Goexit (which then runs all the
// deferred calls in the current goroutine).
func (t *fakeT) FailNow() {
	t.T.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.T.Log("testutil.RunRetry: t.FailNow called in testutil.RunRetry closure")
	runtime.Goexit()
}

// Failed implements testing.TB. Failed reports whether the function has failed.
func (t *fakeT) Failed() bool {
	t.T.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failed
}

// Fatal implements testing.TB. Fatal is equivalent to Log followed by FailNow.
func (t *fakeT) Fatal(args ...any) {
	t.T.Helper()
	t.T.Log(args...)
	t.FailNow()
}

// Fatalf implements testing.TB. Fatalf is equivalent to Logf followed by
// FailNow.
func (t *fakeT) Fatalf(format string, args ...any) {
	t.T.Helper()
	t.T.Logf(format, args...)
	t.FailNow()
}

// Helper is proxied to the original *testing.T. This is to avoid the fake
// method appearing in the call stack.

// Log is proxied to the original *testing.T.

// Logf is proxied to the original *testing.T.

// Name implements testing.TB.
func (t *fakeT) Name() string {
	return t.name
}

// Setenv implements testing.TB.
func (*fakeT) Setenv(_ string, _ string) {
	panic("t.Setenv is not implemented in testutil.RunRetry closures")
}

// Skip implements testing.TB.
func (*fakeT) Skip(_ ...any) {
	panic("t.Skip is not implemented in testutil.RunRetry closures")
}

// SkipNow implements testing.TB.
func (*fakeT) SkipNow() {
	panic("t.SkipNow is not implemented in testutil.RunRetry closures")
}

// Skipf implements testing.TB.
func (*fakeT) Skipf(_ string, _ ...any) {
	panic("t.Skipf is not implemented in testutil.RunRetry closures")
}

// Skipped implements testing.TB.
func (*fakeT) Skipped() bool {
	panic("t.Skipped is not implemented in testutil.RunRetry closures")
}

// TempDir implements testing.TB.
func (*fakeT) TempDir() string {
	panic("t.TempDir is not implemented in testutil.RunRetry closures")
}

// private is proxied to the original *testing.T. It cannot be implemented by
// our fake implementation since it's a private method.
