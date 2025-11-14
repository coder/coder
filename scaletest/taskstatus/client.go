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
)

// createExternalWorkspaceResult contains the results from creating an external workspace.
type createExternalWorkspaceResult struct {
	WorkspaceID uuid.UUID
	AgentToken  string
}

// client abstracts the details of using codersdk.Client for workspace operations.
// This interface allows for easier testing by enabling mock implementations and
// provides a cleaner separation of concerns.
//
// The interface is designed to be initialized in two phases:
// 1. Create the client with newClient(coderClient)
// 2. Configure logging when the io.Writer is available in Run()
type client interface {
	// createExternalWorkspace creates an external workspace and returns the workspace ID
	// and agent token for the first external agent found in the workspace resources.
	createExternalWorkspace(ctx context.Context, req codersdk.CreateWorkspaceRequest) (createExternalWorkspaceResult, error)

	// watchWorkspace watches for updates to a workspace.
	watchWorkspace(ctx context.Context, workspaceID uuid.UUID) (<-chan codersdk.Workspace, error)

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
}

// newClient creates a new client implementation using the provided codersdk.Client.
func newClient(coderClient *codersdk.Client) client {
	return &sdkClient{
		coderClient: coderClient,
	}
}

func (c *sdkClient) createExternalWorkspace(ctx context.Context, req codersdk.CreateWorkspaceRequest) (createExternalWorkspaceResult, error) {
	// Create the workspace
	workspace, err := c.coderClient.CreateUserWorkspace(ctx, codersdk.Me, req)
	if err != nil {
		return createExternalWorkspaceResult{}, err
	}

	// Get the workspace with latest build details
	workspace, err = c.coderClient.WorkspaceByOwnerAndName(ctx, codersdk.Me, workspace.Name, codersdk.WorkspaceOptions{})
	if err != nil {
		return createExternalWorkspaceResult{}, err
	}

	// Find external agents in resources
	for _, resource := range workspace.LatestBuild.Resources {
		if resource.Type != "coder_external_agent" || len(resource.Agents) == 0 {
			continue
		}

		// Get credentials for the first agent
		agent := resource.Agents[0]
		credentials, err := c.coderClient.WorkspaceExternalAgentCredentials(ctx, workspace.ID, agent.Name)
		if err != nil {
			return createExternalWorkspaceResult{}, err
		}

		return createExternalWorkspaceResult{
			WorkspaceID: workspace.ID,
			AgentToken:  credentials.AgentToken,
		}, nil
	}

	return createExternalWorkspaceResult{}, xerrors.Errorf("no external agent found in workspace")
}

func (c *sdkClient) watchWorkspace(ctx context.Context, workspaceID uuid.UUID) (<-chan codersdk.Workspace, error) {
	return c.coderClient.WatchWorkspace(ctx, workspaceID)
}

func (c *sdkClient) initialize(logger slog.Logger) {
	// Configure the coder client logging
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
