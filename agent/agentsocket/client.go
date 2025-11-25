package agentsocket

import (
	"context"
	"net"

	"golang.org/x/xerrors"

	"storj.io/drpc"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/v2/agent/agentsocket/proto"
)

// Client provides a client for communicating with the workspace agentsocket API.
type Client struct {
	client proto.DRPCAgentSocketClient
	conn   drpc.Conn
}

// NewClient creates a new socket client and opens a connection to the socket.
// If path is empty, it will auto-discover the default socket path.
func NewClient(ctx context.Context, path string) (*Client, error) {
	if path == "" {
		var err error
		path, err = getDefaultSocketPath()
		if err != nil {
			return nil, xerrors.Errorf("get default socket path: %w", err)
		}
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, xerrors.Errorf("connect to socket: %w", err)
	}

	drpcConn := drpcconn.New(conn)
	client := proto.NewDRPCAgentSocketClient(drpcConn)

	return &Client{
		client: client,
		conn:   drpcConn,
	}, nil
}

// Close closes the socket connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Ping sends a ping request to the agent.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.client.Ping(ctx, &proto.PingRequest{})
	return err
}

// SyncStart starts a unit in the dependency graph.
func (c *Client) SyncStart(ctx context.Context, unitName string) error {
	_, err := c.client.SyncStart(ctx, &proto.SyncStartRequest{
		Unit: unitName,
	})
	return err
}

// SyncWant declares a dependency between units.
func (c *Client) SyncWant(ctx context.Context, unitName, dependsOn string) error {
	_, err := c.client.SyncWant(ctx, &proto.SyncWantRequest{
		Unit:      unitName,
		DependsOn: dependsOn,
	})
	return err
}

// SyncComplete marks a unit as complete in the dependency graph.
func (c *Client) SyncComplete(ctx context.Context, unitName string) error {
	_, err := c.client.SyncComplete(ctx, &proto.SyncCompleteRequest{
		Unit: unitName,
	})
	return err
}

// SyncReady requests whether a unit is ready to be started. That is, all dependencies are satisfied.
func (c *Client) SyncReady(ctx context.Context, unitName string) (bool, error) {
	resp, err := c.client.SyncReady(ctx, &proto.SyncReadyRequest{
		Unit: unitName,
	})
	return resp.Ready, err
}

// SyncStatus gets the status of a unit and its dependencies.
func (c *Client) SyncStatus(ctx context.Context, unitName string) (*SyncStatusResponse, error) {
	resp, err := c.client.SyncStatus(ctx, &proto.SyncStatusRequest{
		Unit: unitName,
	})
	if err != nil {
		return nil, err
	}

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

// SyncStatusResponse contains the status information for a unit.
type SyncStatusResponse struct {
	Status       string           `json:"status"`
	IsReady      bool             `json:"is_ready"`
	Dependencies []DependencyInfo `json:"dependencies"`
}

// DependencyInfo contains information about a unit dependency.
type DependencyInfo struct {
	DependsOn      string `json:"depends_on"`
	RequiredStatus string `json:"required_status"`
	CurrentStatus  string `json:"current_status"`
	IsSatisfied    bool   `json:"is_satisfied"`
}
