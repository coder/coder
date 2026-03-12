package testutil_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestWaitBuffer_WaitFor_Blocks(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	wb := testutil.NewWaitBuffer()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = wb.WaitFor(ctx, "hello")
	}()

	// Write the signal after the goroutine is blocking.
	_, err := wb.Write([]byte("hello"))
	require.NoError(t, err)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("WaitFor did not unblock after signal was written")
	}
}

func TestWaitBuffer_WaitFor_AlreadyPresent(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	wb := testutil.NewWaitBuffer()
	_, err := wb.Write([]byte("already here"))
	require.NoError(t, err)

	// Signal is already in the buffer; WaitFor returns immediately.
	require.NoError(t, wb.WaitFor(ctx, "already"))
}

func TestWaitBuffer_WaitFor_ContextExpired(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already expired.

	wb := testutil.NewWaitBuffer()
	err := wb.WaitFor(ctx, "never")
	require.ErrorIs(t, err, context.Canceled)
}

func TestWaitBuffer_WaitFor_MultipleWrites(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	wb := testutil.NewWaitBuffer()
	// Write partial content that doesn't satisfy the condition.
	_, err := wb.Write([]byte("hell"))
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = wb.WaitFor(ctx, "hello")
	}()

	// Complete the signal with a second write.
	_, err = wb.Write([]byte("o"))
	require.NoError(t, err)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("WaitFor did not unblock after multiple writes completed the signal")
	}
}

func TestWaitBuffer_WaitForCond(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	wb := testutil.NewWaitBuffer()
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Wait until the buffer has at least 10 bytes.
		_ = wb.WaitForCond(ctx, func(s string) bool {
			return len(s) >= 10
		})
	}()

	_, err := wb.Write([]byte("12345"))
	require.NoError(t, err)
	_, err = wb.Write([]byte("67890"))
	require.NoError(t, err)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("WaitForCond did not unblock when condition was met")
	}
}

func TestWaitBuffer_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	wb := testutil.NewWaitBuffer()
	var wg sync.WaitGroup
	const writers = 10
	const iterations = 100
	wg.Add(writers)
	for i := range writers {
		go func() {
			defer wg.Done()
			for j := range iterations {
				_, _ = wb.Write([]byte(fmt.Sprintf("w%d-%d ", i, j)))
			}
		}()
	}
	wg.Wait()

	// Every write should have landed; verify no data was lost by
	// checking the length is at least as large as expected.
	assert.GreaterOrEqual(t, len(wb.Bytes()), writers*iterations)
}

func TestWaitBuffer_WaitFor_BackgroundGoroutine(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Expire immediately.

	wb := testutil.NewWaitBuffer()

	// WaitFor from a background goroutine should return the
	// context error rather than calling t.Fatal.
	done := make(chan error, 1)
	go func() {
		done <- wb.WaitFor(ctx, "never")
	}()

	err := <-done
	require.ErrorIs(t, err, context.Canceled)
}

func TestWaitBuffer_SequentialWaits(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	wb := testutil.NewWaitBuffer()

	_, err := wb.Write([]byte("first "))
	require.NoError(t, err)
	require.NoError(t, wb.WaitFor(ctx, "first"))

	_, err = wb.Write([]byte("second"))
	require.NoError(t, err)
	require.NoError(t, wb.WaitFor(ctx, "second"))
}

func TestWaitBuffer_WaitForNth_Blocks(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	wb := testutil.NewWaitBuffer()
	_, err := wb.Write([]byte("Foo "))
	require.NoError(t, err)

	// First occurrence is already present, but we want two.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = wb.WaitForNth(ctx, "Foo", 2)
	}()

	_, err = wb.Write([]byte("Bar Foo"))
	require.NoError(t, err)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("WaitForNth did not unblock after second occurrence")
	}
}

func TestWaitBuffer_WaitForNth_AlreadySatisfied(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	wb := testutil.NewWaitBuffer()
	_, err := wb.Write([]byte("Foo Foo Foo"))
	require.NoError(t, err)

	// All three occurrences already present.
	require.NoError(t, wb.WaitForNth(ctx, "Foo", 3))
}

func TestWaitBuffer_RequireWaitFor_Timeout(t *testing.T) {
	t.Parallel()

	// Use a mock testing.TB to capture the fatal call without
	// killing the real test.
	mock := &tbMock{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	wb := testutil.NewWaitBuffer()
	_, err := wb.Write([]byte("some output"))
	require.NoError(t, err)

	wb.RequireWaitFor(ctx, mock, "missing-signal")
	assert.True(t, mock.failed(), "expected RequireWaitFor to fail the mock test")
}

// tbMock is a minimal testing.TB that records Fatalf calls.
type tbMock struct {
	testing.TB // Embed to satisfy the interface.
	mu         sync.Mutex
	fatalCalls int
}

func (*tbMock) Helper() {}

func (m *tbMock) Fatalf(string, ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fatalCalls++
}

func (m *tbMock) failed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fatalCalls > 0
}

func TestWaitBuffer_Bytes_ReturnsCopy(t *testing.T) {
	t.Parallel()

	wb := testutil.NewWaitBuffer()
	_, err := wb.Write([]byte("original"))
	require.NoError(t, err)

	b := wb.Bytes()
	// Mutate the returned slice.
	for i := range b {
		b[i] = 'X'
	}
	// The internal buffer must be unchanged.
	require.Equal(t, "original", wb.String())
}

func TestWaitBuffer_PlainBuffer(t *testing.T) {
	t.Parallel()

	wb := testutil.NewWaitBuffer()
	_, err := wb.Write([]byte("hello "))
	require.NoError(t, err)
	_, err = wb.Write([]byte("world"))
	require.NoError(t, err)

	require.Equal(t, "hello world", wb.String())
	require.Equal(t, []byte("hello world"), wb.Bytes())
}
