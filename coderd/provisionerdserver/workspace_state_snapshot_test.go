package provisionerdserver_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/provisionerd/proto"
)

func TestWorkspaceStateFoundationSnapshot(t *testing.T) {
	t.Parallel()
	srv, db, _, pd := setup(t, false, nil)
	for _, tc := range []struct {
		name       string
		state      []byte
		checkpoint bool
		complete   bool
		invalid    bool
	}{
		{name: "PlanningFailure"},
		{name: "InitFailureReturnsInheritedState", state: []byte("old state")},
		{name: "PartialApply", state: []byte("partial state"), checkpoint: true},
		{name: "PostApplyGraphFailure", state: []byte("applied state"), checkpoint: true},
		{name: "SuccessfulApply", state: []byte("applied state"), complete: true},
		{name: "RejectEmptyCheckpoint", checkpoint: true, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			user := dbgen.User(t, db, database.User{})
			file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
			template := dbgen.Template(t, db, database.Template{OrganizationID: pd.OrganizationID, CreatedBy: user.ID})
			importJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				OrganizationID: pd.OrganizationID, InitiatorID: user.ID, FileID: file.ID,
				Type:        database.ProvisionerJobTypeTemplateVersionImport,
				Tags:        database.StringMap{"snapshot-test": uuid.NewString()},
				CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
			})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				OrganizationID: pd.OrganizationID, CreatedBy: user.ID, JobID: importJob.ID,
				TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true},
			})
			workspaceRow := dbgen.Workspace(t, db, database.WorkspaceTable{
				OrganizationID: pd.OrganizationID, OwnerID: user.ID, TemplateID: template.ID,
			})
			workspace, err := db.GetWorkspaceByID(ctx, workspaceRow.ID)
			require.NoError(t, err)
			// No current binding exists. Cleanup must use only historical inputs.
			foundation := dbgen.Foundation(t, db, database.Foundation{OrganizationID: pd.OrganizationID})
			var revisions []uuid.NullUUID
			for range 2 {
				revision, err := db.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{
					ID: uuid.New(), FoundationID: foundation.ID, OrganizationID: pd.OrganizationID,
					FileID: file.ID, CreatedBy: user.ID, CreatedAt: dbtime.Now(), Source: "# snapshot",
				})
				require.NoError(t, err)
				revisions = append(revisions, uuid.NullUUID{UUID: revision.ID, Valid: true})
			}
			oldOutputs, attemptedOutputs := json.RawMessage(`{"deployment":"old"}`), json.RawMessage(`{"deployment":"attempted"}`)
			var attemptedJob database.ProvisionerJob
			var attemptedBuild database.WorkspaceBuild
			for i := range 2 {
				buildID := uuid.New()
				job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					OrganizationID: pd.OrganizationID, InitiatorID: user.ID, FileID: file.ID,
					Type:      database.ProvisionerJobTypeWorkspaceBuild,
					Input:     must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{WorkspaceBuildID: buildID})),
					StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
					WorkerID:  uuid.NullUUID{UUID: pd.ID, Valid: true},
				})
				build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					ID: buildID, WorkspaceID: workspace.ID, TemplateVersionID: version.ID,
					JobID: job.ID, InitiatorID: user.ID, BuildNumber: int32(i + 1),
				})
				outputs := oldOutputs
				if i == 1 {
					outputs = attemptedOutputs
				}
				require.NoError(t, db.PinWorkspaceBuildTemplateResources(ctx, database.PinWorkspaceBuildTemplateResourcesParams{
					ID: build.ID, TemplateResourceRevisionID: revisions[i], TemplateResourceOutputs: outputs,
					StateTemplateResourceRevisionID: revisions[0], StateTemplateResourceOutputs: oldOutputs,
				}))
				require.NoError(t, db.UpdateWorkspaceBuildProvisionerStateByID(ctx, database.UpdateWorkspaceBuildProvisionerStateByIDParams{
					ID: build.ID, UpdatedAt: dbtime.Now(), ProvisionerState: []byte("old state"),
				}))
				if i == 0 {
					require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
						ID: job.ID, CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, UpdatedAt: dbtime.Now(),
					}))
				} else {
					attemptedBuild, attemptedJob = build, job
				}
			}

			if tc.complete {
				_, err := srv.CompleteJob(ctx, &proto.CompletedJob{
					JobId: attemptedJob.ID.String(), Type: &proto.CompletedJob_WorkspaceBuild_{
						WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{State: tc.state},
					},
				})
				require.NoError(t, err)
			} else {
				_, err := srv.FailJob(ctx, &proto.FailedJob{
					JobId: attemptedJob.ID.String(), Error: tc.name, Type: &proto.FailedJob_WorkspaceBuild_{
						WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{State: tc.state, StateIsCheckpoint: tc.checkpoint},
					},
				})
				if tc.invalid {
					require.ErrorContains(t, err, "checkpoint must contain state")
					job, err := db.GetProvisionerJobByID(ctx, attemptedJob.ID)
					require.NoError(t, err)
					require.False(t, job.CompletedAt.Valid, "checkpoint rejection must roll back job completion")
					_, err = srv.FailJob(ctx, &proto.FailedJob{
						JobId: attemptedJob.ID.String(), Error: "failed without a checkpoint",
						Type: &proto.FailedJob_WorkspaceBuild_{WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{}},
					})
					require.NoError(t, err)
				} else {
					require.NoError(t, err)
				}
			}
			expectedRevision, expectedOutputs, expectedState := revisions[0], oldOutputs, []byte("old state")
			if (tc.checkpoint && !tc.invalid) || tc.complete {
				expectedRevision, expectedOutputs, expectedState = revisions[1], attemptedOutputs, tc.state
			}
			stored, err := db.GetWorkspaceBuildByID(ctx, attemptedBuild.ID)
			require.NoError(t, err)
			require.Equal(t, revisions[1], stored.TemplateResourceRevisionID, "retain attempted inputs for diagnosis")
			require.JSONEq(t, string(attemptedOutputs), string(stored.TemplateResourceOutputs))
			require.Equal(t, expectedRevision, stored.StateTemplateResourceRevisionID)
			require.JSONEq(t, string(expectedOutputs), string(stored.StateTemplateResourceOutputs))

			fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
			for _, transition := range []database.WorkspaceTransition{database.WorkspaceTransitionStop, database.WorkspaceTransitionDelete} {
				builder := wsbuilder.New(workspace, transition, wsbuilder.NoopUsageChecker{})
				cleanup, job, _, err := builder.Build(ctx, db, fc, nil, audit.WorkspaceBuildBaggage{})
				require.NoError(t, err)
				require.Equal(t, expectedRevision, cleanup.TemplateResourceRevisionID)
				require.JSONEq(t, string(expectedOutputs), string(cleanup.TemplateResourceOutputs))
				require.Equal(t, expectedRevision, cleanup.StateTemplateResourceRevisionID)
				require.JSONEq(t, string(expectedOutputs), string(cleanup.StateTemplateResourceOutputs))
				state, err := db.GetWorkspaceBuildProvisionerStateByID(ctx, cleanup.ID)
				require.NoError(t, err)
				require.Equal(t, expectedState, state.ProvisionerState)
				// A failed cleanup plan must preserve provenance for the next cleanup.
				acquired, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
					OrganizationID: pd.OrganizationID, Types: []database.ProvisionerType{job.Provisioner},
					ProvisionerTags: must(json.Marshal(job.Tags)), WorkerID: uuid.NullUUID{UUID: pd.ID, Valid: true},
					StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
				})
				require.NoError(t, err)
				require.Equal(t, job.ID, acquired.ID)
				_, err = srv.FailJob(ctx, &proto.FailedJob{
					JobId: job.ID.String(), Error: "cleanup plan failed",
					Type: &proto.FailedJob_WorkspaceBuild_{WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{}},
				})
				require.NoError(t, err)
			}
		})
	}
}
