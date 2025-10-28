package agentsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"
)

// SocketClient provides a client for communicating with the agent socket
type SocketClient struct {
	conn net.Conn
}

// SocketConfig holds configuration for the socket client
type SocketConfig struct {
	Path string // Socket path (optional, will auto-discover if not set)
}

// NewSocketClient creates a new socket client
func NewSocketClient(config SocketConfig) (*SocketClient, error) {
	path := config.Path
	if path == "" {
		var err error
		path, err = discoverSocketPath()
		if err != nil {
			return nil, xerrors.Errorf("discover socket path: %w", err)
		}
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, xerrors.Errorf("connect to socket: %w", err)
	}

	return &SocketClient{
		conn: conn,
	}, nil
}

// Close closes the socket connection
func (c *SocketClient) Close() error {
	return c.conn.Close()
}

// Ping sends a ping request to the agent
func (c *SocketClient) Ping(ctx context.Context) (*PingResponse, error) {
	req := &Request{
		Version: "1.0",
		Method:  "ping",
		ID:      generateRequestID(),
	}

	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, xerrors.Errorf("ping error: %s", resp.Error.Message)
	}

	var pingResp PingResponse
	if err := json.Unmarshal(resp.Result, &pingResp); err != nil {
		return nil, xerrors.Errorf("unmarshal ping response: %w", err)
	}

	return &pingResp, nil
}

// Health sends a health check request to the agent
func (c *SocketClient) Health(ctx context.Context) (*HealthResponse, error) {
	req := &Request{
		Version: "1.0",
		Method:  "health",
		ID:      generateRequestID(),
	}

	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, xerrors.Errorf("health error: %s", resp.Error.Message)
	}

	var healthResp HealthResponse
	if err := json.Unmarshal(resp.Result, &healthResp); err != nil {
		return nil, xerrors.Errorf("unmarshal health response: %w", err)
	}

	return &healthResp, nil
}

// AgentInfo sends an agent info request
func (c *SocketClient) AgentInfo(ctx context.Context) (*AgentInfo, error) {
	req := &Request{
		Version: "1.0",
		Method:  "agent.info",
		ID:      generateRequestID(),
	}

	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, xerrors.Errorf("agent info error: %s", resp.Error.Message)
	}

	var agentInfo AgentInfo
	if err := json.Unmarshal(resp.Result, &agentInfo); err != nil {
		return nil, xerrors.Errorf("unmarshal agent info response: %w", err)
	}

	return &agentInfo, nil
}

// ListMethods lists available methods
func (c *SocketClient) ListMethods(ctx context.Context) ([]string, error) {
	req := &Request{
		Version: "1.0",
		Method:  "methods.list",
		ID:      generateRequestID(),
	}

	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, xerrors.Errorf("list methods error: %s", resp.Error.Message)
	}

	var methods []string
	if err := json.Unmarshal(resp.Result, &methods); err != nil {
		return nil, xerrors.Errorf("unmarshal methods response: %w", err)
	}

	return methods, nil
}

// sendRequest sends a request and returns the response
func (c *SocketClient) sendRequest(_ context.Context, req *Request) (*Response, error) {
	// Set write deadline
	if err := c.conn.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, xerrors.Errorf("set write deadline: %w", err)
	}

	// Send request
	if err := json.NewEncoder(c.conn).Encode(req); err != nil {
		return nil, xerrors.Errorf("send request: %w", err)
	}

	// Set read deadline
	if err := c.conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, xerrors.Errorf("set read deadline: %w", err)
	}

	// Read response
	var resp Response
	if err := json.NewDecoder(c.conn).Decode(&resp); err != nil {
		return nil, xerrors.Errorf("read response: %w", err)
	}

	return &resp, nil
}

// discoverSocketPath discovers the agent socket path
func discoverSocketPath() (string, error) {
	// Check environment variable first
	if path := os.Getenv("CODER_AGENT_SOCKET_PATH"); path != "" {
		return path, nil
	}

	// Try common socket paths
	paths := []string{
		// XDG runtime directory
		filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "coder-agent.sock"),
		// User-specific temp directory
		filepath.Join(os.TempDir(), fmt.Sprintf("coder-agent-%d.sock", os.Getuid())),
		// Fallback temp directory
		filepath.Join(os.TempDir(), "coder-agent.sock"),
	}

	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", xerrors.New("agent socket not found")
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Request represents a socket request
type Request struct {
	Version string          `json:"version"`
	Method  string          `json:"method"`
	ID      string          `json:"id,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a socket response
type Response struct {
	Version string          `json:"version"`
	ID      string          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error represents a socket error
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// PingResponse represents a ping response
type PingResponse struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
}

// AgentInfo represents agent information
type AgentInfo struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Uptime    string    `json:"uptime"`
}
