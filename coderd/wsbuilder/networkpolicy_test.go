package wsbuilder_test

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/networkpolicy"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
)

func TestBuilderNetworkPolicyStartup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		configure  func(wsbuilder.Builder) wsbuilder.Builder
		transition database.WorkspaceTransition
		pending    bool
		allowed    bool
	}{
		{name: "DefaultRequiresCurrent"},
		{name: "DeploymentAllowsSafeLastApplied", configure: func(b wsbuilder.Builder) wsbuilder.Builder {
			return b.DeploymentValues(&codersdk.DeploymentValues{NetworkPolicyStartupMode: "allow-last-applied"})
		}, allowed: true},
		{name: "FactorySnapshotAllowsSafeLastApplied", configure: func(b wsbuilder.Builder) wsbuilder.Builder {
			options := networkpolicy.StartupOptions{Mode: networkpolicy.StartupAllowLastApplied}
			builder := b.NetworkPolicyStartup(options)
			// The builder must copy settings, not retain caller-owned data.
			options.Mode = networkpolicy.StartupRequireCurrent
			return builder
		}, allowed: true},
		{name: "AllowLastAppliedCannotBypassPending", configure: func(b wsbuilder.Builder) wsbuilder.Builder {
			return b.DeploymentValues(&codersdk.DeploymentValues{NetworkPolicyStartupMode: "allow-last-applied", NetworkPolicyDriftAction: "allow"})
		}, pending: true},
		{name: "AutostartBlocked", configure: func(b wsbuilder.Builder) wsbuilder.Builder { return b.Reason(database.BuildReasonAutostart) }},
		{name: "PrebuildCreationBlocked", configure: func(b wsbuilder.Builder) wsbuilder.Builder { return b.MarkPrebuild() }},
		{name: "PrebuildClaimBlocked", configure: func(b wsbuilder.Builder) wsbuilder.Builder { return b.MarkPrebuiltWorkspaceClaim() }},
		{name: "StopStillAvailable", transition: database.WorkspaceTransitionStop, allowed: true},
		{name: "DeleteStillAvailable", transition: database.WorkspaceTransitionDelete, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transition := tc.transition
			if transition == "" {
				transition = database.WorkspaceTransitionStart
			}
			opts := []txExpect{withTemplateRecord, withInactiveVersionNoParams(), withLastBuildFound}
			if transition == database.WorkspaceTransitionStart {
				opts = append(opts, expectNetworkPolicyStartup(t, startupExpectation{pending: tc.pending, allowed: tc.allowed}))
			}
			if tc.allowed {
				opts = append(opts,
					withLastBuildState, withTemplateVersionVariables(inactiveVersionID, nil), withRichParameters(nil),
					withWorkspaceTags(inactiveVersionID, nil),
					withProvisionerDaemons(nil), expectProvisionerJob(func(p database.InsertProvisionerJobParams) {
						require.Equal(t, database.ProvisionerJobTypeWorkspaceBuild, p.Type)
					}), withInTx,
					expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows), expectBuild(func(p database.InsertWorkspaceBuildParams) { require.Equal(t, transition, p.Transition) }), withBuild,
					expectBuildParameters(func(p database.InsertWorkspaceBuildParametersParams) {
						require.NotEqual(t, uuid.Nil, p.WorkspaceBuildID)
					}),
					func(tx *dbmock.MockStore) {
						tx.EXPECT().GetTemplateVersionParameters(gomock.Any(), inactiveVersionID).Return(nil, sql.ErrNoRows)
					},
				)
				if transition == database.WorkspaceTransitionStart {
					opts = append(opts, withParameterSchemas(inactiveJobID, nil))
				}
			}
			db := expectDB(t, opts...)
			fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
			builder := wsbuilder.New(database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}, transition, wsbuilder.NoopUsageChecker{})
			if tc.configure != nil {
				builder = tc.configure(builder)
			}
			build, job, _, err := builder.Build(t.Context(), db, fc, nil, audit.WorkspaceBuildBaggage{})
			if tc.allowed {
				require.NoError(t, err)
				require.NotNil(t, build)
				require.NotNil(t, job)
			} else {
				require.ErrorIs(t, err, networkpolicy.ErrStartupBlocked)
				var buildError wsbuilder.BuildError
				require.ErrorAs(t, err, &buildError)
				require.Equal(t, http.StatusConflict, buildError.Status)
				require.Nil(t, build)
				require.Nil(t, job)
			}
		})
	}
}

type startupExpectation struct {
	pending bool
	allowed bool
}

func expectNetworkPolicyStartup(t *testing.T, expectation startupExpectation) txExpect {
	t.Helper()
	return func(tx *dbmock.MockStore) {
		policy := database.NetworkPolicy{ID: uuid.New(), OrganizationID: orgID, PublishedRevisionID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
		target := database.NetworkPolicyTarget{
			ID: uuid.New(), PolicyID: policy.ID, OrganizationID: orgID, State: []byte("last applied state"), Status: "succeeded", Generation: 2,
			AppliedRevisionID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, DesiredRevisionID: policy.PublishedRevisionID,
			LastJobID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		}
		if expectation.pending {
			target.ActiveJobID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		}
		run := database.NetworkPolicyRun{
			ID: uuid.New(), JobID: target.LastJobID.UUID, PolicyID: policy.ID, TargetID: target.ID, OrganizationID: orgID,
			RevisionID: target.AppliedRevisionID.UUID, TemplateIDs: []uuid.UUID{templateID}, Generation: target.Generation, Operation: "apply",
		}
		tx.EXPECT().GetNetworkPoliciesByOrganizationID(gomock.Any(), orgID).DoAndReturn(func(ctx context.Context, _ uuid.UUID) ([]database.NetworkPolicy, error) {
			actor, ok := dbauthz.ActorFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, rbac.SubjectTypeSystemRestricted, actor.Type)
			return []database.NetworkPolicy{policy}, nil
		})
		tx.EXPECT().GetNetworkPolicyAttachmentsByPolicyID(gomock.Any(), policy.ID).Return([]database.NetworkPolicyAttachment{{PolicyID: policy.ID, TemplateID: templateID}}, nil)
		tx.EXPECT().GetNetworkPolicyByIDForUpdate(gomock.Any(), policy.ID).Return(policy, nil)
		tx.EXPECT().GetNetworkPolicyTargetsByPolicyID(gomock.Any(), policy.ID).Return([]database.NetworkPolicyTarget{target}, nil)
		tx.EXPECT().GetNetworkPolicyRunsByPolicyID(gomock.Any(), policy.ID).Return([]database.NetworkPolicyRun{run}, nil)
		tx.EXPECT().GetNetworkPolicyTargetByIDForUpdate(gomock.Any(), target.ID).Return(target, nil)
		if expectation.allowed {
			tx.EXPECT().GetProvisionerJobByID(gomock.Any(), run.JobID).Return(database.ProvisionerJob{
				ID: run.JobID, OrganizationID: orgID, Type: database.ProvisionerJobTypeNetworkPolicy,
				CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, WorkerID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			}, nil)
		}
	}
}
