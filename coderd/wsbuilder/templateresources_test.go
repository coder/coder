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
		for _, pinned := range []bool{false, true} {
			name := string(transition) + "/ordinary"
			if pinned {
				name = string(transition) + "/pinned"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				revisionID := uuid.NullUUID{UUID: uuid.New(), Valid: pinned}
				outputs := json.RawMessage(`{"value":"previous"}`)
				opts := []txExpect{
					withTemplate, withInactiveVersionNoParams(),
					func(tx *dbmock.MockStore) { withLastBuildSnapshot(tx, revisionID) },
					withLastBuildState, withTemplateVersionVariables(inactiveVersionID, nil), withRichParameters(nil),
					withWorkspaceTags(inactiveVersionID, nil), withProvisionerDaemons(nil),
					expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
						require.Equal(t, "inactive", job.Tags["version"], "retain ordinary template routing")
						require.Equal(t, provisionersdk.ScopeUser, job.Tags[provisionersdk.TagScope])
						require.Equal(t, userID.String(), job.Tags[provisionersdk.TagOwner])
						if pinned {
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
							if pinned {
								require.Equal(t, revisionID.UUID, arg.TemplateResourceRevisionID.UUID)
								require.JSONEq(t, string(outputs), string(arg.TemplateResourceOutputs))
							}
							return nil
						})
					},
				}
				if transition == database.WorkspaceTransitionStart {
					opts = append(opts, withParameterSchemas(inactiveJobID, nil))
					if pinned {
						opts = append(opts, func(tx *dbmock.MockStore) {
							tx.EXPECT().GetTemplateResourceStateForUpdate(gomock.Any(), templateID).Return(database.TemplateResourceState{TemplateID: templateID, CurrentRevisionID: revisionID, AppliedRevisionID: revisionID, Status: "applied", Outputs: outputs}, nil)
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
