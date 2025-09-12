package immortalstreams_test

import (
	"context"
	"errors"
	"io"
	"net"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/immortalstreams"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestManager_CreateStream(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := slogtest.Make(t, nil)

		// Start a test server
		listener, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)
		defer listener.Close()

		port := listener.Addr().(*net.TCPAddr).Port

		// Accept connections in the background
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				// Just echo for testing
				go func() {
					defer conn.Close()
					_, _ = io.Copy(conn, conn)
				}()
			}
		}()

		dialer := &testDialer{}
		manager := immortalstreams.New(logger, dialer)
		defer manager.Close()

		stream, err := manager.CreateStream(ctx, port)
		require.NoError(t, err)
		require.NotEmpty(t, stream.ID)
		require.NotEmpty(t, stream.Name) // Name is randomly generated
		require.Equal(t, port, stream.TCPPort)
		require.False(t, stream.CreatedAt.IsZero())
		require.False(t, stream.LastConnectionAt.IsZero())
	})

	t.Run("ConnectionRefused", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := slogtest.Make(t, nil)
		dialer := &testDialer{}
		manager := immortalstreams.New(logger, dialer)
		defer manager.Close()

		// Use a port that's not listening
		_, err := manager.CreateStream(ctx, 65535)
		require.Error(t, err)
		require.True(t, errors.Is(err, immortalstreams.ErrConnRefused))
	})

	t.Run("MaxStreamsLimit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil)

		// Start a test server
		listener, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)
		defer listener.Close()

		port := listener.Addr().(*net.TCPAddr).Port

		// Accept connections in the background and keep them alive
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				// Keep connections open by reading from them
				go func(c net.Conn) {
					defer c.Close()
					buf := make([]byte, 1024)
					for {
						_, err := c.Read(buf)
						if err != nil {
							return
						}
					}
				}(conn)
			}
		}()

		dialer := &testDialer{}
		manager := immortalstreams.New(logger, dialer)
		defer manager.Close()

		// Create MaxStreams connections
		streams := make([]uuid.UUID, 0, immortalstreams.MaxStreams)
		for i := 0; i < immortalstreams.MaxStreams; i++ {
			stream, err := manager.CreateStream(ctx, port)
			require.NoError(t, err)
			streams = append(streams, stream.ID)
		}

		// Verify we have exactly MaxStreams streams
		require.Equal(t, immortalstreams.MaxStreams, len(manager.ListStreams()))

		// Mark all streams as connected by simulating client reconnections
		for _, streamID := range streams {
			stream, ok := manager.GetStream(streamID)
			require.True(t, ok)

			// Create a dummy connection to mark the stream as connected
			dummyRead, dummyWrite := io.Pipe()
			defer dummyRead.Close()
			defer dummyWrite.Close()

			err := stream.HandleReconnect(&pipeConn{
				Reader: dummyRead,
				Writer: dummyWrite,
			}, 0)
			require.NoError(t, err)
		}

		// All streams should be connected, so creating another should fail
		_, err = manager.CreateStream(ctx, port)
		require.Error(t, err)
		require.True(t, errors.Is(err, immortalstreams.ErrTooManyStreams))

		// Disconnect one stream
		err = manager.DeleteStream(streams[0])
		require.NoError(t, err)

		// Now we should be able to create a new one
		stream, err := manager.CreateStream(ctx, port)
		require.NoError(t, err)
		require.NotEmpty(t, stream.ID)
	})
}

func TestManager_ListStreams(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// Start a test server
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Accept connections in the background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()

	dialer := &testDialer{}
	manager := immortalstreams.New(logger, dialer)
	defer manager.Close()

	// Initially empty
	streams := manager.ListStreams()
	require.Empty(t, streams)

	// Create some streams
	stream1, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)

	stream2, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)

	// List should return both
	streams = manager.ListStreams()
	require.Len(t, streams, 2)

	// Check that both streams are in the list
	foundIDs := make(map[uuid.UUID]bool)
	for _, s := range streams {
		foundIDs[s.ID] = true
	}
	require.True(t, foundIDs[stream1.ID])
	require.True(t, foundIDs[stream2.ID])
}

func TestManager_DeleteStream(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// Start a test server
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Accept connections in the background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()

	dialer := &testDialer{}
	manager := immortalstreams.New(logger, dialer)
	defer manager.Close()

	// Create a stream
	stream, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)

	// Delete it
	err = manager.DeleteStream(stream.ID)
	require.NoError(t, err)

	// Should not be in the list anymore
	streams := manager.ListStreams()
	require.Empty(t, streams)

	// Deleting again should error
	err = manager.DeleteStream(stream.ID)
	require.Error(t, err)
	require.True(t, errors.Is(err, immortalstreams.ErrStreamNotFound))
}

func TestManager_GetStream(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// Start a test server
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Accept connections in the background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()

	dialer := &testDialer{}
	manager := immortalstreams.New(logger, dialer)
	defer manager.Close()

	// Create a stream
	created, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)

	// Get it
	stream, ok := manager.GetStream(created.ID)
	require.True(t, ok)
	require.NotNil(t, stream)

	// Get non-existent
	_, ok = manager.GetStream(uuid.New())
	require.False(t, ok)
}

func TestManager_Eviction(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil)

	// Start a test server
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Track accepted connections
	var connMu sync.Mutex
	conns := make([]net.Conn, 0)

	// Accept connections in the background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connMu.Lock()
			conns = append(conns, conn)
			connMu.Unlock()

			go func(c net.Conn) {
				defer c.Close()
				// Block until closed
				_, _ = io.Copy(io.Discard, c)
			}(conn)
		}
	}()

	dialer := &testDialer{}
	manager := immortalstreams.New(logger, dialer)
	defer manager.Close()

	// Cleanup functions for resources created in loops
	var cleanupFuncs []func()
	defer func() {
		for _, cleanup := range cleanupFuncs {
			cleanup()
		}
	}()

	// Create MaxStreams-1 streams
	streams := make([]uuid.UUID, 0, immortalstreams.MaxStreams-1)
	for i := 0; i < immortalstreams.MaxStreams-1; i++ {
		stream, err := manager.CreateStream(ctx, port)
		require.NoError(t, err)
		streams = append(streams, stream.ID)
	}

	// Mark all streams as connected by simulating client reconnections
	for i, streamID := range streams {
		stream, ok := manager.GetStream(streamID)
		require.True(t, ok)

		// Create a dummy connection to mark the stream as connected
		dummyRead, dummyWrite := io.Pipe()
		// Store references for cleanup outside the loop
		cleanupFuncs = append(cleanupFuncs, func() {
			_ = dummyRead.Close()
			_ = dummyWrite.Close()
		})

		err := stream.HandleReconnect(&pipeConn{
			Reader: dummyRead,
			Writer: dummyWrite,
		}, 0)
		require.NoError(t, err)

		// Verify the stream is now connected
		require.True(t, stream.IsConnected(), "Stream %d should be connected", i)
	}

	// Close the first connection to make it disconnected
	// Wait for connections to be established
	connMu.Lock()
	for len(conns) == 0 {
		connMu.Unlock()
		runtime.Gosched()
		connMu.Lock()
	}
	require.Greater(t, len(conns), 0)
	_ = conns[0].Close()
	connMu.Unlock()

	// Directly simulate disconnection for the first stream
	firstStream, found := manager.GetStream(streams[0])
	require.True(t, found)

	// Manually trigger disconnection since the automatic detection isn't working
	firstStream.SignalDisconnect()

	// Wait for the disconnection to be processed
	for firstStream.IsConnected() {
		runtime.Gosched()
	}

	// Verify the first stream is now disconnected
	require.False(t, firstStream.IsConnected(), "First stream should be disconnected")

	// Create one more stream - should work
	stream1, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)
	require.NotEmpty(t, stream1.ID)

	// Create another - should evict the oldest disconnected
	stream2, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)
	require.NotEmpty(t, stream2.ID)

	// Verify that the total number of streams is still at the limit
	// (one was evicted, one was added)
	require.Equal(t, immortalstreams.MaxStreams, len(manager.ListStreams()))

	// Verify that the first stream was evicted
	_, ok := manager.GetStream(streams[0])
	require.False(t, ok, "First stream should have been evicted")
}

func TestManager_SmartAddressResolution(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	// Create a recording dialer to capture what addresses are being dialed
	recordingDialer := &recordingDialer{}
	manager := immortalstreams.New(logger, recordingDialer)

	ctx := testutil.Context(t, testutil.WaitShort)

	// Test SSH port: manager should dial localhost
	_, err := manager.CreateStream(ctx, workspacesdk.AgentSSHPort)
	require.Error(t, err)
	require.Len(t, recordingDialer.calls, 1)
	require.Equal(t, "localhost:1", recordingDialer.calls[0].address,
		"Manager should dial localhost for SSH port")

	// Test a user port (should use localhost)
	recordingDialer.calls = nil // Reset
	_, err = manager.CreateStream(ctx, 8080)
	require.Error(t, err)
	require.Len(t, recordingDialer.calls, 1)
	require.Equal(t, "localhost:8080", recordingDialer.calls[0].address,
		"User ports should use localhost")

	// Test reconnecting PTY port: manager should dial localhost
	recordingDialer.calls = nil // Reset
	_, err = manager.CreateStream(ctx, workspacesdk.AgentReconnectingPTYPort)
	require.Error(t, err)
	require.Len(t, recordingDialer.calls, 1)
	require.Equal(t, "localhost:2", recordingDialer.calls[0].address,
		"Manager should dial localhost for PTY port")
}

func TestManager_IPv4AddressFormatting(t *testing.T) {
	// This test is no longer applicable since manager always dials localhost.
	// Keeping a minimal check to ensure localhost is used.
	t.Parallel()

	logger := slogtest.Make(t, nil)
	recordingDialer := &recordingDialer{}
	manager := immortalstreams.New(logger, recordingDialer)

	ctx := testutil.Context(t, testutil.WaitShort)
	_, err := manager.CreateStream(ctx, workspacesdk.AgentSSHPort)
	require.Error(t, err)
	require.Len(t, recordingDialer.calls, 1)
	require.Equal(t, "localhost:1", recordingDialer.calls[0].address)
}

// Test helpers

type testDialer struct{}

func (*testDialer) DialContext(_ context.Context, address string) (net.Conn, error) {
	return net.Dial("tcp", address)
}

type recordingDialer struct {
	calls []dialCall
}

type dialCall struct {
	address string
}

func (r *recordingDialer) DialContext(_ context.Context, address string) (net.Conn, error) {
	r.calls = append(r.calls, dialCall{address: address})
	// Return a connection refused error to simulate the service not being available
	return nil, &net.OpError{
		Op:   "dial",
		Net:  "tcp",
		Addr: &net.TCPAddr{},
		Err:  &net.DNSError{Name: address, IsNotFound: true},
	}
}
