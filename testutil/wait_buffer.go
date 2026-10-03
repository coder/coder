package testutil

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
)

// WaitBuffer is a thread-safe buffer (io.Writer) that supports
// blocking until the accumulated content matches a condition.
// It is intended for tests that need to wait for specific output
// from a command or process before proceeding.
//
// WaitBuffer is safe for concurrent use. Multiple goroutines may
// write to it, and WaitFor/WaitForCond may be called from any
// goroutine.
type WaitBuffer struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	waiters []*wbWaiter
}

type wbWaiter struct {
	cond func(string) bool
	ch   chan struct{}
	once sync.Once
}

// NewWaitBuffer returns a new WaitBuffer. It can be used as a
// plain thread-safe io.Writer even if WaitFor is never called.
func NewWaitBuffer() *WaitBuffer {
	return &WaitBuffer{}
}

// Write implements io.Writer. It is safe for concurrent use.
func (wb *WaitBuffer) Write(p []byte) (int, error) {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	n, err := wb.buf.Write(p)
	s := wb.buf.String()
	for _, w := range wb.waiters {
		if w.cond(s) {
			w.once.Do(func() { close(w.ch) })
		}
	}
	return n, err
}

// WaitFor blocks until the accumulated output contains signal or
// ctx expires. Returns nil on match, ctx.Err() on timeout.
// Safe to call from any goroutine.
func (wb *WaitBuffer) WaitFor(ctx context.Context, signal string) error {
	return wb.WaitForNth(ctx, signal, 1)
}

// WaitForNth blocks until the accumulated output contains at least
// n occurrences of signal, or ctx expires. Returns nil on match,
// ctx.Err() on timeout. Safe to call from any goroutine.
func (wb *WaitBuffer) WaitForNth(ctx context.Context, signal string, n int) error {
	return wb.WaitForCond(ctx, func(s string) bool {
		return strings.Count(s, signal) >= n
	})
}

// WaitForCond blocks until cond returns true for the accumulated
// output, or ctx expires. Returns nil on match, ctx.Err() on
// timeout. Safe to call from any goroutine.
func (wb *WaitBuffer) WaitForCond(ctx context.Context, cond func(string) bool) error {
	wb.mu.Lock()
	if cond(wb.buf.String()) {
		wb.mu.Unlock()
		return nil
	}
	w := &wbWaiter{
		cond: cond,
		ch:   make(chan struct{}),
	}
	wb.waiters = append(wb.waiters, w)
	wb.mu.Unlock()

	select {
	case <-w.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RequireWaitFor blocks until the accumulated output contains
// signal or ctx expires. On timeout, fails the test with a
// message showing what was expected and what was written so far.
//
// Safety: Must only be called from the Go routine that created
// `t`.
func (wb *WaitBuffer) RequireWaitFor(ctx context.Context, t testing.TB, signal string) {
	t.Helper()
	if err := wb.WaitFor(ctx, signal); err != nil {
		t.Fatalf("WaitBuffer: signal %q not found; buffer contents:\n%s", signal, wb.String())
	}
}

// Bytes returns a copy of the accumulated output.
func (wb *WaitBuffer) Bytes() []byte {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	return bytes.Clone(wb.buf.Bytes())
}

// String returns the accumulated output as a string.
func (wb *WaitBuffer) String() string {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	return wb.buf.String()
}
