package testutil

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
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
func RunRetry(ctx context.Context, t *testing.T, count int, fn func(t testing.TB)) {
	t.Helper()

	for i := 1; i <= count; i++ {
		select {
		case <-ctx.Done():
			t.Fatalf("testutil.RunRetry: %s", ctx.Err())
		default:
		}

		attemptT := &fakeT{
			T:    t,
			ctx:  ctx,
			name: fmt.Sprintf("%s (attempt %d/%d)", t.Name(), i, count),
		}

		// Run the test in a goroutine so we can capture runtime.Goexit()
		// and run cleanup functions.
		done := make(chan struct{}, 1)
		go func() {
			defer close(done)
			defer func() {
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
	}
	t.Fatalf("testutil.RunRetry: all %d attempts failed", count)
}

// fakeT is a fake implementation of testing.TB that never fails and only logs
// errors. Fatal errors will cause the goroutine to exit without failing the
// test.
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
	cleanupFns := make([]func(), len(t.cleanupFns))
	copy(cleanupFns, t.cleanupFns)
	t.mu.Unlock()

	// Iterate in reverse order to match the behavior of *testing.T.
	for i := len(cleanupFns) - 1; i >= 0; i-- {
		cleanupFns[i]()
	}
}

// Chdir implements testing.TB.
func (*fakeT) Chdir(_ string) {
	panic("t.Chdir is not implemented in testutil.RunRetry closures")
}

// Cleanup implements testing.TB.
func (t *fakeT) Cleanup(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cleanupFns = append(t.cleanupFns, fn)
}

// Context implements testing.TB.
func (t *fakeT) Context() context.Context {
	return t.ctx
}

// Error implements testing.TB.
func (t *fakeT) Error(args ...any) {
	t.T.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.T.Log(append([]any{"WARN: t.Error called in testutil.RunRetry closure:"}, args...)...)
}

// Errorf implements testing.TB.
func (t *fakeT) Errorf(format string, args ...any) {
	t.T.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.T.Logf("WARN: t.Errorf called in testutil.RunRetry closure: "+format, args...)
}

// Fail implements testing.TB.
func (t *fakeT) Fail() {
	t.T.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.T.Log("WARN: t.Fail called in testutil.RunRetry closure")
	runtime.Goexit()
}

// FailNow implements testing.TB.
func (t *fakeT) FailNow() {
	t.T.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.T.Log("WARN: t.FailNow called in testutil.RunRetry closure")
	runtime.Goexit()
}

// Failed implements testing.TB.
func (t *fakeT) Failed() bool {
	t.T.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failed
}

// Fatal implements testing.TB.
func (t *fakeT) Fatal(args ...any) {
	t.T.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.T.Log(append([]any{"WARN: t.Fatal called in testutil.RunRetry closure:"}, args...)...)
	runtime.Goexit()
}

// Fatalf implements testing.TB.
func (t *fakeT) Fatalf(format string, args ...any) {
	t.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.T.Logf("WARN: t.Fatalf called in testutil.RunRetry closure: "+format, args...)
	runtime.Goexit()
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
