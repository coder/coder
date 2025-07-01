package cli_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

func TestProvisioners_Golden(t *testing.T) {
	t.Parallel()

	// Replace UUIDs with predictable values for golden files.
	replace := make(map[string]string)
	updateReplaceUUIDs := func(coderdAPI *coderd.API) {
		//nolint:gocritic // This is a test.
		systemCtx := dbauthz.AsSystemRestricted(context.Background())
		provisioners, err := coderdAPI.Database.GetProvisionerDaemons(systemCtx)
		require.NoError(t, err)
		slices.SortFunc(provisioners, func(a, b database.ProvisionerDaemon) int {
			return a.CreatedAt.Compare(b.CreatedAt)
		})
		pIdx := 0
		for _, p := range provisioners {
			if _, ok := replace[p.ID.String()]; !ok {
				replace[p.ID.String()] = fmt.Sprintf("00000000-0000-0000-aaaa-%012d", pIdx)
				pIdx++
			}
		}
		jobs, err := coderdAPI.Database.GetProvisionerJobsCreatedAfter(systemCtx, time.Time{})
		require.NoError(t, err)
		slices.SortFunc(jobs, func(a, b database.ProvisionerJob) int {
			return a.CreatedAt.Compare(b.CreatedAt)
		})
		jIdx := 0
		for _, j := range jobs {
			if _, ok := replace[j.ID.String()]; !ok {
				replace[j.ID.String()] = fmt.Sprintf("00000000-0000-0000-bbbb-%012d", jIdx)
				jIdx++
			}
		}
	}

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
	_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Create initial resources with a running provisioner.
	firstProvisioner := coderdtest.NewTaggedProvisionerDaemon(t, coderdAPI, "default-provisioner", map[string]string{"owner": "", "scope": "organization"})
	t.Cleanup(func() { _ = firstProvisioner.Close() })
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	// Stop the provisioner so it doesn't grab any more jobs.
	firstProvisioner.Close()

	// Sanitize the UUIDs for the initial resources.
	replace[version.ID.String()] = "00000000-0000-0000-cccc-000000000000"
	replace[workspace.LatestBuild.ID.String()] = "00000000-0000-0000-dddd-000000000000"

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
	_ = dbgen.ProvisionerDaemon(t, coderdAPI.Database, database.ProvisionerDaemon{
		Name:       "provisioner-3",
		CreatedAt:  dbtime.Now().Add(3 * time.Second),
		LastSeenAt: sql.NullTime{Time: coderdAPI.Clock.Now().Add(time.Hour), Valid: true}, // Stale interval can't be adjusted, keep online.
		KeyID:      codersdk.ProvisionerKeyUUIDBuiltIn,
		Tags:       database.StringMap{"owner": "", "scope": "organization"},
	})

	updateReplaceUUIDs(coderdAPI)

	for id, replaceID := range replace {
		t.Logf("replace[%q] = %q", id, replaceID)
	}

	// Test provisioners list with template admin as members are currently
	// unable to access provisioner jobs. In the future (with RBAC
	// changes), we may allow them to view _their_ jobs.
	t.Run("list", func(t *testing.T) {
		t.Parallel()

		var got bytes.Buffer
		inv, root := clitest.New(t,
			"provisioners",
			"list",
			"--column", "id,created at,last seen at,name,version,tags,key name,status,current job id,current job status,previous job id,previous job status,organization",
		)
		inv.Stdout = &got
		clitest.SetupConfig(t, templateAdminClient, root)
		err := inv.Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, t.Name(), got.Bytes(), replace)
	})

	// Test jobs list with template admin as members are currently
	// unable to access provisioner jobs. In the future (with RBAC
	// changes), we may allow them to view _their_ jobs.
	t.Run("jobs list", func(t *testing.T) {
		t.Parallel()

		var got bytes.Buffer
		inv, root := clitest.New(t,
			"provisioners",
			"jobs",
			"list",
			"--column", "id,created at,status,worker id,tags,template version id,workspace build id,type,available workers,organization,queue",
		)
		inv.Stdout = &got
		clitest.SetupConfig(t, templateAdminClient, root)
		err := inv.Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, t.Name(), got.Bytes(), replace)
	})
}
