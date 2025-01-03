package coderd_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerJobs(t *testing.T) {
	t.Parallel()

	// encode := func(v interface{}) []byte {
	// 	b, err := json.Marshal(v)
	// 	require.NoError(t, err)
	// 	return b
	// }

	// db, ps := dbtestutil.NewDB(t,
	// 	dbtestutil.WithDumpOnFailure(),
	// 	//nolint:gocritic // Use UTC for consistent timestamp length in golden files.
	// 	dbtestutil.WithTimezone("UTC"),
	// )
	// client, _, coderdAPI := coderdtest.NewWithAPI(t, &coderdtest.Options{
	// 	IncludeProvisionerDaemon: true,
	// 	Database:                 db,
	// 	Pubsub:                   ps,
	// })
	// owner := coderdtest.CreateFirstUser(t, client)
	// _, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		Database:                 db,
		Pubsub:                   ps,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	// user := coderdtest.CreateFirstUser(t, client)

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
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		Input:          json.RawMessage(`{"workspace_build_id":"` + wbID.String() + `"}`),
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		ID:                wbID,
		JobID:             job.ID,
		WorkspaceID:       w.ID,
		TemplateVersionID: version.ID,
	})

	t.Run("All", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		jobs, err := memberClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, nil)
		require.NoError(t, err)
		require.Len(t, jobs, 3)
	})

	t.Run("Pending", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		jobs, err := memberClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
			Status: []codersdk.ProvisionerJobStatus{codersdk.ProvisionerJobPending},
		})
		for _, job := range jobs {
			t.Logf("job: %#v", job)
		}
		require.NoError(t, err)
		require.Len(t, jobs, 1)
	})

	t.Run("Limit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		jobs, err := memberClient.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{
			Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, jobs, 1)
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
