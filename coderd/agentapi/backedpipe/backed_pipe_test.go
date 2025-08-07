package backedpipe_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/agentapi/backedpipe"
	"github.com/coder/coder/v2/testutil"
)

// mockConnection implements io.ReadWriteCloser for testing
type mockConnection struct {
	mu          sync.Mutex
	readBuffer  bytes.Buffer
	writeBuffer bytes.Buffer
	closed      bool
	readError   error
	writeError  error
	closeError  error
	readFunc    func([]byte) (int, error)
	writeFunc   func([]byte) (int, error)
	seqNum      uint64
}

func newMockConnection() *mockConnection {
	return &mockConnection{}
}

func (mc *mockConnection) Read(p []byte) (int, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.readFunc != nil {
		return mc.readFunc(p)
	}

	if mc.readError != nil {
		return 0, mc.readError
	}

	return mc.readBuffer.Read(p)
}

func (mc *mockConnection) Write(p []byte) (int, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.writeFunc != nil {
		return mc.writeFunc(p)
	}

	if mc.writeError != nil {
		return 0, mc.writeError
	}

	return mc.writeBuffer.Write(p)
}

func (mc *mockConnection) Close() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.closed = true
	return mc.closeError
}

func (mc *mockConnection) WriteString(s string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	_, _ = mc.readBuffer.WriteString(s)
}

func (mc *mockConnection) ReadString() string {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.writeBuffer.String()
}

func (mc *mockConnection) SetReadError(err error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.readError = err
}

func (mc *mockConnection) SetWriteError(err error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.writeError = err
}

func (mc *mockConnection) Reset() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.readBuffer.Reset()
	mc.writeBuffer.Reset()
	mc.readError = nil
	mc.writeError = nil
	mc.closed = false
}

// mockReconnectFunc creates a unified reconnect function with all behaviors enabled
func mockReconnectFunc(connections ...*mockConnection) (backedpipe.ReconnectFunc, *int, chan struct{}) {
	connectionIndex := 0
	callCount := 0
	signalChan := make(chan struct{}, 1)

	reconnectFn := func(ctx context.Context, writerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
		callCount++

		if connectionIndex >= len(connections) {
			return nil, 0, xerrors.New("no more connections available")
		}

		conn := connections[connectionIndex]
		connectionIndex++

		// Signal when reconnection happens
		if connectionIndex > 1 {
			select {
			case signalChan <- struct{}{}:
			default:
			}
		}

		// Determine readerSeqNum based on call count
		var readerSeqNum uint64
		switch {
		case callCount == 1:
			readerSeqNum = 0
		case conn.seqNum != 0:
			readerSeqNum = conn.seqNum
		default:
			readerSeqNum = writerSeqNum
		}

		return conn, readerSeqNum, nil
	}

	return reconnectFn, &callCount, signalChan
}

func TestBackedPipe_NewBackedPipe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reconnectFn, _, _ := mockReconnectFunc(newMockConnection())

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	require.NotNil(t, bp)
	require.False(t, bp.Connected())
}

func TestBackedPipe_Connect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, callCount, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Connect(ctx)
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 1, *callCount)
}

func TestBackedPipe_ConnectAlreadyConnected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Connect(ctx)
	require.NoError(t, err)

	// Second connect should fail
	err = bp.Connect(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already connected")
}

func TestBackedPipe_ConnectAfterClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Close()
	require.NoError(t, err)

	err = bp.Connect(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "closed")
}

func TestBackedPipe_BasicReadWrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Connect(ctx)
	require.NoError(t, err)

	// Write data
	n, err := bp.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	// Simulate data coming back
	conn.WriteString("world")

	// Read data
	buf := make([]byte, 10)
	n, err = bp.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, "world", string(buf[:n]))
}

func TestBackedPipe_WriteBeforeConnect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	// Write before connecting should succeed (buffered)
	n, err := bp.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	// Connect should replay the buffered data
	err = bp.Connect(ctx)
	require.NoError(t, err)

	// Check that data was replayed to connection
	require.Equal(t, "hello", conn.ReadString())
}

func TestBackedPipe_ReadBlocksWhenDisconnected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reconnectFn, _, _ := mockReconnectFunc(newMockConnection())

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	// Start a read that should block
	readDone := make(chan struct{})
	readStarted := make(chan struct{})
	var readErr error

	go func() {
		defer close(readDone)
		close(readStarted) // Signal that we're about to start the read
		buf := make([]byte, 10)
		_, readErr = bp.Read(buf)
	}()

	// Wait for the goroutine to start
	<-readStarted

	// Give a brief moment for the read to actually block
	time.Sleep(time.Millisecond)

	// Read should still be blocked
	select {
	case <-readDone:
		t.Fatal("Read should be blocked when disconnected")
	default:
		// Good, still blocked
	}

	// Close should unblock the read
	bp.Close()

	select {
	case <-readDone:
		require.Equal(t, io.ErrClosedPipe, readErr)
	case <-time.After(time.Second):
		t.Fatal("Read did not unblock after close")
	}
}

func TestBackedPipe_Reconnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn1 := newMockConnection()
	conn2 := newMockConnection()
	conn2.seqNum = 17 // Remote has received 17 bytes, so replay from sequence 17
	reconnectFn, _, signalChan := mockReconnectFunc(conn1, conn2)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	// Initial connect
	err := bp.Connect(ctx)
	require.NoError(t, err)

	// Write some data before failure
	bp.Write([]byte("before disconnect***"))

	// Simulate connection failure
	conn1.SetReadError(xerrors.New("connection lost"))
	conn1.SetWriteError(xerrors.New("connection lost"))

	// Trigger a write to cause the pipe to notice the failure
	_, _ = bp.Write([]byte("trigger failure "))

	<-signalChan

	err = bp.WaitForConnection(ctx)
	require.NoError(t, err)

	replayedData := conn2.ReadString()
	require.Equal(t, "***trigger failure ", replayedData, "Should replay exactly the data written after sequence 17")

	// Verify that new writes work with the reconnected pipe
	_, err = bp.Write([]byte("new data after reconnect"))
	require.NoError(t, err)

	// Read all data from the connection (replayed + new data)
	allData := conn2.ReadString()
	require.Equal(t, "***trigger failure new data after reconnect", allData, "Should have replayed data plus new data")
}

func TestBackedPipe_Close(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Connect(ctx)
	require.NoError(t, err)

	err = bp.Close()
	require.NoError(t, err)
	require.True(t, conn.closed)

	// Operations after close should fail
	_, err = bp.Read(make([]byte, 10))
	require.Equal(t, io.ErrClosedPipe, err)

	_, err = bp.Write([]byte("test"))
	require.Equal(t, io.ErrClosedPipe, err)
}

func TestBackedPipe_CloseIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = bp.Close()
	require.NoError(t, err)
}

func TestBackedPipe_WaitForConnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	// Should timeout when not connected
	// Use a shorter timeout for this test to speed up test runs
	timeoutCtx, cancel := context.WithTimeout(ctx, testutil.WaitSuperShort)
	defer cancel()

	err := bp.WaitForConnection(timeoutCtx)
	require.Equal(t, context.DeadlineExceeded, err)

	// Connect in background after a brief delay
	connectionStarted := make(chan struct{})
	go func() {
		close(connectionStarted)
		// Small delay to ensure WaitForConnection is called first
		time.Sleep(time.Millisecond)
		bp.Connect(context.Background())
	}()

	// Wait for connection goroutine to start
	<-connectionStarted

	// Should succeed once connected
	err = bp.WaitForConnection(context.Background())
	require.NoError(t, err)
}

func TestBackedPipe_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Connect(ctx)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numWriters := 3
	writesPerWriter := 10

	// Fill read buffer with test data first
	testData := make([]byte, 1000)
	for i := range testData {
		testData[i] = 'A'
	}
	conn.WriteString(string(testData))

	// Channel to collect all written data
	writtenData := make(chan byte, numWriters*writesPerWriter)

	// Start a few readers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 10)
			for j := 0; j < 10; j++ {
				bp.Read(buf)
				time.Sleep(time.Millisecond) // Small delay to avoid busy waiting
			}
		}()
	}

	// Start writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				data := []byte{byte(id + '0')}
				bp.Write(data)
				writtenData <- byte(id + '0')
				time.Sleep(time.Millisecond) // Small delay
			}
		}(i)
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out")
	}

	// Close the channel and collect all written data
	close(writtenData)
	var allWritten []byte
	for b := range writtenData {
		allWritten = append(allWritten, b)
	}

	// Verify that all written data was received by the connection
	// Note: Since this test uses the old mock that returns readerSeqNum = 0,
	// all data will be replayed, so we expect to receive all written data
	receivedData := conn.ReadString()
	require.GreaterOrEqual(t, len(receivedData), len(allWritten), "Connection should have received at least all written data")

	// Check that all written bytes appear in the received data
	for _, writtenByte := range allWritten {
		require.Contains(t, receivedData, string(writtenByte), "Written byte %c should be present in received data", writtenByte)
	}
}

func TestBackedPipe_ReconnectFunctionFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	failingReconnectFn := func(ctx context.Context, writerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
		return nil, 0, xerrors.New("reconnect failed")
	}

	bp := backedpipe.NewBackedPipe(ctx, failingReconnectFn)

	err := bp.Connect(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reconnect failed")
	require.False(t, bp.Connected())
}

func TestBackedPipe_ForceReconnect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn1 := newMockConnection()
	conn2 := newMockConnection()
	reconnectFn, callCount, _ := mockReconnectFunc(conn1, conn2)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	// Initial connect
	err := bp.Connect(ctx)
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 1, *callCount)

	// Write some data to the first connection
	_, err = bp.Write([]byte("test data"))
	require.NoError(t, err)
	require.Equal(t, "test data", conn1.ReadString())

	// Force a reconnection
	err = bp.ForceReconnect(ctx)
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 2, *callCount)

	// Since the mock now returns the proper sequence number, no data should be replayed
	// The new connection should be empty
	require.Equal(t, "", conn2.ReadString())

	// Verify that data can still be written and read after forced reconnection
	_, err = bp.Write([]byte("new data"))
	require.NoError(t, err)
	require.Equal(t, "new data", conn2.ReadString())

	// Verify that reads work with the new connection
	conn2.WriteString("response data")
	buf := make([]byte, 20)
	n, err := bp.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 13, n)
	require.Equal(t, "response data", string(buf[:n]))
}

func TestBackedPipe_ForceReconnectWhenClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	// Close the pipe first
	err := bp.Close()
	require.NoError(t, err)

	// Try to force reconnect when closed
	err = bp.ForceReconnect(ctx)
	require.Error(t, err)
	require.Equal(t, io.ErrClosedPipe, err)
}

func TestBackedPipe_ForceReconnectWhenDisconnected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, callCount, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	// Don't connect initially, just force reconnect
	err := bp.ForceReconnect(ctx)
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 1, *callCount)

	// Verify we can write and read
	_, err = bp.Write([]byte("test"))
	require.NoError(t, err)
	require.Equal(t, "test", conn.ReadString())

	conn.WriteString("response")
	buf := make([]byte, 10)
	n, err := bp.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 8, n)
	require.Equal(t, "response", string(buf[:n]))
}

func TestBackedPipe_EOFTriggersReconnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create connections where we can control when EOF occurs
	conn1 := newMockConnection()
	conn2 := newMockConnection()
	conn2.WriteString("newdata") // Pre-populate conn2 with data

	// Make conn1 return EOF after reading "world"
	hasReadData := false
	conn1.readFunc = func(p []byte) (int, error) {
		// Don't lock here - the Read method already holds the lock

		// First time: return "world"
		if !hasReadData && conn1.readBuffer.Len() > 0 {
			n, _ := conn1.readBuffer.Read(p)
			hasReadData = true
			return n, nil
		}
		// After that: return EOF
		return 0, io.EOF
	}
	conn1.WriteString("world")

	callCount := 0
	reconnectFn := func(ctx context.Context, writerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
		callCount++

		if callCount == 1 {
			return conn1, 0, nil
		}
		if callCount == 2 {
			// Second call is the reconnection after EOF
			return conn2, writerSeqNum, nil // conn2 already has the reader sequence at writerSeqNum
		}

		return nil, 0, xerrors.New("no more connections")
	}

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	// Initial connect
	err := bp.Connect(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, callCount)

	// Write some data
	_, err = bp.Write([]byte("hello"))
	require.NoError(t, err)

	buf := make([]byte, 10)

	// First read should succeed
	n, err := bp.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, "world", string(buf[:n]))

	// Next read will encounter EOF and should trigger reconnection
	// After reconnection, it should read from conn2
	n, err = bp.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 7, n)
	require.Equal(t, "newdata", string(buf[:n]))

	// Verify reconnection happened
	require.Equal(t, 2, callCount)

	// Verify the pipe is still connected and functional
	require.True(t, bp.Connected())

	// Further writes should go to the new connection
	_, err = bp.Write([]byte("aftereof"))
	require.NoError(t, err)
	require.Equal(t, "aftereof", conn2.ReadString())
}

func BenchmarkBackedPipe_Write(b *testing.B) {
	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	bp.Connect(ctx)

	data := make([]byte, 1024) // 1KB writes

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bp.Write(data)
	}
}

func BenchmarkBackedPipe_Read(b *testing.B) {
	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	bp.Connect(ctx)

	buf := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Fill connection with fresh data for each iteration
		conn.WriteString(string(buf))
		bp.Read(buf)
	}
}
