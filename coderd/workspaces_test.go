package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/parameter"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		authz := coderdtest.AssertRBAC(t, api, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		authz.Reset() // Reset all previous checks done in setup.
		ws, err := client.Workspace(ctx, workspace.ID)
		authz.AssertChecked(t, policy.ActionRead, ws)
		require.NoError(t, err)
		require.Equal(t, user.UserID, ws.LatestBuild.InitiatorID)
		require.Equal(t, codersdk.BuildReasonInitiator, ws.LatestBuild.Reason)
	})

	t.Run("Deleted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Getting with deleted=true should still work.
		_, err := client.DeletedWorkspace(ctx, workspace.ID)
		require.NoError(t, err)

		// Delete the workspace
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err, "delete the workspace")
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// Getting with deleted=true should work.
		workspaceNew, err := client.DeletedWorkspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)

		// Getting with deleted=false should not work.
		_, err = client.Workspace(ctx, workspace.ID)
		require.Error(t, err)
		require.ErrorContains(t, err, "410") // gone
	})

	t.Run("Rename", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			AllowWorkspaceRenames:    true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		ws1 := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		ws2 := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws1.LatestBuild.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws2.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		want := ws1.Name + "-test"
		if len(want) > 32 {
			want = want[:32-5] + "-test"
		}
		// Sometimes truncated names result in `--test` which is not an allowed name.
		want = strings.Replace(want, "--", "-", -1)
		err := client.UpdateWorkspace(ctx, ws1.ID, codersdk.UpdateWorkspaceRequest{
			Name: want,
		})
		require.NoError(t, err, "workspace rename failed")

		ws, err := client.Workspace(ctx, ws1.ID)
		require.NoError(t, err)
		require.Equal(t, want, ws.Name, "workspace name not updated")

		err = client.UpdateWorkspace(ctx, ws1.ID, codersdk.UpdateWorkspaceRequest{
			Name: ws2.Name,
		})
		require.Error(t, err, "workspace rename should have failed")
	})

	t.Run("RenameDisabled", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			AllowWorkspaceRenames:    false,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		ws1 := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws1.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		want := "new-name"
		err := client.UpdateWorkspace(ctx, ws1.ID, codersdk.UpdateWorkspaceRequest{
			Name: want,
		})
		require.ErrorContains(t, err, "Workspace renames are not allowed")
	})

	t.Run("TemplateProperties", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		const templateIcon = "/img/icon.svg"
		const templateDisplayName = "This is template"
		templateAllowUserCancelWorkspaceJobs := false
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Icon = templateIcon
			ctr.DisplayName = templateDisplayName
			ctr.AllowUserCancelWorkspaceJobs = &templateAllowUserCancelWorkspaceJobs
		})
		require.NotEmpty(t, template.Name)
		require.NotEmpty(t, template.DisplayName)
		require.NotEmpty(t, template.Icon)
		require.False(t, template.AllowUserCancelWorkspaceJobs)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		ws, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		assert.Equal(t, user.UserID, ws.LatestBuild.InitiatorID)
		assert.Equal(t, codersdk.BuildReasonInitiator, ws.LatestBuild.Reason)
		assert.Equal(t, template.Name, ws.TemplateName)
		assert.Equal(t, templateIcon, ws.TemplateIcon)
		assert.Equal(t, templateDisplayName, ws.TemplateDisplayName)
		assert.Equal(t, templateAllowUserCancelWorkspaceJobs, ws.TemplateAllowUserCancelWorkspaceJobs)
	})

	t.Run("Health", func(t *testing.T) {
		t.Parallel()

		t.Run("Healthy", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete,
				ProvisionApply: []*proto.Response{{
					Type: &proto.Response_Apply{
						Apply: &proto.ApplyComplete{
							Resources: []*proto.Resource{{
								Name: "some",
								Type: "example",
								Agents: []*proto.Agent{{
									Id:   uuid.NewString(),
									Auth: &proto.Agent_Token{},
								}},
							}},
						},
					},
				}},
			})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			workspace, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err)

			agent := workspace.LatestBuild.Resources[0].Agents[0]

			assert.True(t, workspace.Health.Healthy)
			assert.Equal(t, []uuid.UUID{}, workspace.Health.FailingAgents)
			assert.True(t, agent.Health.Healthy)
			assert.Empty(t, agent.Health.Reason)
		})

		t.Run("Unhealthy", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete,
				ProvisionApply: []*proto.Response{{
					Type: &proto.Response_Apply{
						Apply: &proto.ApplyComplete{
							Resources: []*proto.Resource{{
								Name: "some",
								Type: "example",
								Agents: []*proto.Agent{{
									Id:                       uuid.NewString(),
									Auth:                     &proto.Agent_Token{},
									ConnectionTimeoutSeconds: 1,
								}},
							}},
						},
					},
				}},
			})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			var err error
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				workspace, err = client.Workspace(ctx, workspace.ID)
				return assert.NoError(t, err) && !workspace.Health.Healthy
			}, testutil.IntervalMedium)

			agent := workspace.LatestBuild.Resources[0].Agents[0]

			assert.False(t, workspace.Health.Healthy)
			assert.Equal(t, []uuid.UUID{agent.ID}, workspace.Health.FailingAgents)
			assert.False(t, agent.Health.Healthy)
			assert.NotEmpty(t, agent.Health.Reason)
		})

		t.Run("Mixed health", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete,
				ProvisionApply: []*proto.Response{{
					Type: &proto.Response_Apply{
						Apply: &proto.ApplyComplete{
							Resources: []*proto.Resource{{
								Name: "some",
								Type: "example",
								Agents: []*proto.Agent{{
									Id:   uuid.NewString(),
									Name: "a1",
									Auth: &proto.Agent_Token{},
								}, {
									Id:                       uuid.NewString(),
									Name:                     "a2",
									Auth:                     &proto.Agent_Token{},
									ConnectionTimeoutSeconds: 1,
								}},
							}},
						},
					},
				}},
			})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			var err error
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				workspace, err = client.Workspace(ctx, workspace.ID)
				return assert.NoError(t, err) && !workspace.Health.Healthy
			}, testutil.IntervalMedium)

			assert.False(t, workspace.Health.Healthy)
			assert.Len(t, workspace.Health.FailingAgents, 1)

			agent1 := workspace.LatestBuild.Resources[0].Agents[0]
			agent2 := workspace.LatestBuild.Resources[0].Agents[1]

			assert.Equal(t, []uuid.UUID{agent2.ID}, workspace.Health.FailingAgents)
			assert.True(t, agent1.Health.Healthy)
			assert.Empty(t, agent1.Health.Reason)
			assert.False(t, agent2.Health.Healthy)
			assert.NotEmpty(t, agent2.Health.Reason)
		})
	})

	t.Run("Archived", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, ownerClient)

		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

		active := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, active.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, active.ID)
		// We need another version because the active template version cannot be
		// archived.
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil, func(request *codersdk.CreateTemplateVersionRequest) {
			request.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ctx := testutil.Context(t, testutil.WaitMedium)

		err := client.SetArchiveTemplateVersion(ctx, version.ID, true)
		require.NoError(t, err, "archive version")

		_, err = client.CreateWorkspace(ctx, owner.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateVersionID: version.ID,
			Name:              "testworkspace",
		})
		require.Error(t, err, "create workspace with archived version")
		require.ErrorContains(t, err, "Archived template versions cannot")
	})
}

func TestResolveAutostart(t *testing.T) {
	t.Parallel()

	ownerClient, db := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, ownerClient)

	param := database.TemplateVersionParameter{
		Name:         "param",
		DefaultValue: "",
		Required:     true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	client, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	resp := dbfake.WorkspaceBuild(t, db, database.Workspace{
		OwnerID:          member.ID,
		OrganizationID:   owner.OrganizationID,
		AutomaticUpdates: database.AutomaticUpdatesAlways,
	}).Seed(database.WorkspaceBuild{
		InitiatorID: member.ID,
	}).Do()

	workspace := resp.Workspace
	version1 := resp.TemplateVersion

	version2 := dbfake.TemplateVersion(t, db).
		Seed(database.TemplateVersion{
			CreatedBy:      owner.UserID,
			OrganizationID: owner.OrganizationID,
			TemplateID:     version1.TemplateID,
		}).
		Params(param).Do()

	// Autostart shouldn't be possible if parameters do not match.
	resolveResp, err := client.ResolveAutostart(ctx, workspace.ID.String())
	require.NoError(t, err)
	require.True(t, resolveResp.ParameterMismatch)

	_ = dbfake.WorkspaceBuild(t, db, workspace).
		Seed(database.WorkspaceBuild{
			BuildNumber:       2,
			TemplateVersionID: version2.TemplateVersion.ID,
		}).
		Params(database.WorkspaceBuildParameter{
			Name:  "param",
			Value: "hello",
		}).Do()

	// We should be able to autostart since parameters are updated.
	resolveResp, err = client.ResolveAutostart(ctx, workspace.ID.String())
	require.NoError(t, err)
	require.False(t, resolveResp.ParameterMismatch)

	// Create another version that has the same parameters as version2.
	// We should be able to update without issue.
	_ = dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
		CreatedBy:      owner.UserID,
		OrganizationID: owner.OrganizationID,
		TemplateID:     version1.TemplateID,
	}).Params(param).Do()

	// Even though we're out of date we should still be able to autostart
	// since parameters resolve.
	resolveResp, err = client.ResolveAutostart(ctx, workspace.ID.String())
	require.NoError(t, err)
	require.False(t, resolveResp.ParameterMismatch)
}

func TestAdminViewAllWorkspaces(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	_, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)

	otherOrg, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
		Name: "default-test",
	})
	require.NoError(t, err, "create other org")

	// This other user is not in the first user's org. Since other is an admin, they can
	// still see the "first" user's workspace.
	otherOwner, _ := coderdtest.CreateAnotherUser(t, client, otherOrg.ID, rbac.RoleOwner())
	otherWorkspaces, err := otherOwner.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(other) fetch workspaces")

	firstWorkspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(first) fetch workspaces")

	require.ElementsMatch(t, otherWorkspaces.Workspaces, firstWorkspaces.Workspaces)
	require.Equal(t, len(firstWorkspaces.Workspaces), 1, "should be 1 workspace present")

	memberView, _ := coderdtest.CreateAnotherUser(t, client, otherOrg.ID)
	memberViewWorkspaces, err := memberView.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(member) fetch workspaces")
	require.Equal(t, 0, len(memberViewWorkspaces.Workspaces), "member in other org should see 0 workspaces")
}

func TestWorkspacesSortOrder(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	secondUserClient, secondUser := coderdtest.CreateAnotherUserMutators(t, client, firstUser.OrganizationID, []string{"owner"}, func(r *codersdk.CreateUserRequest) {
		r.Username = "zzz"
	})

	// c-workspace should be running
	wsbC := dbfake.WorkspaceBuild(t, db, database.Workspace{Name: "c-workspace", OwnerID: firstUser.UserID, OrganizationID: firstUser.OrganizationID}).Do()

	// b-workspace should be stopped
	wsbB := dbfake.WorkspaceBuild(t, db, database.Workspace{Name: "b-workspace", OwnerID: firstUser.UserID, OrganizationID: firstUser.OrganizationID}).Seed(database.WorkspaceBuild{Transition: database.WorkspaceTransitionStop}).Do()

	// a-workspace should be running
	wsbA := dbfake.WorkspaceBuild(t, db, database.Workspace{Name: "a-workspace", OwnerID: firstUser.UserID, OrganizationID: firstUser.OrganizationID}).Do()

	// d-workspace should be stopped
	wsbD := dbfake.WorkspaceBuild(t, db, database.Workspace{Name: "d-workspace", OwnerID: secondUser.ID, OrganizationID: firstUser.OrganizationID}).Seed(database.WorkspaceBuild{Transition: database.WorkspaceTransitionStop}).Do()

	// e-workspace should also be stopped
	wsbE := dbfake.WorkspaceBuild(t, db, database.Workspace{Name: "e-workspace", OwnerID: secondUser.ID, OrganizationID: firstUser.OrganizationID}).Seed(database.WorkspaceBuild{Transition: database.WorkspaceTransitionStop}).Do()

	// f-workspace is also stopped, but is marked as favorite
	wsbF := dbfake.WorkspaceBuild(t, db, database.Workspace{Name: "f-workspace", OwnerID: firstUser.UserID, OrganizationID: firstUser.OrganizationID}).Seed(database.WorkspaceBuild{Transition: database.WorkspaceTransitionStop}).Do()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	require.NoError(t, client.FavoriteWorkspace(ctx, wsbF.Workspace.ID)) // need to do this via API call for now

	workspacesResponse, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(first) fetch workspaces")
	workspaces := workspacesResponse.Workspaces

	expectedNames := []string{
		wsbF.Workspace.Name, // favorite
		wsbA.Workspace.Name, // running
		wsbC.Workspace.Name, // running
		wsbB.Workspace.Name, // stopped, testuser < zzz
		wsbD.Workspace.Name, // stopped, zzz > testuser
		wsbE.Workspace.Name, // stopped, zzz > testuser
	}

	actualNames := make([]string, 0, len(expectedNames))
	for _, w := range workspaces {
		actualNames = append(actualNames, w.Name)
	}

	// the correct sorting order is:
	// 1. Favorite workspaces (we have one, workspace-f)
	// 2. Running workspaces
	// 3. Sort by usernames
	// 4. Sort by workspace names
	assert.Equal(t, expectedNames, actualNames)

	// Once again but this time as a different user. This time we do not expect to see another
	// user's favorites first.
	workspacesResponse, err = secondUserClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(second) fetch workspaces")
	workspaces = workspacesResponse.Workspaces

	expectedNames = []string{
		wsbA.Workspace.Name, // running
		wsbC.Workspace.Name, // running
		wsbB.Workspace.Name, // stopped, testuser < zzz
		wsbF.Workspace.Name, // stopped, testuser < zzz
		wsbD.Workspace.Name, // stopped, zzz > testuser
		wsbE.Workspace.Name, // stopped, zzz > testuser
	}

	actualNames = make([]string, 0, len(expectedNames))
	for _, w := range workspaces {
		actualNames = append(actualNames, w.Name)
	}

	// the correct sorting order is:
	// 1. Favorite workspaces (we have none this time)
	// 2. Running workspaces
	// 3. Sort by usernames
	// 4. Sort by workspace names
	assert.Equal(t, expectedNames, actualNames)
}

func TestPostWorkspacesByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("InvalidTemplate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateWorkspace(ctx, user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: uuid.New(),
			Name:       "workspace",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("NoTemplateAccess", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleMember(), rbac.RoleOwner())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		org, err := other.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)
		version := coderdtest.CreateTemplateVersion(t, other, org.ID, nil)
		template := coderdtest.CreateTemplate(t, other, org.ID, version.ID)

		_, err = client.CreateWorkspace(ctx, first.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateWorkspace(ctx, user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       workspace.Name,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("CreateWithAuditLogs", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		assert.True(t, auditor.Contains(t, database.AuditLog{
			ResourceType:   database.ResourceTypeWorkspace,
			Action:         database.AuditActionCreate,
			ResourceTarget: workspace.Name,
		}))
	})

	t.Run("CreateFromVersionWithAuditLogs", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		versionDefault := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, versionDefault.ID)
		versionTest := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, nil, template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, versionDefault.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, versionTest.ID)
		defaultWorkspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, uuid.Nil,
			func(c *codersdk.CreateWorkspaceRequest) { c.TemplateVersionID = versionDefault.ID },
		)
		testWorkspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, uuid.Nil,
			func(c *codersdk.CreateWorkspaceRequest) { c.TemplateVersionID = versionTest.ID },
		)
		defaultWorkspaceBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, defaultWorkspace.LatestBuild.ID)
		testWorkspaceBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, testWorkspace.LatestBuild.ID)

		require.Equal(t, testWorkspaceBuild.TemplateVersionID, versionTest.ID)
		require.Equal(t, defaultWorkspaceBuild.TemplateVersionID, versionDefault.ID)
		assert.True(t, auditor.Contains(t, database.AuditLog{
			ResourceType:   database.ResourceTypeWorkspace,
			Action:         database.AuditActionCreate,
			ResourceTarget: defaultWorkspace.Name,
		}))
	})

	t.Run("InvalidCombinationOfTemplateAndTemplateVersion", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		versionTest := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		versionDefault := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, versionDefault.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, versionTest.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, versionDefault.ID)

		name, se := cryptorand.String(8)
		require.NoError(t, se)
		req := codersdk.CreateWorkspaceRequest{
			// Deny setting both of these ID fields, even if they might correlate.
			// Allowing both to be set would just create extra work for everyone involved.
			TemplateID:        template.ID,
			TemplateVersionID: versionTest.ID,
			Name:              name,
			AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
			TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
		}
		_, err := client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.Me, req)

		require.Error(t, err)
	})

	t.Run("CreateWithDeletedTemplate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		err := client.DeleteTemplate(ctx, template.ID)
		require.NoError(t, err)
		_, err = client.CreateWorkspace(ctx, user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "testing",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("TemplateNoTTL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DefaultTTLMillis = ptr.Ref(int64(0))
		})
		// Given: the template has no default TTL set
		require.Zero(t, template.DefaultTTLMillis)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		// When: we create a workspace with autostop not enabled
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = ptr.Ref(int64(0))
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Then: No TTL should be set by the template
		require.Nil(t, workspace.TTLMillis)
	})

	t.Run("TemplateCustomTTL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		templateTTL := 24 * time.Hour.Milliseconds()
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DefaultTTLMillis = ptr.Ref(templateTTL)
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = nil // ensure that no default TTL is set
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// TTL should be set by the template
		require.Equal(t, templateTTL, template.DefaultTTLMillis)
		require.Equal(t, templateTTL, *workspace.TTLMillis)
	})

	t.Run("InvalidTTL", func(t *testing.T) {
		t.Parallel()
		t.Run("BelowMin", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			req := codersdk.CreateWorkspaceRequest{
				TemplateID: template.ID,
				Name:       "testing",
				TTLMillis:  ptr.Ref((59 * time.Second).Milliseconds()),
			}
			_, err := client.CreateWorkspace(ctx, template.OrganizationID, codersdk.Me, req)
			require.Error(t, err)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
			require.Len(t, apiErr.Validations, 1)
			require.Equal(t, "ttl_ms", apiErr.Validations[0].Field)
			require.Equal(t, "time until shutdown must be at least one minute", apiErr.Validations[0].Detail)
		})
	})

	t.Run("TemplateDefaultTTL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		exp := 24 * time.Hour.Milliseconds()
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DefaultTTLMillis = &exp
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// no TTL provided should use template default
		req := codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "testing",
		}
		ws, err := client.CreateWorkspace(ctx, template.OrganizationID, codersdk.Me, req)
		require.NoError(t, err)
		require.EqualValues(t, exp, *ws.TTLMillis)

		// TTL provided should override template default
		req.Name = "testing2"
		exp = 1 * time.Hour.Milliseconds()
		req.TTLMillis = &exp
		ws, err = client.CreateWorkspace(ctx, template.OrganizationID, codersdk.Me, req)
		require.NoError(t, err)
		require.EqualValues(t, exp, *ws.TTLMillis)
	})
}

func TestWorkspaceByOwnerAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.WorkspaceByOwnerAndName(ctx, codersdk.Me, "something", codersdk.WorkspaceOptions{})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.WorkspaceByOwnerAndName(ctx, codersdk.Me, workspace.Name, codersdk.WorkspaceOptions{})
		require.NoError(t, err)
	})
	t.Run("Deleted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Given:
		// We delete the workspace
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err, "delete the workspace")
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// Then:
		// When we call without includes_deleted, we don't expect to get the workspace back
		_, err = client.WorkspaceByOwnerAndName(ctx, workspace.OwnerName, workspace.Name, codersdk.WorkspaceOptions{})
		require.ErrorContains(t, err, "404")

		// Then:
		// When we call with includes_deleted, we should get the workspace back
		workspaceNew, err := client.WorkspaceByOwnerAndName(ctx, workspace.OwnerName, workspace.Name, codersdk.WorkspaceOptions{IncludeDeleted: true})
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)

		// Given:
		// We recreate the workspace with the same name
		workspace, err = client.CreateWorkspace(ctx, user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID:        workspace.TemplateID,
			Name:              workspace.Name,
			AutostartSchedule: workspace.AutostartSchedule,
			TTLMillis:         workspace.TTLMillis,
			AutomaticUpdates:  workspace.AutomaticUpdates,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Then:
		// We can fetch the most recent workspace
		workspaceNew, err = client.WorkspaceByOwnerAndName(ctx, workspace.OwnerName, workspace.Name, codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)

		// Given:
		// We delete the workspace again
		build, err = client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err, "delete the workspace")
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// Then:
		// When we fetch the deleted workspace, we get the most recently deleted one
		workspaceNew, err = client.WorkspaceByOwnerAndName(ctx, workspace.OwnerName, workspace.Name, codersdk.WorkspaceOptions{IncludeDeleted: true})
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)
	})
}

// TestWorkspaceFilterAllStatus tests workspace status is correctly set given a set of conditions.
func TestWorkspaceFilterAllStatus(t *testing.T) {
	t.Parallel()
	if os.Getenv("DB") != "" {
		t.Skip(`This test takes too long with an actual database. Takes 10s on local machine`)
	}

	// For this test, we do not care about permissions.
	// nolint:gocritic // unit testing
	ctx := dbauthz.AsSystemRestricted(context.Background())
	db, pubsub := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})

	owner := coderdtest.CreateFirstUser(t, client)

	file := dbgen.File(t, db, database.File{
		CreatedBy: owner.UserID,
	})
	versionJob := dbgen.ProvisionerJob(t, db, pubsub, database.ProvisionerJob{
		OrganizationID: owner.OrganizationID,
		InitiatorID:    owner.UserID,
		WorkerID:       uuid.NullUUID{},
		FileID:         file.ID,
		Tags: database.StringMap{
			"custom": "true",
		},
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: owner.OrganizationID,
		JobID:          versionJob.ID,
		CreatedBy:      owner.UserID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  owner.OrganizationID,
		ActiveVersionID: version.ID,
		CreatedBy:       owner.UserID,
	})

	makeWorkspace := func(workspace database.Workspace, job database.ProvisionerJob, transition database.WorkspaceTransition) (database.Workspace, database.WorkspaceBuild, database.ProvisionerJob) {
		db := db

		workspace.OwnerID = owner.UserID
		workspace.OrganizationID = owner.OrganizationID
		workspace.TemplateID = template.ID
		workspace = dbgen.Workspace(t, db, workspace)

		jobID := uuid.New()
		job.ID = jobID
		job.Type = database.ProvisionerJobTypeWorkspaceBuild
		job.OrganizationID = owner.OrganizationID
		// Need to prevent acquire from getting this job.
		job.Tags = database.StringMap{
			jobID.String(): "true",
		}
		job = dbgen.ProvisionerJob(t, db, pubsub, job)

		build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: version.ID,
			BuildNumber:       1,
			Transition:        transition,
			InitiatorID:       owner.UserID,
			JobID:             job.ID,
		})

		var err error
		job, err = db.GetProvisionerJobByID(ctx, job.ID)
		require.NoError(t, err)

		return workspace, build, job
	}

	// pending
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusPending),
	}, database.ProvisionerJob{
		StartedAt: sql.NullTime{Valid: false},
	}, database.WorkspaceTransitionStart)

	// starting
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusStarting),
	}, database.ProvisionerJob{
		StartedAt: sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
	}, database.WorkspaceTransitionStart)

	// running
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusRunning),
	}, database.ProvisionerJob{
		CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
	}, database.WorkspaceTransitionStart)

	// stopping
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusStopping),
	}, database.ProvisionerJob{
		StartedAt: sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
	}, database.WorkspaceTransitionStop)

	// stopped
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusStopped),
	}, database.ProvisionerJob{
		StartedAt:   sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
		CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
	}, database.WorkspaceTransitionStop)

	// failed -- delete
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusFailed) + "-deleted",
	}, database.ProvisionerJob{
		StartedAt:   sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
		CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
		Error:       sql.NullString{String: "Some error", Valid: true},
	}, database.WorkspaceTransitionDelete)

	// failed -- stop
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusFailed) + "-stopped",
	}, database.ProvisionerJob{
		StartedAt:   sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
		CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
		Error:       sql.NullString{String: "Some error", Valid: true},
	}, database.WorkspaceTransitionStop)

	// canceling
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusCanceling),
	}, database.ProvisionerJob{
		StartedAt:  sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
		CanceledAt: sql.NullTime{Time: time.Now(), Valid: true},
	}, database.WorkspaceTransitionStart)

	// canceled
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusCanceled),
	}, database.ProvisionerJob{
		StartedAt:   sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
		CanceledAt:  sql.NullTime{Time: time.Now(), Valid: true},
		CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
	}, database.WorkspaceTransitionStart)

	// deleting
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusDeleting),
	}, database.ProvisionerJob{
		StartedAt: sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
	}, database.WorkspaceTransitionDelete)

	// deleted
	makeWorkspace(database.Workspace{
		Name: string(database.WorkspaceStatusDeleted),
	}, database.ProvisionerJob{
		StartedAt:   sql.NullTime{Time: time.Now().Add(time.Second * -2), Valid: true},
		CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
	}, database.WorkspaceTransitionDelete)

	apiCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
	defer cancel()
	workspaces, err := client.Workspaces(apiCtx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)

	// Make sure all workspaces have the correct status
	var statuses []codersdk.WorkspaceStatus
	for _, apiWorkspace := range workspaces.Workspaces {
		expStatus := strings.Split(apiWorkspace.Name, "-")
		if !assert.Equal(t, expStatus[0], string(apiWorkspace.LatestBuild.Status), "workspace has incorrect status") {
			d, _ := json.Marshal(apiWorkspace)
			var buf bytes.Buffer
			_ = json.Indent(&buf, d, "", "\t")
			t.Logf("Incorrect workspace: %s", buf.String())
		}
		statuses = append(statuses, apiWorkspace.LatestBuild.Status)
	}

	// Now test the filter
	for _, status := range statuses {
		ctx, cancel := context.WithTimeout(ctx, testutil.WaitShort)

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Status: string(status),
		})
		require.NoErrorf(t, err, "fetch with status: %s", status)
		for _, workspace := range workspaces.Workspaces {
			assert.Equal(t, status, workspace.LatestBuild.Status, "expect matching status to filter")
		}
		cancel()
	}
}

// TestWorkspaceFilter creates a set of workspaces, users, and organizations
// to run various filters against for testing.
func TestWorkspaceFilter(t *testing.T) {
	t.Parallel()
	// Manual tests still occur below, so this is safe to disable.
	t.Skip("This test is slow and flaky. See: https://github.com/coder/coder/issues/2854")
	// nolint:unused
	type coderUser struct {
		*codersdk.Client
		User codersdk.User
		Org  codersdk.Organization
	}

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	first := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	users := make([]coderUser, 0)
	for i := 0; i < 10; i++ {
		userClient, user := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleOwner())

		if i%3 == 0 {
			var err error
			user, err = client.UpdateUserProfile(ctx, user.ID.String(), codersdk.UpdateUserProfileRequest{
				Username: strings.ToUpper(user.Username),
			})
			require.NoError(t, err, "uppercase username")
		}

		org, err := userClient.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: user.Username + "-org",
		})
		require.NoError(t, err, "create org")

		users = append(users, coderUser{
			Client: userClient,
			User:   user,
			Org:    org,
		})
	}

	type madeWorkspace struct {
		Owner     codersdk.User
		Workspace codersdk.Workspace
		Template  codersdk.Template
	}

	availTemplates := make([]codersdk.Template, 0)
	allWorkspaces := make([]madeWorkspace, 0)
	upperTemplates := make([]string, 0)

	// Create some random workspaces
	var count int
	for i, user := range users {
		version := coderdtest.CreateTemplateVersion(t, client, user.Org.ID, nil)

		// Create a template & workspace in the user's org
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		var template codersdk.Template
		if i%3 == 0 {
			template = coderdtest.CreateTemplate(t, client, user.Org.ID, version.ID, func(request *codersdk.CreateTemplateRequest) {
				request.Name = strings.ToUpper(request.Name)
			})
			upperTemplates = append(upperTemplates, template.Name)
		} else {
			template = coderdtest.CreateTemplate(t, client, user.Org.ID, version.ID)
		}

		availTemplates = append(availTemplates, template)
		workspace := coderdtest.CreateWorkspace(t, user.Client, template.OrganizationID, template.ID, func(request *codersdk.CreateWorkspaceRequest) {
			if count%3 == 0 {
				request.Name = strings.ToUpper(request.Name)
			}
		})
		allWorkspaces = append(allWorkspaces, madeWorkspace{
			Workspace: workspace,
			Template:  template,
			Owner:     user.User,
		})

		// Make a workspace with a random template
		idx, _ := cryptorand.Intn(len(availTemplates))
		randTemplate := availTemplates[idx]
		randWorkspace := coderdtest.CreateWorkspace(t, user.Client, randTemplate.OrganizationID, randTemplate.ID)
		allWorkspaces = append(allWorkspaces, madeWorkspace{
			Workspace: randWorkspace,
			Template:  randTemplate,
			Owner:     user.User,
		})
	}

	// Make sure all workspaces are done. Do it after all are made
	for i, w := range allWorkspaces {
		latest := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, w.Workspace.LatestBuild.ID)
		allWorkspaces[i].Workspace.LatestBuild = latest
	}

	// --- Setup done ---
	testCases := []struct {
		Name   string
		Filter codersdk.WorkspaceFilter
		// If FilterF is true, we include it in the expected results
		FilterF func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool
	}{
		{
			Name:   "All",
			Filter: codersdk.WorkspaceFilter{},
			FilterF: func(_ codersdk.WorkspaceFilter, _ madeWorkspace) bool {
				return true
			},
		},
		{
			Name: "Owner",
			Filter: codersdk.WorkspaceFilter{
				Owner: strings.ToUpper(users[2].User.Username),
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				return strings.EqualFold(workspace.Owner.Username, f.Owner)
			},
		},
		{
			Name: "TemplateName",
			Filter: codersdk.WorkspaceFilter{
				Template: strings.ToUpper(allWorkspaces[5].Template.Name),
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				return strings.EqualFold(workspace.Template.Name, f.Template)
			},
		},
		{
			Name: "UpperTemplateName",
			Filter: codersdk.WorkspaceFilter{
				Template: upperTemplates[0],
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				return strings.EqualFold(workspace.Template.Name, f.Template)
			},
		},
		{
			Name: "Name",
			Filter: codersdk.WorkspaceFilter{
				// Use a common letter... one has to have this letter in it
				Name: "a",
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				return strings.ContainsAny(workspace.Workspace.Name, "Aa")
			},
		},
		{
			Name: "Q-Owner/Name",
			Filter: codersdk.WorkspaceFilter{
				FilterQuery: allWorkspaces[5].Owner.Username + "/" + strings.ToUpper(allWorkspaces[5].Workspace.Name),
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				if strings.EqualFold(workspace.Owner.Username, allWorkspaces[5].Owner.Username) &&
					strings.Contains(strings.ToLower(workspace.Workspace.Name), strings.ToLower(allWorkspaces[5].Workspace.Name)) {
					return true
				}

				return false
			},
		},
		{
			Name: "Many filters",
			Filter: codersdk.WorkspaceFilter{
				Owner:    allWorkspaces[3].Owner.Username,
				Template: allWorkspaces[3].Template.Name,
				Name:     allWorkspaces[3].Workspace.Name,
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				if strings.EqualFold(workspace.Owner.Username, f.Owner) &&
					strings.Contains(strings.ToLower(workspace.Workspace.Name), strings.ToLower(f.Name)) &&
					strings.EqualFold(workspace.Template.Name, f.Template) {
					return true
				}
				return false
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			workspaces, err := client.Workspaces(ctx, c.Filter)
			require.NoError(t, err, "fetch workspaces")

			exp := make([]codersdk.Workspace, 0)
			for _, made := range allWorkspaces {
				if c.FilterF(c.Filter, made) {
					exp = append(exp, made.Workspace)
				}
			}
			require.ElementsMatch(t, exp, workspaces, "expected workspaces returned")
		})
	}
}

// TestWorkspaceFilterManual runs some specific setups with basic checks.
func TestWorkspaceFilterManual(t *testing.T) {
	t.Parallel()

	expectIDs := func(t *testing.T, exp []codersdk.Workspace, got []codersdk.Workspace) {
		t.Helper()
		expIDs := make([]uuid.UUID, 0, len(exp))
		for _, e := range exp {
			expIDs = append(expIDs, e.ID)
		}

		gotIDs := make([]uuid.UUID, 0, len(got))
		for _, g := range got {
			gotIDs = append(gotIDs, g.ID)
		}
		require.ElementsMatchf(t, expIDs, gotIDs, "expected IDs")
	}

	t.Run("Name", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// full match
		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspace.Name,
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1, workspace.Name)
		require.Equal(t, workspace.ID, res.Workspaces[0].ID)

		// partial match
		res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspace.Name[1 : len(workspace.Name)-2],
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		require.Equal(t, workspace.ID, res.Workspaces[0].ID)

		// no match
		res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: "$$$$",
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 0)
	})
	t.Run("IDs", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		alpha := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		bravo := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// full match
		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: fmt.Sprintf("id:%s,%s", alpha.ID, bravo.ID),
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 2)
		require.True(t, slices.ContainsFunc(res.Workspaces, func(workspace codersdk.Workspace) bool {
			return workspace.ID == alpha.ID
		}), "alpha workspace")
		require.True(t, slices.ContainsFunc(res.Workspaces, func(workspace codersdk.Workspace) bool {
			return workspace.ID == alpha.ID
		}), "bravo workspace")

		// no match
		res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: fmt.Sprintf("id:%s", uuid.NewString()),
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 0)
	})
	t.Run("Template", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		version2 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		template2 := coderdtest.CreateTemplate(t, client, user.OrganizationID, version2.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template2.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// empty
		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 2)

		// single template
		res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Template: template.Name,
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		require.Equal(t, workspace.ID, res.Workspaces[0].ID)
	})
	t.Run("Status", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace1 := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		workspace2 := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		// wait for workspaces to be "running"
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace1.LatestBuild.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace2.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// filter finds both running workspaces
		ws1, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, ws1.Workspaces, 2)

		// stop workspace1
		build1 := coderdtest.CreateWorkspaceBuild(t, client, workspace1, database.WorkspaceTransitionStop)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build1.ID)

		// filter finds one running workspace
		ws2, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Status: "running",
		})
		require.NoError(t, err)
		require.Len(t, ws2.Workspaces, 1)
		require.Equal(t, workspace2.ID, ws2.Workspaces[0].ID)

		// stop workspace2
		build2 := coderdtest.CreateWorkspaceBuild(t, client, workspace2, database.WorkspaceTransitionStop)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build2.ID)

		// filter finds no running workspaces
		ws3, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Status: "running",
		})
		require.NoError(t, err)
		require.Len(t, ws3.Workspaces, 0)
	})
	t.Run("FilterQuery", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		version2 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		template2 := coderdtest.CreateTemplate(t, client, user.OrganizationID, version2.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template2.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// single workspace
		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: fmt.Sprintf("template:%s %s/%s", template.Name, workspace.OwnerName, workspace.Name),
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		require.Equal(t, workspace.ID, res.Workspaces[0].ID)
	})
	t.Run("FilterQueryHasAgentConnecting", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: fmt.Sprintf("has-agent:%s", "connecting"),
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		require.Equal(t, workspace.ID, res.Workspaces[0].ID)
	})
	t.Run("FilterQueryHasAgentConnected", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		_ = agenttest.New(t, client.URL, authToken)
		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: fmt.Sprintf("has-agent:%s", "connected"),
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		require.Equal(t, workspace.ID, res.Workspaces[0].ID)
	})
	t.Run("FilterQueryHasAgentTimeout", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agents: []*proto.Agent{{
								Id: uuid.NewString(),
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
								ConnectionTimeoutSeconds: 1,
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		testutil.Eventually(ctx, t, func(ctx context.Context) (done bool) {
			workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				FilterQuery: fmt.Sprintf("has-agent:%s", "timeout"),
			})
			require.NoError(t, err)
			return workspaces.Count == 1
		}, testutil.IntervalMedium, "agent status timeout")
	})
	t.Run("Dormant", func(t *testing.T) {
		// this test has a licensed counterpart in enterprise/coderd/workspaces_test.go: FilterQueryHasDeletingByAndLicensed
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		template := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
			OrganizationID: user.OrganizationID,
			CreatedBy:      user.UserID,
		}).Do().Template

		// update template with inactivity ttl
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		dormantWorkspace := dbfake.WorkspaceBuild(t, db, database.Workspace{
			TemplateID:     template.ID,
			OwnerID:        user.UserID,
			OrganizationID: user.OrganizationID,
		}).Do().Workspace

		// Create another workspace to validate that we do not return active workspaces.
		_ = dbfake.WorkspaceBuild(t, db, database.Workspace{
			TemplateID:     template.ID,
			OwnerID:        user.UserID,
			OrganizationID: user.OrganizationID,
		}).Do()

		err := client.UpdateWorkspaceDormancy(ctx, dormantWorkspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		// Test that no filter returns both workspaces.
		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 2)

		// Test that filtering for dormant only returns our dormant workspace.
		res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: "dormant:true",
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		require.Equal(t, dormantWorkspace.ID, res.Workspaces[0].ID)
		require.NotNil(t, res.Workspaces[0].DormantAt)
	})
	t.Run("LastUsed", func(t *testing.T) {
		t.Parallel()

		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		// update template with inactivity ttl
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		now := dbtime.Now()
		before := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, before.LatestBuild.ID)

		after := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, after.LatestBuild.ID)

		//nolint:gocritic // Unit testing context
		err := api.Database.UpdateWorkspaceLastUsedAt(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceLastUsedAtParams{
			ID:         before.ID,
			LastUsedAt: now.UTC().Add(time.Hour * -1),
		})
		require.NoError(t, err)

		// Unit testing context
		//nolint:gocritic // Unit testing context
		err = api.Database.UpdateWorkspaceLastUsedAt(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceLastUsedAtParams{
			ID:         after.ID,
			LastUsedAt: now.UTC().Add(time.Hour * 1),
		})
		require.NoError(t, err)

		beforeRes, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: fmt.Sprintf("last_used_before:%q", now.Format(time.RFC3339)),
		})
		require.NoError(t, err)
		require.Len(t, beforeRes.Workspaces, 1)
		require.Equal(t, before.ID, beforeRes.Workspaces[0].ID)

		afterRes, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: fmt.Sprintf("last_used_after:%q", now.Format(time.RFC3339)),
		})
		require.NoError(t, err)
		require.Len(t, afterRes.Workspaces, 1)
		require.Equal(t, after.ID, afterRes.Workspaces[0].ID)
	})
	t.Run("Updated", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Workspace is up-to-date
		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: "outdated:false",
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		require.Equal(t, workspace.ID, res.Workspaces[0].ID)

		res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: "outdated:true",
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 0)

		// Now make it out of date
		newTv := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil, func(request *codersdk.CreateTemplateVersionRequest) {
			request.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, newTv.ID)
		err = client.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: newTv.ID,
		})
		require.NoError(t, err)

		// Check the query again
		res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: "outdated:false",
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 0)

		res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: "outdated:true",
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		require.Equal(t, workspace.ID, res.Workspaces[0].ID)
	})
	t.Run("Params", func(t *testing.T) {
		t.Parallel()

		const (
			paramOneName   = "one"
			paramTwoName   = "two"
			paramThreeName = "three"
			paramOptional  = "optional"
		)

		makeParameters := func(extra ...*proto.RichParameter) *echo.Responses {
			return &echo.Responses{
				Parse: echo.ParseComplete,
				ProvisionPlan: []*proto.Response{
					{
						Type: &proto.Response_Plan{
							Plan: &proto.PlanComplete{
								Parameters: append([]*proto.RichParameter{
									{Name: paramOneName, Description: "", Mutable: true, Type: "string"},
									{Name: paramTwoName, DisplayName: "", Description: "", Mutable: true, Type: "string"},
									{Name: paramThreeName, Description: "", Mutable: true, Type: "string"},
								}, extra...),
							},
						},
					},
				},
				ProvisionApply: echo.ApplyComplete,
			}
		}

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, makeParameters(&proto.RichParameter{Name: paramOptional, Description: "", Mutable: true, Type: "string"}))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		noOptionalVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, makeParameters(), func(request *codersdk.CreateTemplateVersionRequest) {
			request.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, noOptionalVersion.ID)

		// foo :: one=foo, two=bar, one=baz, optional=optional
		foo := coderdtest.CreateWorkspace(t, client, user.OrganizationID, uuid.Nil, func(request *codersdk.CreateWorkspaceRequest) {
			request.TemplateVersionID = version.ID
			request.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{
					Name:  paramOneName,
					Value: "foo",
				},
				{
					Name:  paramTwoName,
					Value: "bar",
				},
				{
					Name:  paramThreeName,
					Value: "baz",
				},
				{
					Name:  paramOptional,
					Value: "optional",
				},
			}
		})

		// bar :: one=foo, two=bar, three=baz, optional=optional
		bar := coderdtest.CreateWorkspace(t, client, user.OrganizationID, uuid.Nil, func(request *codersdk.CreateWorkspaceRequest) {
			request.TemplateVersionID = version.ID
			request.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{
					Name:  paramOneName,
					Value: "bar",
				},
				{
					Name:  paramTwoName,
					Value: "bar",
				},
				{
					Name:  paramThreeName,
					Value: "baz",
				},
				{
					Name:  paramOptional,
					Value: "optional",
				},
			}
		})

		// baz :: one=baz, two=baz, three=baz
		baz := coderdtest.CreateWorkspace(t, client, user.OrganizationID, uuid.Nil, func(request *codersdk.CreateWorkspaceRequest) {
			request.TemplateVersionID = noOptionalVersion.ID
			request.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{
					Name:  paramOneName,
					Value: "unique",
				},
				{
					Name:  paramTwoName,
					Value: "baz",
				},
				{
					Name:  paramThreeName,
					Value: "baz",
				},
			}
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		//nolint:tparallel,paralleltest
		t.Run("has_param", func(t *testing.T) {
			// Checks the existence of a param value
			// all match
			all, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				FilterQuery: fmt.Sprintf("param:%s", paramOneName),
			})
			require.NoError(t, err)
			expectIDs(t, []codersdk.Workspace{foo, bar, baz}, all.Workspaces)

			// Some match
			optional, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				FilterQuery: fmt.Sprintf("param:%s", paramOptional),
			})
			require.NoError(t, err)
			expectIDs(t, []codersdk.Workspace{foo, bar}, optional.Workspaces)

			// None match
			none, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				FilterQuery: "param:not-a-param",
			})
			require.NoError(t, err)
			require.Len(t, none.Workspaces, 0)
		})

		//nolint:tparallel,paralleltest
		t.Run("exact_param", func(t *testing.T) {
			// All match
			all, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				FilterQuery: fmt.Sprintf("param:%s=%s", paramThreeName, "baz"),
			})
			require.NoError(t, err)
			expectIDs(t, []codersdk.Workspace{foo, bar, baz}, all.Workspaces)

			// Two match
			two, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				FilterQuery: fmt.Sprintf("param:%s=%s", paramTwoName, "bar"),
			})
			require.NoError(t, err)
			expectIDs(t, []codersdk.Workspace{foo, bar}, two.Workspaces)

			// Only 1 matches
			one, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				FilterQuery: fmt.Sprintf("param:%s=%s", paramOneName, "foo"),
			})
			require.NoError(t, err)
			expectIDs(t, []codersdk.Workspace{foo}, one.Workspaces)
		})

		//nolint:tparallel,paralleltest
		t.Run("exact_param_and_has", func(t *testing.T) {
			all, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				FilterQuery: fmt.Sprintf("param:not=athing param:%s=%s param:%s=%s", paramOptional, "optional", paramOneName, "unique"),
			})
			require.NoError(t, err)
			expectIDs(t, []codersdk.Workspace{foo, bar, baz}, all.Workspaces)
		})
	})
}

func TestOffsetLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

	// Case 1: empty finds all workspaces
	ws, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, ws.Workspaces, 3)

	// Case 2: offset 1 finds 2 workspaces
	ws, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
		Offset: 1,
	})
	require.NoError(t, err)
	require.Len(t, ws.Workspaces, 2)

	// Case 3: offset 1 limit 1 finds 1 workspace
	ws, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
		Offset: 1,
		Limit:  1,
	})
	require.NoError(t, err)
	require.Len(t, ws.Workspaces, 1)

	// Case 4: offset 3 finds no workspaces
	ws, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
		Offset: 3,
	})
	require.NoError(t, err)
	require.Len(t, ws.Workspaces, 0)
	require.Equal(t, ws.Count, 3) // can't find workspaces, but count is non-zero

	// Case 5: offset out of range
	ws, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
		Offset: math.MaxInt32 + 1, // Potential risk: pq: OFFSET must not be negative
	})
	require.Error(t, err)
}

func TestWorkspaceUpdateAutostart(t *testing.T) {
	t.Parallel()
	dublinLoc := mustLocation(t, "Europe/Dublin")

	testCases := []struct {
		name             string
		schedule         *string
		expectedError    string
		at               time.Time
		expectedNext     time.Time
		expectedInterval time.Duration
	}{
		{
			name:          "disable autostart",
			schedule:      ptr.Ref(""),
			expectedError: "",
		},
		{
			name:             "friday to monday",
			schedule:         ptr.Ref("CRON_TZ=Europe/Dublin 30 9 * * 1-5"),
			expectedError:    "",
			at:               time.Date(2022, 5, 6, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 5, 9, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 71*time.Hour + 59*time.Minute,
		},
		{
			name:             "monday to tuesday",
			schedule:         ptr.Ref("CRON_TZ=Europe/Dublin 30 9 * * 1-5"),
			expectedError:    "",
			at:               time.Date(2022, 5, 9, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 5, 10, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 23*time.Hour + 59*time.Minute,
		},
		{
			// DST in Ireland began on Mar 27 in 2022 at 0100. Forward 1 hour.
			name:             "DST start",
			schedule:         ptr.Ref("CRON_TZ=Europe/Dublin 30 9 * * *"),
			expectedError:    "",
			at:               time.Date(2022, 3, 26, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 3, 27, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 22*time.Hour + 59*time.Minute,
		},
		{
			// DST in Ireland ends on Oct 30 in 2022 at 0200. Back 1 hour.
			name:             "DST end",
			schedule:         ptr.Ref("CRON_TZ=Europe/Dublin 30 9 * * *"),
			expectedError:    "",
			at:               time.Date(2022, 10, 29, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 10, 30, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 24*time.Hour + 59*time.Minute,
		},
		{
			name:          "invalid location",
			schedule:      ptr.Ref("CRON_TZ=Imaginary/Place 30 9 * * 1-5"),
			expectedError: "parse schedule: provided bad location Imaginary/Place: unknown time zone Imaginary/Place",
		},
		{
			name:          "invalid schedule",
			schedule:      ptr.Ref("asdf asdf asdf "),
			expectedError: `validate weekly schedule: expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix`,
		},
		{
			name:          "only 3 values",
			schedule:      ptr.Ref("CRON_TZ=Europe/Dublin 30 9 *"),
			expectedError: `validate weekly schedule: expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var (
				auditor   = audit.NewMock()
				client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
				user      = coderdtest.CreateFirstUser(t, client)
				version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
				_         = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
					cwr.AutostartSchedule = nil
					cwr.TTLMillis = nil
				})
			)

			// await job to ensure audit logs for workspace_build start are created
			_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			// ensure test invariant: new workspaces have no autostart schedule.
			require.Empty(t, workspace.AutostartSchedule, "expected newly-minted workspace to have no autostart schedule")

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			err := client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: testCase.schedule,
			})

			if testCase.expectedError != "" {
				require.ErrorContains(t, err, testCase.expectedError, "Invalid autostart schedule")
				return
			}

			require.NoError(t, err, "expected no error setting workspace autostart schedule")

			updated, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err, "fetch updated workspace")

			if testCase.schedule == nil || *testCase.schedule == "" {
				require.Nil(t, updated.AutostartSchedule)
				return
			}

			require.EqualValues(t, *testCase.schedule, *updated.AutostartSchedule, "expected autostart schedule to equal requested")

			sched, err := cron.Weekly(*updated.AutostartSchedule)
			require.NoError(t, err, "parse returned schedule")

			next := sched.Next(testCase.at)
			require.Equal(t, testCase.expectedNext, next, "unexpected next scheduled autostart time")
			interval := next.Sub(testCase.at)
			require.Equal(t, testCase.expectedInterval, interval, "unexpected interval")

			require.Eventually(t, func() bool {
				if len(auditor.AuditLogs()) < 7 {
					return false
				}
				return auditor.AuditLogs()[6].Action == database.AuditActionWrite ||
					auditor.AuditLogs()[5].Action == database.AuditActionWrite
			}, testutil.WaitShort, testutil.IntervalFast)
		})
	}

	t.Run("CustomAutostartDisabledByTemplate", func(t *testing.T) {
		t.Parallel()
		var (
			tss = schedule.MockTemplateScheduleStore{
				GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					return schedule.TemplateScheduleOptions{
						UserAutostartEnabled: false,
						UserAutostopEnabled:  false,
						DefaultTTL:           0,
						AutostopRequirement:  schedule.TemplateAutostopRequirement{},
					}, nil
				},
				SetFn: func(_ context.Context, _ database.Store, tpl database.Template, _ schedule.TemplateScheduleOptions) (database.Template, error) {
					return tpl, nil
				},
			}

			client = coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				TemplateScheduleStore:    tss,
			})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.AutostartSchedule = nil
				cwr.TTLMillis = nil
			})
		)

		// await job to ensure audit logs for workspace_build start are created
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// ensure test invariant: new workspaces have no autostart schedule.
		require.Empty(t, workspace.AutostartSchedule, "expected newly-minted workspace to have no autostart schedule")

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
			Schedule: ptr.Ref("CRON_TZ=Europe/Dublin 30 9 * * 1-5"),
		})
		require.ErrorContains(t, err, "Autostart is not allowed for workspaces using this template")
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			client = coderdtest.New(t, nil)
			_      = coderdtest.CreateFirstUser(t, client)
			wsid   = uuid.New()
			req    = codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: ptr.Ref("9 30 1-5"),
			}
		)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateWorkspaceAutostart(ctx, wsid, req)
		require.IsType(t, err, &codersdk.Error{}, "expected codersdk.Error")
		coderSDKErr, _ := err.(*codersdk.Error) //nolint:errorlint
		require.Equal(t, coderSDKErr.StatusCode(), 404, "expected status code 404")
		require.Contains(t, coderSDKErr.Message, "Resource not found", "unexpected response code")
	})
}

func TestWorkspaceUpdateTTL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		ttlMillis      *int64
		expectedError  string
		modifyTemplate func(*codersdk.CreateTemplateRequest)
	}{
		{
			name:          "disable ttl",
			ttlMillis:     nil,
			expectedError: "",
			modifyTemplate: func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = ptr.Ref((8 * time.Hour).Milliseconds())
			},
		},
		{
			name:          "update ttl",
			ttlMillis:     ptr.Ref(12 * time.Hour.Milliseconds()),
			expectedError: "",
			modifyTemplate: func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = ptr.Ref((8 * time.Hour).Milliseconds())
			},
		},
		{
			name:          "below minimum ttl",
			ttlMillis:     ptr.Ref((30 * time.Second).Milliseconds()),
			expectedError: "time until shutdown must be at least one minute",
		},
		{
			name:          "minimum ttl",
			ttlMillis:     ptr.Ref(time.Minute.Milliseconds()),
			expectedError: "",
		},
		{
			name:          "maximum ttl",
			ttlMillis:     ptr.Ref((24 * 30 * time.Hour).Milliseconds()),
			expectedError: "",
		},
		{
			name:          "above maximum ttl",
			ttlMillis:     ptr.Ref((24*30*time.Hour + time.Minute).Milliseconds()),
			expectedError: "time until shutdown must be less than 30 days",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mutators := make([]func(*codersdk.CreateTemplateRequest), 0)
			if testCase.modifyTemplate != nil {
				mutators = append(mutators, testCase.modifyTemplate)
			}
			var (
				auditor   = audit.NewMock()
				client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
				user      = coderdtest.CreateFirstUser(t, client)
				version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
				_         = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, mutators...)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
					cwr.AutostartSchedule = nil
					cwr.TTLMillis = nil
				})
				_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
			)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			err := client.UpdateWorkspaceTTL(ctx, workspace.ID, codersdk.UpdateWorkspaceTTLRequest{
				TTLMillis: testCase.ttlMillis,
			})

			if testCase.expectedError != "" {
				require.ErrorContains(t, err, testCase.expectedError, "unexpected error when setting workspace autostop schedule")
				return
			}

			require.NoError(t, err, "expected no error setting workspace autostop schedule")

			updated, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err, "fetch updated workspace")

			require.Equal(t, testCase.ttlMillis, updated.TTLMillis, "expected autostop ttl to equal requested")

			require.Eventually(t, func() bool {
				if len(auditor.AuditLogs()) != 7 {
					return false
				}
				return auditor.AuditLogs()[6].Action == database.AuditActionWrite ||
					auditor.AuditLogs()[5].Action == database.AuditActionWrite
			}, testutil.WaitMedium, testutil.IntervalFast, "expected audit log to be written")
		})
	}

	t.Run("CustomAutostopDisabledByTemplate", func(t *testing.T) {
		t.Parallel()
		var (
			tss = schedule.MockTemplateScheduleStore{
				GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					return schedule.TemplateScheduleOptions{
						UserAutostartEnabled: false,
						UserAutostopEnabled:  false,
						DefaultTTL:           0,
						AutostopRequirement:  schedule.TemplateAutostopRequirement{},
					}, nil
				},
				SetFn: func(_ context.Context, _ database.Store, tpl database.Template, _ schedule.TemplateScheduleOptions) (database.Template, error) {
					return tpl, nil
				},
			}

			client = coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				TemplateScheduleStore:    tss,
			})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.AutostartSchedule = nil
				cwr.TTLMillis = nil
			})
		)

		// await job to ensure audit logs for workspace_build start are created
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// ensure test invariant: new workspaces have no autostart schedule.
		require.Empty(t, workspace.AutostartSchedule, "expected newly-minted workspace to have no autostart schedule")

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateWorkspaceTTL(ctx, workspace.ID, codersdk.UpdateWorkspaceTTLRequest{
			TTLMillis: ptr.Ref(time.Hour.Milliseconds()),
		})
		require.ErrorContains(t, err, "Custom autostop TTL is not allowed for workspaces using this template")
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			client = coderdtest.New(t, nil)
			_      = coderdtest.CreateFirstUser(t, client)
			wsid   = uuid.New()
			req    = codersdk.UpdateWorkspaceTTLRequest{
				TTLMillis: ptr.Ref(time.Hour.Milliseconds()),
			}
		)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateWorkspaceTTL(ctx, wsid, req)
		require.IsType(t, err, &codersdk.Error{}, "expected codersdk.Error")
		coderSDKErr, _ := err.(*codersdk.Error) //nolint:errorlint
		require.Equal(t, coderSDKErr.StatusCode(), 404, "expected status code 404")
		require.Contains(t, coderSDKErr.Message, "Resource not found", "unexpected response code")
	})
}

func TestWorkspaceExtend(t *testing.T) {
	t.Parallel()
	var (
		ttl         = 8 * time.Hour
		newDeadline = time.Now().Add(ttl + time.Hour).UTC()
		client      = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user        = coderdtest.CreateFirstUser(t, client)
		version     = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_           = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template    = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace   = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = ptr.Ref(ttl.Milliseconds())
		})
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	workspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "fetch provisioned workspace")
	oldDeadline := workspace.LatestBuild.Deadline.Time

	// Updating the deadline should succeed
	req := codersdk.PutExtendWorkspaceRequest{
		Deadline: newDeadline,
	}
	err = client.PutExtendWorkspace(ctx, workspace.ID, req)
	require.NoError(t, err, "failed to extend workspace")

	// Ensure deadline set correctly
	updated, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "failed to fetch updated workspace")
	require.WithinDuration(t, newDeadline, updated.LatestBuild.Deadline.Time, time.Minute)

	// Zero time should fail
	err = client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: time.Time{},
	})
	require.ErrorContains(t, err, "deadline: Validation failed for tag \"required\" with value: \"0001-01-01 00:00:00 +0000 UTC\"", "setting an empty deadline on a workspace should fail")

	// Updating with a deadline less than 30 minutes in the future should fail
	deadlineTooSoon := time.Now().Add(15 * time.Minute) // XXX: time.Now
	err = client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: deadlineTooSoon,
	})
	require.ErrorContains(t, err, "unexpected status code 400: Cannot extend workspace: new deadline must be at least 30 minutes in the future", "setting a deadline less than 30 minutes in the future should fail")

	// Updating with a deadline 30 minutes in the future should succeed
	deadlineJustSoonEnough := time.Now().Add(30 * time.Minute)
	err = client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: deadlineJustSoonEnough,
	})
	require.NoError(t, err, "setting a deadline at least 30 minutes in the future should succeed")

	// Updating with a deadline an hour before the previous deadline should succeed
	err = client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: oldDeadline.Add(-time.Hour),
	})
	require.NoError(t, err, "setting an earlier deadline should not fail")

	// Ensure deadline still set correctly
	updated, err = client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "failed to fetch updated workspace")
	require.WithinDuration(t, oldDeadline.Add(-time.Hour), updated.LatestBuild.Deadline.Time, time.Minute)
}

func TestWorkspaceUpdateAutomaticUpdates_OK(t *testing.T) {
	t.Parallel()

	var (
		auditor      = audit.NewMock()
		adminClient  = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		admin        = coderdtest.CreateFirstUser(t, adminClient)
		client, user = coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
		version      = coderdtest.CreateTemplateVersion(t, adminClient, admin.OrganizationID, nil)
		_            = coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)
		project      = coderdtest.CreateTemplate(t, adminClient, admin.OrganizationID, version.ID)
		workspace    = coderdtest.CreateWorkspace(t, client, admin.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = nil
			cwr.TTLMillis = nil
			cwr.AutomaticUpdates = codersdk.AutomaticUpdatesNever
		})
	)

	// await job to ensure audit logs for workspace_build start are created
	_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	// ensure test invariant: new workspaces have automatic updates set to never
	require.Equal(t, codersdk.AutomaticUpdatesNever, workspace.AutomaticUpdates, "expected newly-minted workspace to automatic updates set to never")

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	err := client.UpdateWorkspaceAutomaticUpdates(ctx, workspace.ID, codersdk.UpdateWorkspaceAutomaticUpdatesRequest{
		AutomaticUpdates: codersdk.AutomaticUpdatesAlways,
	})
	require.NoError(t, err)

	updated, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.AutomaticUpdatesAlways, updated.AutomaticUpdates)

	require.Eventually(t, func() bool {
		var found bool
		for _, l := range auditor.AuditLogs() {
			if l.Action == database.AuditActionWrite &&
				l.UserID == user.ID &&
				l.ResourceID == workspace.ID {
				found = true
				break
			}
		}
		return found
	}, testutil.WaitShort, testutil.IntervalFast, "did not find expected audit log")
}

func TestUpdateWorkspaceAutomaticUpdates_NotFound(t *testing.T) {
	t.Parallel()
	var (
		client = coderdtest.New(t, nil)
		_      = coderdtest.CreateFirstUser(t, client)
		wsid   = uuid.New()
		req    = codersdk.UpdateWorkspaceAutomaticUpdatesRequest{
			AutomaticUpdates: codersdk.AutomaticUpdatesNever,
		}
	)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	err := client.UpdateWorkspaceAutomaticUpdates(ctx, wsid, req)
	require.IsType(t, err, &codersdk.Error{}, "expected codersdk.Error")
	coderSDKErr, _ := err.(*codersdk.Error) //nolint:errorlint
	require.Equal(t, coderSDKErr.StatusCode(), 404, "expected status code 404")
	require.Contains(t, coderSDKErr.Message, "Resource not found", "unexpected response code")
}

func TestWorkspaceWatcher(t *testing.T) {
	t.Parallel()
	client, closeFunc := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		AllowWorkspaceRenames:    true,
	})
	defer closeFunc.Close()
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.PlanComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id: uuid.NewString(),
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
							ConnectionTimeoutSeconds: 1,
						}},
					}},
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	wc, err := client.WatchWorkspace(ctx, workspace.ID)
	require.NoError(t, err)

	// Wait events are easier to debug with timestamped logs.
	logger := slogtest.Make(t, nil).Named(t.Name()).Leveled(slog.LevelDebug)
	wait := func(event string, ready func(w codersdk.Workspace) bool) {
		for {
			select {
			case <-ctx.Done():
				require.FailNow(t, "timed out waiting for event", event)
			case w, ok := <-wc:
				require.True(t, ok, "watch channel closed: %s", event)
				if ready == nil || ready(w) {
					logger.Info(ctx, "done waiting for event",
						slog.F("event", event),
						slog.F("workspace", w))
					return
				}
				logger.Info(ctx, "skipped update for event",
					slog.F("event", event),
					slog.F("workspace", w))
			}
		}
	}

	coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)
	wait("workspace build being created", nil)
	wait("workspace build being acquired", nil)
	wait("workspace build completing", nil)

	// Unfortunately, this will add ~1s to the test due to the granularity
	// of agent timeout seconds. However, if we don't do this we won't know
	// which trigger we received when waiting for connection.
	//
	// Note that the first timeout is from `coderdtest.CreateWorkspace` and
	// the latter is from `coderdtest.CreateWorkspaceBuild`.
	wait("agent timeout after create", nil)
	wait("agent timeout after start", nil)

	agt := agenttest.New(t, client.URL, authToken)
	_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	wait("agent connected/ready", func(w codersdk.Workspace) bool {
		return w.LatestBuild.Resources[0].Agents[0].Status == codersdk.WorkspaceAgentConnected &&
			w.LatestBuild.Resources[0].Agents[0].LifecycleState == codersdk.WorkspaceAgentLifecycleReady
	})
	agt.Close()
	wait("agent disconnected", func(w codersdk.Workspace) bool {
		return w.LatestBuild.Resources[0].Agents[0].Status == codersdk.WorkspaceAgentDisconnected
	})

	err = client.UpdateWorkspace(ctx, workspace.ID, codersdk.UpdateWorkspaceRequest{
		Name: "another",
	})
	require.NoError(t, err)
	wait("update workspace name", nil)

	// Add a new version that will fail.
	badVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.PlanComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Error: "test error",
				},
			},
		}},
	}, func(req *codersdk.CreateTemplateVersionRequest) {
		req.TemplateID = template.ID
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, badVersion.ID)
	err = client.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
		ID: badVersion.ID,
	})
	require.NoError(t, err)
	wait("update active template version", nil)

	// Build with the new template; should end up with a failure state.
	_ = coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart, func(req *codersdk.CreateWorkspaceBuildRequest) {
		req.TemplateVersionID = badVersion.ID
	})
	// We want to verify pending state here, but it's possible that we reach
	// failed state fast enough that we never see pending.
	sawFailed := false
	wait("workspace build pending or failed", func(w codersdk.Workspace) bool {
		switch w.LatestBuild.Status {
		case codersdk.WorkspaceStatusPending:
			return true
		case codersdk.WorkspaceStatusFailed:
			sawFailed = true
			return true
		default:
			return false
		}
	})
	if !sawFailed {
		wait("workspace build failed", func(w codersdk.Workspace) bool {
			return w.LatestBuild.Status == codersdk.WorkspaceStatusFailed
		})
	}

	closeFunc.Close()
	build := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)
	wait("first is for the workspace build itself", nil)
	err = client.CancelWorkspaceBuild(ctx, build.ID)
	require.NoError(t, err)
	wait("second is for the build cancel", nil)
}

func mustLocation(t *testing.T, location string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(location)
	if err != nil {
		t.Errorf("failed to load location %s: %s", location, err.Error())
	}

	return loc
}

func TestWorkspaceResource(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "beta",
							Type: "example",
							Icon: "/icon/server.svg",
							Agents: []*proto.Agent{{
								Id:   "something",
								Name: "b",
								Auth: &proto.Agent_Token{},
							}, {
								Id:   "another",
								Name: "a",
								Auth: &proto.Agent_Token{},
							}},
						}, {
							Name: "alpha",
							Type: "example",
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 2)
		// Ensure Icon is present
		require.Equal(t, "/icon/server.svg", workspace.LatestBuild.Resources[0].Icon)
	})

	t.Run("Apps", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		apps := []*proto.App{
			{
				Slug:        "code-server",
				DisplayName: "code-server",
				Command:     "some-command",
				Url:         "http://localhost:3000",
				Icon:        "/code.svg",
			},
			{
				Slug:        "code-server-2",
				DisplayName: "code-server-2",
				Command:     "some-command",
				Url:         "http://localhost:3000",
				Icon:        "/code.svg",
				Healthcheck: &proto.Healthcheck{
					Url:       "http://localhost:3000",
					Interval:  5,
					Threshold: 6,
				},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "some",
							Type: "example",
							Agents: []*proto.Agent{{
								Id:   "something",
								Auth: &proto.Agent_Token{},
								Apps: apps,
							}},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
		agent := workspace.LatestBuild.Resources[0].Agents[0]
		require.Len(t, agent.Apps, 2)
		got := agent.Apps[0]
		app := apps[0]
		require.EqualValues(t, app.Command, got.Command)
		require.EqualValues(t, app.Icon, got.Icon)
		require.EqualValues(t, app.DisplayName, got.DisplayName)
		require.EqualValues(t, codersdk.WorkspaceAppHealthDisabled, got.Health)
		require.EqualValues(t, "", got.Healthcheck.URL)
		require.EqualValues(t, 0, got.Healthcheck.Interval)
		require.EqualValues(t, 0, got.Healthcheck.Threshold)
		got = agent.Apps[1]
		app = apps[1]
		require.EqualValues(t, app.Command, got.Command)
		require.EqualValues(t, app.Icon, got.Icon)
		require.EqualValues(t, app.DisplayName, got.DisplayName)
		require.EqualValues(t, codersdk.WorkspaceAppHealthInitializing, got.Health)
		require.EqualValues(t, app.Healthcheck.Url, got.Healthcheck.URL)
		require.EqualValues(t, app.Healthcheck.Interval, got.Healthcheck.Interval)
		require.EqualValues(t, app.Healthcheck.Threshold, got.Healthcheck.Threshold)
	})

	t.Run("Apps_DisplayOrder", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		apps := []*proto.App{
			{
				Slug:        "aaa",
				DisplayName: "aaa",
			},
			{
				Slug:  "aaa-code-server",
				Order: 4,
			},
			{
				Slug:  "bbb-code-server",
				Order: 3,
			},
			{
				Slug: "bbb",
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "some",
							Type: "example",
							Agents: []*proto.Agent{{
								Id:   "something",
								Auth: &proto.Agent_Token{},
								Apps: apps,
							}},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
		agent := workspace.LatestBuild.Resources[0].Agents[0]
		require.Len(t, agent.Apps, 4)
		require.Equal(t, "bbb", agent.Apps[0].Slug)             // empty-display-name < "aaa"
		require.Equal(t, "aaa", agent.Apps[1].Slug)             // no order < any order
		require.Equal(t, "bbb-code-server", agent.Apps[2].Slug) // order = 3 < order = 4
		require.Equal(t, "aaa-code-server", agent.Apps[3].Slug)
	})

	t.Run("Metadata", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "some",
							Type: "example",
							Agents: []*proto.Agent{{
								Id:   "something",
								Auth: &proto.Agent_Token{},
							}},
							Metadata: []*proto.Resource_Metadata{{
								Key:   "foo",
								Value: "bar",
							}, {
								Key:    "null",
								IsNull: true,
							}, {
								Key: "empty",
							}, {
								Key:       "secret",
								Value:     "squirrel",
								Sensitive: true,
							}},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		metadata := workspace.LatestBuild.Resources[0].Metadata
		require.Equal(t, []codersdk.WorkspaceResourceMetadata{{
			Key:   "foo",
			Value: "bar",
		}, {
			Key: "empty",
		}, {
			Key:       "secret",
			Value:     "squirrel",
			Sensitive: true,
		}}, metadata)
	})
}

func TestWorkspaceWithRichParameters(t *testing.T) {
	t.Parallel()

	const (
		firstParameterName        = "first_parameter"
		firstParameterType        = "string"
		firstParameterDescription = "This is _first_ *parameter*"
		firstParameterValue       = "1"

		secondParameterName                = "second_parameter"
		secondParameterDisplayName         = "Second Parameter"
		secondParameterType                = "number"
		secondParameterDescription         = "_This_ is second *parameter*"
		secondParameterValue               = "2"
		secondParameterValidationMonotonic = codersdk.MonotonicOrderIncreasing
	)

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Parameters: []*proto.RichParameter{
							{
								Name:        firstParameterName,
								Type:        firstParameterType,
								Description: firstParameterDescription,
							},
							{
								Name:                secondParameterName,
								DisplayName:         secondParameterDisplayName,
								Type:                secondParameterType,
								Description:         secondParameterDescription,
								ValidationMin:       ptr.Ref(int32(1)),
								ValidationMax:       ptr.Ref(int32(3)),
								ValidationMonotonic: string(secondParameterValidationMonotonic),
							},
						},
					},
				},
			},
		},
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	firstParameterDescriptionPlaintext, err := parameter.Plaintext(firstParameterDescription)
	require.NoError(t, err)
	secondParameterDescriptionPlaintext, err := parameter.Plaintext(secondParameterDescription)
	require.NoError(t, err)

	templateRichParameters, err := client.TemplateVersionRichParameters(ctx, version.ID)
	require.NoError(t, err)
	require.Len(t, templateRichParameters, 2)
	require.Equal(t, firstParameterName, templateRichParameters[0].Name)
	require.Equal(t, firstParameterType, templateRichParameters[0].Type)
	require.Equal(t, firstParameterDescription, templateRichParameters[0].Description)
	require.Equal(t, firstParameterDescriptionPlaintext, templateRichParameters[0].DescriptionPlaintext)
	require.Equal(t, codersdk.ValidationMonotonicOrder(""), templateRichParameters[0].ValidationMonotonic) // no validation for string
	require.Equal(t, secondParameterName, templateRichParameters[1].Name)
	require.Equal(t, secondParameterDisplayName, templateRichParameters[1].DisplayName)
	require.Equal(t, secondParameterType, templateRichParameters[1].Type)
	require.Equal(t, secondParameterDescription, templateRichParameters[1].Description)
	require.Equal(t, secondParameterDescriptionPlaintext, templateRichParameters[1].DescriptionPlaintext)
	require.Equal(t, secondParameterValidationMonotonic, templateRichParameters[1].ValidationMonotonic)

	expectedBuildParameters := []codersdk.WorkspaceBuildParameter{
		{Name: firstParameterName, Value: firstParameterValue},
		{Name: secondParameterName, Value: secondParameterValue},
	}

	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.RichParameterValues = expectedBuildParameters
	})

	workspaceBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, workspaceBuild.Status)

	workspaceBuildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceBuild.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, expectedBuildParameters, workspaceBuildParameters)
}

func TestWorkspaceWithOptionalRichParameters(t *testing.T) {
	t.Parallel()

	const (
		firstParameterName         = "first_parameter"
		firstParameterType         = "string"
		firstParameterDescription  = "This is _first_ *parameter*"
		firstParameterDefaultValue = "1"

		secondParameterName        = "second_parameter"
		secondParameterType        = "number"
		secondParameterDescription = "_This_ is second *parameter*"
		secondParameterRequired    = true
		secondParameterValue       = "333"
	)

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Parameters: []*proto.RichParameter{
							{
								Name:         firstParameterName,
								Type:         firstParameterType,
								Description:  firstParameterDescription,
								DefaultValue: firstParameterDefaultValue,
							},
							{
								Name:        secondParameterName,
								Type:        secondParameterType,
								Description: secondParameterDescription,
								Required:    secondParameterRequired,
							},
						},
					},
				},
			},
		},
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	templateRichParameters, err := client.TemplateVersionRichParameters(ctx, version.ID)
	require.NoError(t, err)
	require.Len(t, templateRichParameters, 2)
	require.Equal(t, firstParameterName, templateRichParameters[0].Name)
	require.Equal(t, firstParameterType, templateRichParameters[0].Type)
	require.Equal(t, firstParameterDescription, templateRichParameters[0].Description)
	require.Equal(t, firstParameterDefaultValue, templateRichParameters[0].DefaultValue)
	require.Equal(t, secondParameterName, templateRichParameters[1].Name)
	require.Equal(t, secondParameterType, templateRichParameters[1].Type)
	require.Equal(t, secondParameterDescription, templateRichParameters[1].Description)
	require.Equal(t, secondParameterRequired, templateRichParameters[1].Required)

	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.RichParameterValues = []codersdk.WorkspaceBuildParameter{
			// First parameter is optional, so coder will pick the default value.
			{Name: secondParameterName, Value: secondParameterValue},
		}
	})

	workspaceBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, workspaceBuild.Status)

	workspaceBuildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceBuild.ID)
	require.NoError(t, err)

	expectedBuildParameters := []codersdk.WorkspaceBuildParameter{
		// Coderd inserts the default for the missing parameter
		{Name: firstParameterName, Value: firstParameterDefaultValue},
		{Name: secondParameterName, Value: secondParameterValue},
	}
	require.ElementsMatch(t, expectedBuildParameters, workspaceBuildParameters)
}

func TestWorkspaceWithEphemeralRichParameters(t *testing.T) {
	t.Parallel()

	const (
		firstParameterName         = "first_parameter"
		firstParameterType         = "string"
		firstParameterDescription  = "This is first parameter"
		firstParameterMutable      = true
		firstParameterDefaultValue = "1"
		firstParameterValue        = "i_am_first_parameter"

		ephemeralParameterName         = "second_parameter"
		ephemeralParameterType         = "string"
		ephemeralParameterDescription  = "This is second parameter"
		ephemeralParameterDefaultValue = ""
		ephemeralParameterMutable      = true
		ephemeralParameterValue        = "i_am_ephemeral"
	)

	// Create template version with ephemeral parameter
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Parameters: []*proto.RichParameter{
							{
								Name:         firstParameterName,
								Type:         firstParameterType,
								Description:  firstParameterDescription,
								DefaultValue: firstParameterDefaultValue,
								Mutable:      firstParameterMutable,
							},
							{
								Name:         ephemeralParameterName,
								Type:         ephemeralParameterType,
								Description:  ephemeralParameterDescription,
								DefaultValue: ephemeralParameterDefaultValue,
								Mutable:      ephemeralParameterMutable,
								Ephemeral:    true,
							},
						},
					},
				},
			},
		},
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	// Create workspace with default values
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	workspaceBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, workspaceBuild.Status)

	// Verify workspace build parameters (default values)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	workspaceBuildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceBuild.ID)
	require.NoError(t, err)

	expectedBuildParameters := []codersdk.WorkspaceBuildParameter{
		{Name: firstParameterName, Value: firstParameterDefaultValue},
		{Name: ephemeralParameterName, Value: ephemeralParameterDefaultValue},
	}
	require.ElementsMatch(t, expectedBuildParameters, workspaceBuildParameters)

	// Trigger workspace build job with ephemeral parameter
	workspaceBuild, err = client.CreateWorkspaceBuild(ctx, workspaceBuild.WorkspaceID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStart,
		RichParameterValues: []codersdk.WorkspaceBuildParameter{
			{
				Name:  ephemeralParameterName,
				Value: ephemeralParameterValue,
			},
		},
	})
	require.NoError(t, err)
	workspaceBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspaceBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, workspaceBuild.Status)

	// Verify workspace build parameters (including ephemeral)
	workspaceBuildParameters, err = client.WorkspaceBuildParameters(ctx, workspaceBuild.ID)
	require.NoError(t, err)

	expectedBuildParameters = []codersdk.WorkspaceBuildParameter{
		{Name: firstParameterName, Value: firstParameterDefaultValue},
		{Name: ephemeralParameterName, Value: ephemeralParameterValue},
	}
	require.ElementsMatch(t, expectedBuildParameters, workspaceBuildParameters)

	// Trigger workspace build one more time without the ephemeral parameter
	workspaceBuild, err = client.CreateWorkspaceBuild(ctx, workspaceBuild.WorkspaceID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStart,
		RichParameterValues: []codersdk.WorkspaceBuildParameter{
			{
				Name:  firstParameterName,
				Value: firstParameterValue,
			},
		},
	})
	require.NoError(t, err)
	workspaceBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspaceBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, workspaceBuild.Status)

	// Verify workspace build parameters (ephemeral should be back to default)
	workspaceBuildParameters, err = client.WorkspaceBuildParameters(ctx, workspaceBuild.ID)
	require.NoError(t, err)

	expectedBuildParameters = []codersdk.WorkspaceBuildParameter{
		{Name: firstParameterName, Value: firstParameterValue},
		{Name: ephemeralParameterName, Value: ephemeralParameterDefaultValue},
	}
	require.ElementsMatch(t, expectedBuildParameters, workspaceBuildParameters)
}

func TestWorkspaceDormant(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		var (
			auditRecorder = audit.NewMock()
			client        = coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Auditor:                  auditRecorder,
			})
			user                     = coderdtest.CreateFirstUser(t, client)
			version                  = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_                        = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			timeTilDormantAutoDelete = time.Minute
		)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](timeTilDormantAutoDelete.Milliseconds())
		})
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		lastUsedAt := workspace.LastUsedAt
		auditRecorder.ResetLogs()
		err := client.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)
		require.True(t, auditRecorder.Contains(t, database.AuditLog{
			Action:         database.AuditActionWrite,
			ResourceType:   database.ResourceTypeWorkspace,
			ResourceTarget: workspace.Name,
		}))

		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.NoError(t, err, "fetch provisioned workspace")
		// The template doesn't have a time_til_dormant_autodelete set so this should be nil.
		require.Nil(t, workspace.DeletingAt)
		require.NotNil(t, workspace.DormantAt)
		require.WithinRange(t, *workspace.DormantAt, time.Now().Add(-time.Second*10), time.Now())
		require.Equal(t, lastUsedAt, workspace.LastUsedAt)

		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		lastUsedAt = workspace.LastUsedAt
		err = client.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: false,
		})
		require.NoError(t, err)

		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch provisioned workspace")
		require.Nil(t, workspace.DormantAt)
		// The template doesn't have a time_til_dormant_autodelete  set so this should be nil.
		require.Nil(t, workspace.DeletingAt)
		// The last_used_at should get updated when we activate the workspace.
		require.True(t, workspace.LastUsedAt.After(lastUsedAt))
	})

	t.Run("CannotStart", func(t *testing.T) {
		t.Parallel()
		var (
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template  = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			_         = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		// Should be able to stop a workspace while it is dormant.
		coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

		// Should not be able to start a workspace while it is dormant.
		_, err = client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransition(database.WorkspaceTransitionStart),
		})
		require.Error(t, err)

		err = client.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: false,
		})
		require.NoError(t, err)
		coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStop, database.WorkspaceTransitionStart)
	})
}

func TestWorkspaceFavoriteUnfavorite(t *testing.T) {
	t.Parallel()
	// Given:
	var (
		auditRecorder = audit.NewMock()
		client, db    = coderdtest.NewWithDatabase(t, &coderdtest.Options{
			Auditor: auditRecorder,
		})
		owner                = coderdtest.CreateFirstUser(t, client)
		memberClient, member = coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		// This will be our 'favorite' workspace
		wsb1 = dbfake.WorkspaceBuild(t, db, database.Workspace{OwnerID: member.ID, OrganizationID: owner.OrganizationID}).Do()
		wsb2 = dbfake.WorkspaceBuild(t, db, database.Workspace{OwnerID: owner.UserID, OrganizationID: owner.OrganizationID}).Do()
	)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Initially, workspace should not be favored for member.
	ws, err := memberClient.Workspace(ctx, wsb1.Workspace.ID)
	require.NoError(t, err)
	require.False(t, ws.Favorite)

	// When user favorites workspace
	err = memberClient.FavoriteWorkspace(ctx, wsb1.Workspace.ID)
	require.NoError(t, err)

	// Then it should be favored for them.
	ws, err = memberClient.Workspace(ctx, wsb1.Workspace.ID)
	require.NoError(t, err)
	require.True(t, ws.Favorite)

	// And it should be audited.
	require.True(t, auditRecorder.Contains(t, database.AuditLog{
		Action:         database.AuditActionWrite,
		ResourceType:   database.ResourceTypeWorkspace,
		ResourceTarget: wsb1.Workspace.Name,
		UserID:         member.ID,
	}))
	auditRecorder.ResetLogs()

	// This should not show for the owner.
	ws, err = client.Workspace(ctx, wsb1.Workspace.ID)
	require.NoError(t, err)
	require.False(t, ws.Favorite)

	// When member unfavorites workspace
	err = memberClient.UnfavoriteWorkspace(ctx, wsb1.Workspace.ID)
	require.NoError(t, err)

	// Then it should no longer be favored
	ws, err = memberClient.Workspace(ctx, wsb1.Workspace.ID)
	require.NoError(t, err)
	require.False(t, ws.Favorite, "no longer favorite")

	// And it should show in the audit logs.
	require.True(t, auditRecorder.Contains(t, database.AuditLog{
		Action:         database.AuditActionWrite,
		ResourceType:   database.ResourceTypeWorkspace,
		ResourceTarget: wsb1.Workspace.Name,
		UserID:         member.ID,
	}))

	// Users without write access to the workspace should not be able to perform the above.
	err = memberClient.FavoriteWorkspace(ctx, wsb2.Workspace.ID)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	err = memberClient.UnfavoriteWorkspace(ctx, wsb2.Workspace.ID)
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

	// You should not be able to favorite any workspace you do not own, even if you are the owner.
	err = client.FavoriteWorkspace(ctx, wsb1.Workspace.ID)
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

	err = client.UnfavoriteWorkspace(ctx, wsb1.Workspace.ID)
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
}
