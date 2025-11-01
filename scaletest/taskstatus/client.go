package taskstatus

import (
	"context"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// client abstracts the details of using codersdk.Client and agentsdk.Client
// for the taskstatus runner. This interface allows for easier testing by enabling
// mock implementations and provides a cleaner separation of concerns.
//
// The interface is designed to be initialized in two phases:
// 1. Create the client with NewClient(coderClient)
// 2. Configure logging when the io.Writer is available in Run()
type client interface {
	// WatchWorkspace watches for updates to a workspace.
	WatchWorkspace(ctx context.Context, workspaceID uuid.UUID) (<-chan codersdk.Workspace, error)

	// PatchAppStatus updates the status of a workspace app.
	PatchAppStatus(ctx context.Context, req agentsdk.PatchAppStatus) error

	// initialize sets up the client with the provided logger, which is only available after Run() is called.
	initialize(logger slog.Logger)
}

// sdkClient is the concrete implementation of the client interface using
// codersdk.Client and agentsdk.Client.
type sdkClient struct {
	coderClient *codersdk.Client
	agentClient *agentsdk.Client
}

// newClient creates a new client implementation using the provided codersdk.Client.
func newClient(coderClient *codersdk.Client) client {
	return &sdkClient{
		coderClient: coderClient,
	}
}

func (c *sdkClient) WatchWorkspace(ctx context.Context, workspaceID uuid.UUID) (<-chan codersdk.Workspace, error) {
	return c.coderClient.WatchWorkspace(ctx, workspaceID)
}

func (c *sdkClient) PatchAppStatus(ctx context.Context, req agentsdk.PatchAppStatus) error {
	if c.agentClient == nil {
		panic("agentClient not initialized - call initialize first")
	}
	return c.agentClient.PatchAppStatus(ctx, req)
}

func (c *sdkClient) initialize(logger slog.Logger) {
	// Configure the coder client logging
	c.coderClient.SetLogger(logger)
	c.coderClient.SetLogBodies(true)

	// Create and configure the agent client with the same logging settings
	c.agentClient = agentsdk.New(
		c.coderClient.URL,
		agentsdk.WithFixedToken(c.coderClient.SessionTokenProvider.GetSessionToken()),
		codersdk.WithLogger(logger),
		codersdk.WithLogBodies(),
	)
}

// Ensure sdkClient implements the client interface.
var _ client = (*sdkClient)(nil)
