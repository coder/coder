package backedpipe_test

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/immortalstreams/backedpipe"
	"github.com/coder/coder/v2/testutil"
)

// mockReader implements io.Reader with controllable behavior for testing
type mockReader struct {
	mu       sync.Mutex
	data     []byte
	pos      int
	err      error
	readFunc func([]byte) (int, error)
}

func newMockReader(data string) *mockReader {
	return &mockReader{data: []byte(data)}
}

func (mr *mockReader) Read(p []byte) (int, error) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.readFunc != nil {
		return mr.readFunc(p)
	}

	if mr.err != nil {
		return 0, mr.err
	}

	if mr.pos >= len(mr.data) {
		return 0, io.EOF
	}

	n := copy(p, mr.data[mr.pos:])
	mr.pos += n
	return n, nil
}

func (mr *mockReader) setError(err error) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.err = err
}

func TestBackedReader_NewBackedReader(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)
	require.NotNil(t, br)
	require.Equal(t, uint64(0), br.SequenceNum())
	require.False(t, br.Connected())
}

func TestBackedReader_BasicReadOperation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)
	reader := newMockReader("hello world")

	// Connect the reader
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number from reader
	seq := testutil.RequireReceive(ctx, t, seqNum)
	require.Equal(t, uint64(0), seq)

	// Send new reader
	testutil.RequireSend(ctx, t, newR, io.Reader(reader))

	// Read data
	buf := make([]byte, 5)
	n, err := br.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, "hello", string(buf))
	require.Equal(t, uint64(5), br.SequenceNum())

	// Read more data
	n, err = br.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, " worl", string(buf))
	require.Equal(t, uint64(10), br.SequenceNum())
}

func TestBackedReader_ReadBlocksWhenDisconnected(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)

	// Start a read operation that should block
	readDone := make(chan struct{})
	var readErr error
	var readBuf []byte
	var readN int

	go func() {
		defer close(readDone)
		buf := make([]byte, 10)
		readN, readErr = br.Read(buf)
		readBuf = buf[:readN]
	}()

	// Ensure the read is actually blocked by verifying it hasn't completed
	// and that the reader is not connected
	select {
	case <-readDone:
		t.Fatal("Read should be blocked when disconnected")
	default:
		// Read is still blocked, which is what we want
	}
	require.False(t, br.Connected(), "Reader should not be connected")

	// Connect and the read should unblock
	reader := newMockReader("test")
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send new reader
	testutil.RequireReceive(ctx, t, seqNum)
	testutil.RequireSend(ctx, t, newR, io.Reader(reader))

	// Wait for read to complete
	testutil.TryReceive(ctx, t, readDone)
	require.NoError(t, readErr)
	require.Equal(t, "test", string(readBuf))
}

func TestBackedReader_ReconnectionAfterFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)
	reader1 := newMockReader("first")

	// Initial connection
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send new reader
	testutil.RequireReceive(ctx, t, seqNum)
	testutil.RequireSend(ctx, t, newR, io.Reader(reader1))

	// Read some data
	buf := make([]byte, 5)
	n, err := br.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "first", string(buf[:n]))
	require.Equal(t, uint64(5), br.SequenceNum())

	// Simulate connection failure
	reader1.setError(xerrors.New("connection lost"))

	// Start a read that will block due to connection failure
	readDone := make(chan error, 1)
	go func() {
		_, err := br.Read(buf)
		readDone <- err
	}()

	// Wait for the error to be reported via error channel
	receivedErrorEvent := testutil.RequireReceive(ctx, t, errChan)
	require.Error(t, receivedErrorEvent.Err)
	require.Equal(t, "reader", receivedErrorEvent.Component)
	require.Contains(t, receivedErrorEvent.Err.Error(), "connection lost")

	// Verify read is still blocked
	select {
	case err := <-readDone:
		t.Fatalf("Read should still be blocked, but completed with: %v", err)
	default:
		// Good, still blocked
	}

	// Verify disconnection
	require.False(t, br.Connected())

	// Reconnect with new reader
	reader2 := newMockReader("second")
	seqNum2 := make(chan uint64, 1)
	newR2 := make(chan io.Reader, 1)

	go br.Reconnect(seqNum2, newR2)

	// Get sequence number and send new reader
	seq := testutil.RequireReceive(ctx, t, seqNum2)
	require.Equal(t, uint64(5), seq) // Should return current sequence number
	testutil.RequireSend(ctx, t, newR2, io.Reader(reader2))

	// Wait for read to unblock and succeed with new data
	readErr := testutil.RequireReceive(ctx, t, readDone)
	require.NoError(t, readErr) // Should succeed with new reader
	require.True(t, br.Connected())
}

func TestBackedReader_Close(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)
	reader := newMockReader("test")

	// Connect
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send new reader
	testutil.RequireReceive(ctx, t, seqNum)
	testutil.RequireSend(ctx, t, newR, io.Reader(reader))

	// First, read all available data
	buf := make([]byte, 10)
	n, err := br.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 4, n) // "test" is 4 bytes

	// Close the reader before EOF triggers reconnection
	err = br.Close()
	require.NoError(t, err)

	// After close, reads should return EOF
	n, err = br.Read(buf)
	require.Equal(t, 0, n)
	require.Equal(t, io.EOF, err)

	// Subsequent reads should return EOF
	_, err = br.Read(buf)
	require.Equal(t, io.EOF, err)
}

func TestBackedReader_CloseIdempotent(t *testing.T) {
	t.Parallel()

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)

	err := br.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = br.Close()
	require.NoError(t, err)
}

func TestBackedReader_ReconnectAfterClose(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)

	err := br.Close()
	require.NoError(t, err)

	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Should get 0 sequence number for closed reader
	seq := testutil.TryReceive(ctx, t, seqNum)
	require.Equal(t, uint64(0), seq)
}

// Helper function to reconnect a reader using channels
func reconnectReader(ctx context.Context, t testing.TB, br *backedpipe.BackedReader, reader io.Reader) {
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send new reader
	testutil.RequireReceive(ctx, t, seqNum)
	testutil.RequireSend(ctx, t, newR, reader)
}

func TestBackedReader_SequenceNumberTracking(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)
	reader := newMockReader("0123456789")

	reconnectReader(ctx, t, br, reader)

	// Read in chunks and verify sequence number
	buf := make([]byte, 3)

	n, err := br.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, uint64(3), br.SequenceNum())

	n, err = br.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, uint64(6), br.SequenceNum())

	n, err = br.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, uint64(9), br.SequenceNum())
}

func TestBackedReader_EOFHandling(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)
	reader := newMockReader("test")

	reconnectReader(ctx, t, br, reader)

	// Read all data
	buf := make([]byte, 10)
	n, err := br.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, "test", string(buf[:n]))

	// Next read should encounter EOF, which triggers disconnection
	// The read should block waiting for reconnection
	readDone := make(chan struct{})
	var readErr error
	var readN int

	go func() {
		defer close(readDone)
		readN, readErr = br.Read(buf)
	}()

	// Wait for EOF to be reported via error channel
	receivedErrorEvent := testutil.RequireReceive(ctx, t, errChan)
	require.Equal(t, io.EOF, receivedErrorEvent.Err)
	require.Equal(t, "reader", receivedErrorEvent.Component)

	// Reader should be disconnected after EOF
	require.False(t, br.Connected())

	// Read should still be blocked
	select {
	case <-readDone:
		t.Fatal("Read should be blocked waiting for reconnection after EOF")
	default:
		// Good, still blocked
	}

	// Reconnect with new data
	reader2 := newMockReader("more")
	reconnectReader(ctx, t, br, reader2)

	// Wait for the blocked read to complete with new data
	testutil.TryReceive(ctx, t, readDone)
	require.NoError(t, readErr)
	require.Equal(t, 4, readN)
	require.Equal(t, "more", string(buf[:readN]))
}

func BenchmarkBackedReader_Read(b *testing.B) {
	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)
	buf := make([]byte, 1024)

	// Create a reader that never returns EOF by cycling through data
	reader := &mockReader{
		readFunc: func(p []byte) (int, error) {
			// Fill buffer with 'x' characters - never EOF
			for i := range p {
				p[i] = 'x'
			}
			return len(p), nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	reconnectReader(ctx, b, br, reader)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		br.Read(buf)
	}
}

func TestBackedReader_PartialReads(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)

	// Create a reader that returns partial reads
	reader := &mockReader{
		readFunc: func(p []byte) (int, error) {
			// Always return just 1 byte at a time
			if len(p) == 0 {
				return 0, nil
			}
			p[0] = 'A'
			return 1, nil
		},
	}

	reconnectReader(ctx, t, br, reader)

	// Read multiple times
	buf := make([]byte, 10)
	for i := 0; i < 5; i++ {
		n, err := br.Read(buf)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Equal(t, byte('A'), buf[0])
	}

	require.Equal(t, uint64(5), br.SequenceNum())
}

func TestBackedReader_CloseWhileBlockedOnUnderlyingReader(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)

	// Create a reader that blocks on Read calls but can be unblocked
	readStarted := make(chan struct{}, 1)
	readUnblocked := make(chan struct{})
	blockingReader := &mockReader{
		readFunc: func(p []byte) (int, error) {
			select {
			case readStarted <- struct{}{}:
			default:
			}
			<-readUnblocked // Block until signaled
			// After unblocking, return an error to simulate connection failure
			return 0, xerrors.New("connection interrupted")
		},
	}

	// Connect the blocking reader
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send blocking reader
	testutil.RequireReceive(ctx, t, seqNum)
	testutil.RequireSend(ctx, t, newR, io.Reader(blockingReader))

	// Start a read that will block on the underlying reader
	readDone := make(chan struct{})
	var readErr error
	var readN int

	go func() {
		defer close(readDone)
		buf := make([]byte, 10)
		readN, readErr = br.Read(buf)
	}()

	// Wait for the read to start and block on the underlying reader
	testutil.RequireReceive(ctx, t, readStarted)

	// Verify read is blocked by checking that it hasn't completed
	// and ensuring we have adequate time for it to reach the blocking state
	require.Eventually(t, func() bool {
		select {
		case <-readDone:
			t.Fatal("Read should be blocked on underlying reader")
			return false
		default:
			// Good, still blocked
			return true
		}
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Start Close() in a goroutine since it will block until the underlying read completes
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- br.Close()
	}()

	// Verify Close() is also blocked waiting for the underlying read
	select {
	case <-closeDone:
		t.Fatal("Close should be blocked until underlying read completes")
	case <-time.After(10 * time.Millisecond):
		// Good, Close is blocked
	}

	// Unblock the underlying reader, which will cause both the read and close to complete
	close(readUnblocked)

	// Wait for both the read and close to complete
	testutil.TryReceive(ctx, t, readDone)
	closeErr := testutil.RequireReceive(ctx, t, closeDone)
	require.NoError(t, closeErr)

	// The read should return EOF because Close() was called while it was blocked,
	// even though the underlying reader returned an error
	require.Equal(t, 0, readN)
	require.Equal(t, io.EOF, readErr)

	// Subsequent reads should return EOF since the reader is now closed
	buf := make([]byte, 10)
	n, err := br.Read(buf)
	require.Equal(t, 0, n)
	require.Equal(t, io.EOF, err)
}

func TestBackedReader_CloseWhileBlockedWaitingForReconnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	errChan := make(chan backedpipe.ErrorEvent, 1)
	br := backedpipe.NewBackedReader(errChan)
	reader1 := newMockReader("initial")

	// Initial connection
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send initial reader
	testutil.RequireReceive(ctx, t, seqNum)
	testutil.RequireSend(ctx, t, newR, io.Reader(reader1))

	// Read initial data
	buf := make([]byte, 10)
	n, err := br.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "initial", string(buf[:n]))

	// Simulate connection failure
	reader1.setError(xerrors.New("connection lost"))

	// Start a read that will block waiting for reconnection
	readDone := make(chan struct{})
	var readErr error
	var readN int

	go func() {
		defer close(readDone)
		readN, readErr = br.Read(buf)
	}()

	// Wait for the error to be reported (indicating disconnection)
	receivedErrorEvent := testutil.RequireReceive(ctx, t, errChan)
	require.Error(t, receivedErrorEvent.Err)
	require.Equal(t, "reader", receivedErrorEvent.Component)
	require.Contains(t, receivedErrorEvent.Err.Error(), "connection lost")

	// Verify read is blocked waiting for reconnection
	select {
	case <-readDone:
		t.Fatal("Read should be blocked waiting for reconnection")
	default:
		// Good, still blocked
	}

	// Verify reader is disconnected
	require.False(t, br.Connected())

	// Close the BackedReader while read is blocked waiting for reconnection
	err = br.Close()
	require.NoError(t, err)

	// The read should unblock and return EOF
	testutil.TryReceive(ctx, t, readDone)
	require.Equal(t, 0, readN)
	require.Equal(t, io.EOF, readErr)
}
