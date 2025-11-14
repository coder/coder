package agentsdk

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"

	"github.com/hashicorp/yamux"
	"storj.io/drpc"

	"github.com/coder/coder/v2/agent/agentsocket/proto"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
)

// SocketClient provides a client for communicating with the agent socket
type SocketClient struct {
	client proto.DRPCAgentSocketClient
	conn   drpc.Conn
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

	// Create yamux session for multiplexing
	configYamux := yamux.DefaultConfig()
	configYamux.Logger = nil // Disable yamux logging
	session, err := yamux.Client(conn, configYamux)
	if err != nil {
		_ = conn.Close()
		return nil, xerrors.Errorf("create yamux client: %w", err)
	}

	// Create drpc connection using the multiplexed connection
	drpcConn := drpcsdk.MultiplexedConn(session)

	// Create drpc client
	client := proto.NewDRPCAgentSocketClient(drpcConn)

	return &SocketClient{
		client: client,
		conn:   drpcConn,
	}, nil
}

// Close closes the socket connection
func (c *SocketClient) Close() error {
	return c.conn.Close()
}

// Ping sends a ping request to the agent
func (c *SocketClient) Ping(ctx context.Context) (*PingResponse, error) {
	resp, err := c.client.Ping(ctx, &proto.PingRequest{})
	if err != nil {
		return nil, err
	}

	return &PingResponse{
		Message:   resp.Message,
		Timestamp: resp.Timestamp.AsTime(),
	}, nil
}

// SyncStart starts a unit in the dependency graph
func (c *SocketClient) SyncStart(ctx context.Context, unitName string) error {
	resp, err := c.client.SyncStart(ctx, &proto.SyncStartRequest{
		Unit: unitName,
	})
	if err != nil {
		return err
	}

	if !resp.Success {
		return xerrors.Errorf("sync start failed: %s", resp.Message)
	}

	return nil
}

// SyncWant declares a dependency between units
func (c *SocketClient) SyncWant(ctx context.Context, unitName, dependsOn string) error {
	resp, err := c.client.SyncWant(ctx, &proto.SyncWantRequest{
		Unit:      unitName,
		DependsOn: dependsOn,
	})
	if err != nil {
		return err
	}

	if !resp.Success {
		return xerrors.Errorf("sync want failed: %s", resp.Message)
	}

	return nil
}

// SyncComplete marks a unit as complete in the dependency graph
func (c *SocketClient) SyncComplete(ctx context.Context, unitName string) error {
	resp, err := c.client.SyncComplete(ctx, &proto.SyncCompleteRequest{
		Unit: unitName,
	})
	if err != nil {
		return err
	}

	if !resp.Success {
		return xerrors.Errorf("sync complete failed: %s", resp.Message)
	}

	return nil
}

// SyncReady requests whether a unit is ready to be started. That is, all dependencies are satisfied.
func (c *SocketClient) SyncReady(ctx context.Context, unitName string) error {
	resp, err := c.client.SyncReady(ctx, &proto.SyncReadyRequest{
		Unit: unitName,
	})
	if err != nil {
		return err
	}

	if !resp.Success {
		// Check if this is a dependencies not satisfied error
		if resp.Message == unit.ErrDependenciesNotSatisfied.Error() {
			return unit.ErrDependenciesNotSatisfied
		}
		return xerrors.Errorf("sync ready failed: %s", resp.Message)
	}

	return nil
}

// SyncStatus gets the status of a unit and its dependencies
func (c *SocketClient) SyncStatus(ctx context.Context, unitName string, recursive bool) (*SyncStatusResponse, error) {
	resp, err := c.client.SyncStatus(ctx, &proto.SyncStatusRequest{
		Unit:      unitName,
		Recursive: recursive,
	})
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, xerrors.Errorf("sync status failed: %s", resp.Message)
	}

	// Convert dependencies
	var dependencies []DependencyInfo
	for _, dep := range resp.Dependencies {
		dependencies = append(dependencies, DependencyInfo{
			DependsOn:      dep.DependsOn,
			RequiredStatus: dep.RequiredStatus,
			CurrentStatus:  dep.CurrentStatus,
			IsSatisfied:    dep.IsSatisfied,
		})
	}

	return &SyncStatusResponse{
		Success:      resp.Success,
		Message:      resp.Message,
		Unit:         resp.Unit,
		Status:       resp.Status,
		IsReady:      resp.IsReady,
		Dependencies: dependencies,
		DOT:          resp.Dot,
	}, nil
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

// Response types for backward compatibility
type PingResponse struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
}

type AgentInfo struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Uptime    string    `json:"uptime"`
}

type SyncStatusResponse struct {
	Success      bool             `json:"success"`
	Message      string           `json:"message"`
	Unit         string           `json:"unit"`
	Status       string           `json:"status"`
	IsReady      bool             `json:"is_ready"`
	Dependencies []DependencyInfo `json:"dependencies"`
	DOT          string           `json:"dot,omitempty"`
}

type DependencyInfo struct {
	DependsOn      string `json:"depends_on"`
	RequiredStatus string `json:"required_status"`
	CurrentStatus  string `json:"current_status"`
	IsSatisfied    bool   `json:"is_satisfied"`
}
