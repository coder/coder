package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerJobs(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		Database:                 db,
		Pubsub:                   ps,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

	time.Sleep(1500 * time.Millisecond) // Ensure the workspace build job has a different timestamp for sorting.
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	// Create a pending job.
	w := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        member.ID,
		TemplateID:     template.ID,
	})
	wbID := uuid.New()
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: w.OrganizationID,
		StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		Input:          json.RawMessage(`{"workspace_build_id":"` + wbID.String() + `"}`),
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		ID:                wbID,
		JobID:             job.ID,
		WorkspaceID:       w.ID,
		TemplateVersionID: version.ID,
	})

	// Add more jobs than the default limit.
	for i := range 60 {
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: owner.OrganizationID,
			Tags:           database.StringMap{"count": strconv.Itoa(i)},
		})
	}

	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		t.Run("Workspace", func(t *testing.T) {
			t.Parallel()
			t.Run("OK", func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitMedium)
				// Note this calls the single job endpoint.
				job2, err := templateAdminClient.OrganizationProvisionerJob(ctx, owner.OrganizationID, job.ID)
				require.NoError(t, err)
				require.Equal(t, job.ID, job2.ID)

				// Verify that job metadata is correct.
				assert.Equal(t, job2.Metadata, codersdk.ProvisionerJobMetadata{
					TemplateVersionName: version.Name,
					TemplateID:          template.ID,
					TemplateName:        template.Name,
					TemplateDisplayName: template.DisplayName,
					TemplateIcon:        template.Icon,
					WorkspaceID:         &w.ID,
					WorkspaceName:       w.Name,
				})
			})
		})
		t.Run("Template Import", func(t *testing.T) {
			t.Parallel()
			t.Run("OK", func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitMedium)
				// Note this calls the single job endpoint.
				job2, err := templateAdminClient.OrganizationProvisionerJob(ctx, owner.OrganizationID, version.Job.ID)
				require.NoError(t, err)
				require.Equal(t, version.Job.ID, job2.ID)

				// Verify that job metadata is correct.
				assert.Equal(t, job2.Metadata, codersdk.ProvisionerJobMetadata{
					TemplateVersionName: version.Name,
					TemplateID:          template.ID,
					TemplateName:        template.Name,
					TemplateDisplayName: template.DisplayName,
					TemplateIcon:        template.Icon,
				})
			})
		})
		t.Run("Missing", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			// Note this calls the single job endpoint.
			_, err := templateAdminClient.OrganizationProvisionerJob(ctx, owner.OrganizationID, uuid.New())
			require.Error(t, err)
		})
	})

	t.Run("Default limit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, nil)
		require.NoError(t, err)
		require.Len(t, jobs, 50)
	})

	t.Run("IDs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
			IDs: []uuid.UUID{workspace.LatestBuild.Job.ID, version.Job.ID},
		})
		require.NoError(t, err)
		require.Len(t, jobs, 2)
	})

	t.Run("Status", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
			Status: []codersdk.ProvisionerJobStatus{codersdk.ProvisionerJobRunning},
		})
		require.NoError(t, err)
		require.Len(t, jobs, 1)
	})

	t.Run("Tags", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
			Tags: map[string]string{"count": "1"},
		})
		require.NoError(t, err)
		require.Len(t, jobs, 1)
	})

	t.Run("Limit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
			Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, jobs, 1)
	})

	// For now, this is not allowed even though the member has created a
	// workspace. Once member-level permissions for jobs are supported
	// by RBAC, this test should be updated.
	t.Run("MemberDenied", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		jobs, err := memberClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, nil)
		require.Error(t, err)
		require.Len(t, jobs, 0)
	})
}

func TestProvisionerJobLogs(t *testing.T) {
	t.Parallel()
	t.Run("StreamAfterComplete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		logs, closer, err := client.WorkspaceBuildLogsAfter(ctx, workspace.LatestBuild.ID, 0)
		require.NoError(t, err)
		defer closer.Close()
		for {
			log, ok := <-logs
			t.Logf("got log: [%s] %s %s", log.Level, log.Stage, log.Output)
			if !ok {
				return
			}
		}
	})

	t.Run("StreamWhileRunning", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		logs, closer, err := client.WorkspaceBuildLogsAfter(ctx, workspace.LatestBuild.ID, 0)
		require.NoError(t, err)
		defer closer.Close()
		for {
			_, ok := <-logs
			if !ok {
				return
			}
		}
	})
}
