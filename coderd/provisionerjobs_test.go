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

	t.Run("ProvisionerJobs", func(t *testing.T) {
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
			InitiatorID:    member.ID,
			Tags:           database.StringMap{"initiatorTest": "true"},
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
				InitiatorID:    owner.UserID,
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

		t.Run("Initiator", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)

			jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
				Initiator: member.ID.String(),
			})
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(jobs), 1)
			require.Equal(t, member.ID, jobs[0].InitiatorID)
		})

		t.Run("InitiatorWithOtherFilters", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)

			// Test filtering by initiator ID combined with status filter
			jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
				Initiator: owner.UserID.String(),
				Status:    []codersdk.ProvisionerJobStatus{codersdk.ProvisionerJobSucceeded},
			})
			require.NoError(t, err)

			// Verify all returned jobs have the correct initiator and status
			for _, job := range jobs {
				require.Equal(t, owner.UserID, job.InitiatorID)
				require.Equal(t, codersdk.ProvisionerJobSucceeded, job.Status)
			}
		})

		t.Run("InitiatorWithLimit", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)

			// Test filtering by initiator ID with limit
			jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
				Initiator: owner.UserID.String(),
				Limit:     1,
			})
			require.NoError(t, err)
			require.Len(t, jobs, 1)

			// Verify the returned job has the correct initiator
			require.Equal(t, owner.UserID, jobs[0].InitiatorID)
		})

		t.Run("InitiatorWithTags", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)

			// Test filtering by initiator ID combined with tags
			jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
				Initiator: member.ID.String(),
				Tags:      map[string]string{"initiatorTest": "true"},
			})
			require.NoError(t, err)
			require.Len(t, jobs, 1)

			// Verify the returned job has the correct initiator and tags
			require.Equal(t, member.ID, jobs[0].InitiatorID)
			require.Equal(t, "true", jobs[0].Tags["initiatorTest"])
		})

		t.Run("InitiatorNotFound", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)

			// Test with non-existent initiator ID
			nonExistentID := uuid.New()
			jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
				Initiator: nonExistentID.String(),
			})
			require.NoError(t, err)
			require.Len(t, jobs, 0)
		})

		t.Run("InitiatorNil", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)

			// Test with nil initiator ID (should return all jobs)
			jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
				Initiator: "",
			})
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(jobs), 50) // Should return all jobs (up to default limit)
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

		t.Run("MemberDeniedWithInitiator", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			// Member should not be able to access jobs even with initiator filter
			jobs, err := memberClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
				Initiator: member.ID.String(),
			})
			require.Error(t, err)
			require.Len(t, jobs, 0)
		})
	})

	// Ensures that when a provisioner job is in the succeeded state,
	// the API response includes both worker_id and worker_name fields
	t.Run("AssignedProvisionerJob", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		client, _, coderdAPI := coderdtest.NewWithAPI(t, &coderdtest.Options{
			IncludeProvisionerDaemon: false,
			Database:                 db,
			Pubsub:                   ps,
		})
		provisionerDaemonName := "provisioner_daemon_test"
		provisionerDaemon := coderdtest.NewTaggedProvisionerDaemon(t, coderdAPI, provisionerDaemonName, map[string]string{"owner": "", "scope": "organization"})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))

		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Stop the provisioner so it doesn't grab any more jobs
		err := provisionerDaemon.Close()
		require.NoError(t, err)

		t.Run("List_IncludesWorkerIDAndName", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)

			// Get provisioner daemon responsible for executing the provisioner jobs
			provisionerDaemons, err := db.GetProvisionerDaemons(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, len(provisionerDaemons))
			if assert.NotEmpty(t, provisionerDaemons) {
				require.Equal(t, provisionerDaemonName, provisionerDaemons[0].Name)
			}

			// Get provisioner jobs
			jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, nil)
			require.NoError(t, err)
			require.Equal(t, 2, len(jobs))

			for _, job := range jobs {
				require.Equal(t, owner.OrganizationID, job.OrganizationID)
				require.Equal(t, database.ProvisionerJobStatusSucceeded, database.ProvisionerJobStatus(job.Status))

				// Guarantee that provisioner jobs contain the provisioner daemon ID and name
				if assert.NotEmpty(t, provisionerDaemons) {
					require.Equal(t, &provisionerDaemons[0].ID, job.WorkerID)
					require.Equal(t, provisionerDaemonName, job.WorkerName)
				}
			}
		})

		t.Run("Get_IncludesWorkerIDAndName", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)

			// Get provisioner daemon responsible for executing the provisioner job
			provisionerDaemons, err := db.GetProvisionerDaemons(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, len(provisionerDaemons))
			if assert.NotEmpty(t, provisionerDaemons) {
				require.Equal(t, provisionerDaemonName, provisionerDaemons[0].Name)
			}

			// Get all provisioner jobs
			jobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, nil)
			require.NoError(t, err)
			require.Equal(t, 2, len(jobs))

			// Find workspace_build provisioner job ID
			var workspaceProvisionerJobID uuid.UUID
			for _, job := range jobs {
				if job.Type == codersdk.ProvisionerJobTypeWorkspaceBuild {
					workspaceProvisionerJobID = job.ID
				}
			}
			require.NotNil(t, workspaceProvisionerJobID)

			// Get workspace_build provisioner job by ID
			workspaceProvisionerJob, err := templateAdminClient.OrganizationProvisionerJob(ctx, owner.OrganizationID, workspaceProvisionerJobID)
			require.NoError(t, err)

			require.Equal(t, owner.OrganizationID, workspaceProvisionerJob.OrganizationID)
			require.Equal(t, database.ProvisionerJobStatusSucceeded, database.ProvisionerJobStatus(workspaceProvisionerJob.Status))

			// Guarantee that provisioner job contains the provisioner daemon ID and name
			if assert.NotEmpty(t, provisionerDaemons) {
				require.Equal(t, &provisionerDaemons[0].ID, workspaceProvisionerJob.WorkerID)
				require.Equal(t, provisionerDaemonName, workspaceProvisionerJob.WorkerName)
			}
		})
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
