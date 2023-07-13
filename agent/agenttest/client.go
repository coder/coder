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

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/tailnet"
)

func NewClient(t testing.TB,
	agentID uuid.UUID,
	manifest agentsdk.Manifest,
	statsChan chan *agentsdk.Stats,
	coordinator tailnet.Coordinator,
) *Client {
	if manifest.AgentID == uuid.Nil {
		manifest.AgentID = agentID
	}
	return &Client{
		t:           t,
		agentID:     agentID,
		manifest:    manifest,
		statsChan:   statsChan,
		coordinator: coordinator,
	}
}

type Client struct {
	t                    testing.TB
	agentID              uuid.UUID
	manifest             agentsdk.Manifest
	metadata             map[string]agentsdk.PostMetadataRequest
	statsChan            chan *agentsdk.Stats
	coordinator          tailnet.Coordinator
	LastWorkspaceAgent   func()
	PatchWorkspaceLogs   func() error
	GetServiceBannerFunc func() (codersdk.ServiceBannerConfig, error)

	mu              sync.Mutex // Protects following.
	lifecycleStates []codersdk.WorkspaceAgentLifecycle
	startup         agentsdk.PostStartupRequest
	logs            []agentsdk.StartupLog
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

func (c *Client) PostLifecycle(_ context.Context, req agentsdk.PostLifecycleRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lifecycleStates = append(c.lifecycleStates, req.State)
	return nil
}

func (*Client) PostAppHealth(_ context.Context, _ agentsdk.PostAppHealthsRequest) error {
	return nil
}

func (c *Client) GetStartup() agentsdk.PostStartupRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startup
}

func (c *Client) GetMetadata() map[string]agentsdk.PostMetadataRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.metadata)
}

func (c *Client) PostMetadata(_ context.Context, key string, req agentsdk.PostMetadataRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.metadata == nil {
		c.metadata = make(map[string]agentsdk.PostMetadataRequest)
	}
	c.metadata[key] = req
	return nil
}

func (c *Client) PostStartup(_ context.Context, startup agentsdk.PostStartupRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startup = startup
	return nil
}

func (c *Client) GetStartupLogs() []agentsdk.StartupLog {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logs
}

func (c *Client) PatchStartupLogs(_ context.Context, logs agentsdk.PatchStartupLogs) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.PatchWorkspaceLogs != nil {
		return c.PatchWorkspaceLogs()
	}
	c.logs = append(c.logs, logs.Logs...)
	return nil
}

func (c *Client) SetServiceBannerFunc(f func() (codersdk.ServiceBannerConfig, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.GetServiceBannerFunc = f
}

func (c *Client) GetServiceBanner(_ context.Context) (codersdk.ServiceBannerConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.GetServiceBannerFunc != nil {
		return c.GetServiceBannerFunc()
	}
	return codersdk.ServiceBannerConfig{}, nil
}

type closeFunc func() error

func (c closeFunc) Close() error {
	return c()
}
