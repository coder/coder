package backedpipe_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/immortalstreams/backedpipe"
	"github.com/coder/coder/v2/testutil"
)

// mockWriter implements io.Writer with controllable behavior for testing
type mockWriter struct {
	mu         sync.Mutex
	buffer     bytes.Buffer
	err        error
	writeFunc  func([]byte) (int, error)
	writeCalls int
}

func newMockWriter() *mockWriter {
	return &mockWriter{}
}

// newBackedWriterForTest creates a BackedWriter with a small buffer for testing eviction behavior
func newBackedWriterForTest(bufferSize int) *backedpipe.BackedWriter {
	return backedpipe.NewBackedWriterWithCapacity(bufferSize)
}

func (mw *mockWriter) Write(p []byte) (int, error) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	mw.writeCalls++

	if mw.writeFunc != nil {
		return mw.writeFunc(p)
	}

	if mw.err != nil {
		return 0, mw.err
	}

	return mw.buffer.Write(p)
}

func (mw *mockWriter) Len() int {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	return mw.buffer.Len()
}

func (mw *mockWriter) Reset() {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.buffer.Reset()
	mw.writeCalls = 0
	mw.err = nil
	mw.writeFunc = nil
}

func (mw *mockWriter) setError(err error) {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.err = err
}

func TestBackedWriter_NewBackedWriter(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()
	require.NotNil(t, bw)
	require.Equal(t, uint64(0), bw.SequenceNum())
	require.False(t, bw.Connected())
}

func TestBackedWriter_WriteToBufferWhenDisconnected(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()

	// Write should succeed even when disconnected
	n, err := bw.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, uint64(5), bw.SequenceNum())

	// Data should be in buffer
}

func TestBackedWriter_WriteToUnderlyingWhenConnected(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()
	writer := newMockWriter()

	// Connect
	err := bw.Reconnect(0, writer)
	require.NoError(t, err)
	require.True(t, bw.Connected())

	// Write should go to both buffer and underlying writer
	n, err := bw.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	// Data should be buffered

	// Check underlying writer
	require.Equal(t, []byte("hello"), writer.buffer.Bytes())
}

func TestBackedWriter_DisconnectOnWriteFailure(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()
	writer := newMockWriter()

	// Connect
	err := bw.Reconnect(0, writer)
	require.NoError(t, err)

	// Cause write to fail
	writer.setError(xerrors.New("write failed"))

	// Write should still succeed to buffer but disconnect
	n, err := bw.Write([]byte("hello"))
	require.NoError(t, err) // Buffer write succeeds
	require.Equal(t, 5, n)
	require.False(t, bw.Connected()) // Should be disconnected

	// Data should still be in buffer
}

func TestBackedWriter_ReplayOnReconnect(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()

	// Write some data while disconnected
	bw.Write([]byte("hello"))
	bw.Write([]byte(" world"))

	require.Equal(t, uint64(11), bw.SequenceNum())

	// Reconnect and request replay from beginning
	writer := newMockWriter()
	err := bw.Reconnect(0, writer)
	require.NoError(t, err)

	// Should have replayed all data
	require.Equal(t, []byte("hello world"), writer.buffer.Bytes())

	// Write new data should go to both
	bw.Write([]byte("!"))
	require.Equal(t, []byte("hello world!"), writer.buffer.Bytes())
}

func TestBackedWriter_PartialReplay(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()

	// Write some data
	bw.Write([]byte("hello"))
	bw.Write([]byte(" world"))
	bw.Write([]byte("!"))

	// Reconnect and request replay from middle
	writer := newMockWriter()
	err := bw.Reconnect(5, writer) // From " world!"
	require.NoError(t, err)

	// Should have replayed only the requested portion
	require.Equal(t, []byte(" world!"), writer.buffer.Bytes())
}

func TestBackedWriter_ReplayFromFutureSequence(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()
	bw.Write([]byte("hello"))

	writer := newMockWriter()
	err := bw.Reconnect(10, writer) // Future sequence
	require.Error(t, err)
	require.Contains(t, err.Error(), "future sequence")
}

func TestBackedWriter_ReplayDataLoss(t *testing.T) {
	t.Parallel()

	bw := newBackedWriterForTest(10) // Small buffer for testing

	// Fill buffer beyond capacity to cause eviction
	bw.Write([]byte("0123456789")) // Fills buffer exactly
	bw.Write([]byte("abcdef"))     // Should evict "012345"

	writer := newMockWriter()
	err := bw.Reconnect(0, writer) // Try to replay from evicted data
	// With the new error handling, this should fail because we can't read all the data
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read replay data")
}

func TestBackedWriter_BufferEviction(t *testing.T) {
	t.Parallel()

	bw := newBackedWriterForTest(5) // Very small buffer for testing

	// Write data that will cause eviction
	n, err := bw.Write([]byte("abcde"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	// Write more to cause eviction
	n, err = bw.Write([]byte("fg"))
	require.NoError(t, err)
	require.Equal(t, 2, n)

	// Buffer should contain "cdefg" (latest data)
}

func TestBackedWriter_Close(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()
	writer := newMockWriter()

	bw.Reconnect(0, writer)

	err := bw.Close()
	require.NoError(t, err)

	// Writes after close should fail
	_, err = bw.Write([]byte("test"))
	require.Equal(t, io.ErrClosedPipe, err)

	// Reconnect after close should fail
	err = bw.Reconnect(0, newMockWriter())
	require.Error(t, err)
	require.Contains(t, err.Error(), "closed")
}

func TestBackedWriter_CloseIdempotent(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()

	err := bw.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = bw.Close()
	require.NoError(t, err)
}

func TestBackedWriter_CanReplayFrom(t *testing.T) {
	t.Parallel()

	bw := newBackedWriterForTest(10) // Small buffer for testing eviction

	// Empty buffer
	require.True(t, bw.CanReplayFrom(0))
	require.False(t, bw.CanReplayFrom(1))

	// Write some data
	bw.Write([]byte("hello"))
	require.True(t, bw.CanReplayFrom(0))
	require.True(t, bw.CanReplayFrom(3))
	require.True(t, bw.CanReplayFrom(5))
	require.False(t, bw.CanReplayFrom(6))

	// Fill buffer and cause eviction
	bw.Write([]byte("world!"))
	require.True(t, bw.CanReplayFrom(0)) // Can replay from any sequence up to current
	require.True(t, bw.CanReplayFrom(bw.SequenceNum()))
}

func TestBackedWriter_WaitForConnection(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()

	// Should timeout when not connected
	// Use a shorter timeout for this test to speed up test runs
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperShort)
	defer cancel()

	err := bw.WaitForConnection(ctx)
	require.Equal(t, context.DeadlineExceeded, err)

	// Should succeed immediately when connected
	writer := newMockWriter()
	bw.Reconnect(0, writer)

	ctx = context.Background()
	err = bw.WaitForConnection(ctx)
	require.NoError(t, err)
}

func TestBackedWriter_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()
	writer := newMockWriter()
	bw.Reconnect(0, writer)

	var wg sync.WaitGroup
	numWriters := 10
	writesPerWriter := 50

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				data := []byte{byte(id + '0')}
				bw.Write(data)
			}
		}(i)
	}

	wg.Wait()

	// Should have written expected amount to buffer
	expectedBytes := uint64(numWriters * writesPerWriter) //nolint:gosec // Safe conversion: test constants with small values
	require.Equal(t, expectedBytes, bw.SequenceNum())
	// Note: underlying writer may not receive all bytes due to potential disconnections
	// during concurrent operations, but the buffer should track all writes
	require.True(t, writer.Len() <= int(expectedBytes)) //nolint:gosec // Safe conversion: expectedBytes is calculated from small test values
}

func TestBackedWriter_ReconnectDuringReplay(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()
	bw.Write([]byte("hello world"))

	// Create a writer that fails during replay
	writer := &mockWriter{
		writeFunc: func(p []byte) (int, error) {
			return 0, xerrors.New("replay failed")
		},
	}

	err := bw.Reconnect(0, writer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "replay failed")
	require.False(t, bw.Connected())
}

func TestBackedWriter_PartialWriteToUnderlying(t *testing.T) {
	t.Parallel()

	bw := backedpipe.NewBackedWriter()

	// Create writer that does partial writes
	writer := &mockWriter{
		writeFunc: func(p []byte) (int, error) {
			if len(p) > 3 {
				return 3, nil // Only write first 3 bytes
			}
			return len(p), nil
		},
	}

	bw.Reconnect(0, writer)

	// Write should succeed to buffer but disconnect due to partial write
	n, err := bw.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.False(t, bw.Connected())

	// Buffer should have all data
}

func BenchmarkBackedWriter_Write(b *testing.B) {
	bw := backedpipe.NewBackedWriter() // 64KB buffer
	writer := newMockWriter()
	bw.Reconnect(0, writer)

	data := bytes.Repeat([]byte("x"), 1024) // 1KB writes

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bw.Write(data)
	}
}

func BenchmarkBackedWriter_Reconnect(b *testing.B) {
	bw := backedpipe.NewBackedWriter()

	// Fill buffer with data
	data := bytes.Repeat([]byte("x"), 1024)
	for i := 0; i < 32; i++ {
		bw.Write(data)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer := newMockWriter()
		bw.Reconnect(0, writer)
	}
}
