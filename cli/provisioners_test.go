package cli_test

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
)

func TestProvisioners(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	client, _, coderdAPI := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
		Database:                 db,
		Pubsub:                   ps,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Create initial resources with a running provisioner.
	firstProvisioner := coderdtest.NewProvisionerDaemon(t, coderdAPI)
	defer firstProvisioner.Close()
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	// Stop the provisioner so it doesn't grab jobs.
	firstProvisioner.Close()

	// Create a provisioner that's working on a job.
	pd1 := dbgen.ProvisionerDaemon(t, coderdAPI.Database, database.ProvisionerDaemon{
		Name:      "provisioner-1",
		CreatedAt: timeParse(t, "2006-01-02", "2024-12-20"),
		KeyID:     uuid.MustParse(codersdk.ProvisionerKeyIDBuiltIn),
	})
	w1 := dbgen.Workspace(t, coderdAPI.Database, database.WorkspaceTable{
		OwnerID:    memberUser.ID,
		TemplateID: template.ID,
	})
	wb1ID := uuid.MustParse("00000000-0000-0000-bbbb-000000000001")
	job1 := dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
		ID:        uuid.MustParse("00000000-0000-0000-cccc-000000000001"),
		WorkerID:  uuid.NullUUID{UUID: pd1.ID, Valid: true},
		Input:     json.RawMessage(`{"workspace_build_id":"` + wb1ID.String() + `"}`),
		StartedAt: sql.NullTime{Time: coderdAPI.Clock.Now(), Valid: true},
		Tags:      database.StringMap{"owner": "", "scope": "organization"},
	})
	dbgen.WorkspaceBuild(t, coderdAPI.Database, database.WorkspaceBuild{
		ID:                wb1ID,
		JobID:             job1.ID,
		WorkspaceID:       w1.ID,
		TemplateVersionID: version.ID,
	})

	// Create another provisioner that completed a job and is offline.
	pd2 := dbgen.ProvisionerDaemon(t, coderdAPI.Database, database.ProvisionerDaemon{
		Name:       "provisioner-2",
		CreatedAt:  timeParse(t, "2006-01-02", "2024-12-20"),
		LastSeenAt: sql.NullTime{Time: coderdAPI.Clock.Now().Add(-time.Hour), Valid: true},
		KeyID:      uuid.MustParse(codersdk.ProvisionerKeyIDBuiltIn),
	})
	w2 := dbgen.Workspace(t, coderdAPI.Database, database.WorkspaceTable{
		OwnerID:    memberUser.ID,
		TemplateID: template.ID,
	})
	wb2ID := uuid.MustParse("00000000-0000-0000-bbbb-000000000002")
	job2 := dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
		ID:          uuid.MustParse("00000000-0000-0000-cccc-000000000002"),
		WorkerID:    uuid.NullUUID{UUID: pd2.ID, Valid: true},
		Input:       json.RawMessage(`{"workspace_build_id":"` + wb2ID.String() + `"}`),
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
		OwnerID:    memberUser.ID,
		TemplateID: template.ID,
	})
	wb3ID := uuid.MustParse("00000000-0000-0000-bbbb-000000000003")
	job3 := dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
		ID:    uuid.MustParse("00000000-0000-0000-cccc-000000000003"),
		Input: json.RawMessage(`{"workspace_build_id":"` + wb3ID.String() + `"}`),
		Tags:  database.StringMap{"owner": "", "scope": "organization"},
	})
	dbgen.WorkspaceBuild(t, coderdAPI.Database, database.WorkspaceBuild{
		ID:                wb3ID,
		JobID:             job3.ID,
		WorkspaceID:       w3.ID,
		TemplateVersionID: version.ID,
	})

	// Create a provisioner that is idle.
	pd3 := dbgen.ProvisionerDaemon(t, coderdAPI.Database, database.ProvisionerDaemon{
		Name:      "provisioner-3",
		CreatedAt: timeParse(t, "2006-01-02", "2024-12-20"),
		KeyID:     uuid.MustParse(codersdk.ProvisionerKeyIDBuiltIn),
	})
	_ = pd3

	t.Run("list", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t,
			"provisioners",
			"list",
			"--column", "id,created at,last seen at,name,version,api version,tags,status,current job id,previous job id,previous job status,organization",
		)
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		// TODO(mafredri): Verify golden output.
	})

	t.Run("jobs list", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t,
			"provisioners",
			"jobs",
			"list",
			"--column", "id,created at,status,worker id,tags,template version id,workspace build id,type,available workers,organization,queue",
		)
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		// TODO(mafredri): Verify golden output.
	})
}

func timeParse(t *testing.T, layout, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(layout, s)
	require.NoError(t, err)
	return tm
}
