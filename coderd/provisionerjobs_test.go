package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
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
	"github.com/coder/coder/v2/coderd/database/testcases"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestProvisionerJob_MultipleJobsAndProvisioners tests OrganizationProvisionerJob single job endpoint by repeatedly
// calling it for every jobID and comparing with expected result.
func TestProvisionerJob_MultipleJobsAndProvisioners(t *testing.T) {
	t.Parallel()

	testCases := testcases.GetAPILevelQueuePositionTestCases()

	// ExtJob contains provisioner-job and related objects
	type ExtJob struct {
		database.ProvisionerJob
		workspace database.WorkspaceTable
	}

	for _, tc := range testCases {
		tc := tc // Capture loop variable to avoid data races
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			now := dbtime.Now()
			ctx := testutil.Context(t, testutil.WaitShort)

			db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Database:                 db,
				Pubsub:                   ps,
			})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))
			_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

			defaultOrg, err := db.GetDefaultOrganization(ctx)
			require.NoError(t, err)
			defaultOrgID := defaultOrg.ID

			// Create provisioner jobs based on provided tags:
			allExtJobs := make([]ExtJob, len(tc.JobTags))
			for idx, tags := range tc.JobTags {
				// Make sure jobs are stored in correct order, first job should have the earliest createdAt timestamp.
				// Example for 3 jobs:
				// job_1 createdAt: now - 3 minutes
				// job_2 createdAt: now - 2 minutes
				// job_3 createdAt: now - 1 minute
				timeOffsetInMinutes := len(tc.JobTags) - idx
				timeOffset := time.Duration(timeOffsetInMinutes) * time.Minute
				createdAt := now.Add(-timeOffset)

				// Create provisioner job and related objects
				w := dbgen.Workspace(t, db, database.WorkspaceTable{
					OrganizationID: owner.OrganizationID,
					OwnerID:        member.ID,
					TemplateID:     template.ID,
				})
				wbID := uuid.New()
				job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					CreatedAt:      createdAt,
					Tags:           tags,
					OrganizationID: w.OrganizationID,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					Input:          json.RawMessage(`{"workspace_build_id":"` + wbID.String() + `"}`),
				})
				dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					ID:                wbID,
					JobID:             job.ID,
					WorkspaceID:       w.ID,
					TemplateVersionID: version.ID,
				})

				allExtJobs[idx] = ExtJob{
					ProvisionerJob: job,
					workspace:      w,
				}

				// Ensure that all jobs and daemons share the same OrganizationID
				require.Equal(t, defaultOrgID, allExtJobs[idx].OrganizationID, "all jobs and daemons should be associated with the same organization")
			}

			// Create provisioner daemons based on provided tags:
			for idx, tags := range tc.DaemonTags {
				daemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
					Name:         fmt.Sprintf("prov_%v", idx),
					Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
					Tags:         tags,
				})

				// Ensure that all jobs and daemons share the same OrganizationID
				require.Equal(t, defaultOrgID, daemon.OrganizationID, "all jobs and daemons should be associated with the same organization")
			}

			filteredJobs := make([]ExtJob, 0)
			for idx, job := range allExtJobs {
				if _, skip := tc.SkipJobIDs[idx]; skip {
					continue
				}

				filteredJobs = append(filteredJobs, job)
			}

			// Then: the jobs should be returned in the correct order (sorted by createdAt in descending order)
			sort.Slice(filteredJobs, func(i, j int) bool {
				return filteredJobs[i].CreatedAt.After(filteredJobs[j].CreatedAt)
			})

			for idx, extJob := range filteredJobs {
				// Note this calls the single job endpoint.
				job2, err := templateAdminClient.OrganizationProvisionerJob(ctx, owner.OrganizationID, extJob.ID)
				require.NoError(t, err)
				require.Equal(t, extJob.ID, job2.ID)

				// Verify that job metadata is correct.
				assert.Equal(t, job2.Metadata, codersdk.ProvisionerJobMetadata{
					TemplateVersionName: version.Name,
					TemplateID:          template.ID,
					TemplateName:        template.Name,
					TemplateDisplayName: template.DisplayName,
					TemplateIcon:        template.Icon,
					WorkspaceID:         &extJob.workspace.ID,
					WorkspaceName:       extJob.workspace.Name,
				})

				require.Equal(t, tc.QueuePositions[idx], int64(job2.QueuePosition))
				require.Equal(t, tc.QueueSizes[idx], int64(job2.QueueSize))
			}
		})
	}
}

// TestProvisionerJobs_MultipleJobsAndProvisioners tests OrganizationProvisionerJobs multiple job endpoint by
// calling it once and comparing with expected result.
func TestProvisionerJobs_MultipleJobsAndProvisioners(t *testing.T) {
	t.Parallel()

	testCases := testcases.GetAPILevelQueuePositionTestCases()

	// ExtJob contains provisioner-job and related objects
	type ExtJob struct {
		database.ProvisionerJob
		workspace database.WorkspaceTable
	}

	for _, tc := range testCases {
		tc := tc // Capture loop variable to avoid data races
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			now := dbtime.Now()
			ctx := testutil.Context(t, testutil.WaitShort)

			db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Database:                 db,
				Pubsub:                   ps,
			})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))
			_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

			defaultOrg, err := db.GetDefaultOrganization(ctx)
			require.NoError(t, err)
			defaultOrgID := defaultOrg.ID

			// Create provisioner jobs based on provided tags:
			allExtJobs := make([]ExtJob, len(tc.JobTags))
			for idx, tags := range tc.JobTags {
				// Make sure jobs are stored in correct order, first job should have the earliest createdAt timestamp.
				// Example for 3 jobs:
				// job_1 createdAt: now - 3 minutes
				// job_2 createdAt: now - 2 minutes
				// job_3 createdAt: now - 1 minute
				timeOffsetInMinutes := len(tc.JobTags) - idx
				timeOffset := time.Duration(timeOffsetInMinutes) * time.Minute
				createdAt := now.Add(-timeOffset)

				// Create provisioner job and related objects
				w := dbgen.Workspace(t, db, database.WorkspaceTable{
					OrganizationID: owner.OrganizationID,
					OwnerID:        member.ID,
					TemplateID:     template.ID,
				})
				wbID := uuid.New()
				job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					CreatedAt:      createdAt,
					Tags:           tags,
					OrganizationID: w.OrganizationID,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					Input:          json.RawMessage(`{"workspace_build_id":"` + wbID.String() + `"}`),
				})
				dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					ID:                wbID,
					JobID:             job.ID,
					WorkspaceID:       w.ID,
					TemplateVersionID: version.ID,
				})

				allExtJobs[idx] = ExtJob{
					ProvisionerJob: job,
					workspace:      w,
				}

				// Ensure that all jobs and daemons share the same OrganizationID
				require.Equal(t, defaultOrgID, allExtJobs[idx].OrganizationID, "all jobs and daemons should be associated with the same organization")
			}

			// Create provisioner daemons based on provided tags:
			for idx, tags := range tc.DaemonTags {
				daemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
					Name:         fmt.Sprintf("prov_%v", idx),
					Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
					Tags:         tags,
				})

				// Ensure that all jobs and daemons share the same OrganizationID
				require.Equal(t, defaultOrgID, daemon.OrganizationID, "all jobs and daemons should be associated with the same organization")
			}

			filteredJobs := make([]ExtJob, 0)
			filteredJobIDs := make([]uuid.UUID, 0)
			for idx, job := range allExtJobs {
				if _, skip := tc.SkipJobIDs[idx]; skip {
					continue
				}

				filteredJobs = append(filteredJobs, job)
				filteredJobIDs = append(filteredJobIDs, job.ID)
			}

			// Then: the jobs should be returned in the correct order (sorted by createdAt in descending order)
			sort.Slice(filteredJobs, func(i, j int) bool {
				return filteredJobs[i].CreatedAt.After(filteredJobs[j].CreatedAt)
			})

			actualJobs, err := templateAdminClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
				IDs: filteredJobIDs,
			})
			require.NoError(t, err)

			for idx, extJob := range filteredJobs {
				// Verify that job metadata is correct.
				assert.Equal(t, actualJobs[idx].Metadata, codersdk.ProvisionerJobMetadata{
					TemplateVersionName: version.Name,
					TemplateID:          template.ID,
					TemplateName:        template.Name,
					TemplateDisplayName: template.DisplayName,
					TemplateIcon:        template.Icon,
					WorkspaceID:         &extJob.workspace.ID,
					WorkspaceName:       extJob.workspace.Name,
				})

				require.Equal(t, tc.QueuePositions[idx], int64(actualJobs[idx].QueuePosition))
				require.Equal(t, tc.QueueSizes[idx], int64(actualJobs[idx].QueueSize))
			}
		})
	}
}

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
