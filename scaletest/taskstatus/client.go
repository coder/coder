package taskstatus

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
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

// appStatusPatcher abstracts the details of using agentsdk.Client for updating app status.
// This interface is separate from client because it requires an agent token which is only
// available after creating an external workspace.
type appStatusPatcher interface {
	// patchAppStatus updates the status of a workspace app.
	patchAppStatus(ctx context.Context, req agentsdk.PatchAppStatus) error

	// initialize sets up the patcher with the provided logger and agent token.
	initialize(logger slog.Logger, agentToken string)
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

// sdkAppStatusPatcher is the concrete implementation of the appStatusPatcher interface
// using agentsdk.Client.
type sdkAppStatusPatcher struct {
	agentClient *agentsdk.Client
	url         *url.URL
	httpClient  *http.Client
}

// newAppStatusPatcher creates a new appStatusPatcher implementation.
func newAppStatusPatcher(client *codersdk.Client) appStatusPatcher {
	return &sdkAppStatusPatcher{
		url:        client.URL,
		httpClient: client.HTTPClient,
	}
}

func (p *sdkAppStatusPatcher) patchAppStatus(ctx context.Context, req agentsdk.PatchAppStatus) error {
	if p.agentClient == nil {
		panic("agentClient not initialized - call initialize first")
	}
	return p.agentClient.PatchAppStatus(ctx, req)
}

func (p *sdkAppStatusPatcher) initialize(logger slog.Logger, agentToken string) {
	// Create and configure the agent client with the provided token
	p.agentClient = agentsdk.New(
		p.url,
		agentsdk.WithFixedToken(agentToken),
		codersdk.WithHTTPClient(p.httpClient),
		codersdk.WithLogger(logger),
		codersdk.WithLogBodies(),
	)
}

// Ensure sdkClient implements the client interface.
var _ client = (*sdkClient)(nil)

// Ensure sdkAppStatusPatcher implements the appStatusPatcher interface.
var _ appStatusPatcher = (*sdkAppStatusPatcher)(nil)
