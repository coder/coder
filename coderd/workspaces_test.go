package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		ws, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Equal(t, user.UserID, ws.LatestBuild.InitiatorID)
		require.Equal(t, codersdk.BuildReasonInitiator, ws.LatestBuild.Reason)
	})

	t.Run("Deleted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

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
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

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
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		ws1 := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		ws2 := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, ws1.LatestBuild.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, ws2.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		want := ws1.Name + "-test"
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

	t.Run("TemplateProperties", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		const templateIcon = "/img/icon.svg"
		const templateDisplayName = "This is template"
		var templateAllowUserCancelWorkspaceJobs = false
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
}

func TestAdminViewAllWorkspaces(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

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
	otherOwner := coderdtest.CreateAnotherUser(t, client, otherOrg.ID, rbac.RoleOwner())
	otherWorkspaces, err := otherOwner.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(other) fetch workspaces")

	firstWorkspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(first) fetch workspaces")

	require.ElementsMatch(t, otherWorkspaces.Workspaces, firstWorkspaces.Workspaces)
	require.Equal(t, len(firstWorkspaces.Workspaces), 1, "should be 1 workspace present")

	memberView := coderdtest.CreateAnotherUser(t, client, otherOrg.ID)
	memberViewWorkspaces, err := memberView.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(member) fetch workspaces")
	require.Equal(t, 0, len(memberViewWorkspaces.Workspaces), "member in other org should see 0 workspaces")
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

		other := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleMember(), rbac.RoleOwner())

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
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
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

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		require.Len(t, auditor.AuditLogs, 4)
		assert.Equal(t, database.AuditActionCreate, auditor.AuditLogs[3].Action)
	})

	t.Run("CreateWithDeletedTemplate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
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
		// Given: the template has no max TTL set
		require.Zero(t, template.DefaultTTLMillis)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		// When: we create a workspace with autostop not enabled
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = ptr.Ref(int64(0))
		})
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = nil // ensure that no default TTL is set
		})
		// TTL should be set by the template
		require.Equal(t, template.DefaultTTLMillis, templateTTL)
		require.Equal(t, template.DefaultTTLMillis, template.DefaultTTLMillis, workspace.TTLMillis)
	})

	t.Run("InvalidTTL", func(t *testing.T) {
		t.Parallel()
		t.Run("BelowMin", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

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
			require.Equal(t, apiErr.Validations[0].Field, "ttl_ms")
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Given:
		// We delete the workspace
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err, "delete the workspace")
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

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
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

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
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		// Then:
		// When we fetch the deleted workspace, we get the most recently deleted one
		workspaceNew, err = client.WorkspaceByOwnerAndName(ctx, workspace.OwnerName, workspace.Name, codersdk.WorkspaceOptions{IncludeDeleted: true})
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)
	})
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
		userClient := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleOwner())
		user, err := userClient.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch me")

		if i%3 == 0 {
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

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
		latest := coderdtest.AwaitWorkspaceBuildJob(t, client, w.Workspace.LatestBuild.ID)
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

	t.Run("Name", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
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
	t.Run("Template", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		template2 := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace1 := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		workspace2 := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		// wait for workspaces to be "running"
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace1.LatestBuild.ID)
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace2.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// filter finds both running workspaces
		ws1, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, ws1.Workspaces, 2)

		// stop workspace1
		build1 := coderdtest.CreateWorkspaceBuild(t, client, workspace1, database.WorkspaceTransitionStop)
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, build1.ID)

		// filter finds one running workspace
		ws2, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Status: "running",
		})
		require.NoError(t, err)
		require.Len(t, ws2.Workspaces, 1)
		require.Equal(t, workspace2.ID, ws2.Workspaces[0].ID)

		// stop workspace2
		build2 := coderdtest.CreateWorkspaceBuild(t, client, workspace2, database.WorkspaceTransitionStop)
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, build2.ID)

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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		template2 := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
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
}

func TestOffsetLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

	// empty finds all workspaces
	ws, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, ws.Workspaces, 3)

	// offset 1 finds 2 workspaces
	ws, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
		Offset: 1,
	})
	require.NoError(t, err)
	require.Len(t, ws.Workspaces, 2)

	// offset 1 limit 1 finds 1 workspace
	ws, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
		Offset: 1,
		Limit:  1,
	})
	require.NoError(t, err)
	require.Len(t, ws.Workspaces, 1)

	// offset 3 finds no workspaces
	ws, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
		Offset: 3,
	})
	require.NoError(t, err)
	require.Len(t, ws.Workspaces, 0)
}

func TestPostWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("NoTemplateVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: uuid.New(),
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("TemplateVersionFailedImport", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			ProvisionApply: []*proto.Provision_Response{{}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateWorkspace(ctx, user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})

	t.Run("AlreadyActive", func(t *testing.T) {
		t.Parallel()
		client, closer := coderdtest.NewWithProvisionerCloser(t, nil)
		defer closer.Close()

		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		closer.Close()
		// Close here so workspace build doesn't process!
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("IncrementBuildNumber", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.BuildNumber+1, build.BuildNumber)
	})

	t.Run("WithState", func(t *testing.T) {
		t.Parallel()
		client, closeDaemon := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		wantState := []byte("something")
		_ = closeDaemon.Close()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
			ProvisionerState:  wantState,
		})
		require.NoError(t, err)
		gotState, err := client.WorkspaceBuildState(ctx, build.ID)
		require.NoError(t, err)
		require.Equal(t, wantState, gotState)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.BuildNumber+1, build.BuildNumber)
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner: user.UserID.String(),
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 0)
	})
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
				_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
					cwr.AutostartSchedule = nil
					cwr.TTLMillis = nil
				})
			)

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

			sched, err := schedule.Weekly(*updated.AutostartSchedule)
			require.NoError(t, err, "parse returned schedule")

			next := sched.Next(testCase.at)
			require.Equal(t, testCase.expectedNext, next, "unexpected next scheduled autostart time")
			interval := next.Sub(testCase.at)
			require.Equal(t, testCase.expectedInterval, interval, "unexpected interval")

			require.Len(t, auditor.AuditLogs, 5)
			assert.Equal(t, database.AuditActionWrite, auditor.AuditLogs[4].Action)
		})
	}

	t.Run("NotFound", func(t *testing.T) {
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
		},
		{
			name:          "update ttl",
			ttlMillis:     ptr.Ref(12 * time.Hour.Milliseconds()),
			expectedError: "",
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
			ttlMillis:     ptr.Ref((24 * 7 * time.Hour).Milliseconds()),
			expectedError: "",
		},
		{
			name:          "above maximum ttl",
			ttlMillis:     ptr.Ref((24*7*time.Hour + time.Minute).Milliseconds()),
			expectedError: "time until shutdown must be less than 7 days",
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
				_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, mutators...)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
					cwr.AutostartSchedule = nil
					cwr.TTLMillis = nil
				})
				_ = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
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

			require.Len(t, auditor.AuditLogs, 5)
			assert.Equal(t, database.AuditActionWrite, auditor.AuditLogs[4].Action)
		})
	}

	t.Run("NotFound", func(t *testing.T) {
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
		_           = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template    = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace   = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = ptr.Ref(ttl.Milliseconds())
		})
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
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

func TestWorkspaceWatcher(t *testing.T) {
	t.Parallel()
	client, closeFunc := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.ProvisionComplete,
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
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
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	wc, err := client.WatchWorkspace(ctx, workspace.ID)
	require.NoError(t, err)

	// Wait events are easier to debug with timestamped logs.
	logger := slogtest.Make(t, nil).Named(t.Name()).Leveled(slog.LevelDebug)
	wait := func(event string) {
		select {
		case <-ctx.Done():
			require.FailNow(t, "timed out waiting for event", event)
		case <-wc:
			logger.Info(ctx, "done waiting for event", slog.F("event", event))
		}
	}

	coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)
	wait("workspace build being created")
	wait("workspace build being acquired")
	wait("workspace build completing")

	// Unfortunately, this will add ~1s to the test due to the granularity
	// of agent timeout seconds. However, if we don't do this we won't know
	// which trigger we received when waiting for connection.
	//
	// Note that the first timeout is from `coderdtest.CreateWorkspace` and
	// the latter is from `coderdtest.CreateWorkspaceBuild`.
	wait("agent timeout after create")
	wait("agent timeout after start")

	agentClient := codersdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: logger.Named("agent"),
	})
	defer func() {
		_ = agentCloser.Close()
	}()

	wait("agent connected")
	agentCloser.Close()
	wait("agent disconnected")

	closeFunc.Close()
	build := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)
	wait("first is for the workspace build itself")
	err = client.CancelWorkspaceBuild(ctx, build.ID)
	require.NoError(t, err)
	wait("second is for the build cancel")

	err = client.UpdateWorkspace(ctx, workspace.ID, codersdk.UpdateWorkspaceRequest{
		Name: "another",
	})
	require.NoError(t, err)
	wait("update workspace name")

	err = client.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
		ID: template.ActiveVersionID,
	})
	require.NoError(t, err)
	wait("update active template version")

	cancel()
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
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

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
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

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

	t.Run("Metadata", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		metadata := workspace.LatestBuild.Resources[0].Metadata
		require.Equal(t, []codersdk.WorkspaceResourceMetadata{{
			Key: "empty",
		}, {
			Key:   "foo",
			Value: "bar",
		}, {
			Key:       "secret",
			Value:     "squirrel",
			Sensitive: true,
		}, {
			Key:   "type",
			Value: "example",
		}}, metadata)
	})
}
