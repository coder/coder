package agentsocket

import (
	"context"

	"golang.org/x/xerrors"
	"storj.io/drpc"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/v2/agent/agentsocket/proto"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/unit"
)

// Option represents a configuration option for NewClient.
type Option func(*options)

type options struct {
	path           string
	contextManager ContextManager
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

// WithContextManager supplies the workspace-context Manager the server uses to
// serve context source CRUD. Server-only; ignored by the client.
func WithContextManager(cm ContextManager) Option {
	return func(opts *options) {
		opts.contextManager = cm
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
	if err != nil {
		return false, xerrors.Errorf("sync ready: %w", err)
	}
	return resp.Ready, nil
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

// SyncList returns all registered units and their current statuses.
func (c *Client) SyncList(ctx context.Context) ([]SyncListItem, error) {
	resp, err := c.client.SyncList(ctx, &proto.SyncListRequest{})
	if err != nil {
		return nil, err
	}

	var items []SyncListItem
	for _, u := range resp.Units {
		items = append(items, SyncListItem{
			UnitName: unit.ID(u.Unit),
			Status:   unit.Status(u.Status),
			IsReady:  u.IsReady,
		})
	}

	return items, nil
}

// UpdateAppStatus forwards an app status update to coderd via the agent.
func (c *Client) UpdateAppStatus(ctx context.Context, req *agentproto.UpdateAppStatusRequest) (*agentproto.UpdateAppStatusResponse, error) {
	return c.client.UpdateAppStatus(ctx, req)
}

// ContextSources lists the workspace-context sources registered on the agent.
func (c *Client) ContextSources(ctx context.Context) ([]ContextSource, error) {
	resp, err := c.client.ContextSources(ctx, &proto.ContextSourcesRequest{})
	if err != nil {
		return nil, err
	}
	sources := make([]ContextSource, 0, len(resp.Sources))
	for _, s := range resp.Sources {
		sources = append(sources, ContextSource{Path: s.GetPath()})
	}
	return sources, nil
}

// GetContextSource returns a single registered source. The path is
// canonicalized by the agent before matching.
func (c *Client) GetContextSource(ctx context.Context, path string) (ContextSource, error) {
	resp, err := c.client.GetContextSource(ctx, &proto.GetContextSourceRequest{Path: path})
	if err != nil {
		return ContextSource{}, err
	}
	return ContextSource{Path: resp.GetSource().GetPath()}, nil
}

// AddContextSource registers a new scan root on the agent.
func (c *Client) AddContextSource(ctx context.Context, path string) (ContextSource, error) {
	resp, err := c.client.AddContextSource(ctx, &proto.AddContextSourceRequest{Path: path})
	if err != nil {
		return ContextSource{}, err
	}
	return ContextSource{Path: resp.GetSource().GetPath()}, nil
}

// RemoveContextSource removes a previously-registered scan root.
func (c *Client) RemoveContextSource(ctx context.Context, path string) error {
	_, err := c.client.RemoveContextSource(ctx, &proto.RemoveContextSourceRequest{Path: path})
	return err
}

// GetContextSnapshot returns the agent's current resolved snapshot without
// forcing a re-walk.
func (c *Client) GetContextSnapshot(ctx context.Context) (ContextSnapshot, error) {
	resp, err := c.client.GetContextSnapshot(ctx, &proto.ContextSnapshotRequest{})
	if err != nil {
		return ContextSnapshot{}, err
	}
	return contextSnapshotFromProto(resp.GetSnapshot()), nil
}

// ResyncContext forces a re-walk and synchronous push, returning the resulting
// snapshot. Use it as a barrier before fanning out a refresh.
func (c *Client) ResyncContext(ctx context.Context) (ContextSnapshot, error) {
	resp, err := c.client.ResyncContext(ctx, &proto.ResyncContextRequest{})
	if err != nil {
		return ContextSnapshot{}, err
	}
	return contextSnapshotFromProto(resp.GetSnapshot()), nil
}

func contextSnapshotFromProto(s *proto.ContextSnapshot) ContextSnapshot {
	if s == nil {
		return ContextSnapshot{}
	}
	out := ContextSnapshot{
		Version:       s.GetVersion(),
		AggregateHash: s.GetAggregateHash(),
		Resources:     make([]ContextResource, 0, len(s.GetResources())),
		PayloadBytes:  s.GetPayloadBytes(),
		SnapshotError: s.GetSnapshotError(),
	}
	for _, r := range s.GetResources() {
		out.Resources = append(out.Resources, ContextResource{
			ID:          r.GetId(),
			Kind:        r.GetKind(),
			Source:      r.GetSource(),
			SourcePath:  r.GetSourcePath(),
			ContentHash: r.GetContentHash(),
			SizeBytes:   r.GetSizeBytes(),
			Status:      r.GetStatus(),
			Error:       r.GetError(),
			Name:        r.GetName(),
			Description: r.GetDescription(),
		})
	}
	return out
}

// SyncStatusResponse contains the status information for a unit.
type SyncStatusResponse struct {
	UnitName     unit.ID          `table:"unit,default_sort" json:"unit_name"`
	Status       unit.Status      `table:"status" json:"status"`
	IsReady      bool             `table:"ready" json:"is_ready"`
	Dependencies []DependencyInfo `table:"dependencies" json:"dependencies"`
}

// SyncListItem contains summary information for a single unit.
type SyncListItem struct {
	UnitName unit.ID     `table:"unit,default_sort" json:"unit_name"`
	Status   unit.Status `table:"status" json:"status"`
	IsReady  bool        `table:"ready" json:"is_ready"`
}

// DependencyInfo contains information about a unit dependency.
type DependencyInfo struct {
	DependsOn      unit.ID     `table:"depends on,default_sort" json:"depends_on"`
	RequiredStatus unit.Status `table:"required status" json:"required_status"`
	CurrentStatus  unit.Status `table:"current status" json:"current_status"`
	IsSatisfied    bool        `table:"satisfied" json:"is_satisfied"`
}

// ContextSource is a registered workspace-context scan root.
type ContextSource struct {
	Path string `table:"path,default_sort" json:"path"`
}

// ContextResource is a resolved workspace-context resource. Payload bytes are
// never carried over the socket.
type ContextResource struct {
	Kind        string `table:"kind,default_sort" json:"kind"`
	Name        string `table:"name" json:"name"`
	Source      string `table:"source" json:"source"`
	SourcePath  string `table:"source path" json:"source_path"`
	Status      string `table:"status" json:"status"`
	SizeBytes   uint64 `table:"size bytes" json:"size_bytes"`
	Error       string `table:"error" json:"error"`
	Description string `table:"-" json:"description"`
	ID          string `table:"-" json:"id"`
	ContentHash string `table:"-" json:"content_hash"`
}

// ContextSnapshot is the agent's resolved workspace-context state.
type ContextSnapshot struct {
	Version       uint64            `json:"version"`
	AggregateHash string            `json:"aggregate_hash"`
	Resources     []ContextResource `json:"resources"`
	PayloadBytes  uint64            `json:"payload_bytes"`
	SnapshotError string            `json:"snapshot_error"`
}
