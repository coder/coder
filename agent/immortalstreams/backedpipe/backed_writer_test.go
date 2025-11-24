package backedpipe_test

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"

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
	errChan := make(chan backedpipe.ErrorEvent, 1)
	return backedpipe.NewBackedWriter(bufferSize, errChan)
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

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)
	require.NotNil(t, bw)
	require.Equal(t, uint64(0), bw.SequenceNum())
	require.False(t, bw.Connected())
}

func TestBackedWriter_WriteBlocksWhenDisconnected(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Write should block when disconnected
	writeComplete := make(chan struct{})
	var writeErr error
	var n int

	go func() {
		defer close(writeComplete)
		n, writeErr = bw.Write([]byte("hello"))
	}()

	// Verify write is blocked
	select {
	case <-writeComplete:
		t.Fatal("Write should have blocked when disconnected")
	case <-time.After(50 * time.Millisecond):
		// Expected - write is blocked
	}

	// Connect and verify write completes
	writer := newMockWriter()
	err := bw.Reconnect(0, writer)
	require.NoError(t, err)

	// Write should now complete
	testutil.TryReceive(ctx, t, writeComplete)

	require.NoError(t, writeErr)
	require.Equal(t, 5, n)
	require.Equal(t, uint64(5), bw.SequenceNum())
	require.Equal(t, []byte("hello"), writer.buffer.Bytes())
}

func TestBackedWriter_WriteToUnderlyingWhenConnected(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)
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
	require.Equal(t, uint64(5), bw.SequenceNum())

	// Check underlying writer
	require.Equal(t, []byte("hello"), writer.buffer.Bytes())
}

func TestBackedWriter_BlockOnWriteFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)
	writer := newMockWriter()

	// Connect
	err := bw.Reconnect(0, writer)
	require.NoError(t, err)

	// Cause write to fail
	writer.setError(xerrors.New("write failed"))

	// Write should block when underlying writer fails, not succeed immediately
	writeComplete := make(chan struct{})
	var writeErr error
	var n int

	go func() {
		defer close(writeComplete)
		n, writeErr = bw.Write([]byte("hello"))
	}()

	// Verify write is blocked
	select {
	case <-writeComplete:
		t.Fatal("Write should have blocked when underlying writer fails")
	case <-time.After(50 * time.Millisecond):
		// Expected - write is blocked
	}

	// Wait for error event which implies writer was marked disconnected
	receivedErrorEvent := testutil.RequireReceive(ctx, t, errChan)
	require.Contains(t, receivedErrorEvent.Err.Error(), "write failed")
	require.Equal(t, "writer", receivedErrorEvent.Component)
	require.False(t, bw.Connected())

	// Reconnect with working writer and verify write completes
	writer2 := newMockWriter()
	err = bw.Reconnect(0, writer2) // Replay from beginning
	require.NoError(t, err)

	// Write should now complete
	testutil.TryReceive(ctx, t, writeComplete)

	require.NoError(t, writeErr)
	require.Equal(t, 5, n)
	require.Equal(t, uint64(5), bw.SequenceNum())
	require.Equal(t, []byte("hello"), writer2.buffer.Bytes())
}

func TestBackedWriter_ReplayOnReconnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Connect initially to write some data
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)

	// Write some data while connected
	_, err = bw.Write([]byte("hello"))
	require.NoError(t, err)
	_, err = bw.Write([]byte(" world"))
	require.NoError(t, err)

	require.Equal(t, uint64(11), bw.SequenceNum())

	// Disconnect by causing a write failure
	writer1.setError(xerrors.New("connection lost"))

	// Write should block when underlying writer fails
	writeComplete := make(chan struct{})
	var writeErr error
	var n int

	go func() {
		defer close(writeComplete)
		n, writeErr = bw.Write([]byte("test"))
	}()

	// Verify write is blocked
	select {
	case <-writeComplete:
		t.Fatal("Write should have blocked when underlying writer fails")
	case <-time.After(50 * time.Millisecond):
		// Expected - write is blocked
	}

	// Wait for error event which implies writer was marked disconnected
	receivedErrorEvent := testutil.RequireReceive(ctx, t, errChan)
	require.Contains(t, receivedErrorEvent.Err.Error(), "connection lost")
	require.Equal(t, "writer", receivedErrorEvent.Component)
	require.False(t, bw.Connected())

	// Reconnect with new writer and request replay from beginning
	writer2 := newMockWriter()
	err = bw.Reconnect(0, writer2)
	require.NoError(t, err)

	// Write should now complete
	select {
	case <-writeComplete:
		// Expected - write completed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Write should have completed after reconnection")
	}

	require.NoError(t, writeErr)
	require.Equal(t, 4, n)

	// Should have replayed all data including the failed write that was buffered
	require.Equal(t, []byte("hello worldtest"), writer2.buffer.Bytes())

	// Write new data should go to both
	_, err = bw.Write([]byte("!"))
	require.NoError(t, err)
	require.Equal(t, []byte("hello worldtest!"), writer2.buffer.Bytes())
}

func TestBackedWriter_PartialReplay(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Connect initially to write some data
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)

	// Write some data
	_, err = bw.Write([]byte("hello"))
	require.NoError(t, err)
	_, err = bw.Write([]byte(" world"))
	require.NoError(t, err)
	_, err = bw.Write([]byte("!"))
	require.NoError(t, err)

	// Reconnect with new writer and request replay from middle
	writer2 := newMockWriter()
	err = bw.Reconnect(5, writer2) // From " world!"
	require.NoError(t, err)

	// Should have replayed only the requested portion
	require.Equal(t, []byte(" world!"), writer2.buffer.Bytes())
}

func TestBackedWriter_ReplayFromFutureSequence(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Connect initially to write some data
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)

	_, err = bw.Write([]byte("hello"))
	require.NoError(t, err)

	writer2 := newMockWriter()
	err = bw.Reconnect(10, writer2) // Future sequence
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrFutureSequence)
}

func TestBackedWriter_ReplayDataLoss(t *testing.T) {
	t.Parallel()

	bw := newBackedWriterForTest(10) // Small buffer for testing

	// Connect initially to write some data
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)

	// Fill buffer beyond capacity to cause eviction
	_, err = bw.Write([]byte("0123456789")) // Fills buffer exactly
	require.NoError(t, err)
	_, err = bw.Write([]byte("abcdef")) // Should evict "012345"
	require.NoError(t, err)

	writer2 := newMockWriter()
	err = bw.Reconnect(0, writer2) // Try to replay from evicted data
	// With the new error handling, this should fail because we can't read all the data
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrReplayDataUnavailable)
}

func TestBackedWriter_BufferEviction(t *testing.T) {
	t.Parallel()

	bw := newBackedWriterForTest(5) // Very small buffer for testing

	// Connect initially
	writer := newMockWriter()
	err := bw.Reconnect(0, writer)
	require.NoError(t, err)

	// Write data that will cause eviction
	n, err := bw.Write([]byte("abcde"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	// Write more to cause eviction
	n, err = bw.Write([]byte("fg"))
	require.NoError(t, err)
	require.Equal(t, 2, n)

	// Verify that the buffer contains only the latest data after eviction
	// Total sequence number should be 7 (5 + 2)
	require.Equal(t, uint64(7), bw.SequenceNum())

	// Try to reconnect from the beginning - this should fail because
	// the early data was evicted from the buffer
	writer2 := newMockWriter()
	err = bw.Reconnect(0, writer2)
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrReplayDataUnavailable)

	// However, reconnecting from a sequence that's still in the buffer should work
	// The buffer should contain the last 5 bytes: "cdefg"
	writer3 := newMockWriter()
	err = bw.Reconnect(2, writer3) // From sequence 2, should replay "cdefg"
	require.NoError(t, err)
	require.Equal(t, []byte("cdefg"), writer3.buffer.Bytes())
	require.True(t, bw.Connected())
}

func TestBackedWriter_Close(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)
	writer := newMockWriter()

	bw.Reconnect(0, writer)

	err := bw.Close()
	require.NoError(t, err)

	// Writes after close should fail
	_, err = bw.Write([]byte("test"))
	require.Equal(t, os.ErrClosed, err)

	// Reconnect after close should fail
	err = bw.Reconnect(0, newMockWriter())
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrWriterClosed)
}

func TestBackedWriter_CloseIdempotent(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	err := bw.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = bw.Close()
	require.NoError(t, err)
}

func TestBackedWriter_ReconnectDuringReplay(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Connect initially to write some data
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)

	_, err = bw.Write([]byte("hello world"))
	require.NoError(t, err)

	// Create a writer that fails during replay
	writer2 := &mockWriter{
		err: backedpipe.ErrReplayFailed,
	}

	err = bw.Reconnect(0, writer2)
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrReplayFailed)
	require.False(t, bw.Connected())
}

func TestBackedWriter_BlockOnPartialWrite(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

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

	// Write should block due to partial write
	writeComplete := make(chan struct{})
	var writeErr error
	var n int

	go func() {
		defer close(writeComplete)
		n, writeErr = bw.Write([]byte("hello"))
	}()

	// Verify write is blocked
	select {
	case <-writeComplete:
		t.Fatal("Write should have blocked when underlying writer does partial write")
	case <-time.After(50 * time.Millisecond):
		// Expected - write is blocked
	}

	// Wait for error event which implies writer was marked disconnected
	receivedErrorEvent := testutil.RequireReceive(ctx, t, errChan)
	require.Contains(t, receivedErrorEvent.Err.Error(), "short write")
	require.Equal(t, "writer", receivedErrorEvent.Component)
	require.False(t, bw.Connected())

	// Reconnect with working writer and verify write completes
	writer2 := newMockWriter()
	err := bw.Reconnect(0, writer2) // Replay from beginning
	require.NoError(t, err)

	// Write should now complete
	testutil.TryReceive(ctx, t, writeComplete)

	require.NoError(t, writeErr)
	require.Equal(t, 5, n)
	require.Equal(t, uint64(5), bw.SequenceNum())
	require.Equal(t, []byte("hello"), writer2.buffer.Bytes())
}

func TestBackedWriter_WriteUnblocksOnReconnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Start a single write that should block
	writeResult := make(chan error, 1)
	go func() {
		_, err := bw.Write([]byte("test"))
		writeResult <- err
	}()

	// Verify write is blocked
	select {
	case <-writeResult:
		t.Fatal("Write should have blocked when disconnected")
	case <-time.After(50 * time.Millisecond):
		// Expected - write is blocked
	}

	// Connect and verify write completes
	writer := newMockWriter()
	err := bw.Reconnect(0, writer)
	require.NoError(t, err)

	// Write should now complete
	err = testutil.RequireReceive(ctx, t, writeResult)
	require.NoError(t, err)

	// Write should have been written to the underlying writer
	require.Equal(t, "test", writer.buffer.String())
}

func TestBackedWriter_CloseUnblocksWaitingWrites(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Start a write that should block
	writeComplete := make(chan error, 1)
	go func() {
		_, err := bw.Write([]byte("test"))
		writeComplete <- err
	}()

	// Verify write is blocked
	select {
	case <-writeComplete:
		t.Fatal("Write should have blocked when disconnected")
	case <-time.After(50 * time.Millisecond):
		// Expected - write is blocked
	}

	// Close the writer
	err := bw.Close()
	require.NoError(t, err)

	// Write should now complete with error
	err = testutil.RequireReceive(ctx, t, writeComplete)
	require.Equal(t, os.ErrClosed, err)
}

func TestBackedWriter_WriteBlocksAfterDisconnection(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)
	writer := newMockWriter()

	// Connect initially
	err := bw.Reconnect(0, writer)
	require.NoError(t, err)

	// Write should succeed when connected
	_, err = bw.Write([]byte("hello"))
	require.NoError(t, err)

	// Cause disconnection - the write should now block instead of returning an error
	writer.setError(xerrors.New("connection lost"))

	// This write should block
	writeComplete := make(chan error, 1)
	go func() {
		_, err := bw.Write([]byte("world"))
		writeComplete <- err
	}()

	// Verify write is blocked
	select {
	case <-writeComplete:
		t.Fatal("Write should have blocked after disconnection")
	case <-time.After(50 * time.Millisecond):
		// Expected - write is blocked
	}

	// Wait for error event which implies writer was marked disconnected
	receivedErrorEvent := testutil.RequireReceive(ctx, t, errChan)
	require.Contains(t, receivedErrorEvent.Err.Error(), "connection lost")
	require.Equal(t, "writer", receivedErrorEvent.Component)
	require.False(t, bw.Connected())

	// Reconnect and verify write completes
	writer2 := newMockWriter()
	err = bw.Reconnect(5, writer2) // Replay from after "hello"
	require.NoError(t, err)

	err = testutil.RequireReceive(ctx, t, writeComplete)
	require.NoError(t, err)

	// Check that only "world" was written during replay (not duplicated)
	require.Equal(t, []byte("world"), writer2.buffer.Bytes()) // Only "world" since we replayed from sequence 5
}

func TestBackedWriter_ConcurrentWriteAndClose(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Don't connect initially - this will cause writes to block in blockUntilConnectedOrClosed()

	writeStarted := make(chan struct{}, 1)

	// Start a write operation that will block waiting for connection
	writeComplete := make(chan struct{})
	var writeErr error
	var n int

	go func() {
		defer close(writeComplete)
		// Signal that we're about to start the write
		writeStarted <- struct{}{}
		// This write will block in blockUntilConnectedOrClosed() since no writer is connected
		n, writeErr = bw.Write([]byte("hello"))
	}()

	// Wait for write goroutine to start
	ctx := testutil.Context(t, testutil.WaitShort)
	testutil.RequireReceive(ctx, t, writeStarted)

	// Ensure the write is actually blocked by repeatedly checking that:
	// 1. The write hasn't completed yet
	// 2. The writer is still not connected
	// We use require.Eventually to give it a fair chance to reach the blocking state
	require.Eventually(t, func() bool {
		select {
		case <-writeComplete:
			t.Fatal("Write should be blocked when no writer is connected")
			return false
		default:
			// Write is still blocked, which is what we want
			return !bw.Connected()
		}
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Close the writer while the write is blocked waiting for connection
	closeErr := bw.Close()
	require.NoError(t, closeErr)

	// Wait for write to complete
	select {
	case <-writeComplete:
		// Good, write completed
	case <-ctx.Done():
		t.Fatal("Write did not complete in time")
	}

	// The write should have failed with os.ErrClosed because Close() was called
	// while it was waiting for connection
	require.ErrorIs(t, writeErr, os.ErrClosed)
	require.Equal(t, 0, n)

	// Subsequent writes should also fail
	n, err := bw.Write([]byte("world"))
	require.Equal(t, 0, n)
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestBackedWriter_ConcurrentWriteAndReconnect(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Initial connection
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)

	// Write some initial data
	_, err = bw.Write([]byte("initial"))
	require.NoError(t, err)

	// Start reconnection which will block new writes
	replayStarted := make(chan struct{}, 1) // Buffered to prevent race condition
	replayCanComplete := make(chan struct{})
	writer2 := &mockWriter{
		writeFunc: func(p []byte) (int, error) {
			// Signal that replay has started
			select {
			case replayStarted <- struct{}{}:
			default:
				// Signal already sent, which is fine
			}
			// Wait for test to allow replay to complete
			<-replayCanComplete
			return len(p), nil
		},
	}

	// Start the reconnection in a goroutine so we can control timing
	reconnectComplete := make(chan error, 1)
	go func() {
		reconnectComplete <- bw.Reconnect(0, writer2)
	}()

	ctx := testutil.Context(t, testutil.WaitShort)
	// Wait for replay to start
	testutil.RequireReceive(ctx, t, replayStarted)

	// Now start a write operation that will be blocked by the ongoing reconnect
	writeStarted := make(chan struct{}, 1)
	writeComplete := make(chan struct{})
	var writeErr error
	var n int

	go func() {
		defer close(writeComplete)
		// Signal that we're about to start the write
		writeStarted <- struct{}{}
		// This write should be blocked during reconnect
		n, writeErr = bw.Write([]byte("blocked"))
	}()

	// Wait for write to start
	testutil.RequireReceive(ctx, t, writeStarted)

	// Use a small timeout to ensure the write goroutine has a chance to get blocked
	// on the mutex before we check if it's still blocked
	writeCheckTimer := time.NewTimer(testutil.IntervalFast)
	defer writeCheckTimer.Stop()

	select {
	case <-writeComplete:
		t.Fatal("Write should be blocked during reconnect")
	case <-writeCheckTimer.C:
		// Write is still blocked after a reasonable wait
	}

	// Allow replay to complete, which will allow reconnect to finish
	close(replayCanComplete)

	// Wait for reconnection to complete
	select {
	case reconnectErr := <-reconnectComplete:
		require.NoError(t, reconnectErr)
	case <-ctx.Done():
		t.Fatal("Reconnect did not complete in time")
	}

	// Wait for write to complete
	<-writeComplete

	// Write should succeed after reconnection completes
	require.NoError(t, writeErr)
	require.Equal(t, 7, n) // "blocked" is 7 bytes

	// Verify the writer is connected
	require.True(t, bw.Connected())
}

func TestBackedWriter_ConcurrentReconnectAndClose(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Initial connection and write some data
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)
	_, err = bw.Write([]byte("test data"))
	require.NoError(t, err)

	// Start reconnection with slow replay
	reconnectStarted := make(chan struct{}, 1)
	replayCanComplete := make(chan struct{})
	reconnectComplete := make(chan struct{})
	var reconnectErr error

	go func() {
		defer close(reconnectComplete)
		writer2 := &mockWriter{
			writeFunc: func(p []byte) (int, error) {
				// Signal that replay has started
				select {
				case reconnectStarted <- struct{}{}:
				default:
				}
				// Wait for test to allow replay to complete
				<-replayCanComplete
				return len(p), nil
			},
		}
		reconnectErr = bw.Reconnect(0, writer2)
	}()

	// Wait for reconnection to start
	ctx := testutil.Context(t, testutil.WaitShort)
	testutil.RequireReceive(ctx, t, reconnectStarted)

	// Start Close() in a separate goroutine since it will block until Reconnect() completes
	closeStarted := make(chan struct{}, 1)
	closeComplete := make(chan error, 1)
	go func() {
		closeStarted <- struct{}{} // Signal that Close() is starting
		closeComplete <- bw.Close()
	}()

	// Wait for Close() to start, then give it a moment to attempt to acquire the mutex
	testutil.RequireReceive(ctx, t, closeStarted)
	closeCheckTimer := time.NewTimer(testutil.IntervalFast)
	defer closeCheckTimer.Stop()

	select {
	case <-closeComplete:
		t.Fatal("Close should be blocked during reconnect")
	case <-closeCheckTimer.C:
		// Good, Close is still blocked after a reasonable wait
	}

	// Allow replay to complete so reconnection can finish
	close(replayCanComplete)

	// Wait for reconnect to complete
	select {
	case <-reconnectComplete:
		// Good, reconnect completed
	case <-ctx.Done():
		t.Fatal("Reconnect did not complete in time")
	}

	// Wait for close to complete
	select {
	case closeErr := <-closeComplete:
		require.NoError(t, closeErr)
	case <-ctx.Done():
		t.Fatal("Close did not complete in time")
	}

	// With mutex held during replay, Close() waits for Reconnect() to finish.
	// So Reconnect() should succeed, then Close() runs and closes the writer.
	require.NoError(t, reconnectErr)

	// Verify writer is closed (Close() ran after Reconnect() completed)
	require.False(t, bw.Connected())
}

func TestBackedWriter_MultipleWritesDuringReconnect(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Initial connection
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)

	// Write some initial data
	_, err = bw.Write([]byte("initial"))
	require.NoError(t, err)

	// Start multiple write operations
	numWriters := 5
	var wg sync.WaitGroup
	writeResults := make([]error, numWriters)
	writesStarted := make(chan struct{}, numWriters)

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Signal that this write is starting
			writesStarted <- struct{}{}
			data := []byte{byte('A' + id)}
			_, writeResults[id] = bw.Write(data)
		}(i)
	}

	// Wait for all writes to start
	ctx := testutil.Context(t, testutil.WaitLong)
	for i := 0; i < numWriters; i++ {
		testutil.RequireReceive(ctx, t, writesStarted)
	}

	// Use a timer to ensure all write goroutines have had a chance to start executing
	// and potentially get blocked on the mutex before we start the reconnection
	writesReadyTimer := time.NewTimer(testutil.IntervalFast)
	defer writesReadyTimer.Stop()
	<-writesReadyTimer.C

	// Start reconnection with controlled replay
	replayStarted := make(chan struct{}, 1)
	replayCanComplete := make(chan struct{})
	writer2 := &mockWriter{
		writeFunc: func(p []byte) (int, error) {
			// Signal that replay has started
			select {
			case replayStarted <- struct{}{}:
			default:
			}
			// Wait for test to allow replay to complete
			<-replayCanComplete
			return len(p), nil
		},
	}

	// Start reconnection in a goroutine so we can control timing
	reconnectComplete := make(chan error, 1)
	go func() {
		reconnectComplete <- bw.Reconnect(0, writer2)
	}()

	// Wait for replay to start
	testutil.RequireReceive(ctx, t, replayStarted)

	// Allow replay to complete
	close(replayCanComplete)

	// Wait for reconnection to complete
	select {
	case reconnectErr := <-reconnectComplete:
		require.NoError(t, reconnectErr)
	case <-ctx.Done():
		t.Fatal("Reconnect did not complete in time")
	}

	// Wait for all writes to complete
	wg.Wait()

	// All writes should succeed
	for i, err := range writeResults {
		require.NoError(t, err, "Write %d should succeed", i)
	}

	// Verify the writer is connected
	require.True(t, bw.Connected())
}

func BenchmarkBackedWriter_Write(b *testing.B) {
	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan) // 64KB buffer
	writer := newMockWriter()
	bw.Reconnect(0, writer)

	data := bytes.Repeat([]byte("x"), 1024) // 1KB writes

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bw.Write(data)
	}
}

func BenchmarkBackedWriter_Reconnect(b *testing.B) {
	errChan := make(chan backedpipe.ErrorEvent, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errChan)

	// Connect initially to fill buffer with data
	initialWriter := newMockWriter()
	err := bw.Reconnect(0, initialWriter)
	if err != nil {
		b.Fatal(err)
	}

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
