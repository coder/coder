package mcp_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMCPHTTP_SiblingCleanupProtection(t *testing.T) {
	t.Parallel()
	for _, retained := range []bool{false, true} {
		name := "lease"
		if retained {
			name = "retained"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			clock := quartz.NewMock(t)
			clock.Set(dbtime.Time(time.Now())).MustWait(ctx)
			handler, cancel, url, options := coderdtest.NewOptions(t, &coderdtest.Options{Database: db, Pubsub: ps, Clock: clock, DeploymentValues: mcpDeploymentValues(t)})
			options.DeploymentValues.WorkspaceExecutionCleanup = true
			api := coderd.New(options)
			t.Cleanup(func() { cancel(); require.NoError(t, api.Close()) })
			handler(api.RootHandler)
			coderdtest.NewProvisionerDaemon(t, api)
			client := codersdk.New(url)
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			session := acquisitionClient(ctx, t, client)
			owner, err := acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, toolsdk.AcquireWorkspaceExecutionArgs{OrganizationID: user.OrganizationID.String(), AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{RequestID: uuid.New(), OwnerID: user.UserID, Create: &codersdk.CreateWorkspaceRequest{Name: "protected-" + uuid.NewString()[:8], TemplateID: template.ID}, Disposable: true, Retained: new(false), LeaseExpiresAt: clock.Now().Add(10 * time.Second), Declarations: codersdk.WorkspaceExecutionDeclarations{ResultPaths: []string{}}}})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, *owner.AcquisitionBuildID)
			sibling, err := acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, toolsdk.AcquireWorkspaceExecutionArgs{OrganizationID: user.OrganizationID.String(), AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{RequestID: uuid.New(), OwnerID: user.UserID, WorkspaceID: *owner.WorkspaceID, Retained: &retained, LeaseExpiresAt: clock.Now().Add(20 * time.Second), Declarations: codersdk.WorkspaceExecutionDeclarations{ResultPaths: []string{}}}})
			require.NoError(t, err)
			protectedUntil := clock.Now().Add(40 * time.Second)
			if !retained {
				request := &mcp.CallToolParams{Name: toolsdk.ToolNameRenewWorkspaceExecutionSession, Arguments: toolsdk.RenewWorkspaceExecutionArgs{WorkspaceExecutionControlArgs: toolsdk.WorkspaceExecutionControlArgs{OrganizationID: user.OrganizationID.String(), SessionID: sibling.ID, ExpectedRevision: sibling.Revision}, LeaseExpiresAt: protectedUntil}}
				result, err := session.CallTool(ctx, request)
				require.NoError(t, err)
				require.False(t, result.IsError, "%+v", result.Content)
				stale, err := session.CallTool(ctx, request)
				require.NoError(t, err)
				require.True(t, stale.IsError, "renewal must reject a stale revision")
			}
			checkUntil := clock.Now().Add(30 * time.Second)
			for clock.Now().Before(checkUntil) {
				_, waiter := clock.AdvanceNext()
				waiter.MustWait(ctx)
			}
			observed, err := acquisitionCall(ctx, session, toolsdk.ToolNameGetWorkspaceExecutionSession, toolsdk.GetWorkspaceExecutionSessionArgs{OrganizationID: user.OrganizationID.String(), SessionID: owner.ID})
			require.NoError(t, err)
			require.NotEqual(t, "completed", observed.State)
			if retained {
				require.Contains(t, observed.Error, "retained")
			} else {
				require.Contains(t, observed.Error, "lease")
			}
			workspace, err := client.Workspace(ctx, *owner.WorkspaceID)
			require.NoError(t, err)
			require.Equal(t, *owner.AcquisitionBuildID, workspace.LatestBuild.ID)
			if !retained {
				require.NoError(t, session.Close())
				require.True(t, testutil.Eventually(ctx, t, func(ctx context.Context) bool {
					_, waiter := clock.AdvanceNext()
					waiter.MustWait(ctx)
					current, e := client.WorkspaceExecutionSession(ctx, user.OrganizationID, owner.ID)
					require.NoError(t, e)
					return current.State == "completed"
				}, testutil.IntervalFast))
			}
		})
	}
}
