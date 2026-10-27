package provisionerdserver

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/provisionersdk"
)

func TestTemplateOutputsWorkerCapability(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		version    string
		capability string
		wantErr    bool
	}{
		{name: "OldEvenWithTag", version: "1.22", capability: "1", wantErr: true},
		{name: "MissingTag", version: "1.23", wantErr: true},
		{name: "WrongTag", version: "1.23", capability: "0", wantErr: true},
		{name: "InvalidVersion", version: "unknown", capability: "1", wantErr: true},
		{name: "Supported", version: "1.23", capability: "1"},
		{name: "Newer", version: "1.24", capability: "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &server{apiVersion: tc.version, Tags: Tags{provisionersdk.TagTemplateResources: tc.capability}}
			for _, snapshot := range [][2]string{{`{"id":"frozen"}`, ""}, {"{}", uuid.NewString()}} {
				err := s.validateTemplateOutputsWorker(snapshot[0], snapshot[1])
				if tc.wantErr {
					require.ErrorContains(t, err, "shared resource outputs require a provisioner with API v1.23 or later and coder.template_resources=1 capability")
				} else {
					require.NoError(t, err)
				}
			}
			require.NoError(t, s.validateTemplateOutputsWorker("{}", ""), "ordinary unconfigured jobs need no capability")
		})
	}
}

func TestTemplateOutputsAcquisitionRejectsOldWorker(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, jobType := range []database.ProvisionerJobType{
		database.ProvisionerJobTypeWorkspaceBuild,
		database.ProvisionerJobTypeTemplateVersionImport,
		database.ProvisionerJobTypeTemplateVersionDryRun,
	} {
		t.Run(string(jobType), func(t *testing.T) {
			t.Parallel()
			f := newTemplateResourcesFixture(t, db, ps, "apply")
			f.server.AccessURL = &url.URL{}
			f.server.apiVersion = "1.22"
			// Even a stale or forged capability tag cannot grant protocol support.
			f.server.Tags = Tags{provisionersdk.TagTemplateResources: "1"}
			ctx := t.Context()
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				OrganizationID: f.job.OrganizationID, CreatedBy: f.job.InitiatorID,
				TemplateID: uuid.NullUUID{UUID: f.run.TemplateID, Valid: true},
			})
			buildID := uuid.New()
			var input any
			switch jobType {
			case database.ProvisionerJobTypeWorkspaceBuild:
				input = WorkspaceProvisionJob{WorkspaceBuildID: buildID}
			case database.ProvisionerJobTypeTemplateVersionImport:
				input = TemplateVersionImportJob{TemplateID: version.TemplateID, TemplateVersionID: version.ID}
			case database.ProvisionerJobTypeTemplateVersionDryRun:
				input = TemplateVersionDryRunJob{TemplateVersionID: version.ID}
			}
			encoded, err := json.Marshal(input)
			require.NoError(t, err)
			job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				OrganizationID: f.job.OrganizationID, InitiatorID: f.job.InitiatorID, FileID: f.job.FileID,
				Type: jobType, Input: encoded, StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, WorkerID: f.job.WorkerID,
			})
			if jobType == database.ProvisionerJobTypeWorkspaceBuild {
				workspace := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: f.run.TemplateID, OrganizationID: f.job.OrganizationID, OwnerID: f.job.InitiatorID})
				dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{ID: buildID, WorkspaceID: workspace.ID, TemplateVersionID: version.ID, JobID: job.ID})
				require.NoError(t, db.PinWorkspaceBuildTemplateResources(ctx, database.PinWorkspaceBuildTemplateResourcesParams{
					ID: buildID, TemplateResourceOutputs: json.RawMessage(`{"id":"pinned"}`), TemplateResourceRevisionID: f.state.AppliedRevisionID,
				}))
			}
			// The queued validation job has no capability requirement. A newly
			// successful apply supplies latest outputs only after it was queued.
			_, err = db.CompleteTemplateResourceRun(ctx, database.CompleteTemplateResourceRunParams{
				TemplateID: f.state.TemplateID, RunID: f.state.ActiveRunID, Status: "applied", State: f.state.State,
				Outputs: json.RawMessage(`{"id":"applied-after-enqueue"}`), AppliedRevisionID: f.state.AppliedRevisionID, UpdatedAt: dbtime.Now(),
			})
			require.NoError(t, err)
			payload, err := f.server.acquireProtoJob(dbauthz.AsProvisionerd(ctx), job)
			require.Nil(t, payload)
			require.ErrorContains(t, err, "shared resource outputs require a provisioner with API v1.23 or later and coder.template_resources=1 capability")
			stored, err := db.GetProvisionerJobByID(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, stored.CompletedAt.Valid)
			require.Contains(t, stored.Error.String, "coder.template_resources=1")
		})
	}
}
