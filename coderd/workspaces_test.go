package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coder/coder/coderd/rbac"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		_, err := client.Workspace(context.Background(), workspace.ID)
		require.NoError(t, err)
	})

	t.Run("Deleted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		// Getting with deleted=true should fail.
		_, err := client.DeletedWorkspace(context.Background(), workspace.ID)
		require.Error(t, err)
		require.ErrorContains(t, err, "400") // bad request

		// Delete the workspace
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err, "delete the workspace")
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		// Getting with deleted=true should work.
		workspaceNew, err := client.DeletedWorkspace(context.Background(), workspace.ID)
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)

		// Getting with deleted=false should not work.
		_, err = client.Workspace(context.Background(), workspace.ID)
		require.Error(t, err)
		require.ErrorContains(t, err, "410") // gone
	})
}

func TestAdminViewAllWorkspaces(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	_, err := client.Workspace(context.Background(), workspace.ID)
	require.NoError(t, err)

	otherOrg, err := client.CreateOrganization(context.Background(), codersdk.Me, codersdk.CreateOrganizationRequest{
		Name: "default-test",
	})
	require.NoError(t, err, "create other org")

	// This other user is not in the first user's org. Since other is an admin, they can
	// still see the "first" user's workspace.
	other := coderdtest.CreateAnotherUser(t, client, otherOrg.ID, rbac.RoleAdmin(), rbac.RoleMember())
	otherWorkspaces, err := other.Workspaces(context.Background(), codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(other) fetch workspaces")

	firstWorkspaces, err := other.Workspaces(context.Background(), codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(first) fetch workspaces")

	require.ElementsMatch(t, otherWorkspaces, firstWorkspaces)
}

func TestPostWorkspacesByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("InvalidTemplate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.CreateWorkspaceRequest{
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

		other := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleMember(), rbac.RoleAdmin())
		org, err := other.CreateOrganization(context.Background(), codersdk.Me, codersdk.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)
		version := coderdtest.CreateTemplateVersion(t, other, org.ID, nil)
		template := coderdtest.CreateTemplate(t, other, org.ID, version.ID)

		_, err = client.CreateWorkspace(context.Background(), first.OrganizationID, codersdk.CreateWorkspaceRequest{
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
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.CreateWorkspaceRequest{
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
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	})
}

func TestWorkspacesByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.WorkspacesByOrganization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		workspaces, err := client.WorkspacesByOrganization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})
}

func TestWorkspacesByOwner(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.WorkspacesByOwner(context.Background(), user.OrganizationID, codersdk.Me)
		require.NoError(t, err)
	})

	t.Run("ListMine", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		me, err := client.User(context.Background(), codersdk.Me)
		require.NoError(t, err)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		// Create noise workspace that should be filtered out
		other := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_ = coderdtest.CreateWorkspace(t, other, user.OrganizationID, template.ID)

		// Use a username
		workspaces, err := client.Workspaces(context.Background(), codersdk.WorkspaceFilter{
			OrganizationID: user.OrganizationID,
			Owner:          me.Username,
		})
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})
}

func TestWorkspaceByOwnerAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.WorkspaceByOwnerAndName(context.Background(), user.OrganizationID, codersdk.Me, "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.WorkspaceByOwnerAndName(context.Background(), user.OrganizationID, codersdk.Me, workspace.Name)
		require.NoError(t, err)
	})
}

func TestPostWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("NoTemplateVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
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
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Provision: []*proto.Provision_Response{{}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_, err := client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})

	t.Run("AlreadyActive", func(t *testing.T) {
		t.Parallel()
		client, coderAPI := coderdtest.NewWithAPI(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		closeDaemon := coderdtest.NewProvisionerDaemon(t, coderAPI)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		// Close here so workspace build doesn't process!
		closeDaemon.Close()
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
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
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.BuildNumber+1, build.BuildNumber)
	})

	t.Run("WithState", func(t *testing.T) {
		t.Parallel()
		client, coderAPI := coderdtest.NewWithAPI(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		closeDaemon := coderdtest.NewProvisionerDaemon(t, coderAPI)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		_ = closeDaemon.Close()
		wantState := []byte("something")
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
			ProvisionerState:  wantState,
		})
		require.NoError(t, err)
		gotState, err := client.WorkspaceBuildState(context.Background(), build.ID)
		require.NoError(t, err)
		require.Equal(t, wantState, gotState)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.BuildNumber+1, build.BuildNumber)
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		workspaces, err := client.WorkspacesByOwner(context.Background(), user.OrganizationID, user.UserID.String())
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})
}

func TestWorkspaceBuildByName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.WorkspaceBuildByName(context.Background(), workspace.ID, "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build, err := client.WorkspaceBuild(context.Background(), workspace.LatestBuild.ID)
		require.NoError(t, err)
		_, err = client.WorkspaceBuildByName(context.Background(), workspace.ID, build.Name)
		require.NoError(t, err)
	})
}

func TestWorkspaceUpdateAutostart(t *testing.T) {
	t.Parallel()
	var dublinLoc = mustLocation(t, "Europe/Dublin")

	testCases := []struct {
		name             string
		schedule         string
		expectedError    string
		at               time.Time
		expectedNext     time.Time
		expectedInterval time.Duration
	}{
		{
			name:          "disable autostart",
			schedule:      "",
			expectedError: "",
		},
		{
			name:             "friday to monday",
			schedule:         "CRON_TZ=Europe/Dublin 30 9 * * 1-5",
			expectedError:    "",
			at:               time.Date(2022, 5, 6, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 5, 9, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 71*time.Hour + 59*time.Minute,
		},
		{
			name:             "monday to tuesday",
			schedule:         "CRON_TZ=Europe/Dublin 30 9 * * 1-5",
			expectedError:    "",
			at:               time.Date(2022, 5, 9, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 5, 10, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 23*time.Hour + 59*time.Minute,
		},
		{
			// DST in Ireland began on Mar 27 in 2022 at 0100. Forward 1 hour.
			name:             "DST start",
			schedule:         "CRON_TZ=Europe/Dublin 30 9 * * *",
			expectedError:    "",
			at:               time.Date(2022, 3, 26, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 3, 27, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 22*time.Hour + 59*time.Minute,
		},
		{
			// DST in Ireland ends on Oct 30 in 2022 at 0200. Back 1 hour.
			name:             "DST end",
			schedule:         "CRON_TZ=Europe/Dublin 30 9 * * *",
			expectedError:    "",
			at:               time.Date(2022, 10, 29, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 10, 30, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 24*time.Hour + 59*time.Minute,
		},
		{
			name:          "invalid location",
			schedule:      "CRON_TZ=Imaginary/Place 30 9 * * 1-5",
			expectedError: "status code 500: invalid autostart schedule: parse schedule: provided bad location Imaginary/Place: unknown time zone Imaginary/Place",
		},
		{
			name:          "invalid schedule",
			schedule:      "asdf asdf asdf ",
			expectedError: `status code 500: invalid autostart schedule: validate weekly schedule: expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix`,
		},
		{
			name:          "only 3 values",
			schedule:      "CRON_TZ=Europe/Dublin 30 9 *",
			expectedError: `status code 500: invalid autostart schedule: validate weekly schedule: expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var (
				ctx       = context.Background()
				client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
				user      = coderdtest.CreateFirstUser(t, client)
				version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
				_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
					cwr.AutostartSchedule = nil
					cwr.TTL = nil
				})
			)

			// ensure test invariant: new workspaces have no autostart schedule.
			require.Empty(t, workspace.AutostartSchedule, "expected newly-minted workspace to have no autostart schedule")

			err := client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: testCase.schedule,
			})

			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError, "unexpected error when setting workspace autostart schedule")
				return
			}

			require.NoError(t, err, "expected no error setting workspace autostart schedule")

			updated, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err, "fetch updated workspace")

			require.Equal(t, testCase.schedule, updated.AutostartSchedule, "expected autostart schedule to equal requested")

			if testCase.schedule == "" {
				return
			}
			sched, err := schedule.Weekly(updated.AutostartSchedule)
			require.NoError(t, err, "parse returned schedule")

			next := sched.Next(testCase.at)
			require.Equal(t, testCase.expectedNext, next, "unexpected next scheduled autostart time")
			interval := next.Sub(testCase.at)
			require.Equal(t, testCase.expectedInterval, interval, "unexpected interval")
		})
	}

	t.Run("NotFound", func(t *testing.T) {
		var (
			ctx    = context.Background()
			client = coderdtest.New(t, nil)
			_      = coderdtest.CreateFirstUser(t, client)
			wsid   = uuid.New()
			req    = codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: "9 30 1-5",
			}
		)

		err := client.UpdateWorkspaceAutostart(ctx, wsid, req)
		require.IsType(t, err, &codersdk.Error{}, "expected codersdk.Error")
		coderSDKErr, _ := err.(*codersdk.Error) //nolint:errorlint
		require.Equal(t, coderSDKErr.StatusCode(), 404, "expected status code 404")
		require.Equal(t, fmt.Sprintf("workspace %q does not exist", wsid), coderSDKErr.Message, "unexpected response code")
	})
}

func TestWorkspaceUpdateAutostop(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		ttl           *time.Duration
		expectedError string
	}{
		{
			name:          "disable ttl",
			ttl:           nil,
			expectedError: "",
		},
		{
			name:          "enable ttl",
			ttl:           ptr(time.Hour),
			expectedError: "",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var (
				ctx       = context.Background()
				client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
				user      = coderdtest.CreateFirstUser(t, client)
				version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
				_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
					cwr.AutostartSchedule = nil
					cwr.TTL = nil
				})
			)

			// ensure test invariant: new workspaces have no autostop schedule.
			require.Nil(t, workspace.TTL, "expected newly-minted workspace to have no TTL")

			err := client.UpdateWorkspaceTTL(ctx, workspace.ID, codersdk.UpdateWorkspaceTTLRequest{
				TTL: testCase.ttl,
			})

			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError, "unexpected error when setting workspace autostop schedule")
				return
			}

			require.NoError(t, err, "expected no error setting workspace autostop schedule")

			updated, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err, "fetch updated workspace")

			require.Equal(t, testCase.ttl, updated.TTL, "expected autostop ttl to equal requested")
		})
	}

	t.Run("NotFound", func(t *testing.T) {
		var (
			ctx    = context.Background()
			client = coderdtest.New(t, nil)
			_      = coderdtest.CreateFirstUser(t, client)
			wsid   = uuid.New()
			req    = codersdk.UpdateWorkspaceTTLRequest{
				TTL: ptr(time.Hour),
			}
		)

		err := client.UpdateWorkspaceTTL(ctx, wsid, req)
		require.IsType(t, err, &codersdk.Error{}, "expected codersdk.Error")
		coderSDKErr, _ := err.(*codersdk.Error) //nolint:errorlint
		require.Equal(t, coderSDKErr.StatusCode(), 404, "expected status code 404")
		require.Equal(t, fmt.Sprintf("workspace %q does not exist", wsid), coderSDKErr.Message, "unexpected response code")
	})
}

func TestWorkspaceWatcher(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	w, err := client.Workspace(context.Background(), workspace.ID)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wc, err := client.WatchWorkspace(ctx, w.ID)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, more := <-wc
		require.True(t, more)
	}
	cancel()
	require.EqualValues(t, codersdk.Workspace{}, <-wc)
}

func mustLocation(t *testing.T, location string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(location)
	if err != nil {
		t.Errorf("failed to load location %s: %s", location, err.Error())
	}

	return loc
}

func ptr[T any](x T) *T {
	return &x
}
