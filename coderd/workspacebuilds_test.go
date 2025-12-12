package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceBuild(t *testing.T) {
	t.Parallel()
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	ctx := testutil.Context(t, testutil.WaitLong)
	auditor := audit.NewMock()
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		Auditor:                  auditor,
	})
	user := coderdtest.CreateFirstUser(t, client)
	up, err := db.UpdateUserProfile(dbauthz.AsSystemRestricted(ctx), database.UpdateUserProfileParams{
		ID:        user.UserID,
		Email:     coderdtest.FirstUserParams.Email,
		Username:  coderdtest.FirstUserParams.Username,
		Name:      "Admin",
		AvatarURL: client.URL.String(),
		UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	auditor.ResetLogs()
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	// Create workspace will also start a build, so we need to wait for
	// it to ensure all events are recorded.
	require.Eventually(t, func() bool {
		logs := auditor.AuditLogs()
		return len(logs) == 2 &&
			assert.Equal(t, logs[0].Ip.IPNet.IP.String(), "127.0.0.1") &&
			assert.Equal(t, logs[1].Ip.IPNet.IP.String(), "127.0.0.1")
	}, testutil.WaitShort, testutil.IntervalFast)
	wb, err := client.WorkspaceBuild(testutil.Context(t, testutil.WaitShort), workspace.LatestBuild.ID)
	require.NoError(t, err)
	require.Equal(t, up.Username, wb.WorkspaceOwnerName)
	require.Equal(t, up.AvatarURL, wb.WorkspaceOwnerAvatarURL)
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
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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
		second, secondUser := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleOwner())

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
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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

		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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

		workspace = coderdtest.CreateWorkspace(t, regularUser, template.ID)
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

		t.Run("WithoutDelete", func(t *testing.T) {
			t.Parallel()
			client, store := coderdtest.NewWithDatabase(t, nil)
			first := coderdtest.CreateFirstUser(t, client)
			templateAdmin, templateAdminUser := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleTemplateAdmin())

			r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
				OwnerID:        templateAdminUser.ID,
				OrganizationID: first.OrganizationID,
			}).Do()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// Trying to orphan without delete transition fails.
			_, err := templateAdmin.CreateWorkspaceBuild(ctx, r.Workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: r.TemplateVersion.ID,
				Transition:        codersdk.WorkspaceTransitionStart,
				Orphan:            true,
			})
			require.Error(t, err, "Orphan is only permitted when deleting a workspace.")
			cerr := coderdtest.SDKError(t, err)
			require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
		})

		t.Run("WithState", func(t *testing.T) {
			t.Parallel()
			client, store := coderdtest.NewWithDatabase(t, nil)
			first := coderdtest.CreateFirstUser(t, client)
			templateAdmin, templateAdminUser := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleTemplateAdmin())

			r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
				OwnerID:        templateAdminUser.ID,
				OrganizationID: first.OrganizationID,
			}).Do()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// Providing both state and orphan fails.
			_, err := templateAdmin.CreateWorkspaceBuild(ctx, r.Workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: r.TemplateVersion.ID,
				Transition:        codersdk.WorkspaceTransitionDelete,
				ProvisionerState:  []byte(" "),
				Orphan:            true,
			})
			require.Error(t, err)
			cerr := coderdtest.SDKError(t, err)
			require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
		})

		t.Run("NoPermission", func(t *testing.T) {
			t.Parallel()
			client, store := coderdtest.NewWithDatabase(t, nil)
			first := coderdtest.CreateFirstUser(t, client)
			member, memberUser := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

			r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
				OwnerID:        memberUser.ID,
				OrganizationID: first.OrganizationID,
			}).Do()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// Trying to orphan without being a template admin fails.
			_, err := member.CreateWorkspaceBuild(ctx, r.Workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: r.TemplateVersion.ID,
				Transition:        codersdk.WorkspaceTransitionDelete,
				Orphan:            true,
			})
			require.Error(t, err)
			cerr := coderdtest.SDKError(t, err)
			require.Equal(t, http.StatusForbidden, cerr.StatusCode())
		})

		t.Run("OK", func(t *testing.T) {
			// Include a provisioner so that we can test that provisionerdserver
			// performs deletion.
			auditor := audit.NewMock()
			client, store := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
			first := coderdtest.CreateFirstUser(t, client)
			templateAdmin, templateAdminUser := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleTemplateAdmin())

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			// This is a valid zip file. Without this the job will fail to complete.
			// TODO: add this to dbfake by default.
			zipBytes := make([]byte, 22)
			zipBytes[0] = 80
			zipBytes[1] = 75
			zipBytes[2] = 0o5
			zipBytes[3] = 0o6
			uploadRes, err := client.Upload(ctx, codersdk.ContentTypeZip, bytes.NewReader(zipBytes))
			require.NoError(t, err)

			tv := dbfake.TemplateVersion(t, store).
				FileID(uploadRes.ID).
				Seed(database.TemplateVersion{
					OrganizationID: first.OrganizationID,
					CreatedBy:      templateAdminUser.ID,
				}).
				Do()

			r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
				OwnerID:        templateAdminUser.ID,
				OrganizationID: first.OrganizationID,
				TemplateID:     tv.Template.ID,
			}).Do()

			auditor.ResetLogs()
			// Regular orphan operation succeeds.
			build, err := templateAdmin.CreateWorkspaceBuild(ctx, r.Workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: r.TemplateVersion.ID,
				Transition:        codersdk.WorkspaceTransitionDelete,
				Orphan:            true,
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

			// Validate that the deletion was audited. This happens after the transaction
			// is committed, so it may not show up in the mock auditor immediately.
			testutil.Eventually(ctx, t, func(context.Context) bool {
				return auditor.Contains(t, database.AuditLog{
					ResourceID: build.ID,
					Action:     database.AuditActionDelete,
				})
			}, testutil.IntervalFast)
		})

		t.Run("NoProvisioners", func(t *testing.T) {
			t.Parallel()
			auditor := audit.NewMock()
			client, store := coderdtest.NewWithDatabase(t, &coderdtest.Options{Auditor: auditor})
			first := coderdtest.CreateFirstUser(t, client)
			templateAdmin, templateAdminUser := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleTemplateAdmin())

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
				OwnerID:        templateAdminUser.ID,
				OrganizationID: first.OrganizationID,
			}).Do()

			daemons, err := store.GetProvisionerDaemons(dbauthz.AsSystemReadProvisionerDaemons(ctx))
			require.NoError(t, err)
			require.Empty(t, daemons, "Provisioner daemons should be empty for this test")

			// Orphan deletion still succeeds despite no provisioners being available.
			build, err := templateAdmin.CreateWorkspaceBuild(ctx, r.Workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: r.TemplateVersion.ID,
				Transition:        codersdk.WorkspaceTransitionDelete,
				Orphan:            true,
			})
			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionDelete, build.Transition)
			require.Equal(t, codersdk.ProvisionerJobSucceeded, build.Job.Status)
			require.Empty(t, build.Job.Error)

			ws, err := client.Workspace(ctx, r.Workspace.ID)
			require.Empty(t, ws)
			require.Equal(t, http.StatusGone, coderdtest.SDKError(t, err).StatusCode())

			// Validate that the deletion was audited. This happens after the transaction
			// is committed, so it may not show up in the mock auditor immediately.
			testutil.Eventually(ctx, t, func(context.Context) bool {
				return auditor.Contains(t, database.AuditLog{
					ResourceID: build.ID,
					Action:     database.AuditActionDelete,
				})
			}, testutil.IntervalFast)
		})
	})
}

func TestPatchCancelWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("User is allowed to cancel", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionInit:  echo.InitComplete,
			ProvisionGraph: echo.GraphComplete,
			ProvisionPlan:  echo.PlanComplete,
			// Echo will never applying since there is no complete message
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		var build codersdk.WorkspaceBuild

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		require.Eventually(t, func() bool {
			var err error
			build, err = client.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
			return assert.NoError(t, err) && build.Job.Status == codersdk.ProvisionerJobRunning
		}, testutil.WaitShort, testutil.IntervalFast)

		require.Eventually(t, func() bool {
			err := client.CancelWorkspaceBuild(ctx, build.ID, codersdk.CancelWorkspaceBuildParams{})
			return err == nil
		}, testutil.WaitShort, testutil.IntervalMedium)

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
			Parse:          echo.ParseComplete,
			ProvisionInit:  echo.InitComplete,
			ProvisionGraph: echo.GraphComplete,
			ProvisionPlan:  echo.PlanComplete,
			// Echo will never applying
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		workspace := coderdtest.CreateWorkspace(t, userClient, template.ID)
		var build codersdk.WorkspaceBuild

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		require.Eventually(t, func() bool {
			var err error
			build, err = userClient.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
			return assert.NoError(t, err) && build.Job.Status == codersdk.ProvisionerJobRunning
		}, testutil.WaitShort, testutil.IntervalFast)
		err := userClient.CancelWorkspaceBuild(ctx, build.ID, codersdk.CancelWorkspaceBuildParams{})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})

	t.Run("Cancel with expect_state=pending", func(t *testing.T) {
		t.Parallel()

		// Given: a coderd instance with a provisioner daemon
		store, ps, db := dbtestutil.NewDBWithSQLDB(t)
		client, closeDaemon := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
			Database:                 store,
			Pubsub:                   ps,
			IncludeProvisionerDaemon: true,
		})
		defer closeDaemon.Close()
		// Given: a user, template, and workspace
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Stop the provisioner daemon.
		require.NoError(t, closeDaemon.Close())
		ctx := testutil.Context(t, testutil.WaitLong)
		// Given: no provisioner daemons exist.
		_, err := db.ExecContext(ctx, `DELETE FROM provisioner_daemons;`)
		require.NoError(t, err)

		// When: a new workspace build is created
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		// Then: the request should succeed.
		require.NoError(t, err)
		// Then: the provisioner job should remain pending.
		require.Equal(t, codersdk.ProvisionerJobPending, build.Job.Status)

		// Then: the response should indicate no provisioners are available.
		if assert.NotNil(t, build.MatchedProvisioners) {
			assert.Zero(t, build.MatchedProvisioners.Count)
			assert.Zero(t, build.MatchedProvisioners.Available)
			assert.Zero(t, build.MatchedProvisioners.MostRecentlySeen.Time)
			assert.False(t, build.MatchedProvisioners.MostRecentlySeen.Valid)
		}

		// When: the workspace build is canceled
		err = client.CancelWorkspaceBuild(ctx, build.ID, codersdk.CancelWorkspaceBuildParams{
			ExpectStatus: codersdk.CancelWorkspaceBuildStatusPending,
		})
		require.NoError(t, err)

		// Then: the workspace build should be canceled.
		build, err = client.WorkspaceBuild(ctx, build.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ProvisionerJobCanceled, build.Job.Status)
	})

	t.Run("Cancel with expect_state=pending when job is running - should fail with 412", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionInit:  echo.InitComplete,
			ProvisionGraph: echo.GraphComplete,
			ProvisionPlan:  echo.PlanComplete,
			// Echo will never applying
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		var build codersdk.WorkspaceBuild
		require.Eventually(t, func() bool {
			var err error
			build, err = client.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
			return assert.NoError(t, err) && build.Job.Status == codersdk.ProvisionerJobRunning
		}, testutil.WaitShort, testutil.IntervalFast)

		// When: a cancel request is made with expect_state=pending
		err := client.CancelWorkspaceBuild(ctx, build.ID, codersdk.CancelWorkspaceBuildParams{
			ExpectStatus: codersdk.CancelWorkspaceBuildStatusPending,
		})
		// Then: the request should fail with 412.
		require.Error(t, err)

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})

	t.Run("Cancel with expect_state=running when job is pending - should fail with 412", func(t *testing.T) {
		t.Parallel()

		// Given: a coderd instance with a provisioner daemon
		store, ps, db := dbtestutil.NewDBWithSQLDB(t)
		client, closeDaemon := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
			Database:                 store,
			Pubsub:                   ps,
			IncludeProvisionerDaemon: true,
		})
		defer closeDaemon.Close()
		// Given: a user, template, and workspace
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Stop the provisioner daemon.
		require.NoError(t, closeDaemon.Close())
		ctx := testutil.Context(t, testutil.WaitLong)
		// Given: no provisioner daemons exist.
		_, err := db.ExecContext(ctx, `DELETE FROM provisioner_daemons;`)
		require.NoError(t, err)

		// When: a new workspace build is created
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		// Then: the request should succeed.
		require.NoError(t, err)
		// Then: the provisioner job should remain pending.
		require.Equal(t, codersdk.ProvisionerJobPending, build.Job.Status)

		// Then: the response should indicate no provisioners are available.
		if assert.NotNil(t, build.MatchedProvisioners) {
			assert.Zero(t, build.MatchedProvisioners.Count)
			assert.Zero(t, build.MatchedProvisioners.Available)
			assert.Zero(t, build.MatchedProvisioners.MostRecentlySeen.Time)
			assert.False(t, build.MatchedProvisioners.MostRecentlySeen.Valid)
		}

		// When: a cancel request is made with expect_state=running
		err = client.CancelWorkspaceBuild(ctx, build.ID, codersdk.CancelWorkspaceBuildParams{
			ExpectStatus: codersdk.CancelWorkspaceBuildStatusRunning,
		})
		// Then: the request should fail with 412.
		require.Error(t, err)

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})

	t.Run("Cancel with expect_state - invalid status", func(t *testing.T) {
		t.Parallel()

		// Given: a coderd instance with a provisioner daemon
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionInit:  echo.InitComplete,
			ProvisionGraph: echo.GraphComplete,
			ProvisionPlan:  echo.PlanComplete,
			// Echo will never applying
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		// When: a cancel request is made with invalid expect_state
		err := client.CancelWorkspaceBuild(ctx, workspace.LatestBuild.ID, codersdk.CancelWorkspaceBuildParams{
			ExpectStatus: "invalid_status",
		})
		// Then: the request should fail with 400.
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Invalid expect_status")
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
			ProvisionGraph: []*proto.Response{{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
						Resources: []*proto.Resource{{
							Name: "first_resource",
							Type: "example",
							Agents: []*proto.Agent{{
								Id:    "something-1",
								Name:  "something-1",
								Auth:  &proto.Agent_Token{},
								Order: 3,
							}},
						}, {
							Name: "second_resource",
							Type: "example",
							Agents: []*proto.Agent{{
								Id:    "something-2",
								Name:  "something-2",
								Auth:  &proto.Agent_Token{},
								Order: 1,
							}, {
								Id:    "something-3",
								Name:  "something-3",
								Auth:  &proto.Agent_Token{},
								Order: 2,
							}},
						}, {
							Name: "third_resource",
							Type: "example",
						}, {
							Name: "fourth_resource",
							Type: "example",
						}, {
							Name: "fifth_resource",
							Type: "example",
							Agents: []*proto.Agent{{
								Id:   "something-4",
								Name: "something-4",
								Auth: &proto.Agent_Token{},
							}},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.NotNil(t, workspace.LatestBuild.Resources)
		require.Len(t, workspace.LatestBuild.Resources, 5)
		assertWorkspaceResource(t, workspace.LatestBuild.Resources[0], "fifth_resource", "example", 1)  // resource has agent with implicit order = 0
		assertWorkspaceResource(t, workspace.LatestBuild.Resources[1], "second_resource", "example", 2) // resource has 2 agents, one with low order value (2)
		assertWorkspaceResource(t, workspace.LatestBuild.Resources[2], "first_resource", "example", 1)  // resource has 1 agent with explicit order
		assertWorkspaceResource(t, workspace.LatestBuild.Resources[3], "fourth_resource", "example", 0) // resource has no agents, sorted by name
		assertWorkspaceResource(t, workspace.LatestBuild.Resources[4], "third_resource", "example", 0)  // resource is the last one
	})
}

func TestWorkspaceBuildWithUpdatedTemplateVersionSendsNotification(t *testing.T) {
	t.Parallel()

	t.Run("NoRepeatedNotifications", func(t *testing.T) {
		t.Parallel()

		notify := &notificationstest.FakeEnqueuer{}

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, NotificationsEnqueuer: notify})
		first := coderdtest.CreateFirstUser(t, client)
		templateAdminClient, templateAdmin := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleTemplateAdmin())
		userClient, user := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		// Create a template with an initial version
		version := coderdtest.CreateTemplateVersion(t, templateAdminClient, first.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdminClient, version.ID)
		template := coderdtest.CreateTemplate(t, templateAdminClient, first.OrganizationID, version.ID)

		// Create a workspace using this template
		workspace := coderdtest.CreateWorkspace(t, userClient, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)
		coderdtest.MustTransitionWorkspace(t, userClient, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Create a new version of the template
		newVersion := coderdtest.CreateTemplateVersion(t, templateAdminClient, first.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdminClient, newVersion.ID)

		// Create a workspace build using this new template version
		build := coderdtest.CreateWorkspaceBuild(t, userClient, workspace, database.WorkspaceTransitionStart, func(cwbr *codersdk.CreateWorkspaceBuildRequest) {
			cwbr.TemplateVersionID = newVersion.ID
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, build.ID)
		coderdtest.MustTransitionWorkspace(t, userClient, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Create the workspace build _again_. We are doing this to
		// ensure we do not create _another_ notification. This is
		// separate to the notifications subsystem dedupe mechanism
		// as this build shouldn't create a notification. It shouldn't
		// create another notification as this new build isn't changing
		// the template version.
		build = coderdtest.CreateWorkspaceBuild(t, userClient, workspace, database.WorkspaceTransitionStart, func(cwbr *codersdk.CreateWorkspaceBuildRequest) {
			cwbr.TemplateVersionID = newVersion.ID
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, build.ID)
		coderdtest.MustTransitionWorkspace(t, userClient, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// We're going to have two notifications (one for the first user and one for the template admin)
		// By ensuring we only have these two, we are sure the second build didn't trigger more
		// notifications.
		sent := notify.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceManuallyUpdated))
		require.Len(t, sent, 2)

		receivers := make([]uuid.UUID, len(sent))
		for idx, notif := range sent {
			receivers[idx] = notif.UserID
		}

		// Check the notification was sent to the first user and template admin
		// (both of whom have the "template admin" role), and explicitly not the
		// workspace owner (since they initiated the workspace build).
		require.Contains(t, receivers, templateAdmin.ID)
		require.Contains(t, receivers, first.UserID)
		require.NotContains(t, receivers, user.ID)

		require.Contains(t, sent[0].Targets, template.ID)
		require.Contains(t, sent[0].Targets, workspace.ID)
		require.Contains(t, sent[0].Targets, workspace.OrganizationID)
		require.Contains(t, sent[0].Targets, workspace.OwnerID)

		require.Contains(t, sent[1].Targets, template.ID)
		require.Contains(t, sent[1].Targets, workspace.ID)
		require.Contains(t, sent[1].Targets, workspace.OrganizationID)
		require.Contains(t, sent[1].Targets, workspace.OwnerID)
	})

	t.Run("ToCorrectUser", func(t *testing.T) {
		t.Parallel()

		notify := &notificationstest.FakeEnqueuer{}

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, NotificationsEnqueuer: notify})
		first := coderdtest.CreateFirstUser(t, client)
		templateAdminClient, templateAdmin := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleTemplateAdmin())
		userClient, user := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		// Create a template with an initial version
		version := coderdtest.CreateTemplateVersion(t, templateAdminClient, first.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdminClient, version.ID)
		template := coderdtest.CreateTemplate(t, templateAdminClient, first.OrganizationID, version.ID)

		// Create a workspace using this template
		workspace := coderdtest.CreateWorkspace(t, userClient, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)
		coderdtest.MustTransitionWorkspace(t, userClient, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Create a new version of the template
		newVersion := coderdtest.CreateTemplateVersion(t, templateAdminClient, first.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdminClient, newVersion.ID)

		// Create a workspace build using this new template version from a different user
		ctx := testutil.Context(t, testutil.WaitShort)
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition:        codersdk.WorkspaceTransitionStart,
			TemplateVersionID: newVersion.ID,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, build.ID)
		coderdtest.MustTransitionWorkspace(t, userClient, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Ensure we receive only 1 workspace manually updated notification and to the right user
		sent := notify.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceManuallyUpdated))
		require.Len(t, sent, 1)
		require.Equal(t, templateAdmin.ID, sent[0].UserID)
		require.Contains(t, sent[0].Targets, template.ID)
		require.Contains(t, sent[0].Targets, workspace.ID)
		require.Contains(t, sent[0].Targets, workspace.OrganizationID)
		require.Contains(t, sent[0].Targets, workspace.OwnerID)

		owner, ok := sent[0].Data["owner"].(map[string]any)
		require.True(t, ok, "notification data should have owner")
		require.Equal(t, user.ID, owner["id"])
		require.Equal(t, user.Name, owner["name"])
		require.Equal(t, user.Email, owner["email"])
	})
}

func assertWorkspaceResource(t *testing.T, actual codersdk.WorkspaceResource, name, aType string, numAgents int) {
	assert.Equal(t, name, actual.Name)
	assert.Equal(t, aType, actual.Type)
	assert.Len(t, actual.Agents, numAgents)
}

func TestWorkspaceBuildLogs(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionGraph: []*proto.Response{{
			Type: &proto.Response_Log{
				Log: &proto.Log{
					Level:  proto.LogLevel_INFO,
					Output: "example",
				},
			},
		}, {
			Type: &proto.Response_Graph{
				Graph: &proto.GraphComplete{
					Resources: []*proto.Resource{{
						Name: "some",
						Type: "example",
						Agents: []*proto.Agent{{
							Id:   "something",
							Name: "dev",
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
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)

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
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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

	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
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
	err = client.CancelWorkspaceBuild(ctx, build.ID, codersdk.CancelWorkspaceBuildParams{})
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

func TestWorkspaceDeleteSuspendedUser(t *testing.T) {
	t.Parallel()
	const providerID = "fake-github"
	fake := oidctest.NewFakeIDP(t, oidctest.WithServing())

	validateCalls := 0
	userSuspended := false
	owner := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		ExternalAuthConfigs: []*externalauth.Config{
			fake.ExternalAuthConfig(t, providerID, &oidctest.ExternalAuthConfigOptions{
				ValidatePayload: func(email string) (interface{}, int, error) {
					validateCalls++
					if userSuspended {
						// Simulate the user being suspended from the IDP too.
						return "", http.StatusForbidden, xerrors.New("user is suspended")
					}
					return "OK", 0, nil
				},
			}),
		},
	})

	first := coderdtest.CreateFirstUser(t, owner)

	// New user that we will suspend when we try to delete the workspace.
	client, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleTemplateAdmin())
	fake.ExternalLogin(t, client)

	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: []*proto.Response{{
			Type: &proto.Response_Graph{
				Graph: &proto.GraphComplete{
					Error:      "",
					Resources:  nil,
					Parameters: nil,
					ExternalAuthProviders: []*proto.ExternalAuthProviderResource{
						{
							Id:       providerID,
							Optional: false,
						},
					},
				},
			},
		}},
	})

	validateCalls = 0 // Reset
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, 1, validateCalls) // Ensure the external link is working

	// Suspend the user
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := owner.UpdateUserStatus(ctx, user.ID.String(), codersdk.UserStatusSuspended)
	require.NoError(t, err, "suspend user")

	// Now delete the workspace build
	userSuspended = true
	build, err := owner.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionDelete,
	})
	require.NoError(t, err)
	build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, owner, build.ID)
	require.Equal(t, 2, validateCalls)
	require.Equal(t, codersdk.WorkspaceStatusDeleted, build.Status)
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
		workspace := coderdtest.CreateWorkspace(t, adminClient, template.ID)
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
		workspace := coderdtest.CreateWorkspace(t, regularUserClient, template.ID)
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
		owner := coderdtest.CreateFirstUser(t, adminClient)
		templateAuthorClient, _ := coderdtest.CreateAnotherUser(t, adminClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

		// Template author: create a template
		version := coderdtest.CreateTemplateVersion(t, templateAuthorClient, owner.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, templateAuthorClient, owner.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAuthorClient, version.ID)

		// Template author: create a workspace
		workspace := coderdtest.CreateWorkspace(t, templateAuthorClient, template.ID)
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
		owner := coderdtest.CreateFirstUser(t, adminClient)

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
		version := coderdtest.CreateTemplateVersion(t, adminClient, owner.OrganizationID, echoResponses)
		template := coderdtest.CreateTemplate(t, adminClient, owner.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)

		// Create workspace
		workspace := coderdtest.CreateWorkspace(t, adminClient, template.ID)
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

func TestPostWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("NoTemplateVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)

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
			ProvisionPlan: []*proto.Response{
				{
					Type: &proto.Response_Plan{
						Plan: &proto.PlanComplete{
							Error: "failed to plan",
						},
					},
				},
			},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		version = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateWorkspace(ctx, user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("AlreadyActive", func(t *testing.T) {
		t.Parallel()
		client, closer := coderdtest.NewWithProvisionerCloser(t, nil)
		defer closer.Close()

		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		closer.Close()
		// Close here so workspace build doesn't process!
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)

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

	t.Run("Audit", func(t *testing.T) {
		t.Parallel()

		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			),
		)
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		auditor.ResetLogs()
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		if assert.NotNil(t, build.MatchedProvisioners) {
			require.Equal(t, 1, build.MatchedProvisioners.Count)
			require.Equal(t, 1, build.MatchedProvisioners.Available)
			require.NotZero(t, build.MatchedProvisioners.MostRecentlySeen.Time)
		}

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		require.Eventually(t, func() bool {
			logs := auditor.AuditLogs()
			return len(logs) > 0 &&
				assert.Equal(t, logs[0].Ip.IPNet.IP.String(), "127.0.0.1")
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("IncrementBuildNumber", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		if assert.NotNil(t, build.MatchedProvisioners) {
			require.Equal(t, 1, build.MatchedProvisioners.Count)
			require.Equal(t, 1, build.MatchedProvisioners.Available)
			require.NotZero(t, build.MatchedProvisioners.MostRecentlySeen.Time)
		}

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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
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
		if assert.NotNil(t, build.MatchedProvisioners) {
			require.Equal(t, 1, build.MatchedProvisioners.Count)
			require.Equal(t, 1, build.MatchedProvisioners.Available)
			require.NotZero(t, build.MatchedProvisioners.MostRecentlySeen.Time)
		}

		gotState, err := client.WorkspaceBuildState(ctx, build.ID)
		require.NoError(t, err)
		require.Equal(t, wantState, gotState)
	})

	t.Run("SetsPresetID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionGraph: []*proto.Response{{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
						Presets: []*proto.Preset{
							{
								Name: "autodetected",
							},
							{
								Name: "manual",
								Parameters: []*proto.PresetParameter{
									{
										Name:  "param1",
										Value: "value1",
									},
								},
							},
						},
					},
				},
			}},
			ProvisionApply: echo.ApplyComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		presets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Equal(t, 2, len(presets))
		require.Equal(t, "autodetected", presets[0].Name)
		require.Equal(t, "manual", presets[1].Name)

		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		// Preset ID was detected based on the workspace parameters:
		require.Equal(t, presets[0].ID, *workspace.LatestBuild.TemplateVersionPresetID)

		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID:       version.ID,
			Transition:              codersdk.WorkspaceTransitionStart,
			TemplateVersionPresetID: presets[1].ID,
		})
		require.NoError(t, err)
		require.NotNil(t, build.TemplateVersionPresetID)

		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Equal(t, presets[1].ID, *workspace.LatestBuild.TemplateVersionPresetID)
		require.Equal(t, build.TemplateVersionPresetID, workspace.LatestBuild.TemplateVersionPresetID)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.BuildNumber+1, build.BuildNumber)
		if assert.NotNil(t, build.MatchedProvisioners) {
			require.Equal(t, 1, build.MatchedProvisioners.Count)
			require.Equal(t, 1, build.MatchedProvisioners.Available)
			require.NotZero(t, build.MatchedProvisioners.MostRecentlySeen.Time)
		}

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner: user.UserID.String(),
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 0)
	})

	t.Run("NoProvisionersAvailable", func(t *testing.T) {
		t.Parallel()

		// Given: a coderd instance with a provisioner daemon
		store, ps, db := dbtestutil.NewDBWithSQLDB(t)
		client, closeDaemon := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
			Database:                 store,
			Pubsub:                   ps,
			IncludeProvisionerDaemon: true,
		})
		defer closeDaemon.Close()
		// Given: a user, template, and workspace
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Stop the provisioner daemon.
		require.NoError(t, closeDaemon.Close())
		ctx := testutil.Context(t, testutil.WaitLong)
		// Given: no provisioner daemons exist.
		_, err := db.ExecContext(ctx, `DELETE FROM provisioner_daemons;`)
		require.NoError(t, err)

		// When: a new workspace build is created
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		// Then: the request should succeed.
		require.NoError(t, err)
		// Then: the provisioner job should remain pending.
		require.Equal(t, codersdk.ProvisionerJobPending, build.Job.Status)
		// Then: the response should indicate no provisioners are available.
		if assert.NotNil(t, build.MatchedProvisioners) {
			assert.Zero(t, build.MatchedProvisioners.Count)
			assert.Zero(t, build.MatchedProvisioners.Available)
			assert.Zero(t, build.MatchedProvisioners.MostRecentlySeen.Time)
			assert.False(t, build.MatchedProvisioners.MostRecentlySeen.Valid)
		}
	})

	t.Run("AllProvisionersStale", func(t *testing.T) {
		t.Parallel()

		// Given: a coderd instance with a provisioner daemon
		store, ps, db := dbtestutil.NewDBWithSQLDB(t)
		client, closeDaemon := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
			Database:                 store,
			Pubsub:                   ps,
			IncludeProvisionerDaemon: true,
		})
		defer closeDaemon.Close()
		// Given: a user, template, and workspace
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx := testutil.Context(t, testutil.WaitLong)
		// Given: all provisioner daemons are stale
		// First stop the provisioner
		require.NoError(t, closeDaemon.Close())
		newLastSeenAt := dbtime.Now().Add(-time.Hour)
		// Update the last seen at for all provisioner daemons. We have to use the
		// SQL db directly because store.UpdateProvisionerDaemonLastSeenAt has a
		// built-in check to prevent updating the last seen at to a time in the past.
		_, err := db.ExecContext(ctx, `UPDATE provisioner_daemons SET last_seen_at = $1;`, newLastSeenAt)
		require.NoError(t, err)

		// When: a new workspace build is created
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		// Then: the request should succeed
		require.NoError(t, err)
		// Then: the provisioner job should remain pending
		require.Equal(t, codersdk.ProvisionerJobPending, build.Job.Status)
		// Then: the response should indicate no provisioners are available
		if assert.NotNil(t, build.MatchedProvisioners) {
			assert.Zero(t, build.MatchedProvisioners.Available)
			assert.Equal(t, 1, build.MatchedProvisioners.Count)
			assert.Equal(t, newLastSeenAt.UTC(), build.MatchedProvisioners.MostRecentlySeen.Time.UTC())
			assert.True(t, build.MatchedProvisioners.MostRecentlySeen.Valid)
		}
	})
	t.Run("WithReason", func(t *testing.T) {
		t.Parallel()
		client, closeDaemon := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		_ = closeDaemon.Close()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
			Reason:            codersdk.CreateWorkspaceBuildReasonDashboard,
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.BuildReasonDashboard, build.Reason)
	})
	t.Run("DeletedWorkspace", func(t *testing.T) {
		t.Parallel()

		// Given: a workspace that has already been deleted
		var (
			ctx             = testutil.Context(t, testutil.WaitShort)
			logger          = slogtest.Make(t, &slogtest.Options{}).Leveled(slog.LevelError)
			adminClient, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
				Logger: &logger,
			})
			admin                         = coderdtest.CreateFirstUser(t, adminClient)
			workspaceOwnerClient, member1 = coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
			otherMemberClient, _          = coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
			ws                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{OwnerID: member1.ID, OrganizationID: admin.OrganizationID}).
							Seed(database.WorkspaceBuild{Transition: database.WorkspaceTransitionDelete}).
							Do()
		)

		// This needs to be done separately as provisionerd handles marking the workspace as deleted
		// and we're skipping provisionerd here for speed.
		require.NoError(t, db.UpdateWorkspaceDeletedByID(dbauthz.AsProvisionerd(ctx), database.UpdateWorkspaceDeletedByIDParams{
			ID:      ws.Workspace.ID,
			Deleted: true,
		}))

		// Assert test invariant: Workspace should be deleted
		dbWs, err := db.GetWorkspaceByID(dbauthz.AsProvisionerd(ctx), ws.Workspace.ID)
		require.NoError(t, err)
		require.True(t, dbWs.Deleted, "workspace should be deleted")

		for _, tc := range []struct {
			user         *codersdk.Client
			tr           codersdk.WorkspaceTransition
			expectStatus int
		}{
			// You should not be allowed to mess with a workspace you don't own, regardless of its deleted state.
			{otherMemberClient, codersdk.WorkspaceTransitionStart, http.StatusNotFound},
			{otherMemberClient, codersdk.WorkspaceTransitionStop, http.StatusNotFound},
			{otherMemberClient, codersdk.WorkspaceTransitionDelete, http.StatusNotFound},
			// Starting or stopping a workspace is not allowed when it is deleted.
			{workspaceOwnerClient, codersdk.WorkspaceTransitionStart, http.StatusConflict},
			{workspaceOwnerClient, codersdk.WorkspaceTransitionStop, http.StatusConflict},
			// We allow a delete just in case a retry is required. In most cases, this will be a no-op.
			// Note: this is the last test case because it will change the state of the workspace.
			{workspaceOwnerClient, codersdk.WorkspaceTransitionDelete, http.StatusOK},
		} {
			// When: we create a workspace build with the given transition
			_, err = tc.user.CreateWorkspaceBuild(ctx, ws.Workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: tc.tr,
			})

			// Then: we allow ONLY a delete build for a deleted workspace.
			if tc.expectStatus < http.StatusBadRequest {
				require.NoError(t, err, "creating a %s build for a deleted workspace should not error", tc.tr)
			} else {
				var apiError *codersdk.Error
				require.Error(t, err, "creating a %s build for a deleted workspace should return an error", tc.tr)
				require.ErrorAs(t, err, &apiError)
				require.Equal(t, tc.expectStatus, apiError.StatusCode())
			}
		}
	})
}

func TestWorkspaceBuildTimings(t *testing.T) {
	t.Parallel()

	// Setup the test environment with a template and version
	db, pubsub := dbtestutil.NewDB(t)
	ownerClient := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	client, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	file := dbgen.File(t, db, database.File{
		CreatedBy: owner.UserID,
	})
	versionJob := dbgen.ProvisionerJob(t, db, pubsub, database.ProvisionerJob{
		OrganizationID: owner.OrganizationID,
		InitiatorID:    user.ID,
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

	// Tests will run in parallel. To avoid conflicts and race conditions on the
	// build number, each test will have its own workspace and build.
	makeBuild := func(t *testing.T) database.WorkspaceBuild {
		ws := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: owner.OrganizationID,
			TemplateID:     template.ID,
		})
		jobID := uuid.New()
		job := dbgen.ProvisionerJob(t, db, pubsub, database.ProvisionerJob{
			ID:             jobID,
			OrganizationID: owner.OrganizationID,
			Tags:           database.StringMap{jobID.String(): "true"},
		})
		return dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       ws.ID,
			TemplateVersionID: version.ID,
			InitiatorID:       owner.UserID,
			JobID:             job.ID,
			BuildNumber:       1,
		})
	}

	t.Run("NonExistentBuild", func(t *testing.T) {
		t.Parallel()

		// Given: a non-existent build
		buildID := uuid.New()

		// When: fetching timings for the build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)
		_, err := client.WorkspaceBuildTimings(ctx, buildID)

		// Then: expect a not found error
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("EmptyTimings", func(t *testing.T) {
		t.Parallel()

		// Given: a build with no timings
		build := makeBuild(t)

		// When: fetching timings for the build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)
		res, err := client.WorkspaceBuildTimings(ctx, build.ID)

		// Then: return a response with empty timings
		require.NoError(t, err)
		require.Empty(t, res.ProvisionerTimings)
		require.Empty(t, res.AgentScriptTimings)
	})

	t.Run("ProvisionerTimings", func(t *testing.T) {
		t.Parallel()

		// Given: a build with provisioner timings
		build := makeBuild(t)
		provisionerTimings := dbgen.ProvisionerJobTimings(t, db, build, 5)

		// When: fetching timings for the build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)
		res, err := client.WorkspaceBuildTimings(ctx, build.ID)
		require.NoError(t, err)

		// Then: return a response with the expected timings
		require.Len(t, res.ProvisionerTimings, 5)
		for i := range res.ProvisionerTimings {
			timingRes := res.ProvisionerTimings[i]
			genTiming := provisionerTimings[i]
			require.Equal(t, genTiming.Resource, timingRes.Resource)
			require.Equal(t, genTiming.Action, timingRes.Action)
			require.Equal(t, string(genTiming.Stage), string(timingRes.Stage))
			require.Equal(t, genTiming.JobID.String(), timingRes.JobID.String())
			require.Equal(t, genTiming.Source, timingRes.Source)
			require.Equal(t, genTiming.StartedAt.UnixMilli(), timingRes.StartedAt.UnixMilli())
			require.Equal(t, genTiming.EndedAt.UnixMilli(), timingRes.EndedAt.UnixMilli())
		}
	})

	t.Run("MultipleTimingsForSameAgentScript", func(t *testing.T) {
		t.Parallel()

		// Given: a build with multiple timings for the same script
		build := makeBuild(t)
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: build.JobID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})
		script := dbgen.WorkspaceAgentScript(t, db, database.WorkspaceAgentScript{
			WorkspaceAgentID: agent.ID,
		})
		timings := make([]database.WorkspaceAgentScriptTiming, 3)
		scriptStartedAt := dbtime.Now()
		for i := range timings {
			timings[i] = dbgen.WorkspaceAgentScriptTiming(t, db, database.WorkspaceAgentScriptTiming{
				StartedAt: scriptStartedAt,
				EndedAt:   scriptStartedAt.Add(1 * time.Minute),
				ScriptID:  script.ID,
			})

			// Add an hour to the previous "started at" so we can
			// reliably differentiate the scripts from each other.
			scriptStartedAt = scriptStartedAt.Add(1 * time.Hour)
		}

		// When: fetching timings for the build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)
		res, err := client.WorkspaceBuildTimings(ctx, build.ID)
		require.NoError(t, err)

		// Then: return a response with the first agent script timing
		require.Len(t, res.AgentScriptTimings, 1)

		require.Equal(t, timings[0].StartedAt.UnixMilli(), res.AgentScriptTimings[0].StartedAt.UnixMilli())
		require.Equal(t, timings[0].EndedAt.UnixMilli(), res.AgentScriptTimings[0].EndedAt.UnixMilli())
	})

	t.Run("AgentScriptTimings", func(t *testing.T) {
		t.Parallel()

		// Given: a build with agent script timings
		build := makeBuild(t)
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: build.JobID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})
		scripts := dbgen.WorkspaceAgentScripts(t, db, 5, database.WorkspaceAgentScript{
			WorkspaceAgentID: agent.ID,
		})
		agentScriptTimings := dbgen.WorkspaceAgentScriptTimings(t, db, scripts)

		// When: fetching timings for the build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)
		res, err := client.WorkspaceBuildTimings(ctx, build.ID)
		require.NoError(t, err)

		// Then: return a response with the expected timings
		require.Len(t, res.AgentScriptTimings, 5)
		slices.SortFunc(res.AgentScriptTimings, func(a, b codersdk.AgentScriptTiming) int {
			return a.StartedAt.Compare(b.StartedAt)
		})
		slices.SortFunc(agentScriptTimings, func(a, b database.WorkspaceAgentScriptTiming) int {
			return a.StartedAt.Compare(b.StartedAt)
		})
		for i := range res.AgentScriptTimings {
			timingRes := res.AgentScriptTimings[i]
			genTiming := agentScriptTimings[i]
			require.Equal(t, genTiming.ExitCode, timingRes.ExitCode)
			require.Equal(t, string(genTiming.Status), timingRes.Status)
			require.Equal(t, string(genTiming.Stage), string(timingRes.Stage))
			require.Equal(t, genTiming.StartedAt.UnixMilli(), timingRes.StartedAt.UnixMilli())
			require.Equal(t, genTiming.EndedAt.UnixMilli(), timingRes.EndedAt.UnixMilli())
			require.Equal(t, agent.ID.String(), timingRes.WorkspaceAgentID)
			require.Equal(t, agent.Name, timingRes.WorkspaceAgentName)
		}
	})

	t.Run("NoAgentScripts", func(t *testing.T) {
		t.Parallel()

		// Given: a build with no agent scripts
		build := makeBuild(t)
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: build.JobID,
		})
		dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})

		// When: fetching timings for the build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)
		res, err := client.WorkspaceBuildTimings(ctx, build.ID)
		require.NoError(t, err)

		// Then: return a response with empty agent script timings
		require.Empty(t, res.AgentScriptTimings)
	})

	// Some workspaces might not have agents. It is improbable, but possible.
	t.Run("NoAgents", func(t *testing.T) {
		t.Parallel()

		// Given: a build with no agents
		build := makeBuild(t)
		dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: build.JobID,
		})

		// When: fetching timings for the build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)
		res, err := client.WorkspaceBuildTimings(ctx, build.ID)
		require.NoError(t, err)

		// Then: return a response with empty agent script timings
		require.Empty(t, res.AgentScriptTimings)
		require.Empty(t, res.AgentConnectionTimings)
	})

	t.Run("AgentConnectionTimings", func(t *testing.T) {
		t.Parallel()

		// Given: a build with an agent
		build := makeBuild(t)
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: build.JobID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID:       resource.ID,
			FirstConnectedAt: sql.NullTime{Valid: true, Time: dbtime.Now().Add(-time.Hour)},
		})

		// When: fetching timings for the build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)
		res, err := client.WorkspaceBuildTimings(ctx, build.ID)
		require.NoError(t, err)

		// Then: return a response with the expected timings
		require.Len(t, res.AgentConnectionTimings, 1)
		for i := range res.ProvisionerTimings {
			timingRes := res.AgentConnectionTimings[i]
			require.Equal(t, agent.ID.String(), timingRes.WorkspaceAgentID)
			require.Equal(t, agent.Name, timingRes.WorkspaceAgentName)
			require.NotEmpty(t, timingRes.StartedAt)
			require.NotEmpty(t, timingRes.EndedAt)
		}
	})

	t.Run("MultipleAgents", func(t *testing.T) {
		t.Parallel()

		// Given: a build with multiple agents
		build := makeBuild(t)
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: build.JobID,
		})
		agents := make([]database.WorkspaceAgent, 5)
		for i := range agents {
			agents[i] = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ResourceID:       resource.ID,
				FirstConnectedAt: sql.NullTime{Valid: true, Time: dbtime.Now().Add(-time.Duration(i) * time.Hour)},
			})
		}

		// When: fetching timings for the build
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)
		res, err := client.WorkspaceBuildTimings(ctx, build.ID)
		require.NoError(t, err)

		// Then: return a response with the expected timings
		require.Len(t, res.AgentConnectionTimings, 5)
	})
}
