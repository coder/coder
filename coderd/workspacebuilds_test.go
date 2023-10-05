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

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceBuild(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		builds, err := client.WorkspaceBuilds(ctx,
			codersdk.WorkspaceBuildsRequest{WorkspaceID: workspace.ID})
		require.Len(t, builds, 1)
		require.Equal(t, int32(1), builds[0].BuildNumber)
		require.Equal(t, user.Username, builds[0].InitiatorUsername)
		require.NoError(t, err)

		// Test since
		builds, err = client.WorkspaceBuilds(ctx,
			codersdk.WorkspaceBuildsRequest{WorkspaceID: workspace.ID, Since: dbtime.Now().Add(time.Minute)},
		)
		require.NoError(t, err)
		require.Len(t, builds, 0)
		// Should never be nil for API consistency
		require.NotNil(t, builds)

		builds, err = client.WorkspaceBuilds(ctx,
			codersdk.WorkspaceBuildsRequest{WorkspaceID: workspace.ID, Since: dbtime.Now().Add(-time.Hour)},
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace, err := second.CreateWorkspace(ctx, first.OrganizationID, first.UserID.String(), codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "example",
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		var expectedBuilds []codersdk.WorkspaceBuild
		extraBuilds := 4
		for i := 0; i < extraBuilds; i++ {
			b := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)
			expectedBuilds = append(expectedBuilds, b)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, b.ID)
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionDelete,
			ProvisionerState:  []byte(" "),
		})
		require.Nil(t, err)

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// A regular user on the very same template must not be able to modify the
		// state.
		regularUser, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		workspace = coderdtest.CreateWorkspace(t, regularUser, first.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, regularUser, workspace.LatestBuild.ID)

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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		workspace := coderdtest.CreateWorkspace(t, client, first.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

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
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

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
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{},
				},
			}},
			ProvisionPlan: echo.PlanComplete,
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
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
			// job gets marked Failed when there is an Error; in practice we never get to Status = Canceled
			// because provisioners report an Error when canceled. We check the Error string to ensure we don't mask
			// other errors in this test.
			return assert.NoError(t, err) &&
				build.Job.Error == "canceled" &&
				build.Job.Status == codersdk.ProvisionerJobFailed
		}, testutil.WaitShort, testutil.IntervalFast)
	})
	t.Run("User is not allowed to cancel", func(t *testing.T) {
		t.Parallel()

		// need to include our own logger because the provisioner (rightly) drops error logs when we shut down the
		// test with a build in progress.
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Logger: &logger})
		owner := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{},
				},
			}},
			ProvisionPlan: echo.PlanComplete,
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
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
						}, {
							Name: "another",
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
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Log{
				Log: &proto.Log{
					Level:  proto.LogLevel_INFO,
					Output: "example",
				},
			},
		}, {
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
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
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
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
		ProvisionPlan: echo.PlanComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					State: wantState,
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

	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	closeDaemon.Close()
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	numLogs++ // add an audit log for template creation

	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	numLogs++ // add an audit log for workspace creation

	// initial returned state is "pending"
	require.EqualValues(t, codersdk.WorkspaceStatusPending, workspace.LatestBuild.Status)

	closeDaemon = coderdtest.NewProvisionerDaemon(t, api)
	// after successful build is "running"
	_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	workspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceStatusRunning, workspace.LatestBuild.Status)

	numLogs++ // add an audit log for workspace_build starting

	// after successful stop is "stopped"
	build := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStop)
	_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
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
	_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
	workspace, err = client.DeletedWorkspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceStatusDeleted, workspace.LatestBuild.Status)
}

func TestWorkspaceBuildDebugMode(t *testing.T) {
	t.Parallel()

	t.Run("DebugModeDisabled", func(t *testing.T) {
		t.Parallel()

		// Create user
		deploymentValues := coderdtest.DeploymentValues(t)
		err := deploymentValues.EnableTerraformDebugMode.Set("false")
		require.NoError(t, err)

		adminClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, DeploymentValues: deploymentValues})
		owner := coderdtest.CreateFirstUser(t, adminClient)

		// Template author: create a template
		version := coderdtest.CreateTemplateVersion(t, adminClient, owner.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, adminClient, owner.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)

		// Template author: create a workspace
		workspace := coderdtest.CreateWorkspace(t, adminClient, owner.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, adminClient, workspace.LatestBuild.ID)

		// Template author: try to start a workspace build in debug mode
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err = adminClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
			LogLevel:          "debug",
		})

		// Template author: expect an error as the debug mode is disabled
		require.NotNil(t, err)
		var sdkError *codersdk.Error
		isSdkError := xerrors.As(err, &sdkError)
		require.True(t, isSdkError)
		require.Contains(t, sdkError.Message, "Terraform debug mode is disabled in the deployment configuration.")
	})
	t.Run("AsRegularUser", func(t *testing.T) {
		t.Parallel()

		// Create users
		deploymentValues := coderdtest.DeploymentValues(t)
		deploymentValues.EnableTerraformDebugMode = true

		templateAuthorClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, DeploymentValues: deploymentValues})
		templateAuthor := coderdtest.CreateFirstUser(t, templateAuthorClient)
		regularUserClient, _ := coderdtest.CreateAnotherUser(t, templateAuthorClient, templateAuthor.OrganizationID)

		// Template owner: create a template
		version := coderdtest.CreateTemplateVersion(t, templateAuthorClient, templateAuthor.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, templateAuthorClient, templateAuthor.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAuthorClient, version.ID)

		// Regular user: create a workspace
		workspace := coderdtest.CreateWorkspace(t, regularUserClient, templateAuthor.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, regularUserClient, workspace.LatestBuild.ID)

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
		require.Contains(t, sdkError.Message, "Workspace builds with a custom log level are restricted to administrators only.")
	})
	t.Run("AsTemplateAuthor", func(t *testing.T) {
		t.Parallel()

		// Create users
		deploymentValues := coderdtest.DeploymentValues(t)
		deploymentValues.EnableTerraformDebugMode = true

		adminClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, DeploymentValues: deploymentValues})
		admin := coderdtest.CreateFirstUser(t, adminClient)
		templateAuthorClient, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID, rbac.RoleTemplateAdmin())

		// Template author: create a template
		version := coderdtest.CreateTemplateVersion(t, templateAuthorClient, admin.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, templateAuthorClient, admin.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAuthorClient, version.ID)

		// Template author: create a workspace
		workspace := coderdtest.CreateWorkspace(t, templateAuthorClient, admin.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAuthorClient, workspace.LatestBuild.ID)

		// Template author: try to start a workspace build in debug mode
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := templateAuthorClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
			LogLevel:          "debug",
		})

		// Template author: expect an error as the debug mode is disabled
		require.NotNil(t, err)
		var sdkError *codersdk.Error
		isSdkError := xerrors.As(err, &sdkError)
		require.True(t, isSdkError)
		require.Contains(t, sdkError.Message, "Workspace builds with a custom log level are restricted to administrators only.")
	})
	t.Run("AsAdmin", func(t *testing.T) {
		t.Parallel()

		// Create users
		deploymentValues := coderdtest.DeploymentValues(t)
		deploymentValues.EnableTerraformDebugMode = true

		adminClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, DeploymentValues: deploymentValues})
		admin := coderdtest.CreateFirstUser(t, adminClient)

		// Interact as template admin
		echoResponses := &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_DEBUG,
						Output: "want-it",
					},
				},
			}, {
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_TRACE,
						Output: "dont-want-it",
					},
				},
			}, {
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_DEBUG,
						Output: "done",
					},
				},
			}, {
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{},
				},
			}},
		}
		version := coderdtest.CreateTemplateVersion(t, adminClient, admin.OrganizationID, echoResponses)
		template := coderdtest.CreateTemplate(t, adminClient, admin.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)

		// Create workspace
		workspace := coderdtest.CreateWorkspace(t, adminClient, admin.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, adminClient, workspace.LatestBuild.ID)

		// Create workspace build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := adminClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
			ProvisionerState:  []byte(" "),
			LogLevel:          "debug",
		})
		require.Nil(t, err)

		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, adminClient, build.ID)

		// Watch for incoming logs
		logs, closer, err := adminClient.WorkspaceBuildLogsAfter(ctx, build.ID, 0)
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
				t.Logf("got log: %s -- %s | %s | %s", log.Level, log.Stage, log.Source, log.Output)
				if log.Source != "provisioner" {
					continue
				}
				logsProcessed++

				require.NotEqual(t, "dont-want-it", log.Output, "unexpected log message", "%s log message shouldn't be logged: %s")

				if log.Output == "done" {
					break processingLogs
				}
			}
		}
		require.Equal(t, 2, logsProcessed)
	})
}
