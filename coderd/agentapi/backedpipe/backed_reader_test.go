package backedpipe_test

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/agentapi/backedpipe"
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

	br := backedpipe.NewBackedReader()
	assert.NotNil(t, br)
	assert.Equal(t, uint64(0), br.SequenceNum())
	assert.False(t, br.Connected())
}

func TestBackedReader_BasicReadOperation(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()
	reader := newMockReader("hello world")

	// Connect the reader
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number from reader
	seq := <-seqNum
	assert.Equal(t, uint64(0), seq)

	// Send new reader
	newR <- reader

	// Read data
	buf := make([]byte, 5)
	n, err := br.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(buf))
	assert.Equal(t, uint64(5), br.SequenceNum())

	// Read more data
	n, err = br.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, " worl", string(buf))
	assert.Equal(t, uint64(10), br.SequenceNum())
}

func TestBackedReader_ReadBlocksWhenDisconnected(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()

	// Start a read operation that should block
	readDone := make(chan struct{})
	readStarted := make(chan struct{})
	var readErr error

	go func() {
		defer close(readDone)
		close(readStarted) // Signal that we're about to start the read
		buf := make([]byte, 10)
		_, readErr = br.Read(buf)
	}()

	// Wait for the goroutine to start
	<-readStarted

	// Give a brief moment for the read to actually block on the condition variable
	// This is much shorter and more deterministic than the previous approach
	time.Sleep(time.Millisecond)

	// Read should still be blocked
	select {
	case <-readDone:
		t.Fatal("Read should be blocked when disconnected")
	default:
		// Good, still blocked
	}

	// Connect and the read should unblock
	reader := newMockReader("test")
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send new reader
	<-seqNum
	newR <- reader

	// Wait for read to complete
	select {
	case <-readDone:
		assert.NoError(t, readErr)
	case <-time.After(time.Second):
		t.Fatal("Read did not unblock after reconnection")
	}
}

func TestBackedReader_ReconnectionAfterFailure(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()
	reader1 := newMockReader("first")

	// Initial connection
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send new reader
	<-seqNum
	newR <- reader1

	// Read some data
	buf := make([]byte, 5)
	n, err := br.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "first", string(buf[:n]))
	assert.Equal(t, uint64(5), br.SequenceNum())

	// Set up error callback to verify error notification
	errorReceived := make(chan error, 1)
	br.SetErrorCallback(func(err error) {
		errorReceived <- err
	})

	// Simulate connection failure
	reader1.setError(xerrors.New("connection lost"))

	// Start a read that will block due to connection failure
	readDone := make(chan error, 1)
	go func() {
		_, err := br.Read(buf)
		readDone <- err
	}()

	// Wait for the error to be reported via callback
	select {
	case receivedErr := <-errorReceived:
		assert.Error(t, receivedErr)
		assert.Contains(t, receivedErr.Error(), "connection lost")
	case <-time.After(time.Second):
		t.Fatal("Error callback was not invoked within timeout")
	}

	// Verify disconnection
	assert.False(t, br.Connected())

	// Reconnect with new reader
	reader2 := newMockReader("second")
	seqNum2 := make(chan uint64, 1)
	newR2 := make(chan io.Reader, 1)

	go br.Reconnect(seqNum2, newR2)

	// Get sequence number and send new reader
	seq := <-seqNum2
	assert.Equal(t, uint64(5), seq) // Should return current sequence number
	newR2 <- reader2

	// Wait for read to unblock and succeed with new data
	select {
	case readErr := <-readDone:
		assert.NoError(t, readErr) // Should succeed with new reader
	case <-time.After(time.Second):
		t.Fatal("Read did not unblock after reconnection")
	}
}

func TestBackedReader_Close(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()
	reader := newMockReader("test")

	// Connect
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send new reader
	<-seqNum
	newR <- reader

	// First, read all available data
	buf := make([]byte, 10)
	n, err := br.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 4, n) // "test" is 4 bytes

	// Close the reader before EOF triggers reconnection
	err = br.Close()
	require.NoError(t, err)

	// After close, reads should return ErrClosedPipe
	n, err = br.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.ErrClosedPipe, err)

	// Subsequent reads should return ErrClosedPipe
	_, err = br.Read(buf)
	assert.Equal(t, io.ErrClosedPipe, err)
}

func TestBackedReader_CloseIdempotent(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()

	err := br.Close()
	assert.NoError(t, err)

	// Second close should be no-op
	err = br.Close()
	assert.NoError(t, err)
}

func TestBackedReader_ReconnectAfterClose(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()

	err := br.Close()
	require.NoError(t, err)

	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Should get 0 sequence number for closed reader
	seq := <-seqNum
	assert.Equal(t, uint64(0), seq)
}

// Helper function to reconnect a reader using channels
func reconnectReader(br *backedpipe.BackedReader, reader io.Reader) {
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go br.Reconnect(seqNum, newR)

	// Get sequence number and send new reader
	<-seqNum
	newR <- reader
}

func TestBackedReader_SequenceNumberTracking(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()
	reader := newMockReader("0123456789")

	reconnectReader(br, reader)

	// Read in chunks and verify sequence number
	buf := make([]byte, 3)

	n, err := br.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, uint64(3), br.SequenceNum())

	n, err = br.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, uint64(6), br.SequenceNum())

	n, err = br.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, uint64(9), br.SequenceNum())
}

func TestBackedReader_ConcurrentReads(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()
	reader := newMockReader(strings.Repeat("a", 1000))

	reconnectReader(br, reader)

	var wg sync.WaitGroup
	numReaders := 5
	readsPerReader := 10

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 10)
			for j := 0; j < readsPerReader; j++ {
				br.Read(buf)
			}
		}()
	}

	wg.Wait()

	// Should have read some data (exact amount depends on scheduling)
	assert.True(t, br.SequenceNum() > 0)
	assert.True(t, br.SequenceNum() <= 1000)
}

func TestBackedReader_EOFHandling(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()
	reader := newMockReader("test")

	// Set up error callback to track when EOF triggers disconnection
	errorReceived := make(chan error, 1)
	br.SetErrorCallback(func(err error) {
		errorReceived <- err
	})

	reconnectReader(br, reader)

	// Read all data
	buf := make([]byte, 10)
	n, err := br.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "test", string(buf[:n]))

	// Next read should encounter EOF, which triggers disconnection
	// The read should block waiting for reconnection
	readDone := make(chan struct{})
	var readErr error
	var readN int

	go func() {
		defer close(readDone)
		readN, readErr = br.Read(buf)
	}()

	// Wait for EOF to be reported via error callback
	select {
	case receivedErr := <-errorReceived:
		assert.Equal(t, io.EOF, receivedErr)
	case <-time.After(time.Second):
		t.Fatal("EOF was not reported via error callback within timeout")
	}

	// Reader should be disconnected after EOF
	assert.False(t, br.Connected())

	// Read should still be blocked
	select {
	case <-readDone:
		t.Fatal("Read should be blocked waiting for reconnection after EOF")
	default:
		// Good, still blocked
	}

	// Reconnect with new data
	reader2 := newMockReader("more")
	reconnectReader(br, reader2)

	// Wait for the blocked read to complete with new data
	select {
	case <-readDone:
		require.NoError(t, readErr)
		assert.Equal(t, 4, readN)
		assert.Equal(t, "more", string(buf[:readN]))
	case <-time.After(time.Second):
		t.Fatal("Read did not unblock after reconnection")
	}
}

func BenchmarkBackedReader_Read(b *testing.B) {
	br := backedpipe.NewBackedReader()
	buf := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create fresh reader with data for each iteration
		data := strings.Repeat("x", 1024) // 1KB of data per iteration
		reader := newMockReader(data)
		reconnectReader(br, reader)

		br.Read(buf)
	}
}

func TestBackedReader_PartialReads(t *testing.T) {
	t.Parallel()

	br := backedpipe.NewBackedReader()

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

	reconnectReader(br, reader)

	// Read multiple times
	buf := make([]byte, 10)
	for i := 0; i < 5; i++ {
		n, err := br.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Equal(t, byte('A'), buf[0])
	}

	assert.Equal(t, uint64(5), br.SequenceNum())
}
