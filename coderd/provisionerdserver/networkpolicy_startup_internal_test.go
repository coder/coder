package provisionerdserver

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestNetworkPolicyQueuedWorkspaceStartupRechecked(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, mode := range []string{"require-current", "allow-last-applied"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			f := newNetworkPolicyFixture(t, db, ps, "apply")
			f.server.DeploymentValues = &codersdk.DeploymentValues{NetworkPolicyStartupMode: serpent.String(mode)}
			importJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				OrganizationID: f.job.OrganizationID, InitiatorID: f.job.InitiatorID, FileID: f.job.FileID, Type: database.ProvisionerJobTypeTemplateVersionImport,
				CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
			})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: f.run.TemplateIDs[0], Valid: true}, OrganizationID: f.job.OrganizationID,
				JobID: importJob.ID, CreatedBy: f.job.InitiatorID,
			})
			workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
				TemplateID: f.run.TemplateIDs[0], OrganizationID: f.job.OrganizationID, OwnerID: f.job.InitiatorID,
			})
			buildID := uuid.New()
			input, err := json.Marshal(WorkspaceProvisionJob{WorkspaceBuildID: buildID})
			require.NoError(t, err)
			queued := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				OrganizationID: f.job.OrganizationID, InitiatorID: f.job.InitiatorID, FileID: f.job.FileID, Type: database.ProvisionerJobTypeWorkspaceBuild,
				Input: input, StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, WorkerID: uuid.NullUUID{UUID: f.server.ID, Valid: true},
			})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				ID: buildID, WorkspaceID: workspace.ID, TemplateVersionID: version.ID, JobID: queued.ID,
				InitiatorID: f.job.InitiatorID, Transition: database.WorkspaceTransitionStart,
			})
			require.NoError(t, db.UpdateWorkspaceBuildProvisionerStateByID(t.Context(), database.UpdateWorkspaceBuildProvisionerStateByIDParams{
				ID: build.ID, UpdatedAt: dbtime.Now(), ProvisionerState: []byte("workspace state"),
			}))
			// This workspace job was already enqueued, but an attached policy
			// is not ready at dispatch. No Terraform payload may be delivered.
			payload, err := f.server.acquireProtoJob(dbauthz.AsProvisionerd(t.Context()), queued)
			require.ErrorContains(t, err, "network policy startup check")
			require.Nil(t, payload)
			failed, err := db.GetProvisionerJobByID(t.Context(), queued.ID)
			require.NoError(t, err)
			require.True(t, failed.CompletedAt.Valid)
			require.True(t, failed.Error.Valid)
			state, err := db.GetWorkspaceBuildProvisionerStateByID(t.Context(), build.ID)
			require.NoError(t, err)
			require.Equal(t, []byte("workspace state"), state.ProvisionerState)
		})
	}
}
