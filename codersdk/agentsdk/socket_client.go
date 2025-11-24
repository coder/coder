package agentsdk

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"

	"storj.io/drpc"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/v2/agent/agentsocket/proto"
)

// SocketClient provides a client for communicating with the workspace agentsocket API
type SocketClient struct {
	client proto.DRPCAgentSocketClient
	conn   drpc.Conn
}

// SocketConfig holds configuration for the socket client
type SocketConfig struct {
	Path string // Socket path (optional, will auto-discover if not set)
}

// NewSocketClient creates a new socket client
func NewSocketClient(ctx context.Context, config SocketConfig) (*SocketClient, error) {
	path := config.Path
	if path == "" {
		var err error
		path, err = discoverSocketPath()
		if err != nil {
			return nil, xerrors.Errorf("discover socket path: %w", err)
		}
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, xerrors.Errorf("connect to socket: %w", err)
	}

	// Create drpc connection using the multiplexed connection
	drpcConn := drpcconn.New(conn)

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
func (c *SocketClient) Ping(ctx context.Context) error {
	_, err := c.client.Ping(ctx, &proto.PingRequest{})
	if err != nil {
		return err
	}

	return nil
}

// SyncStart starts a unit in the dependency graph
func (c *SocketClient) SyncStart(ctx context.Context, unitName string) error {
	_, err := c.client.SyncStart(ctx, &proto.SyncStartRequest{
		Unit: unitName,
	})
	if err != nil {
		return err
	}

	return nil
}

// SyncWant declares a dependency between units
func (c *SocketClient) SyncWant(ctx context.Context, unitName, dependsOn string) error {
	_, err := c.client.SyncWant(ctx, &proto.SyncWantRequest{
		Unit:      unitName,
		DependsOn: dependsOn,
	})
	if err != nil {
		return err
	}

	return nil
}

// SyncComplete marks a unit as complete in the dependency graph
func (c *SocketClient) SyncComplete(ctx context.Context, unitName string) error {
	_, err := c.client.SyncComplete(ctx, &proto.SyncCompleteRequest{
		Unit: unitName,
	})
	if err != nil {
		return err
	}

	return nil
}

// SyncReady requests whether a unit is ready to be started. That is, all dependencies are satisfied.
func (c *SocketClient) SyncReady(ctx context.Context, unitName string) (bool, error) {
	resp, err := c.client.SyncReady(ctx, &proto.SyncReadyRequest{
		Unit: unitName,
	})
	if err != nil {
		return false, err
	}

	return resp.Ready, nil
}

// SyncStatus gets the status of a unit and its dependencies
func (c *SocketClient) SyncStatus(ctx context.Context, unitName string) (*SyncStatusResponse, error) {
	resp, err := c.client.SyncStatus(ctx, &proto.SyncStatusRequest{
		Unit: unitName,
	})
	if err != nil {
		return nil, err
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
		Status:       resp.Status,
		IsReady:      resp.IsReady,
		Dependencies: dependencies,
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
