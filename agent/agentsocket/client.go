package agentsocket

import (
	"context"

	"golang.org/x/xerrors"
	"storj.io/drpc"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/v2/agent/agentsocket/proto"
	"github.com/coder/coder/v2/agent/unit"
)

// Option represents a configuration option for NewClient.
type Option func(*options)

type options struct {
	path        string
	unitManager *unit.Manager
}

// WithPath sets the socket path. If not provided or empty, the client will
// auto-discover the default socket path.
func WithPath(path string) Option {
	return func(opts *options) {
		if path == "" {
			return
		}
		opts.path = path
	}
}

// WithUnitManager sets the unit manager to use. If not provided, a new one
// will be created.
func WithUnitManager(unitManager *unit.Manager) Option {
	return func(opts *options) {
		opts.unitManager = unitManager
	}
}

// Client provides a client for communicating with the workspace agentsocket API.
type Client struct {
	client proto.DRPCAgentSocketClient
	conn   drpc.Conn
}

// NewClient creates a new socket client and opens a connection to the socket.
// If path is not provided via WithPath or is empty, it will auto-discover the
// default socket path.
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	options := &options{}
	for _, opt := range opts {
		opt(options)
	}

	conn, err := dialSocket(ctx, options.path)
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
func (c *Client) SyncStart(ctx context.Context, unitName unit.ID) error {
	_, err := c.client.SyncStart(ctx, &proto.SyncStartRequest{
		Unit: string(unitName),
	})
	return err
}

// SyncWant declares a dependency between units.
func (c *Client) SyncWant(ctx context.Context, unitName, dependsOn unit.ID) error {
	_, err := c.client.SyncWant(ctx, &proto.SyncWantRequest{
		Unit:      string(unitName),
		DependsOn: string(dependsOn),
	})
	return err
}

// SyncComplete marks a unit as complete in the dependency graph.
func (c *Client) SyncComplete(ctx context.Context, unitName unit.ID) error {
	_, err := c.client.SyncComplete(ctx, &proto.SyncCompleteRequest{
		Unit: string(unitName),
	})
	return err
}

// SyncReady requests whether a unit is ready to be started. That is, all dependencies are satisfied.
func (c *Client) SyncReady(ctx context.Context, unitName unit.ID) (bool, error) {
	resp, err := c.client.SyncReady(ctx, &proto.SyncReadyRequest{
		Unit: string(unitName),
	})
	return resp.Ready, err
}

// SyncStatus gets the status of a unit and its dependencies.
func (c *Client) SyncStatus(ctx context.Context, unitName unit.ID) (SyncStatusResponse, error) {
	resp, err := c.client.SyncStatus(ctx, &proto.SyncStatusRequest{
		Unit: string(unitName),
	})
	if err != nil {
		return SyncStatusResponse{}, err
	}

	var dependencies []DependencyInfo
	for _, dep := range resp.Dependencies {
		dependencies = append(dependencies, DependencyInfo{
			DependsOn:      unit.ID(dep.DependsOn),
			RequiredStatus: unit.Status(dep.RequiredStatus),
			CurrentStatus:  unit.Status(dep.CurrentStatus),
			IsSatisfied:    dep.IsSatisfied,
		})
	}

	return SyncStatusResponse{
		UnitName:     unitName,
		Status:       unit.Status(resp.Status),
		IsReady:      resp.IsReady,
		Dependencies: dependencies,
	}, nil
}

// SyncList returns a list of all units in the dependency graph.
func (c *Client) SyncList(ctx context.Context) ([]ScriptInfo, error) {
	resp, err := c.client.SyncList(ctx, &proto.SyncListRequest{})
	if err != nil {
		return nil, err
	}

	var scriptInfos []ScriptInfo
	for _, script := range resp.Scripts {
		scriptInfos = append(scriptInfos, ScriptInfo{
			ID:     script.Id,
			Status: script.Status,
		})
	}

	return scriptInfos, nil
}

// ScriptInfo contains information about a unit in the dependency graph.
type ScriptInfo struct {
	ID     string `table:"id,default_sort" json:"id"`
	Status string `table:"status" json:"status"`
}

// SyncStatusResponse contains the status information for a unit.
type SyncStatusResponse struct {
	UnitName     unit.ID          `table:"unit,default_sort" json:"unit_name"`
	Status       unit.Status      `table:"status" json:"status"`
	IsReady      bool             `table:"ready" json:"is_ready"`
	Dependencies []DependencyInfo `table:"dependencies" json:"dependencies"`
}

// DependencyInfo contains information about a unit dependency.
type DependencyInfo struct {
	DependsOn      unit.ID     `table:"depends on,default_sort" json:"depends_on"`
	RequiredStatus unit.Status `table:"required status" json:"required_status"`
	CurrentStatus  unit.Status `table:"current status" json:"current_status"`
	IsSatisfied    bool        `table:"satisfied" json:"is_satisfied"`
}
