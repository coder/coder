package backedpipe_test

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/immortalstreams/backedpipe"
	"github.com/coder/coder/v2/testutil"
)

const forceReconnectSingleflightKey = "force-reconnect"

// singleflightDupsForBackedPipe returns the current singleflight duplicate count
// for the given key.
//
// This is test-only introspection used to avoid flakes caused by goroutine
// scheduling between signaling "about to call" and actually entering
// ForceReconnect's singleflight.
func singleflightDupsForBackedPipe(bp *backedpipe.BackedPipe, key string) (int, bool) {
	if bp == nil {
		return 0, false
	}

	sfVal := reflect.ValueOf(bp).Elem().FieldByName("sf")
	if !sfVal.IsValid() {
		return 0, false
	}

	muVal := sfVal.FieldByName("mu")
	if !muVal.IsValid() {
		return 0, false
	}

	mu := (*sync.Mutex)(unsafe.Pointer(muVal.Addr().Pointer()))
	mu.Lock()
	defer mu.Unlock()

	mVal := sfVal.FieldByName("m")
	if !mVal.IsValid() || mVal.IsNil() {
		return 0, false
	}

	callVal := mVal.MapIndex(reflect.ValueOf(key))
	if !callVal.IsValid() || callVal.IsNil() {
		return 0, false
	}

	dupsVal := callVal.Elem().FieldByName("dups")
	if !dupsVal.IsValid() {
		return 0, false
	}

	return int(dupsVal.Int()), true
}

// mockConnection implements io.ReadWriteCloser for testing.
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

// mockReconnector implements the Reconnector interface for testing
type mockReconnector struct {
	mu              sync.Mutex
	connections     []*mockConnection
	connectionIndex int
	callCount       int
	signalChan      chan struct{}
}

// Reconnect implements the Reconnector interface
func (m *mockReconnector) Reconnect(ctx context.Context, readerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++

	if m.connectionIndex >= len(m.connections) {
		return nil, 0, xerrors.New("no more connections available")
	}

	conn := m.connections[m.connectionIndex]
	m.connectionIndex++

	// Signal when reconnection happens
	if m.connectionIndex > 1 {
		select {
		case m.signalChan <- struct{}{}:
		default:
		}
	}

	// Determine remoteReaderSeqNum (how many bytes of our outbound data the remote has read)
	var remoteReaderSeqNum uint64
	switch {
	case m.callCount == 1:
		remoteReaderSeqNum = 0
	case conn.seqNum != 0:
		remoteReaderSeqNum = conn.seqNum
	default:
		// Default to 0 if unspecified
		remoteReaderSeqNum = 0
	}

	return conn, remoteReaderSeqNum, nil
}

// GetCallCount returns the current call count in a thread-safe manner
func (m *mockReconnector) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// mockReconnectFunc creates a unified reconnector with all behaviors enabled
func mockReconnectFunc(connections ...*mockConnection) (*mockReconnector, chan struct{}) {
	signalChan := make(chan struct{}, 1)

	reconnector := &mockReconnector{
		connections: connections,
		signalChan:  signalChan,
	}

	return reconnector, signalChan
}

// blockingReconnector is a reconnector that blocks on a channel for deterministic testing
type blockingReconnector struct {
	conn1       *mockConnection
	conn2       *mockConnection
	callCount   int
	blockChan   <-chan struct{}
	blockedChan chan struct{}
	mu          sync.Mutex
	signalOnce  sync.Once // Ensure we only signal once for the first actual reconnect
}

func (b *blockingReconnector) Reconnect(ctx context.Context, readerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
	b.mu.Lock()
	b.callCount++
	currentCall := b.callCount
	b.mu.Unlock()

	if currentCall == 1 {
		// Initial connect
		return b.conn1, 0, nil
	}

	// Signal that we're about to block, but only once for the first reconnect attempt
	// This ensures we properly test singleflight deduplication
	b.signalOnce.Do(func() {
		select {
		case b.blockedChan <- struct{}{}:
		default:
			// If channel is full, don't block
		}
	})

	// For subsequent calls, block until channel is closed
	select {
	case <-b.blockChan:
		// Channel closed, proceed with reconnection
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}

	return b.conn2, 0, nil
}

// GetCallCount returns the current call count in a thread-safe manner
func (b *blockingReconnector) GetCallCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.callCount
}

func mockBlockingReconnectFunc(conn1, conn2 *mockConnection, blockChan <-chan struct{}) (*blockingReconnector, chan struct{}) {
	blockedChan := make(chan struct{}, 1)
	reconnector := &blockingReconnector{
		conn1:       conn1,
		conn2:       conn2,
		blockChan:   blockChan,
		blockedChan: blockedChan,
	}

	return reconnector, blockedChan
}

// eofTestReconnector is a custom reconnector for the EOF test case
type eofTestReconnector struct {
	mu        sync.Mutex
	conn1     io.ReadWriteCloser
	conn2     io.ReadWriteCloser
	callCount int
}

func (e *eofTestReconnector) Reconnect(ctx context.Context, readerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.callCount++

	if e.callCount == 1 {
		return e.conn1, 0, nil
	}
	if e.callCount == 2 {
		// Second call is the reconnection after EOF
		// Return 5 to indicate remote has read all 5 bytes of "hello"
		return e.conn2, 5, nil
	}

	return nil, 0, xerrors.New("no more connections")
}

// GetCallCount returns the current call count in a thread-safe manner
func (e *eofTestReconnector) GetCallCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.callCount
}

func TestBackedPipe_NewBackedPipe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reconnectFn, _ := mockReconnectFunc(newMockConnection())

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	defer bp.Close()
	require.NotNil(t, bp)
	require.False(t, bp.Connected())
}

func TestBackedPipe_Connect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnector, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnector)
	defer bp.Close()

	err := bp.Connect()
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 1, reconnector.GetCallCount())
}

func TestBackedPipe_ConnectAlreadyConnected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	defer bp.Close()

	err := bp.Connect()
	require.NoError(t, err)

	// Second connect should fail
	err = bp.Connect()
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrPipeAlreadyConnected)
}

func TestBackedPipe_ConnectAfterClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Close()
	require.NoError(t, err)

	err = bp.Connect()
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrPipeClosed)
}

func TestBackedPipe_BasicReadWrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	defer bp.Close()

	err := bp.Connect()
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
	ctx := testutil.Context(t, testutil.WaitShort)

	conn := newMockConnection()
	reconnectFn, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	defer bp.Close()

	// Write before connecting should block
	writeComplete := make(chan error, 1)
	go func() {
		_, err := bp.Write([]byte("hello"))
		writeComplete <- err
	}()

	// Verify write is blocked
	select {
	case <-writeComplete:
		t.Fatal("Write should have blocked when disconnected")
	case <-time.After(100 * time.Millisecond):
		// Expected - write is blocked
	}

	// Connect should unblock the write
	err := bp.Connect()
	require.NoError(t, err)

	// Write should now complete
	err = testutil.RequireReceive(ctx, t, writeComplete)
	require.NoError(t, err)

	// Check that data was replayed to connection
	require.Equal(t, "hello", conn.ReadString())
}

func TestBackedPipe_ReadBlocksWhenDisconnected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testCtx := testutil.Context(t, testutil.WaitShort)
	reconnectFn, _ := mockReconnectFunc(newMockConnection())

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	defer bp.Close()

	// Start a read that should block
	readDone := make(chan struct{})
	readStarted := make(chan struct{}, 1)
	var readErr error

	go func() {
		defer close(readDone)
		readStarted <- struct{}{} // Signal that we're about to start the read
		buf := make([]byte, 10)
		_, readErr = bp.Read(buf)
	}()

	// Wait for the goroutine to start
	testutil.TryReceive(testCtx, t, readStarted)

	// Ensure the read is actually blocked by verifying it hasn't completed
	require.Eventually(t, func() bool {
		select {
		case <-readDone:
			t.Fatal("Read should be blocked when disconnected")
			return false
		default:
			// Good, still blocked
			return true
		}
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Close should unblock the read
	bp.Close()

	testutil.TryReceive(testCtx, t, readDone)
	require.Equal(t, io.EOF, readErr)
}

func TestBackedPipe_Reconnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testCtx := testutil.Context(t, testutil.WaitShort)
	conn1 := newMockConnection()
	conn2 := newMockConnection()
	conn2.seqNum = 17 // Remote has received 17 bytes, so replay from sequence 17
	reconnectFn, signalChan := mockReconnectFunc(conn1, conn2)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	defer bp.Close()

	// Initial connect
	err := bp.Connect()
	require.NoError(t, err)

	// Write some data before failure
	bp.Write([]byte("before disconnect***"))

	// Simulate connection failure
	conn1.SetReadError(xerrors.New("connection lost"))
	conn1.SetWriteError(xerrors.New("connection lost"))

	// Trigger a write to cause the pipe to notice the failure
	_, _ = bp.Write([]byte("trigger failure "))

	testutil.RequireReceive(testCtx, t, signalChan)

	// Wait for reconnection to complete
	require.Eventually(t, func() bool {
		return bp.Connected()
	}, testutil.WaitShort, testutil.IntervalFast, "pipe should reconnect")

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
	reconnectFn, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Connect()
	require.NoError(t, err)

	err = bp.Close()
	require.NoError(t, err)
	require.True(t, conn.closed)

	// Operations after close should fail
	_, err = bp.Read(make([]byte, 10))
	require.Equal(t, io.EOF, err)

	_, err = bp.Write([]byte("test"))
	require.Equal(t, io.EOF, err)
}

func TestBackedPipe_CloseIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	err := bp.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = bp.Close()
	require.NoError(t, err)
}

func TestBackedPipe_ReconnectFunctionFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	failingReconnector := &mockReconnector{
		connections: nil, // No connections available
	}

	bp := backedpipe.NewBackedPipe(ctx, failingReconnector)
	defer bp.Close()

	err := bp.Connect()
	require.Error(t, err)
	require.ErrorIs(t, err, backedpipe.ErrReconnectFailed)
	require.False(t, bp.Connected())
}

func TestBackedPipe_ForceReconnect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn1 := newMockConnection()
	conn2 := newMockConnection()
	// Set conn2 sequence number to 9 to indicate remote has read all 9 bytes of "test data"
	conn2.seqNum = 9
	reconnector, _ := mockReconnectFunc(conn1, conn2)

	bp := backedpipe.NewBackedPipe(ctx, reconnector)
	defer bp.Close()

	// Initial connect
	err := bp.Connect()
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 1, reconnector.GetCallCount())

	// Write some data to the first connection
	_, err = bp.Write([]byte("test data"))
	require.NoError(t, err)
	require.Equal(t, "test data", conn1.ReadString())

	// Force a reconnection
	err = bp.ForceReconnect()
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 2, reconnector.GetCallCount())

	// Since the mock returns the proper sequence number, no data should be replayed
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
	reconnectFn, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)

	// Close the pipe first
	err := bp.Close()
	require.NoError(t, err)

	// Try to force reconnect when closed
	err = bp.ForceReconnect()
	require.Error(t, err)
	require.Equal(t, io.EOF, err)
}

func TestBackedPipe_StateTransitionsAndGenerationTracking(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn1 := newMockConnection()
	conn2 := newMockConnection()
	conn3 := newMockConnection()
	reconnector, signalChan := mockReconnectFunc(conn1, conn2, conn3)

	bp := backedpipe.NewBackedPipe(ctx, reconnector)
	defer bp.Close()

	// Initial state should be disconnected
	require.False(t, bp.Connected())

	// Connect should transition to connected
	err := bp.Connect()
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 1, reconnector.GetCallCount())

	// Write some data
	_, err = bp.Write([]byte("test data gen 1"))
	require.NoError(t, err)

	// Simulate connection failure by setting errors on connection
	conn1.SetReadError(xerrors.New("connection lost"))
	conn1.SetWriteError(xerrors.New("connection lost"))

	// Trigger a write to cause the pipe to notice the failure
	_, _ = bp.Write([]byte("trigger failure"))

	// Wait for reconnection signal
	testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, signalChan)

	// Wait for reconnection to complete
	require.Eventually(t, func() bool {
		return bp.Connected()
	}, testutil.WaitShort, testutil.IntervalFast, "should reconnect")
	require.Equal(t, 2, reconnector.GetCallCount())

	// Force another reconnection
	err = bp.ForceReconnect()
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 3, reconnector.GetCallCount())

	// Close should transition to closed state
	err = bp.Close()
	require.NoError(t, err)
	require.False(t, bp.Connected())

	// Operations on closed pipe should fail
	err = bp.Connect()
	require.Equal(t, backedpipe.ErrPipeClosed, err)

	err = bp.ForceReconnect()
	require.Equal(t, io.EOF, err)
}

func TestBackedPipe_GenerationFiltering(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn1 := newMockConnection()
	conn2 := newMockConnection()
	reconnector, _ := mockReconnectFunc(conn1, conn2)

	bp := backedpipe.NewBackedPipe(ctx, reconnector)
	defer bp.Close()

	// Connect
	err := bp.Connect()
	require.NoError(t, err)
	require.True(t, bp.Connected())

	// Simulate multiple rapid errors from the same connection generation
	// Only the first one should trigger reconnection
	conn1.SetReadError(xerrors.New("error 1"))
	conn1.SetWriteError(xerrors.New("error 2"))

	// Trigger multiple errors quickly
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = bp.Write([]byte("trigger error 1"))
	}()
	go func() {
		defer wg.Done()
		_, _ = bp.Write([]byte("trigger error 2"))
	}()

	// Wait for both writes to complete
	wg.Wait()

	// Wait for reconnection to complete
	require.Eventually(t, func() bool {
		return bp.Connected()
	}, testutil.WaitShort, testutil.IntervalFast, "should reconnect once")

	// Should have only reconnected once despite multiple errors
	require.Equal(t, 2, reconnector.GetCallCount()) // Initial connect + 1 reconnect
}

func TestBackedPipe_DuplicateReconnectionPrevention(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testCtx := testutil.Context(t, testutil.WaitShort)

	// Create a blocking reconnector for deterministic testing
	conn1 := newMockConnection()
	conn2 := newMockConnection()
	blockChan := make(chan struct{})
	reconnector, blockedChan := mockBlockingReconnectFunc(conn1, conn2, blockChan)

	bp := backedpipe.NewBackedPipe(ctx, reconnector)
	defer bp.Close()

	// Initial connect
	err := bp.Connect()
	require.NoError(t, err)
	require.Equal(t, 1, reconnector.GetCallCount(), "should have exactly 1 call after initial connect")

	// We'll use channels to coordinate the test execution:
	// 1. Start all goroutines but have them wait
	// 2. Release the first one and wait for it to block
	// 3. Release the others while the first is still blocked

	const numConcurrent = 3
	startSignals := make([]chan struct{}, numConcurrent)
	startedSignals := make([]chan struct{}, numConcurrent)
	for i := range startSignals {
		startSignals[i] = make(chan struct{})
		startedSignals[i] = make(chan struct{})
	}

	errors := make([]error, numConcurrent)
	var wg sync.WaitGroup

	// Start all goroutines
	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Wait for the signal to start
			<-startSignals[idx]
			// Signal that we're about to call ForceReconnect
			close(startedSignals[idx])
			errors[idx] = bp.ForceReconnect()
		}(i)
	}

	// Start the first ForceReconnect and wait for it to block
	close(startSignals[0])
	<-startedSignals[0]

	// Wait for the first reconnect to actually start and block
	testutil.RequireReceive(testCtx, t, blockedChan)

	// Now start all the other ForceReconnect calls
	// They should all join the same singleflight operation
	for i := 1; i < numConcurrent; i++ {
		close(startSignals[i])
	}

	// Wait for all additional goroutines to have started their calls
	for i := 1; i < numConcurrent; i++ {
		<-startedSignals[i]
	}

	// Ensure followers *actually* joined the in-flight singleflight call before we
	// unblock the leader reconnect. Otherwise a follower can be preempted after
	// signaling startedSignals[i] but before it enters ForceReconnect, and then
	// run after the leader completes, causing an extra reconnect (flake).
	require.Eventually(t, func() bool {
		dups, ok := singleflightDupsForBackedPipe(bp, forceReconnectSingleflightKey)
		return ok && dups == numConcurrent-1
	}, testutil.WaitShort, testutil.IntervalFast, "all ForceReconnect calls should join the same singleflight")

	// At this point, one reconnect has started and is blocked,
	// and all other goroutines have called ForceReconnect and should be
	// waiting on the same singleflight operation.
	// Due to singleflight, only one reconnect should have been attempted.
	require.Equal(t, 2, reconnector.GetCallCount(), "should have exactly 2 calls: initial connect + 1 reconnect due to singleflight")

	// Release the blocking reconnect function
	close(blockChan)

	// Wait for all ForceReconnect calls to complete
	wg.Wait()

	// All calls should succeed (they share the same result from singleflight)
	for i, err := range errors {
		require.NoError(t, err, "ForceReconnect %d should succeed", i, err)
	}

	// Final verification: call count should still be exactly 2
	require.Equal(t, 2, reconnector.GetCallCount(), "final call count should be exactly 2: initial connect + 1 singleflight reconnect")
}

func TestBackedPipe_SingleReconnectionOnMultipleErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testCtx := testutil.Context(t, testutil.WaitShort)

	// Create connections for initial connect and reconnection
	conn1 := newMockConnection()
	conn2 := newMockConnection()
	reconnector, signalChan := mockReconnectFunc(conn1, conn2)

	bp := backedpipe.NewBackedPipe(ctx, reconnector)
	defer bp.Close()

	// Initial connect
	err := bp.Connect()
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 1, reconnector.GetCallCount())

	// Write some initial data to establish the connection
	_, err = bp.Write([]byte("initial data"))
	require.NoError(t, err)

	// Set up both read and write errors on the connection
	conn1.SetReadError(xerrors.New("read connection lost"))
	conn1.SetWriteError(xerrors.New("write connection lost"))

	// Trigger write error (this will trigger reconnection)
	go func() {
		_, _ = bp.Write([]byte("trigger write error"))
	}()

	// Wait for reconnection to start
	testutil.RequireReceive(testCtx, t, signalChan)

	// Wait for reconnection to complete
	require.Eventually(t, func() bool {
		return bp.Connected()
	}, testutil.WaitShort, testutil.IntervalFast, "should reconnect after write error")

	// Verify that only one reconnection occurred
	require.Equal(t, 2, reconnector.GetCallCount(), "should have exactly 2 calls: initial connect + 1 reconnection")
	require.True(t, bp.Connected(), "should be connected after reconnection")
}

func TestBackedPipe_ForceReconnectWhenDisconnected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	conn := newMockConnection()
	reconnector, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnector)
	defer bp.Close()

	// Don't connect initially, just force reconnect
	err := bp.ForceReconnect()
	require.NoError(t, err)
	require.True(t, bp.Connected())
	require.Equal(t, 1, reconnector.GetCallCount())

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

	reconnector := &eofTestReconnector{
		conn1: conn1,
		conn2: conn2,
	}

	bp := backedpipe.NewBackedPipe(ctx, reconnector)
	defer bp.Close()

	// Initial connect
	err := bp.Connect()
	require.NoError(t, err)
	require.Equal(t, 1, reconnector.GetCallCount())

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
	require.Equal(t, 2, reconnector.GetCallCount())

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
	reconnectFn, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	bp.Connect()
	b.Cleanup(func() {
		_ = bp.Close()
	})

	data := make([]byte, 1024) // 1KB writes

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bp.Write(data)
	}
}

func BenchmarkBackedPipe_Read(b *testing.B) {
	ctx := context.Background()
	conn := newMockConnection()
	reconnectFn, _ := mockReconnectFunc(conn)

	bp := backedpipe.NewBackedPipe(ctx, reconnectFn)
	bp.Connect()
	b.Cleanup(func() {
		_ = bp.Close()
	})

	buf := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Fill connection with fresh data for each iteration
		conn.WriteString(string(buf))
		bp.Read(buf)
	}
}
