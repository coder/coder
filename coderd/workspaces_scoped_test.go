package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// TestCompositeWorkspaceScopes verifies that the composite
// coder:workspaces.* scopes grant the permissions needed for
// workspace lifecycle operations when used on scoped API tokens.
func TestCompositeWorkspaceScopes(t *testing.T) {
	t.Parallel()

	// setupWorkspace creates a server with a provisioner daemon, an
	// admin user, a template, and a workspace. It returns the admin
	// client and the workspace so sub-tests can create scoped tokens
	// and act on them.
	type setupResult struct {
		adminClient *codersdk.Client
		workspace   codersdk.Workspace
	}
	setup := func(t *testing.T) setupResult {
		t.Helper()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		firstUser := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, firstUser.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.GraphComplete,
		})
		template := coderdtest.CreateTemplate(t, client, firstUser.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		return setupResult{
			adminClient: client,
			workspace:   workspace,
		}
	}

	// scopedClient creates an API token restricted to the given scopes
	// and returns a new client authenticated with that token.
	scopedClient := func(t *testing.T, adminClient *codersdk.Client, scopes []codersdk.APIKeyScope) *codersdk.Client {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitShort)
		defer cancel()

		resp, err := adminClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Scopes: scopes,
		})
		require.NoError(t, err, "creating scoped token")

		scoped := codersdk.New(adminClient.URL, codersdk.WithSessionToken(resp.Key))
		t.Cleanup(func() { scoped.HTTPClient.CloseIdleConnections() })
		return scoped
	}

	// coder:workspaces.create — token should be able to create a
	// workspace via POST /users/{user}/workspaces.
	t.Run("WorkspacesCreate", func(t *testing.T) {
		t.Parallel()
		s := setup(t)

		scoped := scopedClient(t, s.adminClient, []codersdk.APIKeyScope{
			codersdk.APIKeyScopeCoderWorkspacesCreate,
		})

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		defer cancel()

		// List workspaces (requires workspace:read, included in the
		// composite scope).
		workspaces, err := scoped.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err, "listing workspaces with coder:workspaces.create scope")
		require.NotEmpty(t, workspaces.Workspaces, "should see at least the existing workspace")

		_, err = scoped.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: s.workspace.TemplateID,
			Name:       coderdtest.RandomUsername(t),
		})
		require.NoError(t, err, "creating workspace with coder:workspaces.create scope")
	})

	// coder:workspaces.operate — token should be able to read and
	// update workspace metadata.
	t.Run("WorkspacesOperate", func(t *testing.T) {
		t.Parallel()
		s := setup(t)

		scoped := scopedClient(t, s.adminClient, []codersdk.APIKeyScope{
			codersdk.APIKeyScopeCoderWorkspacesOperate,
		})

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		defer cancel()

		// Read the workspace by ID (requires workspace:read).
		ws, err := scoped.Workspace(ctx, s.workspace.ID)
		require.NoError(t, err, "reading workspace with coder:workspaces.operate scope")
		require.Equal(t, s.workspace.ID, ws.ID)

		// Update the workspace metadata (requires workspace:update). This goes
		// through the PATCH /workspaces/{workspace} endpoint.
		err = scoped.UpdateWorkspaceTTL(ctx, s.workspace.ID, codersdk.UpdateWorkspaceTTLRequest{
			TTLMillis: ptr.Ref[int64]((time.Hour).Milliseconds()),
		})
		require.NoError(t, err, "updating workspace with coder:workspaces.operate scope")

		// Trigger a start build (requires workspace:update). This goes
		// through POST /workspaces/{workspace}/builds.
		started, err := scoped.CreateWorkspaceBuild(ctx, s.workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: ws.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.NoError(t, err, "starting workspace with coder:workspaces.operate scope")
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, scoped, started.ID)

		_, err = scoped.CreateWorkspaceBuild(ctx, s.workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: ws.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionStop,
		})
		require.NoError(t, err, "starting workspace with coder:workspaces.operate scope")

		// Verify we cannot create a new workspace — the operate scope
		// should not include workspace:create or template:read/use.
		_, err = scoped.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: s.workspace.TemplateID,
			Name:       coderdtest.RandomUsername(t),
		})
		require.Error(t, err, "creating workspace should fail with coder:workspaces.operate scope")
	})

	// coder:workspaces.delete — token should be able to read
	// workspaces and trigger a delete build.
	t.Run("WorkspacesDelete", func(t *testing.T) {
		t.Parallel()
		s := setup(t)

		scoped := scopedClient(t, s.adminClient, []codersdk.APIKeyScope{
			codersdk.APIKeyScopeCoderWorkspacesDelete,
		})

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		defer cancel()

		// Read the workspace by ID (requires workspace:read).
		ws, err := scoped.Workspace(ctx, s.workspace.ID)
		require.NoError(t, err, "reading workspace with coder:workspaces.delete scope")
		require.Equal(t, s.workspace.ID, ws.ID)

		// Delete the workspace via a delete transition build.
		_, err = scoped.CreateWorkspaceBuild(ctx, s.workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: ws.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err, "deleting workspace with coder:workspaces.delete scope")
	})
}
