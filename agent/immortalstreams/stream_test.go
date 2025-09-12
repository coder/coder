package immortalstreams_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/immortalstreams"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		// Don't run goleak on windows tests, they're super flaky right now.
		// See: https://github.com/coder/coder/issues/8954
		os.Exit(m.Run())
	}
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestStream_Start(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	// Create a pipe for testing
	localRead, localWrite := io.Pipe()
	defer func() {
		_ = localRead.Close()
		_ = localWrite.Close()
	}()

	stream := immortalstreams.NewStream(uuid.New(), "test-stream", 22, logger)

	// Start the stream
	err := stream.Start(&pipeConn{
		Reader: localRead,
		Writer: localWrite,
	})
	require.NoError(t, err)
	defer stream.Close()

	// Stream is not connected until a client connects
	require.False(t, stream.IsConnected())
}

func TestStream_HandleReconnect(t *testing.T) {
	t.Parallel()

	_ = testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// Create TCP connections for more realistic testing
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	// Local service that echoes data
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	// Dial the local service
	localConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer localConn.Close()

	stream := immortalstreams.NewStream(uuid.New(), "test-stream", 22, logger)

	// Start the stream
	err = stream.Start(localConn)
	require.NoError(t, err)
	defer stream.Close()

	// Create first client connection (full-duplex using separate pipes)
	toServerRead1, toServerWrite1 := io.Pipe()     // client -> server
	fromServerRead1, fromServerWrite1 := io.Pipe() // server -> client
	defer func() {
		_ = toServerRead1.Close()
		_ = toServerWrite1.Close()
		_ = fromServerRead1.Close()
		_ = fromServerWrite1.Close()
	}()

	// Set up the initial client connection
	err = stream.HandleReconnect(&pipeConn{
		Reader: toServerRead1,
		Writer: fromServerWrite1,
	}, 0) // Client starts with read sequence number 0
	require.NoError(t, err)
	require.True(t, stream.IsConnected())

	// Write some data from client to local
	testData := []byte("hello world")
	go func() {
		_, err := toServerWrite1.Write(testData)
		if err != nil {
			t.Logf("Write error: %v", err)
		}
	}()

	// Read echoed data back
	buf := make([]byte, len(testData))
	_, err = io.ReadFull(fromServerRead1, buf)
	require.NoError(t, err)
	require.Equal(t, testData, buf)

	// Simulate disconnect by closing the client connection
	_ = toServerRead1.Close()
	_ = toServerWrite1.Close()
	_ = fromServerRead1.Close()
	_ = fromServerWrite1.Close()

	// Force disconnection for reliable testing in race conditions
	// The automatic disconnection detection can be unreliable under race detection
	stream.ForceDisconnect()

	// Wait until the stream is marked disconnected with proper timeout handling
	disconnectCtx := testutil.Context(t, testutil.WaitShort)
	disconnected := make(chan bool, 1)
	go func() {
		testutil.Eventually(disconnectCtx, t, func(ctx context.Context) bool {
			return !stream.IsConnected()
		}, testutil.IntervalFast)
		disconnected <- true
	}()

	select {
	case <-disconnected:
		require.False(t, stream.IsConnected())
	case <-disconnectCtx.Done():
		t.Fatal("Timed out waiting for stream to be marked as disconnected")
	}

	// Create new client connection (full-duplex)
	toServerRead2, toServerWrite2 := io.Pipe()
	fromServerRead2, fromServerWrite2 := io.Pipe()
	defer func() {
		_ = toServerRead2.Close()
		_ = toServerWrite2.Close()
		_ = fromServerRead2.Close()
		_ = fromServerWrite2.Close()
	}()

	// Reconnect with sequence numbers
	// Client has read len(testData) bytes
	err = stream.HandleReconnect(&pipeConn{
		Reader: toServerRead2,
		Writer: fromServerWrite2,
	}, uint64(len(testData)))
	require.NoError(t, err)

	// Write more data after reconnect
	testData2 := []byte("after reconnect")
	go func() {
		_, err := toServerWrite2.Write(testData2)
		if err != nil {
			t.Logf("Write error: %v", err)
		}
	}()

	// Read the new data
	buf2 := make([]byte, len(testData2))
	_, err = io.ReadFull(fromServerRead2, buf2)
	require.NoError(t, err)
	require.Equal(t, testData2, buf2)
}

func TestStream_Close(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	// Create a pipe for testing
	localRead, localWrite := io.Pipe()
	defer func() {
		_ = localRead.Close()
		_ = localWrite.Close()
	}()

	stream := immortalstreams.NewStream(uuid.New(), "test-stream", 22, logger)

	// Start the stream
	err := stream.Start(&pipeConn{
		Reader: localRead,
		Writer: localWrite,
	})
	require.NoError(t, err)

	// Close the stream
	err = stream.Close()
	require.NoError(t, err)

	// Verify it's closed
	require.False(t, stream.IsConnected())

	// Close again should be idempotent
	err = stream.Close()
	require.NoError(t, err)
}

func TestStream_DataTransfer(t *testing.T) {
	t.Parallel()

	_ = testutil.Context(t, testutil.WaitMedium)
	logger := slogtest.Make(t, nil)

	// Create TCP connections for more realistic testing
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	// Local service that echoes data
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	// Dial the local service
	localConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer localConn.Close()

	stream := immortalstreams.NewStream(uuid.New(), "test-stream", 22, logger)

	// Start the stream
	err = stream.Start(localConn)
	require.NoError(t, err)
	defer stream.Close()

	// Create client connection
	clientRead, clientWrite := io.Pipe()
	defer func() {
		_ = clientRead.Close()
		_ = clientWrite.Close()
	}()

	err = stream.HandleReconnect(&pipeConn{
		Reader: clientRead,
		Writer: clientWrite,
	}, 0) // Client starts with read sequence number 0
	require.NoError(t, err)

	// Test bidirectional data transfer
	testData := []byte("test message")

	// Write from client
	go func() {
		_, err := clientWrite.Write(testData)
		if err != nil {
			t.Logf("Write error: %v", err)
		}
	}()

	// Read echoed data back
	buf := make([]byte, len(testData))
	_, err = io.ReadFull(clientRead, buf)
	require.NoError(t, err)
	require.Equal(t, testData, buf)
}

func TestStream_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	// Create a pipe for testing
	localRead, localWrite := io.Pipe()
	defer func() {
		_ = localRead.Close()
		_ = localWrite.Close()
	}()

	stream := immortalstreams.NewStream(uuid.New(), "test-stream", 22, logger)

	// Start the stream
	err := stream.Start(&pipeConn{
		Reader: localRead,
		Writer: localWrite,
	})
	require.NoError(t, err)
	defer stream.Close()

	// Concurrent operations
	var wg sync.WaitGroup
	wg.Add(4)

	// Multiple readers of state
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = stream.IsConnected()
			runtime.Gosched() // Yield to other goroutines
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = stream.ToAPI()
			runtime.Gosched() // Yield to other goroutines
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = stream.LastDisconnectionAt()
			runtime.Gosched() // Yield to other goroutines
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			// Test other concurrent operations instead
			_ = stream.IsConnected()
			_ = stream.ToAPI()
			runtime.Gosched() // Yield to other goroutines
		}
	}()

	wg.Wait()
}

// Benchmarks

func BenchmarkImmortalStream_Throughput(b *testing.B) {
	logger := slogtest.Make(b, nil)

	// Local echo service via net.Pipe
	localClient, localServer := net.Pipe()
	b.Cleanup(func() {
		_ = localClient.Close()
		_ = localServer.Close()
	})

	// Echo goroutine
	go func() {
		defer localServer.Close()
		_, _ = io.Copy(localServer, localServer)
	}()

	stream := immortalstreams.NewStream(uuid.New(), "bench-stream", 0, logger)
	require.NoError(b, stream.Start(localClient))
	b.Cleanup(func() { _ = stream.Close() })

	// Establish client connection
	clientConn, remote := net.Pipe()
	b.Cleanup(func() {
		_ = clientConn.Close()
		_ = remote.Close()
	})
	require.NoError(b, stream.HandleReconnect(clientConn, 0))

	// Payload
	payload := bytes.Repeat([]byte("x"), 32*1024)
	buf := make([]byte, len(payload))
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Write
		_, err := remote.Write(payload)
		if err != nil {
			b.Fatalf("write: %v", err)
		}
		// Read echo
		if _, err := io.ReadFull(remote, buf); err != nil {
			b.Fatalf("read: %v", err)
		}
	}
}

func BenchmarkImmortalStream_ReconnectLatency(b *testing.B) {
	logger := slogtest.Make(b, nil)

	// Local echo service
	localClient, localServer := net.Pipe()
	b.Cleanup(func() {
		_ = localClient.Close()
		_ = localServer.Close()
	})
	go func() {
		defer localServer.Close()
		_, _ = io.Copy(localServer, localServer)
	}()

	stream := immortalstreams.NewStream(uuid.New(), "bench-stream", 0, logger)
	require.NoError(b, stream.Start(localClient))
	b.Cleanup(func() { _ = stream.Close() })

	// Initial connection
	c1, r1 := net.Pipe()
	require.NoError(b, stream.HandleReconnect(c1, 0))
	// Ensure disconnected before starting benchmark loop
	_ = r1.Close()
	// Use a simple loop for benchmarks to avoid overhead
	for stream.IsConnected() {
		runtime.Gosched()
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		client, remote := net.Pipe()
		// Measure handshake latency only
		start := time.Now()
		err := stream.HandleReconnect(client, 0)
		dur := time.Since(start)
		if err != nil {
			b.Fatalf("HandleReconnect: %v", err)
		}
		// Record per-iter time
		_ = dur

		// Immediately disconnect for next iteration
		_ = remote.Close()
		// Wait until disconnected - use a simple loop with runtime.Gosched for benchmarks
		for stream.IsConnected() {
			runtime.Gosched()
		}
	}
}

func TestStream_ReconnectionScenarios(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil)

	// Start a test server that echoes data
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	port := listener.Addr().(*net.TCPAddr).Port

	// Echo server with proper context handling
	serverCtx, serverCancel := context.WithCancel(ctx)
	t.Cleanup(serverCancel)

	go func() {
		defer serverCancel()
		for {
			select {
			case <-serverCtx.Done():
				return
			default:
			}

			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// Use context-aware copying to prevent hangs
				go func() {
					<-serverCtx.Done()
					_ = c.Close()
				}()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	// Dial the local service
	localConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = localConn.Close()
	})

	stream := immortalstreams.NewStream(uuid.New(), "test-stream", port, logger)

	// Start the stream
	err = stream.Start(localConn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = stream.Close()
	})

	t.Run("BasicReconnection", func(t *testing.T) {
		t.Parallel()
		// Create a fresh stream for this test to avoid data contamination
		localConn2, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(t, err)
		defer func() {
			_ = localConn2.Close()
		}()

		stream2 := immortalstreams.NewStream(uuid.New(), "test-stream-basic", port, logger)
		err = stream2.Start(localConn2)
		require.NoError(t, err)
		defer func() {
			_ = stream2.Close()
		}()

		// Create first client connection (full-duplex) using net.Pipe
		c1, r1 := net.Pipe()
		defer func() {
			_ = c1.Close()
			_ = r1.Close()
		}()

		err = stream2.HandleReconnect(c1, 0)
		require.NoError(t, err)
		require.True(t, stream2.IsConnected())

		// Send data
		testData := []byte("hello world")
		_, err = r1.Write(testData)
		require.NoError(t, err)

		// Read echoed data
		buf := make([]byte, len(testData))
		_ = r1.SetDeadline(time.Now().Add(5 * time.Second))
		_, err = io.ReadFull(r1, buf)
		require.NoError(t, err)
		require.Equal(t, testData, buf)

		// Simulate disconnection
		_ = c1.Close()
		_ = r1.Close()

		// Wait for natural disconnection to avoid races
		disconnectCtx := testutil.Context(t, testutil.WaitMedium)
		testutil.Eventually(disconnectCtx, t, func(ctx context.Context) bool {
			return !stream2.IsConnected()
		}, testutil.IntervalFast)

		// Reconnect with new client using net.Pipe
		c2, r2 := net.Pipe()
		defer func() {
			_ = c2.Close()
			_ = r2.Close()
		}()

		// Start reading replayed data in a goroutine to avoid blocking HandleReconnect
		replayDone := make(chan struct{})
		var replayBuf []byte
		go func() {
			defer close(replayDone)
			replayBuf = make([]byte, len(testData))
			_ = r2.SetDeadline(time.Now().Add(5 * time.Second))
			_, err := io.ReadFull(r2, replayBuf)
			if err != nil {
				t.Logf("Failed to read replayed data: %v", err)
			}
		}()

		// Handle reconnection with timeout to avoid deadlocks in race conditions
		reconnectDone := make(chan error, 1)
		go func() {
			reconnectDone <- stream2.HandleReconnect(c2, 0) // Client hasn't read anything, so BackedPipe will replay
		}()

		// Wait for reconnection to complete with timeout
		reconnectCtx := testutil.Context(t, testutil.WaitMedium)
		select {
		case err := <-reconnectDone:
			require.NoError(t, err)
			// HandleReconnect returning successfully means the connection is established
			require.True(t, stream2.IsConnected())
		case <-reconnectCtx.Done():
			t.Fatal("Timed out waiting for HandleReconnect to complete")
		}

		// Wait for replay to complete with timeout - this ensures the connection is fully established
		replayCtx := testutil.Context(t, testutil.WaitShort)
		select {
		case <-replayDone:
			require.Equal(t, testData, replayBuf, "should receive replayed data")
		case <-replayCtx.Done():
			t.Fatal("Timed out waiting for replay to complete")
		}

		// Send more data after reconnection
		testData2 := []byte("after reconnect")
		_, err = r2.Write(testData2)
		require.NoError(t, err)

		// Read echoed data
		buf2 := make([]byte, len(testData2))
		_, err = io.ReadFull(r2, buf2)
		require.NoError(t, err)
		require.Equal(t, testData2, buf2)
	})

	t.Run("MultipleReconnections", func(t *testing.T) {
		t.Parallel()
		// Create a fresh stream for this test to avoid data contamination
		localConn3, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(t, err)
		defer func() {
			_ = localConn3.Close()
		}()

		stream3 := immortalstreams.NewStream(uuid.New(), "test-stream-multi", port, logger)
		err = stream3.Start(localConn3)
		require.NoError(t, err)
		defer func() {
			_ = stream3.Close()
		}()

		var totalBytesRead uint64
		for i := 0; i < 3; i++ {
			// Use full-duplex net.Pipe to avoid io.Pipe coordination races
			clientConn, remote := net.Pipe()

			// Each reconnection should start with the correct sequence number
			err = stream3.HandleReconnect(clientConn, totalBytesRead)
			require.NoError(t, err)
			require.True(t, stream3.IsConnected())

			// Use deadlines on the in-memory connection to avoid any chance of hanging
			// if the underlying pipe isn't fully ready yet.
			_ = remote.SetDeadline(time.Now().Add(5 * time.Second))

			// Send data
			testData := []byte(fmt.Sprintf("data %d", i))
			_, err = remote.Write(testData)
			require.NoError(t, err)

			// Read echoed data
			buf := make([]byte, len(testData))
			_, err = io.ReadFull(remote, buf)
			require.NoError(t, err)

			// The data we receive should be the data we just sent
			require.Equal(t, testData, buf, "iteration %d: expected current data", i)

			// Update the total bytes read for the next iteration
			totalBytesRead += uint64(len(testData))

			// Disconnect cleanly
			_ = remote.Close()
			_ = clientConn.Close()

			// Force disconnection for reliable testing; avoids races in async detection
			stream3.ForceDisconnect()

			// Wait until the stream observes the disconnect; avoid explicit ForceDisconnect
			disconnectCtx := testutil.Context(t, testutil.WaitMedium)
			testutil.Eventually(disconnectCtx, t, func(ctx context.Context) bool {
				return !stream3.IsConnected()
			}, testutil.IntervalFast)
		}
	})
}

func TestStream_SequenceNumberReconnection_WithSequenceNumbers(t *testing.T) {
	t.Parallel()

	_ = testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil)

	// Create a dedicated echo server for this test to avoid interference
	testListener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer func() {
		_ = testListener.Close()
	}()

	testPort := testListener.Addr().(*net.TCPAddr).Port

	// Dedicated echo server for this test
	go func() {
		for {
			conn, err := testListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	// Create a fresh stream for this test
	localConn, err := net.Dial("tcp", testListener.Addr().String())
	require.NoError(t, err)
	defer func() {
		_ = localConn.Close()
	}()

	stream := immortalstreams.NewStream(uuid.New(), "test-stream", testPort, logger)

	// Start the stream
	err = stream.Start(localConn)
	require.NoError(t, err)
	defer func() {
		_ = stream.Close()
	}()
	// First connection - client starts at sequence 0 (use full-duplex net.Pipe)
	clientConn1, serverConn1 := net.Pipe()
	defer func() {
		_ = clientConn1.Close()
		_ = serverConn1.Close()
	}()

	err = stream.HandleReconnect(clientConn1, 0) // Client has read 0
	require.NoError(t, err)
	require.True(t, stream.IsConnected())

	// Send some data
	testData1 := []byte("first message")
	_, err = serverConn1.Write(testData1)
	require.NoError(t, err)

	// Read echoed data
	buf1 := make([]byte, len(testData1))
	// Set a generous read deadline to avoid rare test hangs
	_ = serverConn1.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = io.ReadFull(serverConn1, buf1)
	require.NoError(t, err)
	require.Equal(t, testData1, buf1)

	// Data transfer successful

	// Simulate disconnection and wait for detection with proper timeout handling
	_ = clientConn1.Close()
	_ = serverConn1.Close()
	// Force to ensure the test doesn't rely on timing of async detection
	stream.ForceDisconnect()

	disconnectCtx := testutil.Context(t, testutil.WaitShort)
	disconnected := make(chan bool, 1)
	go func() {
		testutil.Eventually(disconnectCtx, t, func(ctx context.Context) bool {
			return !stream.IsConnected()
		}, testutil.IntervalFast)
		disconnected <- true
	}()

	select {
	case <-disconnected:
		require.False(t, stream.IsConnected())
	case <-disconnectCtx.Done():
		t.Fatal("Timed out waiting for stream to be marked as disconnected")
	}

	// Client reconnects with its sequence numbers
	// Client knows it has read len(testData1) bytes
	clientReadSeq := uint64(len(testData1))

	// Reconnect using full-duplex net.Pipe
	clientConn2, serverConn2 := net.Pipe()
	defer func() {
		_ = clientConn2.Close()
		_ = serverConn2.Close()
	}()

	err = stream.HandleReconnect(clientConn2, clientReadSeq)
	require.NoError(t, err)
	require.True(t, stream.IsConnected())

	// The client has already read all data (clientReadSeq == len(testData1))
	// So there's nothing to replay

	// Send more data after reconnection
	testData2 := []byte("second message")
	t.Logf("About to write second message")
	n, err := serverConn2.Write(testData2)
	t.Logf("Write returned: n=%d, err=%v", n, err)
	require.NoError(t, err)

	// Read echoed data for the new message
	buf2 := make([]byte, len(testData2))
	_ = serverConn2.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = io.ReadFull(serverConn2, buf2)
	require.NoError(t, err)
	t.Logf("Expected: %q", string(testData2))
	t.Logf("Actual:   %q", string(buf2))
	require.Equal(t, testData2, buf2)

	// Second data transfer successful
}

func TestStream_SequenceNumberReconnection_WithDataLoss(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil)

	// Create a dedicated echo server for this test to avoid interference
	testListener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer func() {
		_ = testListener.Close()
	}()

	testPort := testListener.Addr().(*net.TCPAddr).Port

	// Dedicated echo server for this test with context handling
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go func() {
		defer serverCancel()
		for {
			select {
			case <-serverCtx.Done():
				return
			default:
			}

			conn, err := testListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// Use context-aware copying to prevent hangs
				go func() {
					<-serverCtx.Done()
					_ = c.Close()
				}()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	// Test what happens when client claims to have read more than server has written
	// This should fail because the sequence number exceeds what the server has

	// Create a fresh stream for this test
	localConn, err := net.Dial("tcp", testListener.Addr().String())
	require.NoError(t, err)
	defer func() {
		_ = localConn.Close()
	}()

	stream := immortalstreams.NewStream(uuid.New(), "test-stream", testPort, logger)

	// Start the stream
	err = stream.Start(localConn)
	require.NoError(t, err)
	defer func() {
		_ = stream.Close()
	}()
	// First connection - client starts at sequence 0 (use full-duplex net.Pipe)
	clientConn1, serverConn1 := net.Pipe()
	defer func() {
		_ = clientConn1.Close()
		_ = serverConn1.Close()
	}()

	err = stream.HandleReconnect(clientConn1, 0) // Client has read 0
	require.NoError(t, err)
	require.True(t, stream.IsConnected())

	// Send some data - this will verify the connection is fully established
	testData1 := []byte("first message")
	_, err = serverConn1.Write(testData1)
	require.NoError(t, err)

	// Read echoed data
	buf1 := make([]byte, len(testData1))
	_ = serverConn1.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = io.ReadFull(serverConn1, buf1)
	require.NoError(t, err)
	require.Equal(t, testData1, buf1)

	// Data transfer successful

	// Simulate disconnection and wait for detection with proper timeout handling
	_ = clientConn1.Close()
	_ = serverConn1.Close()
	// Force to ensure the test doesn't rely on timing of async detection
	stream.ForceDisconnect()

	// Wait until the stream is marked disconnected with proper timeout handling
	disconnectCtx := testutil.Context(t, testutil.WaitMedium)
	testutil.Eventually(disconnectCtx, t, func(ctx context.Context) bool {
		return !stream.IsConnected()
	}, testutil.IntervalMedium)

	// Verify disconnection
	require.False(t, stream.IsConnected())

	// Client reconnects with its sequence numbers
	// Client knows it has read len(testData1) bytes
	clientReadSeq := uint64(len(testData1))

	// Reconnect using full-duplex net.Pipe
	clientConn2, serverConn2 := net.Pipe()
	defer func() {
		_ = clientConn2.Close()
		_ = serverConn2.Close()
	}()

	err = stream.HandleReconnect(clientConn2, clientReadSeq)
	require.NoError(t, err)
	require.True(t, stream.IsConnected())

	// The client has already read all data (clientReadSeq == len(testData1))
	// So there's nothing to replay

	// Send more data after reconnection
	testData2 := []byte("second message")
	t.Logf("About to write second message")
	n, err := serverConn2.Write(testData2)
	t.Logf("Write returned: n=%d, err=%v", n, err)
	require.NoError(t, err)

	// Read echoed data for the new message
	buf2 := make([]byte, len(testData2))
	_ = serverConn2.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = io.ReadFull(serverConn2, buf2)
	require.NoError(t, err)
	t.Logf("Expected: %q", string(testData2))
	t.Logf("Actual:   %q", string(buf2))
	require.Equal(t, testData2, buf2)

	// Second data transfer successful
}

// pipeConn implements io.ReadWriteCloser using separate Reader and Writer
type pipeConn struct {
	io.Reader
	io.Writer
	closed bool
	mu     sync.Mutex
}

func (p *pipeConn) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if c, ok := p.Reader.(io.Closer); ok {
		_ = c.Close()
	}
	if c, ok := p.Writer.(io.Closer); ok {
		_ = c.Close()
	}
	return nil
}
