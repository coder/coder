package wsbuilder_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/provisionersdk"
)

func TestBuilderTemplateResourceRouting(t *testing.T) {
	t.Parallel()
	for _, transition := range []database.WorkspaceTransition{database.WorkspaceTransitionStart, database.WorkspaceTransitionStop, database.WorkspaceTransitionDelete} {
		for _, tc := range []struct {
			name                        string
			pinned, bound, missingState bool
		}{
			{name: "ordinary"},
			{name: "pinned", pinned: true, bound: true},
			{name: "attachedAfterImport", bound: true},
			{name: "attachedAfterImportWithoutState", bound: true, missingState: true},
		} {
			t.Run(string(transition)+"/"+tc.name, func(t *testing.T) {
				pinned := tc.pinned
				t.Parallel()
				foundationID := uuid.New()
				revisionID := uuid.NullUUID{UUID: uuid.New(), Valid: pinned}
				stateOutputs := json.RawMessage(`{"value":"previous"}`)
				outputs := stateOutputs
				if transition == database.WorkspaceTransitionStart {
					outputs = json.RawMessage(`{"value":"current"}`)
				}
				opts := []txExpect{
					// The imported version has no Foundation capability tag, as
					// with versions imported before a Foundation was attached.
					withTemplate, withInactiveVersionNoParams(),
					func(tx *dbmock.MockStore) { withLastBuildSnapshot(tx, revisionID) },
					withLastBuildState, withTemplateVersionVariables(inactiveVersionID, nil), withRichParameters(nil),
					withWorkspaceTags(inactiveVersionID, nil), withProvisionerDaemons(nil),
					expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
						require.Equal(t, "inactive", job.Tags["version"], "retain ordinary template routing")
						require.Equal(t, provisionersdk.ScopeUser, job.Tags[provisionersdk.TagScope])
						require.Equal(t, userID.String(), job.Tags[provisionersdk.TagOwner])
						if pinned || (transition == database.WorkspaceTransitionStart && tc.bound) {
							require.Equal(t, provisionersdk.TemplateResourcesCapabilityVersion, job.Tags[provisionersdk.TagTemplateResources])
						} else {
							require.NotContains(t, job.Tags, provisionersdk.TagTemplateResources, "ordinary jobs must remain eligible for older workers")
						}
					}),
					withInTx, expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
					expectBuild(func(build database.InsertWorkspaceBuildParams) { require.Equal(t, transition, build.Transition) }),
					withBuild, expectBuildParameters(func(database.InsertWorkspaceBuildParametersParams) {}),
					func(tx *dbmock.MockStore) {
						tx.EXPECT().GetTemplateVersionParameters(gomock.Any(), inactiveVersionID).Return(nil, sql.ErrNoRows)
						tx.EXPECT().PinWorkspaceBuildTemplateResources(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.PinWorkspaceBuildTemplateResourcesParams) error {
							require.Equal(t, pinned, arg.TemplateResourceRevisionID.Valid)
							require.Equal(t, pinned, arg.StateTemplateResourceRevisionID.Valid)
							if pinned {
								require.Equal(t, revisionID.UUID, arg.StateTemplateResourceRevisionID.UUID)
								require.JSONEq(t, string(stateOutputs), string(arg.StateTemplateResourceOutputs))
								require.Equal(t, revisionID.UUID, arg.TemplateResourceRevisionID.UUID)
								require.JSONEq(t, string(outputs), string(arg.TemplateResourceOutputs))
							} else {
								require.JSONEq(t, `{}`, string(arg.TemplateResourceOutputs))
								require.JSONEq(t, `{}`, string(arg.StateTemplateResourceOutputs))
							}
							return nil
						})
					},
				}
				if transition == database.WorkspaceTransitionStart {
					opts = append(opts, withParameterSchemas(inactiveJobID, nil))
					if tc.bound {
						opts = append(opts, func(tx *dbmock.MockStore) {
							tx.EXPECT().GetFoundationByTemplateID(gomock.Any(), uuid.NullUUID{UUID: templateID, Valid: true}).Return(database.Foundation{ID: foundationID}, nil)
							tx.EXPECT().LockFoundation(gomock.Any(), foundationID).Return(nil)
							state := database.TemplateResourceState{FoundationID: foundationID, Status: "unconfigured"}
							if pinned {
								state.CurrentRevisionID, state.AppliedRevisionID = revisionID, revisionID
								state.Status, state.Outputs = "applied", outputs
							}
							var stateErr error
							if tc.missingState {
								stateErr = sql.ErrNoRows
							}
							tx.EXPECT().GetTemplateResourceStateForUpdate(gomock.Any(), foundationID).Return(state, stateErr)
						})
					}
				}
				db := expectDB(t, opts...)
				fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
				builder := wsbuilder.New(database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}, transition, wsbuilder.NoopUsageChecker{})
				_, _, _, err := builder.Build(t.Context(), db, fc, nil, audit.WorkspaceBuildBaggage{})
				require.NoError(t, err)
			})
		}
	}
}
