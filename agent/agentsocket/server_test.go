package agentsocket

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

func TestServer_StartStop(t *testing.T) {
	t.Parallel()

	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create server
	server := NewServer(Config{
		Path:   socketPath,
		Logger: slog.Make().Leveled(slog.LevelDebug),
	})

	// Register a test handler
	server.RegisterHandler("test", func(ctx Context, req *Request) (*Response, error) {
		return NewResponse(req.ID, map[string]string{"message": "test response"})
	})

	// Start server
	err := server.Start()
	require.NoError(t, err)
	defer server.Stop()

	// Verify socket file exists
	_, err = os.Stat(socketPath)
	require.NoError(t, err)

	// Test connection
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send test request
	req := Request{
		Version: "1.0",
		Method:  "test",
		ID:      "test-1",
	}

	err = json.NewEncoder(conn).Encode(req)
	require.NoError(t, err)

	// Read response
	var resp Response
	err = json.NewDecoder(conn).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, "test-1", resp.ID)
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)

	// Verify response content
	var result map[string]string
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	assert.Equal(t, "test response", result["message"])
}

func TestServer_ErrorHandling(t *testing.T) {
	t.Parallel()

	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create server
	server := NewServer(Config{
		Path:   socketPath,
		Logger: slog.Make().Leveled(slog.LevelDebug),
	})

	// Start server
	err := server.Start()
	require.NoError(t, err)
	defer server.Stop()

	// Test connection
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send request for non-existent method
	req := Request{
		Version: "1.0",
		Method:  "nonexistent",
		ID:      "test-1",
	}

	err = json.NewEncoder(conn).Encode(req)
	require.NoError(t, err)

	// Read response
	var resp Response
	err = json.NewDecoder(conn).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, "test-1", resp.ID)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, ErrCodeMethodNotFound, resp.Error.Code)
	assert.Equal(t, "Method not found", resp.Error.Message)
}

func TestServer_DefaultHandlers(t *testing.T) {
	t.Parallel()

	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create server
	server := NewServer(Config{
		Path:   socketPath,
		Logger: slog.Make().Leveled(slog.LevelDebug),
	})

	// Register default handlers
	handlerCtx := CreateHandlerContext(
		"test-agent-id",
		"1.0.0",
		"ready",
		time.Now().Add(-time.Hour),
		slog.Make(),
	)
	RegisterDefaultHandlers(server, handlerCtx)

	// Start server
	err := server.Start()
	require.NoError(t, err)
	defer server.Stop()

	// Test ping
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	req := Request{
		Version: "1.0",
		Method:  "ping",
		ID:      "ping-1",
	}

	err = json.NewEncoder(conn).Encode(req)
	require.NoError(t, err)

	var resp Response
	err = json.NewDecoder(conn).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, "ping-1", resp.ID)
	assert.Nil(t, resp.Error)

	var pingResp PingResponse
	err = json.Unmarshal(resp.Result, &pingResp)
	require.NoError(t, err)
	assert.Equal(t, "pong", pingResp.Message)
}

func TestServer_ConcurrentConnections(t *testing.T) {
	t.Parallel()

	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create server
	server := NewServer(Config{
		Path:   socketPath,
		Logger: slog.Make().Leveled(slog.LevelDebug),
	})

	// Register a test handler
	server.RegisterHandler("test", func(ctx Context, req *Request) (*Response, error) {
		time.Sleep(10 * time.Millisecond) // Simulate some work
		return NewResponse(req.ID, map[string]string{"message": "test response"})
	})

	// Start server
	err := server.Start()
	require.NoError(t, err)
	defer server.Stop()

	// Test multiple concurrent connections
	const numConnections = 5
	results := make(chan error, numConnections)

	for i := 0; i < numConnections; i++ {
		go func(i int) {
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				results <- err
				return
			}
			defer conn.Close()

			req := Request{
				Version: "1.0",
				Method:  "test",
				ID:      fmt.Sprintf("test-%d", i),
			}

			err = json.NewEncoder(conn).Encode(req)
			if err != nil {
				results <- err
				return
			}

			var resp Response
			err = json.NewDecoder(conn).Decode(&resp)
			if err != nil {
				results <- err
				return
			}

			if resp.Error != nil {
				results <- xerrors.Errorf("server error: %s", resp.Error.Message)
				return
			}

			results <- nil
		}(i)
	}

	// Wait for all connections to complete
	for i := 0; i < numConnections; i++ {
		select {
		case err := <-results:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent connections")
		}
	}
}
