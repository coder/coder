package agenttest

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func NewClient(t testing.TB,
	logger slog.Logger,
	agentID uuid.UUID,
	manifest agentsdk.Manifest,
	statsChan chan *agentsdk.Stats,
	coordinator tailnet.Coordinator,
) *Client {
	if manifest.AgentID == uuid.Nil {
		manifest.AgentID = agentID
	}
	return &Client{
		t:              t,
		logger:         logger.Named("client"),
		agentID:        agentID,
		manifest:       manifest,
		statsChan:      statsChan,
		coordinator:    coordinator,
		derpMapUpdates: make(chan agentsdk.DERPMapUpdate),
	}
}

type Client struct {
	t                    testing.TB
	logger               slog.Logger
	agentID              uuid.UUID
	manifest             agentsdk.Manifest
	metadata             map[string]agentsdk.Metadata
	statsChan            chan *agentsdk.Stats
	coordinator          tailnet.Coordinator
	LastWorkspaceAgent   func()
	PatchWorkspaceLogs   func() error
	GetServiceBannerFunc func() (codersdk.ServiceBannerConfig, error)

	mu              sync.Mutex // Protects following.
	lifecycleStates []codersdk.WorkspaceAgentLifecycle
	startup         agentsdk.PostStartupRequest
	logs            []agentsdk.Log
	derpMapUpdates  chan agentsdk.DERPMapUpdate
}

func (c *Client) Manifest(_ context.Context) (agentsdk.Manifest, error) {
	return c.manifest, nil
}

func (c *Client) Listen(_ context.Context) (net.Conn, error) {
	clientConn, serverConn := net.Pipe()
	closed := make(chan struct{})
	c.LastWorkspaceAgent = func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
		<-closed
	}
	c.t.Cleanup(c.LastWorkspaceAgent)
	go func() {
		_ = c.coordinator.ServeAgent(serverConn, c.agentID, "")
		close(closed)
	}()
	return clientConn, nil
}

func (c *Client) ReportStats(ctx context.Context, _ slog.Logger, statsChan <-chan *agentsdk.Stats, setInterval func(time.Duration)) (io.Closer, error) {
	doneCh := make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(doneCh)

		setInterval(500 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				return
			case stat := <-statsChan:
				select {
				case c.statsChan <- stat:
				case <-ctx.Done():
					return
				default:
					// We don't want to send old stats.
					continue
				}
			}
		}
	}()
	return closeFunc(func() error {
		cancel()
		<-doneCh
		close(c.statsChan)
		return nil
	}), nil
}

func (c *Client) GetLifecycleStates() []codersdk.WorkspaceAgentLifecycle {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lifecycleStates
}

func (c *Client) PostLifecycle(ctx context.Context, req agentsdk.PostLifecycleRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lifecycleStates = append(c.lifecycleStates, req.State)
	c.logger.Debug(ctx, "post lifecycle", slog.F("req", req))
	return nil
}

func (c *Client) PostAppHealth(ctx context.Context, req agentsdk.PostAppHealthsRequest) error {
	c.logger.Debug(ctx, "post app health", slog.F("req", req))
	return nil
}

func (c *Client) GetStartup() agentsdk.PostStartupRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startup
}

func (c *Client) GetMetadata() map[string]agentsdk.Metadata {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.metadata)
}

func (c *Client) PostMetadata(ctx context.Context, req agentsdk.PostMetadataRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.metadata == nil {
		c.metadata = make(map[string]agentsdk.Metadata)
	}
	for _, md := range req.Metadata {
		c.metadata[md.Key] = md
		c.logger.Debug(ctx, "post metadata", slog.F("key", md.Key), slog.F("md", md))
	}
	return nil
}

func (c *Client) PostStartup(ctx context.Context, startup agentsdk.PostStartupRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startup = startup
	c.logger.Debug(ctx, "post startup", slog.F("req", startup))
	return nil
}

func (c *Client) GetStartupLogs() []agentsdk.Log {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logs
}

func (c *Client) PatchLogs(ctx context.Context, logs agentsdk.PatchLogs) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.PatchWorkspaceLogs != nil {
		return c.PatchWorkspaceLogs()
	}
	c.logs = append(c.logs, logs.Logs...)
	c.logger.Debug(ctx, "patch startup logs", slog.F("req", logs))
	return nil
}

func (c *Client) SetServiceBannerFunc(f func() (codersdk.ServiceBannerConfig, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.GetServiceBannerFunc = f
}

func (c *Client) GetServiceBanner(ctx context.Context) (codersdk.ServiceBannerConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger.Debug(ctx, "get service banner")
	if c.GetServiceBannerFunc != nil {
		return c.GetServiceBannerFunc()
	}
	return codersdk.ServiceBannerConfig{}, nil
}

func (c *Client) PushDERPMapUpdate(update agentsdk.DERPMapUpdate) error {
	timer := time.NewTimer(testutil.WaitShort)
	defer timer.Stop()
	select {
	case c.derpMapUpdates <- update:
	case <-timer.C:
		return xerrors.New("timeout waiting to push derp map update")
	}

	return nil
}

func (c *Client) DERPMapUpdates(_ context.Context) (<-chan agentsdk.DERPMapUpdate, io.Closer, error) {
	closed := make(chan struct{})
	return c.derpMapUpdates, closeFunc(func() error {
		close(closed)
		return nil
	}), nil
}

type closeFunc func() error

func (c closeFunc) Close() error {
	return c()
}
