package coderd_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestWorkspaceBuild(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	_, err := client.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
	require.NoError(t, err)
}

func TestWorkspaceBuildByBuildNumber(t *testing.T) {
	t.Parallel()
	t.Run("Successful", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch me")
		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		_, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
			ctx,
			user.Username,
			workspace.Name,
			strconv.FormatInt(int64(workspace.LatestBuild.BuildNumber), 10),
		)
		require.NoError(t, err)
	})

	t.Run("BuildNumberNotInt", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch me")
		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		_, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
			ctx,
			user.Username,
			workspace.Name,
			"buildNumber",
		)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusBadRequest, apiError.StatusCode())
		require.ErrorContains(t, apiError, "Failed to parse build number as integer.")
	})

	t.Run("WorkspaceNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch me")
		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		_, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
			ctx,
			user.Username,
			"workspaceName",
			strconv.FormatInt(int64(workspace.LatestBuild.BuildNumber), 10),
		)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusNotFound, apiError.StatusCode())
		require.ErrorContains(t, apiError, "Resource not found")
	})

	t.Run("WorkspaceBuildNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch me")
		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		_, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
			ctx,
			user.Username,
			workspace.Name,
			"200",
		)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusNotFound, apiError.StatusCode())
		require.ErrorContains(t, apiError, fmt.Sprintf("Workspace %q Build 200 does not exist.", workspace.Name))
	})
}

func TestWorkspaceBuilds(t *testing.T) {
	t.Parallel()
	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch me")
		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		builds, err := client.WorkspaceBuilds(ctx,
			codersdk.WorkspaceBuildsRequest{WorkspaceID: workspace.ID})
		require.Len(t, builds, 1)
		require.Equal(t, int32(1), builds[0].BuildNumber)
		require.Equal(t, user.Username, builds[0].InitiatorUsername)
		require.NoError(t, err)

		// Test since
		builds, err = client.WorkspaceBuilds(ctx,
			codersdk.WorkspaceBuildsRequest{WorkspaceID: workspace.ID, Since: database.Now().Add(time.Minute)},
		)
		require.NoError(t, err)
		require.Len(t, builds, 0)
		// Should never be nil for API consistency
		require.NotNil(t, builds)

		builds, err = client.WorkspaceBuilds(ctx,
			codersdk.WorkspaceBuildsRequest{WorkspaceID: workspace.ID, Since: database.Now().Add(-time.Hour)},
		)
		require.NoError(t, err)
		require.Len(t, builds, 1)
	})

	t.Run("DeletedInitiator", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		first := coderdtest.CreateFirstUser(t, client)
		second, secondUser := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, "owner")

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace, err := second.CreateWorkspace(ctx, first.OrganizationID, first.UserID.String(), codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "example",
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		err = client.DeleteUser(ctx, secondUser.ID)
		require.NoError(t, err)

		builds, err := client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{WorkspaceID: workspace.ID})
		require.Len(t, builds, 1)
		require.Equal(t, int32(1), builds[0].BuildNumber)
		require.Equal(t, secondUser.Username, builds[0].InitiatorUsername)
		require.NoError(t, err)
	})

	t.Run("PaginateNonExistentRow", func(t *testing.T) {
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

		_, err := client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{
			WorkspaceID: workspace.ID,
			Pagination: codersdk.Pagination{
				AfterID: uuid.New(),
			},
		})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusBadRequest, apiError.StatusCode())
		require.Contains(t, apiError.Message, "does not exist")
	})

	t.Run("PaginateLimitOffset", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		var expectedBuilds []codersdk.WorkspaceBuild
		extraBuilds := 4
		for i := 0; i < extraBuilds; i++ {
			b := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)
			expectedBuilds = append(expectedBuilds, b)
			coderdtest.AwaitWorkspaceBuildJob(t, client, b.ID)
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		pageSize := 3
		firstPage, err := client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{
			WorkspaceID: workspace.ID,
			Pagination:  codersdk.Pagination{Limit: pageSize, Offset: 0},
		})
		require.NoError(t, err)
		require.Len(t, firstPage, pageSize)
		for i := 0; i < pageSize; i++ {
			require.Equal(t, expectedBuilds[extraBuilds-i-1].ID, firstPage[i].ID)
		}
		secondPage, err := client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{
			WorkspaceID: workspace.ID,
			Pagination:  codersdk.Pagination{Limit: pageSize, Offset: pageSize},
		})
		require.NoError(t, err)
		require.Len(t, secondPage, 2)
		require.Equal(t, expectedBuilds[0].ID, secondPage[0].ID)
		require.Equal(t, workspace.LatestBuild.ID, secondPage[1].ID) // build created while creating workspace
	})
}

func TestWorkspaceBuildsProvisionerState(t *testing.T) {
	t.Parallel()

	t.Run("Permissions", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionDelete,
			ProvisionerState:  []byte(" "),
		})
		require.Nil(t, err)

		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		// A regular user on the very same template must not be able to modify the
		// state.
		regularUser, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		workspace = coderdtest.CreateWorkspace(t, regularUser, first.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, regularUser, workspace.LatestBuild.ID)

		_, err = regularUser.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        workspace.LatestBuild.Transition,
			ProvisionerState:  []byte(" "),
		})
		require.Error(t, err)

		var cerr *codersdk.Error
		require.True(t, errors.As(err, &cerr))

		code := cerr.StatusCode()
		require.Equal(t, http.StatusForbidden, code, "unexpected status %s", http.StatusText(code))
	})

	t.Run("Orphan", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		// Providing both state and orphan fails.
		_, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionDelete,
			ProvisionerState:  []byte(" "),
			Orphan:            true,
		})
		require.Error(t, err)
		cerr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())

		// Regular orphan operation succeeds.
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionDelete,
			Orphan:            true,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		_, err = client.Workspace(ctx, workspace.ID)
		require.Error(t, err)
		require.Equal(t, http.StatusGone, coderdtest.SDKError(t, err).StatusCode())
	})
}

func TestPatchCancelWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("User is allowed to cancel", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{},
				},
			}},
			ProvisionPlan: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		var build codersdk.WorkspaceBuild

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		require.Eventually(t, func() bool {
			var err error
			build, err = client.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
			return assert.NoError(t, err) && build.Job.Status == codersdk.ProvisionerJobRunning
		}, testutil.WaitShort, testutil.IntervalFast)
		err := client.CancelWorkspaceBuild(ctx, build.ID)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			var err error
			build, err = client.WorkspaceBuild(ctx, build.ID)
			return assert.NoError(t, err) &&
				// The job will never actually cancel successfully because it will never send a
				// provision complete response.
				assert.Empty(t, build.Job.Error) &&
				build.Job.Status == codersdk.ProvisionerJobCanceling
		}, testutil.WaitShort, testutil.IntervalFast)
	})
	t.Run("User is not allowed to cancel", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{},
				},
			}},
			ProvisionPlan: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		workspace := coderdtest.CreateWorkspace(t, userClient, owner.OrganizationID, template.ID)
		var build codersdk.WorkspaceBuild

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		require.Eventually(t, func() bool {
			var err error
			build, err = userClient.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
			return assert.NoError(t, err) && build.Job.Status == codersdk.ProvisionerJobRunning
		}, testutil.WaitShort, testutil.IntervalFast)
		err := userClient.CancelWorkspaceBuild(ctx, build.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})
}

func TestWorkspaceBuildResources(t *testing.T) {
	t.Parallel()
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
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
						}, {
							Name: "another",
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
		require.NotNil(t, workspace.LatestBuild.Resources)
		require.Len(t, workspace.LatestBuild.Resources, 2)
		require.Equal(t, "some", workspace.LatestBuild.Resources[0].Name)
		require.Equal(t, "example", workspace.LatestBuild.Resources[1].Type)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
	})
}

func TestWorkspaceBuildLogs(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Log{
				Log: &proto.Log{
					Level:  proto.LogLevel_INFO,
					Output: "example",
				},
			},
		}, {
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "some",
						Type: "example",
						Agents: []*proto.Agent{{
							Id:   "something",
							Auth: &proto.Agent_Token{},
						}},
					}, {
						Name: "another",
						Type: "example",
					}},
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	logs, closer, err := client.WorkspaceBuildLogsAfter(ctx, workspace.LatestBuild.ID, 0)
	require.NoError(t, err)
	defer closer.Close()
	for {
		log, ok := <-logs
		if !ok {
			break
		}
		if log.Output == "example" {
			return
		}
	}
	require.Fail(t, "example message never happened")
}

func TestWorkspaceBuildState(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	wantState := []byte("some kinda state")
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.ProvisionComplete,
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					State: wantState,
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

	gotState, err := client.WorkspaceBuildState(ctx, workspace.LatestBuild.ID)
	require.NoError(t, err)
	require.Equal(t, wantState, gotState)
}

func TestWorkspaceBuildStatus(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMock()
	numLogs := len(auditor.AuditLogs())
	client, closeDaemon, api := coderdtest.NewWithAPI(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
	user := coderdtest.CreateFirstUser(t, client)
	numLogs++ // add an audit log for login
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	numLogs++ // add an audit log for template version creation
	numLogs++ // add an audit log for template version update

	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	closeDaemon.Close()
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	numLogs++ // add an audit log for template creation

	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	numLogs++ // add an audit log for workspace creation

	// initial returned state is "pending"
	require.EqualValues(t, codersdk.WorkspaceStatusPending, workspace.LatestBuild.Status)

	closeDaemon = coderdtest.NewProvisionerDaemon(t, api)
	// after successful build is "running"
	_ = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	workspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceStatusRunning, workspace.LatestBuild.Status)

	numLogs++ // add an audit log for workspace_build starting

	// after successful stop is "stopped"
	build := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStop)
	_ = coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)
	workspace, err = client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceStatusStopped, workspace.LatestBuild.Status)

	// assert an audit log has been created for workspace stopping
	numLogs++ // add an audit log for workspace_build stop
	require.Len(t, auditor.AuditLogs(), numLogs)
	require.Equal(t, database.AuditActionStop, auditor.AuditLogs()[numLogs-1].Action)

	_ = closeDaemon.Close()
	// after successful cancel is "canceled"
	build = coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)
	err = client.CancelWorkspaceBuild(ctx, build.ID)
	require.NoError(t, err)

	workspace, err = client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceStatusCanceled, workspace.LatestBuild.Status)

	_ = coderdtest.NewProvisionerDaemon(t, api)
	// after successful delete is "deleted"
	build = coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionDelete)
	_ = coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)
	workspace, err = client.DeletedWorkspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceStatusDeleted, workspace.LatestBuild.Status)
}

func TestMigrateLegacyToRichParameters(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// 1. Prepare a template with legacy parameters.
	templateVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: []*proto.Parse_Response{{
			Type: &proto.Parse_Response_Complete{
				Complete: &proto.Parse_Complete{
					ParameterSchemas: []*proto.ParameterSchema{
						{
							AllowOverrideSource: true,
							Name:                "example",
							Description:         "description 1",
							DefaultSource: &proto.ParameterSource{
								Scheme: proto.ParameterSource_DATA,
								Value:  "tomato",
							},
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							},
						},
					},
				},
			},
		}},
		ProvisionApply: echo.ProvisionComplete,
		ProvisionPlan:  echo.ProvisionComplete,
	})
	coderdtest.AwaitTemplateVersionJob(t, client, templateVersion.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, templateVersion.ID)

	// Create a workspace
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.ParameterValues = []codersdk.CreateParameterRequest{
			{
				Name:              "example",
				SourceValue:       "carrot",
				SourceScheme:      codersdk.ParameterSourceSchemeData,
				DestinationScheme: codersdk.ParameterDestinationSchemeEnvironmentVariable,
			},
		}
	})
	workspaceBuild := coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, workspaceBuild.Status)

	// 2. Upload the template with legacy and rich parameters.
	templateWithParameters := &echo.Responses{
		Parse: []*proto.Parse_Response{{
			Type: &proto.Parse_Response_Complete{
				Complete: &proto.Parse_Complete{
					ParameterSchemas: []*proto.ParameterSchema{
						{
							AllowOverrideSource: true,
							Name:                "example",
							Description:         "description 1",
							DefaultSource: &proto.ParameterSource{
								Scheme: proto.ParameterSource_DATA,
								Value:  "tomato",
							},
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							},
						},
					},
				},
			},
		}},
		ProvisionPlan: []*proto.Provision_Response{
			{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Parameters: []*proto.RichParameter{
							{
								Name:               "new_example",
								Type:               "string",
								Mutable:            true,
								Required:           true,
								LegacyVariableName: "example",
							},
						},
					},
				},
			},
		},
		ProvisionApply: echo.ProvisionComplete,
	}
	templateVersion = coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, templateWithParameters, template.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, templateVersion.ID)

	// Check if rich parameters are expected
	richParameters, err := client.TemplateVersionRichParameters(ctx, templateVersion.ID)
	require.NoError(t, err)
	require.Len(t, richParameters, 1)
	require.Equal(t, "new_example", richParameters[0].Name)

	// Update workspace to use rich parameters and template variables
	workspaceBuild, err = client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		TemplateVersionID: templateVersion.ID,
		Transition:        codersdk.WorkspaceTransitionStart,
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		workspaceBuild = coderdtest.AwaitWorkspaceBuildJob(t, client, workspaceBuild.ID)
		return codersdk.WorkspaceStatusRunning == workspaceBuild.Status
	}, testutil.WaitLong, testutil.IntervalFast)

	// Check if variable value has been imported
	buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceBuild.ID)
	require.NoError(t, err)
	require.Len(t, buildParameters, 1)
	require.Equal(t, "carrot", buildParameters[0].Value)

	// 3. Upload the template with rich parameters only
	templateWithParameters = &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Provision_Response{
			{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Parameters: []*proto.RichParameter{
							{
								Name:               "new_example",
								Type:               "string",
								Mutable:            true,
								Required:           true,
								LegacyVariableName: "example",
							},
						},
					},
				},
			},
		},
		ProvisionApply: echo.ProvisionComplete,
	}
	templateVersion = coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, templateWithParameters, template.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, templateVersion.ID)

	// Check if rich parameters are expected
	richParameters, err = client.TemplateVersionRichParameters(ctx, templateVersion.ID)
	require.NoError(t, err)
	require.Len(t, richParameters, 1)
	require.Equal(t, "new_example", richParameters[0].Name)

	// Update workspace to use rich parameters and template variables
	workspaceBuild, err = client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		TemplateVersionID: templateVersion.ID,
		Transition:        codersdk.WorkspaceTransitionStart,
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		workspaceBuild = coderdtest.AwaitWorkspaceBuildJob(t, client, workspaceBuild.ID)
		return codersdk.WorkspaceStatusRunning == workspaceBuild.Status
	}, testutil.WaitLong, testutil.IntervalFast)

	// Check if build parameters have been pulled from last build
	buildParameters, err = client.WorkspaceBuildParameters(ctx, workspaceBuild.ID)
	require.NoError(t, err)
	require.Len(t, buildParameters, 1)
	require.Equal(t, "carrot", buildParameters[0].Value)
}

func TestWorkspaceBuildDebugMode(t *testing.T) {
	t.Parallel()

	t.Run("AsRegularUser", func(t *testing.T) {
		t.Parallel()

		// Create users
		templateAuthorClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		templateAuthor := coderdtest.CreateFirstUser(t, templateAuthorClient)
		regularUserClient, _ := coderdtest.CreateAnotherUser(t, templateAuthorClient, templateAuthor.OrganizationID)

		// Template owner: create a template
		version := coderdtest.CreateTemplateVersion(t, templateAuthorClient, templateAuthor.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, templateAuthorClient, templateAuthor.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, templateAuthorClient, version.ID)

		// Regular user: create a workspace
		workspace := coderdtest.CreateWorkspace(t, regularUserClient, templateAuthor.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, regularUserClient, workspace.LatestBuild.ID)

		// Regular user: try to start a workspace build in debug mode
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := regularUserClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
			LogLevel:          "debug",
		})

		// Regular user: expect an error
		require.NotNil(t, err)
		var sdkError *codersdk.Error
		isSdkError := xerrors.As(err, &sdkError)
		require.True(t, isSdkError)
		require.Contains(t, sdkError.Message, "Workspace builds with a custom log level are restricted to template authors only.")
	})
	t.Run("AsTemplateAuthor", func(t *testing.T) {
		t.Parallel()

		// Create users
		adminClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		admin := coderdtest.CreateFirstUser(t, adminClient)
		templateAdminClient, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID, rbac.RoleTemplateAdmin())

		// Interact as template admin
		echoResponses := &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.ProvisionComplete,
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_DEBUG,
						Output: "want-it",
					},
				},
			}, {
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_TRACE,
						Output: "dont-want-it",
					},
				},
			}, {
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_DEBUG,
						Output: "done",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		}
		version := coderdtest.CreateTemplateVersion(t, templateAdminClient, admin.OrganizationID, echoResponses)
		template := coderdtest.CreateTemplate(t, templateAdminClient, admin.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, templateAdminClient, version.ID)

		// Create workspace
		workspace := coderdtest.CreateWorkspace(t, templateAdminClient, admin.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, templateAdminClient, workspace.LatestBuild.ID)

		// Create workspace build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := templateAdminClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
			ProvisionerState:  []byte(" "),
			LogLevel:          "debug",
		})
		require.Nil(t, err)

		build = coderdtest.AwaitWorkspaceBuildJob(t, templateAdminClient, build.ID)

		// Watch for incoming logs
		logs, closer, err := templateAdminClient.WorkspaceBuildLogsAfter(ctx, build.ID, 0)
		require.NoError(t, err)
		defer closer.Close()

		var logsProcessed int

	processingLogs:
		for {
			select {
			case <-ctx.Done():
				require.Fail(t, "timeout occurred while processing logs")
				return
			case log, ok := <-logs:
				if !ok {
					break processingLogs
				}

				logsProcessed++

				require.NotEqual(t, "dont-want-it", log.Output, "unexpected log message", "%s log message shouldn't be logged: %s")

				if log.Output == "done" {
					break processingLogs
				}
			}
		}

		require.Len(t, echoResponses.ProvisionApply, logsProcessed)
	})
}
