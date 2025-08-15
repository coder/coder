package backedpipe_test

import (
	"bytes"
	"io"
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
	errorChan := make(chan error, 1)
	return backedpipe.NewBackedWriter(bufferSize, errorChan)
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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)
	require.NotNil(t, bw)
	require.Equal(t, uint64(0), bw.SequenceNum())
	require.False(t, bw.Connected())
}

func TestBackedWriter_WriteBlocksWhenDisconnected(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)
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

func TestBackedWriter_BlockOnWriteFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)
	writer := newMockWriter()

	// Connect
	err := bw.Reconnect(0, writer)
	require.NoError(t, err)

	// Cause write to fail
	writer.setError(xerrors.New("write failed"))

	// Write should block when underlying writer fails
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

	// Should be disconnected
	require.False(t, bw.Connected())

	// Error should be sent to error channel
	select {
	case receivedErr := <-errorChan:
		require.Contains(t, receivedErr.Error(), "write failed")
	default:
		t.Fatal("Expected error to be sent to error channel")
	}

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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

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
}

func TestBackedWriter_Close(t *testing.T) {
	t.Parallel()

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)
	writer := newMockWriter()

	bw.Reconnect(0, writer)

	err := bw.Close()
	require.NoError(t, err)

	// Writes after close should fail
	_, err = bw.Write([]byte("test"))
	require.Equal(t, io.EOF, err)

	// Reconnect after close should fail
	err = bw.Reconnect(0, newMockWriter())
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrWriterClosed)
}

func TestBackedWriter_CloseIdempotent(t *testing.T) {
	t.Parallel()

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

	err := bw.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = bw.Close()
	require.NoError(t, err)
}

func TestBackedWriter_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)
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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

	// Connect initially to write some data
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)

	_, err = bw.Write([]byte("hello world"))
	require.NoError(t, err)

	// Create a writer that fails during replay
	writer2 := &mockWriter{
		writeFunc: func(p []byte) (int, error) {
			return 0, xerrors.New("replay failed")
		},
	}

	err = bw.Reconnect(0, writer2)
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrReplayFailed)
	require.False(t, bw.Connected())
}

func TestBackedWriter_BlockOnPartialWrite(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

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

	// Should be disconnected
	require.False(t, bw.Connected())

	// Error should be sent to error channel
	select {
	case receivedErr := <-errorChan:
		require.Contains(t, receivedErr.Error(), "short write")
	default:
		t.Fatal("Expected error to be sent to error channel")
	}

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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

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
	require.Equal(t, io.EOF, err)
}

func TestBackedWriter_WriteBlocksAfterDisconnection(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)
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

	// Should be disconnected
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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)
	writer := newMockWriter()
	bw.Reconnect(0, writer)

	// Start a write operation that will be interrupted by close
	writeComplete := make(chan struct{})
	var writeErr error
	var n int

	go func() {
		defer close(writeComplete)
		// Write some data that should succeed
		n, writeErr = bw.Write([]byte("hello"))
	}()

	// Give write a chance to start
	time.Sleep(10 * time.Millisecond)

	// Close the writer
	closeErr := bw.Close()
	require.NoError(t, closeErr)

	// Wait for write to complete
	<-writeComplete

	// Write should have either succeeded (if it completed before close)
	// or failed with EOF (if close interrupted it)
	if writeErr == nil {
		require.Equal(t, 5, n)
	} else {
		require.ErrorIs(t, writeErr, io.EOF)
	}

	// Subsequent writes should fail
	n, err := bw.Write([]byte("world"))
	require.Equal(t, 0, n)
	require.ErrorIs(t, err, io.EOF)
}

func TestBackedWriter_ConcurrentWriteAndReconnect(t *testing.T) {
	t.Parallel()

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

	// Initial connection
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)

	// Write some initial data
	_, err = bw.Write([]byte("initial"))
	require.NoError(t, err)

	// Start a write operation that will be blocked by reconnect
	writeComplete := make(chan struct{})
	var writeErr error
	var n int

	go func() {
		defer close(writeComplete)
		// This write should be blocked during reconnect
		n, writeErr = bw.Write([]byte("blocked"))
	}()

	// Give write a chance to start
	time.Sleep(10 * time.Millisecond)

	// Start reconnection which will cause the write to wait
	writer2 := &mockWriter{
		writeFunc: func(p []byte) (int, error) {
			// Simulate slow replay
			time.Sleep(50 * time.Millisecond)
			return len(p), nil
		},
	}

	reconnectErr := bw.Reconnect(0, writer2)
	require.NoError(t, reconnectErr)

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

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

	// Initial connection and write some data
	writer1 := newMockWriter()
	err := bw.Reconnect(0, writer1)
	require.NoError(t, err)
	_, err = bw.Write([]byte("test data"))
	require.NoError(t, err)

	// Start reconnection with slow replay
	reconnectComplete := make(chan struct{})
	var reconnectErr error

	go func() {
		defer close(reconnectComplete)
		writer2 := &mockWriter{
			writeFunc: func(p []byte) (int, error) {
				// Simulate slow replay - this should be interrupted by close
				time.Sleep(100 * time.Millisecond)
				return len(p), nil
			},
		}
		reconnectErr = bw.Reconnect(0, writer2)
	}()

	// Give reconnect a chance to start
	time.Sleep(10 * time.Millisecond)

	// Close while reconnection is in progress
	closeErr := bw.Close()
	require.NoError(t, closeErr)

	// Wait for reconnect to complete
	<-reconnectComplete

	// With mutex held during replay, Close() waits for Reconnect() to finish.
	// So Reconnect() should succeed, then Close() runs and closes the writer.
	require.NoError(t, reconnectErr)

	// Verify writer is closed (Close() ran after Reconnect() completed)
	require.False(t, bw.Connected())
}

func TestBackedWriter_MultipleWritesDuringReconnect(t *testing.T) {
	t.Parallel()

	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

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

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			data := []byte{byte('A' + id)}
			_, writeResults[id] = bw.Write(data)
		}(i)
	}

	// Give writes a chance to start
	time.Sleep(10 * time.Millisecond)

	// Start reconnection with slow replay
	writer2 := &mockWriter{
		writeFunc: func(p []byte) (int, error) {
			// Simulate slow replay
			time.Sleep(50 * time.Millisecond)
			return len(p), nil
		},
	}

	reconnectErr := bw.Reconnect(0, writer2)
	require.NoError(t, reconnectErr)

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
	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan) // 64KB buffer
	writer := newMockWriter()
	bw.Reconnect(0, writer)

	data := bytes.Repeat([]byte("x"), 1024) // 1KB writes

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bw.Write(data)
	}
}

func BenchmarkBackedWriter_Reconnect(b *testing.B) {
	errorChan := make(chan error, 1)
	bw := backedpipe.NewBackedWriter(backedpipe.DefaultBufferSize, errorChan)

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
