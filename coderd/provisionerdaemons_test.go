package coderd_test

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerDaemons(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t,
		dbtestutil.WithDumpOnFailure(),
		//nolint:gocritic // Use UTC for consistent timestamp length in golden files.
		dbtestutil.WithTimezone("UTC"),
	)
	client, _, coderdAPI := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
		Database:                 db,
		Pubsub:                   ps,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Create initial resources with a running provisioner.
	firstProvisioner := coderdtest.NewTaggedProvisionerDaemon(t, coderdAPI, "default-provisioner", map[string]string{"owner": "", "scope": "organization"})
	t.Cleanup(func() { _ = firstProvisioner.Close() })
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	// Stop the provisioner so it doesn't grab any more jobs.
	firstProvisioner.Close()

	// Create a provisioner that's working on a job.
	pd1 := dbgen.ProvisionerDaemon(t, coderdAPI.Database, database.ProvisionerDaemon{
		Name:       "provisioner-1",
		CreatedAt:  dbtime.Now().Add(1 * time.Second),
		LastSeenAt: sql.NullTime{Time: coderdAPI.Clock.Now().Add(time.Hour), Valid: true}, // Stale interval can't be adjusted, keep online.
		KeyID:      codersdk.ProvisionerKeyUUIDBuiltIn,
		Tags:       database.StringMap{"owner": "", "scope": "organization", "foo": "bar"},
	})
	w1 := dbgen.Workspace(t, coderdAPI.Database, database.WorkspaceTable{
		OwnerID:    member.ID,
		TemplateID: template.ID,
	})
	wb1ID := uuid.MustParse("00000000-0000-0000-dddd-000000000001")
	job1 := dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
		WorkerID:  uuid.NullUUID{UUID: pd1.ID, Valid: true},
		Input:     json.RawMessage(`{"workspace_build_id":"` + wb1ID.String() + `"}`),
		CreatedAt: dbtime.Now().Add(2 * time.Second),
		StartedAt: sql.NullTime{Time: coderdAPI.Clock.Now(), Valid: true},
		Tags:      database.StringMap{"owner": "", "scope": "organization", "foo": "bar"},
	})
	dbgen.WorkspaceBuild(t, coderdAPI.Database, database.WorkspaceBuild{
		ID:                wb1ID,
		JobID:             job1.ID,
		WorkspaceID:       w1.ID,
		TemplateVersionID: version.ID,
	})

	// Create a provisioner that completed a job previously and is offline.
	pd2 := dbgen.ProvisionerDaemon(t, coderdAPI.Database, database.ProvisionerDaemon{
		Name:       "provisioner-2",
		CreatedAt:  dbtime.Now().Add(2 * time.Second),
		LastSeenAt: sql.NullTime{Time: coderdAPI.Clock.Now().Add(-time.Hour), Valid: true},
		KeyID:      codersdk.ProvisionerKeyUUIDBuiltIn,
		Tags:       database.StringMap{"owner": "", "scope": "organization"},
	})
	w2 := dbgen.Workspace(t, coderdAPI.Database, database.WorkspaceTable{
		OwnerID:    member.ID,
		TemplateID: template.ID,
	})
	wb2ID := uuid.MustParse("00000000-0000-0000-dddd-000000000002")
	job2 := dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
		WorkerID:    uuid.NullUUID{UUID: pd2.ID, Valid: true},
		Input:       json.RawMessage(`{"workspace_build_id":"` + wb2ID.String() + `"}`),
		CreatedAt:   dbtime.Now().Add(3 * time.Second),
		StartedAt:   sql.NullTime{Time: coderdAPI.Clock.Now().Add(-2 * time.Hour), Valid: true},
		CompletedAt: sql.NullTime{Time: coderdAPI.Clock.Now().Add(-time.Hour), Valid: true},
		Tags:        database.StringMap{"owner": "", "scope": "organization"},
	})
	dbgen.WorkspaceBuild(t, coderdAPI.Database, database.WorkspaceBuild{
		ID:                wb2ID,
		JobID:             job2.ID,
		WorkspaceID:       w2.ID,
		TemplateVersionID: version.ID,
	})

	// Create a pending job.
	w3 := dbgen.Workspace(t, coderdAPI.Database, database.WorkspaceTable{
		OwnerID:    member.ID,
		TemplateID: template.ID,
	})
	wb3ID := uuid.MustParse("00000000-0000-0000-dddd-000000000003")
	job3 := dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
		Input:     json.RawMessage(`{"workspace_build_id":"` + wb3ID.String() + `"}`),
		CreatedAt: dbtime.Now().Add(4 * time.Second),
		Tags:      database.StringMap{"owner": "", "scope": "organization"},
	})
	dbgen.WorkspaceBuild(t, coderdAPI.Database, database.WorkspaceBuild{
		ID:                wb3ID,
		JobID:             job3.ID,
		WorkspaceID:       w3.ID,
		TemplateVersionID: version.ID,
	})

	// Create a provisioner that is idle.
	pd3 := dbgen.ProvisionerDaemon(t, coderdAPI.Database, database.ProvisionerDaemon{
		Name:       "provisioner-3",
		CreatedAt:  dbtime.Now().Add(3 * time.Second),
		LastSeenAt: sql.NullTime{Time: coderdAPI.Clock.Now().Add(time.Hour), Valid: true},
		KeyID:      codersdk.ProvisionerKeyUUIDBuiltIn,
		Tags:       database.StringMap{"owner": "", "scope": "organization"},
	})

	// Add more provisioners than the default limit.
	var userDaemons []database.ProvisionerDaemon
	for i := range 50 {
		userDaemons = append(userDaemons, dbgen.ProvisionerDaemon(t, coderdAPI.Database, database.ProvisionerDaemon{
			Name:      "user-provisioner-" + strconv.Itoa(i),
			CreatedAt: dbtime.Now().Add(3 * time.Second),
			KeyID:     codersdk.ProvisionerKeyUUIDUserAuth,
			Tags:      database.StringMap{"count": strconv.Itoa(i)},
		}))
	}

	t.Run("Default limit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		daemons, err := templateAdminClient.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, nil)
		require.NoError(t, err)
		require.Len(t, daemons, 50)
	})

	t.Run("IDs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		daemons, err := templateAdminClient.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerDaemonsOptions{
			IDs: []uuid.UUID{pd1.ID, pd2.ID},
		})
		require.NoError(t, err)
		require.Len(t, daemons, 2)
		require.Equal(t, pd1.ID, daemons[1].ID)
		require.Equal(t, pd2.ID, daemons[0].ID)
	})

	t.Run("Tags", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		daemons, err := templateAdminClient.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerDaemonsOptions{
			Tags: map[string]string{"count": "1"},
		})
		require.NoError(t, err)
		require.Len(t, daemons, 1)
		require.Equal(t, userDaemons[1].ID, daemons[0].ID)
	})

	t.Run("Limit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		daemons, err := templateAdminClient.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerDaemonsOptions{
			Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, daemons, 1)
	})

	t.Run("Busy", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		daemons, err := templateAdminClient.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerDaemonsOptions{
			IDs: []uuid.UUID{pd1.ID},
		})
		require.NoError(t, err)
		require.Len(t, daemons, 1)
		// Verify status.
		require.NotNil(t, daemons[0].Status)
		require.Equal(t, codersdk.ProvisionerDaemonBusy, *daemons[0].Status)
		require.NotNil(t, daemons[0].CurrentJob)
		require.Nil(t, daemons[0].PreviousJob)
		// Verify job.
		require.Equal(t, job1.ID, daemons[0].CurrentJob.ID)
		require.Equal(t, codersdk.ProvisionerJobRunning, daemons[0].CurrentJob.Status)
		require.Equal(t, template.Name, daemons[0].CurrentJob.TemplateName)
		require.Equal(t, template.DisplayName, daemons[0].CurrentJob.TemplateDisplayName)
		require.Equal(t, template.Icon, daemons[0].CurrentJob.TemplateIcon)
	})

	t.Run("Offline", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		daemons, err := templateAdminClient.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerDaemonsOptions{
			IDs: []uuid.UUID{pd2.ID},
		})
		require.NoError(t, err)
		require.Len(t, daemons, 1)
		// Verify status.
		require.NotNil(t, daemons[0].Status)
		require.Equal(t, codersdk.ProvisionerDaemonOffline, *daemons[0].Status)
		require.Nil(t, daemons[0].CurrentJob)
		require.NotNil(t, daemons[0].PreviousJob)
		// Verify job.
		require.Equal(t, job2.ID, daemons[0].PreviousJob.ID)
		require.Equal(t, codersdk.ProvisionerJobSucceeded, daemons[0].PreviousJob.Status)
		require.Equal(t, template.Name, daemons[0].PreviousJob.TemplateName)
		require.Equal(t, template.DisplayName, daemons[0].PreviousJob.TemplateDisplayName)
		require.Equal(t, template.Icon, daemons[0].PreviousJob.TemplateIcon)
	})

	t.Run("Idle", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		daemons, err := templateAdminClient.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerDaemonsOptions{
			IDs: []uuid.UUID{pd3.ID},
		})
		require.NoError(t, err)
		require.Len(t, daemons, 1)
		// Verify status.
		require.NotNil(t, daemons[0].Status)
		require.Equal(t, codersdk.ProvisionerDaemonIdle, *daemons[0].Status)
		require.Nil(t, daemons[0].CurrentJob)
		require.Nil(t, daemons[0].PreviousJob)
	})

	// For now, this is not allowed even though the member has created a
	// workspace. Once member-level permissions for jobs are supported
	// by RBAC, this test should be updated.
	t.Run("MemberDenied", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		daemons, err := memberClient.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, nil)
		require.Error(t, err)
		require.Len(t, daemons, 0)
	})
}
