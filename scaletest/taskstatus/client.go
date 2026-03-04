package taskstatus

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/quartz"
)

// client abstracts the details of using codersdk.Client for workspace operations.
// This interface allows for easier testing by enabling mock implementations and
// provides a cleaner separation of concerns.
//
// The interface is designed to be initialized in two phases:
// 1. Create the client with newClient(coderClient)
// 2. Configure logging when the io.Writer is available in Run()
type client interface {
	// CreateUserWorkspace creates a workspace for a user.
	CreateUserWorkspace(ctx context.Context, userID string, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error)

	// WorkspaceByOwnerAndName retrieves a workspace by owner and name.
	WorkspaceByOwnerAndName(ctx context.Context, owner string, name string, params codersdk.WorkspaceOptions) (codersdk.Workspace, error)

	// WorkspaceExternalAgentCredentials retrieves credentials for an external agent.
	WorkspaceExternalAgentCredentials(ctx context.Context, workspaceID uuid.UUID, agentName string) (codersdk.ExternalAgentCredentials, error)

	// watchWorkspace watches for updates to a workspace.
	watchWorkspace(ctx context.Context, workspaceID uuid.UUID) (<-chan codersdk.Workspace, error)

	// deleteWorkspace deletes the workspace by creating a build with delete transition.
	deleteWorkspace(ctx context.Context, workspaceID uuid.UUID) error

	// initialize sets up the client with the provided logger, which is only available after Run() is called.
	initialize(logger slog.Logger)
}

// appStatusUpdater abstracts the details of updating app status via the
// Agent dRPC API. This interface is separate from client because it
// requires an agent token which is only available after creating an
// external workspace.
type appStatusUpdater interface {
	// updateAppStatus sends a status update for a workspace app.
	updateAppStatus(ctx context.Context, req *agentproto.UpdateAppStatusRequest) error

	// initialize establishes the dRPC connection using the provided
	// agent token. Must be called before updateAppStatus.
	initialize(ctx context.Context, logger slog.Logger, agentToken string) error

	// close cleanly shuts down the underlying dRPC connection.
	close() error
}

// sdkClient is the concrete implementation of the client interface using
// codersdk.Client.
type sdkClient struct {
	coderClient *codersdk.Client
	clock       quartz.Clock
	logger      slog.Logger
}

// newClient creates a new client implementation using the provided codersdk.Client.
func newClient(coderClient *codersdk.Client) client {
	return &sdkClient{
		coderClient: coderClient,
		clock:       quartz.NewReal(),
	}
}

func (c *sdkClient) CreateUserWorkspace(ctx context.Context, userID string, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
	return c.coderClient.CreateUserWorkspace(ctx, userID, req)
}

func (c *sdkClient) WorkspaceByOwnerAndName(ctx context.Context, owner string, name string, params codersdk.WorkspaceOptions) (codersdk.Workspace, error) {
	return c.coderClient.WorkspaceByOwnerAndName(ctx, owner, name, params)
}

func (c *sdkClient) WorkspaceExternalAgentCredentials(ctx context.Context, workspaceID uuid.UUID, agentName string) (codersdk.ExternalAgentCredentials, error) {
	return c.coderClient.WorkspaceExternalAgentCredentials(ctx, workspaceID, agentName)
}

func (c *sdkClient) watchWorkspace(ctx context.Context, workspaceID uuid.UUID) (<-chan codersdk.Workspace, error) {
	return c.coderClient.WatchWorkspace(ctx, workspaceID)
}

func (c *sdkClient) deleteWorkspace(ctx context.Context, workspaceID uuid.UUID) error {
	// Create a build with delete transition to delete the workspace
	_, err := c.coderClient.CreateWorkspaceBuild(ctx, workspaceID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionDelete,
		Reason:     codersdk.CreateWorkspaceBuildReasonCLI,
	})
	if err != nil {
		return xerrors.Errorf("create delete build: %w", err)
	}
	return nil
}

func (c *sdkClient) initialize(logger slog.Logger) {
	// Configure the coder client logging
	c.logger = logger
	c.coderClient.SetLogger(logger)
	c.coderClient.SetLogBodies(true)
}

// sdkAppStatusUpdater is the concrete implementation of the
// appStatusUpdater interface. It dials the Agent dRPC endpoint once
// during initialize and reuses the connection for all subsequent
// UpdateAppStatus calls.
type sdkAppStatusUpdater struct {
	drpcClient agentproto.DRPCAgentClient28
	url        *url.URL
	httpClient *http.Client
}

// newAppStatusUpdater creates a new appStatusUpdater implementation.
func newAppStatusUpdater(client *codersdk.Client) appStatusUpdater {
	return &sdkAppStatusUpdater{
		url:        client.URL,
		httpClient: client.HTTPClient,
	}
}

func (u *sdkAppStatusUpdater) updateAppStatus(ctx context.Context, req *agentproto.UpdateAppStatusRequest) error {
	if u.drpcClient == nil {
		return xerrors.New("dRPC client not initialized - call initialize first")
	}
	_, err := u.drpcClient.UpdateAppStatus(ctx, req)
	return err
}

func (u *sdkAppStatusUpdater) close() error {
	if u.drpcClient == nil {
		return nil
	}
	return u.drpcClient.DRPCConn().Close()
}

func (u *sdkAppStatusUpdater) initialize(ctx context.Context, logger slog.Logger, agentToken string) error {
	agentClient := agentsdk.New(
		u.url,
		agentsdk.WithFixedToken(agentToken),
		codersdk.WithHTTPClient(u.httpClient),
		codersdk.WithLogger(logger),
		codersdk.WithLogBodies(),
	)
	drpcClient, _, err := agentClient.ConnectRPC28WithRole(ctx, "")
	if err != nil {
		return xerrors.Errorf("connect to agent dRPC endpoint: %w", err)
	}
	u.drpcClient = drpcClient
	return nil
}

// Ensure sdkClient implements the client interface.
var _ client = (*sdkClient)(nil)

// Ensure sdkAppStatusUpdater implements the appStatusUpdater interface.
var _ appStatusUpdater = (*sdkAppStatusUpdater)(nil)
