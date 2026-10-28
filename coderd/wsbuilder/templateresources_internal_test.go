package wsbuilder

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

func TestTemplateResourceSnapshot(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, status                         string
		absent, unbound, noRevision, pending bool
		published, emptyOutputs              bool
	}{
		{name: "Unbound", unbound: true},
		{name: "BoundWithoutState", absent: true},
		{name: "BoundWithoutRevision", status: "unconfigured", noRevision: true},
		{name: "UnappliedDraft", status: "unconfigured"},
		{name: "InitialApplyActive", status: "unconfigured", pending: true},
		{name: "Applied", status: "applied", published: true},
		{name: "AppliedWithoutOutputs", status: "applied", published: true, emptyOutputs: true},
		{name: "MutationActive", status: "applied", pending: true, published: true},
		{name: "Failed", status: "failed", published: true},
		{name: "FailedWithoutPublishedOutputs", status: "failed"},
		{name: "Unknown", status: "unknown", published: true},
		{name: "UnknownWithoutPublishedOutputs", status: "unknown"},
		{name: "Destroyed", status: "destroyed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := dbmock.NewMockStore(gomock.NewController(t))
			templateID, foundationID, appliedID := uuid.New(), uuid.New(), uuid.New()
			db.EXPECT().LockTemplateResourceTemplate(gomock.Any(), templateID).Return(nil)
			state := database.TemplateResourceState{FoundationID: foundationID, CurrentRevisionID: uuid.NullUUID{UUID: uuid.New(), Valid: !tc.noRevision}, AppliedRevisionID: uuid.NullUUID{UUID: appliedID, Valid: tc.published}, Status: tc.status, Outputs: json.RawMessage(`{"list":[1,true,"value"]}`)}
			if tc.emptyOutputs {
				state.Outputs = nil
			}
			if tc.pending {
				state.ActiveRunID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
			}
			var stateErr error
			if tc.absent {
				stateErr = sql.ErrNoRows
			}
			if tc.unbound {
				db.EXPECT().GetFoundationByTemplateID(gomock.Any(), uuid.NullUUID{UUID: templateID, Valid: true}).Return(database.Foundation{}, sql.ErrNoRows)
			} else {
				db.EXPECT().GetFoundationByTemplateID(gomock.Any(), uuid.NullUUID{UUID: templateID, Valid: true}).Return(database.Foundation{ID: foundationID}, nil)
				db.EXPECT().LockFoundation(gomock.Any(), foundationID).Return(nil)
				db.EXPECT().GetTemplateResourceStateForUpdate(gomock.Any(), foundationID).Return(state, stateErr)
			}
			b := New(database.Workspace{TemplateID: templateID}, database.WorkspaceTransitionStart, NoopUsageChecker{})
			b.ctx, b.store = t.Context(), db
			revision, outputs, bound, err := b.templateResourceSnapshot()
			require.NoError(t, err)
			require.Equal(t, !tc.unbound, bound)
			if !tc.published {
				require.Equal(t, uuid.NullUUID{}, revision)
				require.JSONEq(t, `{}`, string(outputs))
			} else {
				require.True(t, revision.Valid)
				require.Equal(t, appliedID, revision.UUID)
				if tc.emptyOutputs {
					require.JSONEq(t, `{}`, string(outputs))
				} else {
					require.JSONEq(t, string(state.Outputs), string(outputs))
				}
				require.NotEqual(t, state.CurrentRevisionID.UUID, revision.UUID, "pin applied revision, not latest source")
			}
		})
	}
	t.Run("EachStartSelectsPublishedOutputs", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		templateID, foundationID, revisionID := uuid.New(), uuid.New(), uuid.New()
		db.EXPECT().LockTemplateResourceTemplate(gomock.Any(), templateID).Return(nil).Times(3)
		db.EXPECT().GetFoundationByTemplateID(gomock.Any(), uuid.NullUUID{UUID: templateID, Valid: true}).Return(database.Foundation{ID: foundationID}, nil).Times(3)
		db.EXPECT().LockFoundation(gomock.Any(), foundationID).Return(nil).Times(3)
		// Applying the same source again can publish different output values.
		gomock.InOrder(
			db.EXPECT().GetTemplateResourceStateForUpdate(gomock.Any(), foundationID).Return(database.TemplateResourceState{}, nil),
			db.EXPECT().GetTemplateResourceStateForUpdate(gomock.Any(), foundationID).Return(database.TemplateResourceState{
				AppliedRevisionID: uuid.NullUUID{UUID: revisionID, Valid: true}, Outputs: json.RawMessage(`{"deployment":"first"}`),
			}, nil),
			db.EXPECT().GetTemplateResourceStateForUpdate(gomock.Any(), foundationID).Return(database.TemplateResourceState{
				AppliedRevisionID: uuid.NullUUID{UUID: revisionID, Valid: true}, Outputs: json.RawMessage(`{"deployment":"second"}`),
			}, nil),
		)
		var snapshots []json.RawMessage
		for i := range 3 {
			b := New(database.Workspace{TemplateID: templateID}, database.WorkspaceTransitionStart, NoopUsageChecker{})
			b.ctx, b.store = t.Context(), db
			revision, outputs, bound, err := b.templateResourceSnapshot()
			require.NoError(t, err)
			require.True(t, bound)
			require.Equal(t, i > 0, revision.Valid)
			if revision.Valid {
				require.Equal(t, revisionID, revision.UUID)
			}
			snapshots = append(snapshots, outputs)
		}
		require.JSONEq(t, `{}`, string(snapshots[0]))
		require.JSONEq(t, `{"deployment":"first"}`, string(snapshots[1]))
		require.JSONEq(t, `{"deployment":"second"}`, string(snapshots[2]))
	})
	for _, transition := range []database.WorkspaceTransition{database.WorkspaceTransitionStop, database.WorkspaceTransitionDelete} {
		t.Run(string(transition), func(t *testing.T) {
			t.Parallel()
			// Cleanup never consults current bindings, even without a pinned revision.
			for _, pinned := range []bool{false, true} {
				db := dbmock.NewMockStore(gomock.NewController(t))
				b := New(database.Workspace{TemplateID: uuid.New()}, transition, NoopUsageChecker{})
				b.ctx, b.store = t.Context(), db
				b.lastBuild = &database.WorkspaceBuild{
					TemplateResourceRevisionID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
					TemplateResourceOutputs:    json.RawMessage(`{"failed_attempt":true}`),
				}
				expectedOutputs := `{}`
				if pinned {
					b.lastBuild.StateTemplateResourceRevisionID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
					b.lastBuild.StateTemplateResourceOutputs = json.RawMessage(`{"old":true}`)
					expectedOutputs = string(b.lastBuild.StateTemplateResourceOutputs)
				}
				revision, outputs, bound, err := b.templateResourceSnapshot()
				require.NoError(t, err)
				require.False(t, bound, "cleanup does not consult current bindings")
				require.Equal(t, b.lastBuild.StateTemplateResourceRevisionID, revision)
				require.JSONEq(t, expectedOutputs, string(outputs))
			}
		})
	}
}
