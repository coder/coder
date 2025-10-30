package cli_test

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/clitest"
)

// mockAgentSocketServer simulates the agent socket server for testing
type mockAgentSocketServer struct {
	listener net.Listener
	handlers map[string]func(string) (string, error)
}

func newMockAgentSocketServer(t *testing.T, socketPath string) *mockAgentSocketServer {
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	server := &mockAgentSocketServer{
		listener: listener,
		handlers: make(map[string]func(string) (string, error)),
	}

	// Set up default handlers
	server.handlers["sync.wait"] = func(unitName string) (string, error) {
		// Always return dependencies not satisfied to trigger polling
		return "", unit.ErrDependenciesNotSatisfied
	}

	server.handlers["sync.start"] = func(unitName string) (string, error) {
		return "Unit " + unitName + " started successfully", nil
	}

	go server.serve(t)
	return server
}

func (s *mockAgentSocketServer) serve(t *testing.T) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				t.Logf("Accept error: %v", err)
			}
			return
		}

		go s.handleConnection(t, conn)
	}
}

func (s *mockAgentSocketServer) handleConnection(t *testing.T, conn net.Conn) {
	defer conn.Close()

	// Simple JSON-RPC-like protocol simulation
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Logf("Read error: %v", err)
		return
	}

	request := string(buf[:n])

	// Parse method from request (simplified)
	var method string
	if strings.Contains(request, "sync.wait") {
		method = "sync.wait"
	} else if strings.Contains(request, "sync.start") {
		method = "sync.start"
	}

	handler, exists := s.handlers[method]
	if !exists {
		response := `{"error": {"code": -32601, "message": "Method not found"}}`
		_, _ = conn.Write([]byte(response))
		return
	}

	// Extract unit name from request (simplified)
	unitName := "test-unit"
	if strings.Contains(request, "test-unit") {
		unitName = "test-unit"
	}

	message, err := handler(unitName)
	if err != nil {
		response := fmt.Sprintf(`{"error": {"code": -32603, "message": %q}}`, err.Error())
		_, _ = conn.Write([]byte(response))
		return
	}

	response := fmt.Sprintf(`{"result": {"success": true, "message": %q}}`, message)
	_, _ = conn.Write([]byte(response))
}

func (s *mockAgentSocketServer) setHandler(method string, handler func(string) (string, error)) {
	s.handlers[method] = handler
}

func (s *mockAgentSocketServer) close() {
	_ = s.listener.Close()
}

func TestSyncStartTimeout(t *testing.T) {
	t.Parallel()

	// Create a unique temporary socket file
	socketPath := fmt.Sprintf("/tmp/coder-test-%d.sock", time.Now().UnixNano())
	// Remove existing socket if it exists
	_ = os.Remove(socketPath)
	defer func() { _ = os.Remove(socketPath) }()

	// Start mock server
	server := newMockAgentSocketServer(t, socketPath)
	defer server.close()

	// Test with a short timeout
	inv, _ := clitest.New(t, "exp", "sync", "start", "test-unit", "--timeout", "100ms")

	// Override the socket path for this test
	inv.Args = append(inv.Args, "--agent-socket", socketPath)

	start := time.Now()
	err := inv.Run()
	duration := time.Since(start)

	// Should timeout after approximately 100ms
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout waiting for dependencies of unit 'test-unit'")

	// Should timeout within a reasonable range (100ms + some buffer for test execution)
	assert.True(t, duration >= 100*time.Millisecond, "Duration should be at least 100ms, got %v", duration)
	assert.True(t, duration < 2*time.Second, "Duration should be less than 2s, got %v", duration)
}

func TestSyncWaitTimeout(t *testing.T) {
	t.Parallel()

	// Create a unique temporary socket file
	socketPath := fmt.Sprintf("/tmp/coder-test-%d.sock", time.Now().UnixNano())
	// Remove existing socket if it exists
	_ = os.Remove(socketPath)
	defer func() { _ = os.Remove(socketPath) }()

	// Start mock server
	server := newMockAgentSocketServer(t, socketPath)
	defer server.close()

	// Test with a short timeout
	inv, _ := clitest.New(t, "exp", "sync", "wait", "test-unit", "--timeout", "100ms")

	// Override the socket path for this test
	inv.Args = append(inv.Args, "--agent-socket", socketPath)

	start := time.Now()
	err := inv.Run()
	duration := time.Since(start)

	// Should timeout after approximately 100ms
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout waiting for dependencies of unit 'test-unit'")

	// Should timeout within a reasonable range (100ms + some buffer for test execution)
	assert.True(t, duration >= 100*time.Millisecond, "Duration should be at least 100ms, got %v", duration)
	assert.True(t, duration < 2*time.Second, "Duration should be less than 2s, got %v", duration)
}

func TestSyncStartNoTimeout(t *testing.T) {
	t.Parallel()

	// Create a unique temporary socket file
	socketPath := fmt.Sprintf("/tmp/coder-test-%d.sock", time.Now().UnixNano())
	// Remove existing socket if it exists
	_ = os.Remove(socketPath)
	defer func() { _ = os.Remove(socketPath) }()

	// Start mock server
	server := newMockAgentSocketServer(t, socketPath)
	defer server.close()

	// Set up handler that will eventually succeed
	callCount := 0
	server.setHandler("sync.wait", func(unitName string) (string, error) {
		callCount++
		if callCount >= 3 {
			// After 3 calls, dependencies are satisfied
			return "Dependencies satisfied", nil
		}
		return "", unit.ErrDependenciesNotSatisfied
	})

	// Test without timeout - should eventually succeed
	inv, _ := clitest.New(t, "exp", "sync", "start", "test-unit")

	// Override the socket path for this test
	inv.Args = append(inv.Args, "--agent-socket", socketPath)

	start := time.Now()
	err := inv.Run()
	duration := time.Since(start)

	// Should succeed after a few polling cycles
	assert.NoError(t, err)

	// Should take at least 2 seconds (2 polling cycles at 1s interval)
	assert.True(t, duration >= 2*time.Second, "Duration should be at least 2s, got %v", duration)
	assert.True(t, callCount >= 3, "Should have made at least 3 calls, got %d", callCount)
}

func TestSyncWaitNoTimeout(t *testing.T) {
	t.Parallel()

	// Create a unique temporary socket file
	socketPath := fmt.Sprintf("/tmp/coder-test-%d.sock", time.Now().UnixNano())
	// Remove existing socket if it exists
	_ = os.Remove(socketPath)
	defer func() { _ = os.Remove(socketPath) }()

	// Start mock server
	server := newMockAgentSocketServer(t, socketPath)
	defer server.close()

	// Set up handler that will eventually succeed
	callCount := 0
	server.setHandler("sync.wait", func(unitName string) (string, error) {
		callCount++
		if callCount >= 3 {
			// After 3 calls, dependencies are satisfied
			return "Dependencies satisfied", nil
		}
		return "", unit.ErrDependenciesNotSatisfied
	})

	// Test without timeout - should eventually succeed
	inv, _ := clitest.New(t, "exp", "sync", "wait", "test-unit")

	// Override the socket path for this test
	inv.Args = append(inv.Args, "--agent-socket", socketPath)

	start := time.Now()
	err := inv.Run()
	duration := time.Since(start)

	// Should succeed after a few polling cycles
	assert.NoError(t, err)

	// Should take at least 2 seconds (2 polling cycles at 1s interval)
	assert.True(t, duration >= 2*time.Second, "Duration should be at least 2s, got %v", duration)
	assert.True(t, callCount >= 3, "Should have made at least 3 calls, got %d", callCount)
}

func TestSyncStartTimeoutWithDifferentValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		timeout  string
		expected time.Duration
	}{
		{"50ms", "50ms", 50 * time.Millisecond},
		{"200ms", "200ms", 200 * time.Millisecond},
		{"500ms", "500ms", 500 * time.Millisecond},
		{"1s", "1s", 1 * time.Second},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a unique temporary socket file
			socketPath := fmt.Sprintf("/tmp/coder-test-%d.sock", time.Now().UnixNano())
			// Remove existing socket if it exists
			_ = os.Remove(socketPath)
			defer func() { _ = os.Remove(socketPath) }()

			// Start mock server
			server := newMockAgentSocketServer(t, socketPath)
			defer server.close()

			// Test with specified timeout
			inv, _ := clitest.New(t, "exp", "sync", "start", "test-unit", "--timeout", tc.timeout)

			// Override the socket path for this test
			inv.Args = append(inv.Args, "--agent-socket", socketPath)

			start := time.Now()
			err := inv.Run()
			duration := time.Since(start)

			// Should timeout after approximately the specified duration
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "timeout waiting for dependencies of unit 'test-unit'")

			// Should timeout within a reasonable range
			assert.True(t, duration >= tc.expected, "Duration should be at least %v, got %v", tc.expected, duration)
			assert.True(t, duration < tc.expected+2*time.Second, "Duration should be less than %v, got %v", tc.expected+2*time.Second, duration)
		})
	}
}

func TestSyncWaitTimeoutWithDifferentValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		timeout  string
		expected time.Duration
	}{
		{"50ms", "50ms", 50 * time.Millisecond},
		{"200ms", "200ms", 200 * time.Millisecond},
		{"500ms", "500ms", 500 * time.Millisecond},
		{"1s", "1s", 1 * time.Second},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a unique temporary socket file
			socketPath := fmt.Sprintf("/tmp/coder-test-%d.sock", time.Now().UnixNano())
			// Remove existing socket if it exists
			_ = os.Remove(socketPath)
			defer func() { _ = os.Remove(socketPath) }()

			// Start mock server
			server := newMockAgentSocketServer(t, socketPath)
			defer server.close()

			// Test with specified timeout
			inv, _ := clitest.New(t, "exp", "sync", "wait", "test-unit", "--timeout", tc.timeout)

			// Override the socket path for this test
			inv.Args = append(inv.Args, "--agent-socket", socketPath)

			start := time.Now()
			err := inv.Run()
			duration := time.Since(start)

			// Should timeout after approximately the specified duration
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "timeout waiting for dependencies of unit 'test-unit'")

			// Should timeout within a reasonable range
			assert.True(t, duration >= tc.expected, "Duration should be at least %v, got %v", tc.expected, duration)
			assert.True(t, duration < tc.expected+2*time.Second, "Duration should be less than %v, got %v", tc.expected+2*time.Second, duration)
		})
	}
}
