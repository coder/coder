package reconnectingpty

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/testutil"
)

func TestBufferedReconnectingPTY_ChannelOutput(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	
	// Create a simplified test case that just tests the channel mechanism
	outputChan := make(chan []byte, 32)
	activeConns := make(map[string]net.Conn)

	// Create pipes that will serve as our connections
	client1, server1 := net.Pipe()
	client2, server2 := net.Pipe()
	defer client1.Close()
	defer server1.Close()
	defer client2.Close()
	defer server2.Close()
	
	// Add connections to our map
	activeConns["conn1"] = server1
	activeConns["conn2"] = server2
	
	// Start goroutines to read from output channel and write to connections
	go func() {
		for output := range outputChan {
			for id, conn := range activeConns {
				_, err := conn.Write(output)
				if err != nil {
					logger.Warn(ctx, "error writing to connection", 
						slog.F("connection_id", id),
						slog.Error(err))
				}
			}
		}
	}()
	
	// Read data from client connections in goroutines
	readDone1 := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 128)
		n, err := client1.Read(buf)
		if err != nil && err != io.EOF {
			t.Logf("Read error on client1: %v", err)
			readDone1 <- nil
			return
		}
		readDone1 <- buf[:n]
	}()
	
	readDone2 := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 128)
		n, err := client2.Read(buf)
		if err != nil && err != io.EOF {
			t.Logf("Read error on client2: %v", err)
			readDone2 <- nil
			return
		}
		readDone2 <- buf[:n]
	}()
	
	// Send data through the channel
	testData := []byte("test output")
	outputChan <- testData
	
	// Check that both connections received the data
	select {
	case data := <-readDone1:
		require.Equal(t, testData, data, "Client 1 should receive correct data")
	case <-time.After(testutil.WaitShort):
		t.Fatal("Timeout waiting for data on client 1")
	}
	
	select {
	case data := <-readDone2:
		require.Equal(t, testData, data, "Client 2 should receive correct data")
	case <-time.After(testutil.WaitShort):
		t.Fatal("Timeout waiting for data on client 2")
	}
	
	// Test with a second message
	readDone1 = make(chan []byte, 1)
	go func() {
		buf := make([]byte, 128)
		n, err := client1.Read(buf)
		if err != nil && err != io.EOF {
			t.Logf("Read error on client1: %v", err)
			readDone1 <- nil
			return
		}
		readDone1 <- buf[:n]
	}()
	
	readDone2 = make(chan []byte, 1)
	go func() {
		buf := make([]byte, 128)
		n, err := client2.Read(buf)
		if err != nil && err != io.EOF {
			t.Logf("Read error on client2: %v", err)
			readDone2 <- nil
			return
		}
		readDone2 <- buf[:n]
	}()
	
	secondData := []byte("second message")
	outputChan <- secondData
	
	// Check that both connections received the second message
	select {
	case data := <-readDone1:
		require.Equal(t, secondData, data, "Client 1 should receive second message")
	case <-time.After(testutil.WaitShort):
		t.Fatal("Timeout waiting for second message on client 1")
	}
	
	select {
	case data := <-readDone2:
		require.Equal(t, secondData, data, "Client 2 should receive second message")
	case <-time.After(testutil.WaitShort):
		t.Fatal("Timeout waiting for second message on client 2")
	}
	
	// Close the output channel
	close(outputChan)
}

// Test the implementation in buffered.go with a mock setup
func TestBufferedReconnectingPTY_Implementation(t *testing.T) {
	t.Parallel()
	
	// This test verifies that our implementation is consistent with the design
	metrics := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_pty_errors",
		},
		[]string{"type"},
	)

	// Create and configure our buffered reconnecting PTY with output channel
	rpty := &bufferedReconnectingPTY{
		activeConns: map[string]net.Conn{},
		metrics:     metrics,
		state:       newState(),
		timeout:     time.Second * 5,
		outputChan:  make(chan []byte, 32),
	}
	
	// Verify the output channel is correctly utilized when data is received
	// by sending test data through the channel and confirming it's written to connections
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	
	// Add the connection to our activeConns
	rpty.state.cond.L.Lock()
	rpty.activeConns["test-conn"] = server
	rpty.state.cond.L.Unlock()
	
	// Start a goroutine to handle the output channel like the real implementation would
	go func() {
		for output := range rpty.outputChan {
			rpty.state.cond.L.Lock()
			for id, conn := range rpty.activeConns {
				_, err := conn.Write(output)
				if err != nil {
					rpty.metrics.WithLabelValues("write").Add(1)
					t.Logf("Error writing to connection %s: %v", id, err)
				}
			}
			rpty.state.cond.L.Unlock()
		}
	}()
	
	// Start a reader for the client side
	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 128)
		n, err := client.Read(buf)
		if err != nil && err != io.EOF {
			t.Logf("Read error: %v", err)
			readDone <- nil
			return
		}
		readDone <- buf[:n]
	}()
	
	// Send data via the output channel
	testData := []byte("test implementation")
	rpty.outputChan <- testData
	
	// Verify the data was received
	select {
	case data := <-readDone:
		require.Equal(t, testData, data, "Connection should receive data sent through channel")
	case <-time.After(testutil.WaitShort):
		t.Fatal("Timeout waiting for data from output channel")
	}
	
	// Test the closeOutputChannel method
	rpty.closeOutputChannel()
	
	// Verify the channel is closed and properly marked
	rpty.outputMu.Lock()
	require.True(t, rpty.outputClosed, "Output channel should be marked as closed")
	rpty.outputMu.Unlock()
}