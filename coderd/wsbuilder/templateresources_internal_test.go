package wsbuilder

import (
	"database/sql"
	"encoding/json"
	"net/http"
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
		name, status             string
		absent, pending, allowed bool
	}{
		{name: "NoRoot", absent: true, allowed: true},
		{name: "InitialApplyRequired", status: "unconfigured"},
		{name: "Applied", status: "applied", allowed: true},
		{name: "MutationActive", status: "applied", pending: true},
		{name: "Failed", status: "failed"},
		{name: "Unknown", status: "unknown"},
		{name: "Destroyed", status: "destroyed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := dbmock.NewMockStore(gomock.NewController(t))
			templateID, appliedID := uuid.New(), uuid.New()
			db.EXPECT().LockTemplateResourceTemplate(gomock.Any(), templateID).Return(nil)
			state := database.TemplateResourceState{TemplateID: templateID, CurrentRevisionID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, AppliedRevisionID: uuid.NullUUID{UUID: appliedID, Valid: tc.status == "applied"}, Status: tc.status, Outputs: json.RawMessage(`{"list":[1,true,"value"]}`)}
			if tc.pending {
				state.ActiveRunID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
			}
			var stateErr error
			if tc.absent {
				stateErr = sql.ErrNoRows
			}
			db.EXPECT().GetTemplateResourceStateForUpdate(gomock.Any(), templateID).Return(state, stateErr)
			b := New(database.Workspace{TemplateID: templateID}, database.WorkspaceTransitionStart, NoopUsageChecker{})
			b.ctx, b.store = t.Context(), db
			revision, outputs, err := b.templateResourceSnapshot()
			if !tc.allowed {
				var buildErr BuildError
				require.ErrorAs(t, err, &buildErr)
				require.Equal(t, http.StatusConflict, buildErr.Status)
				return
			}
			require.NoError(t, err)
			if tc.absent {
				require.False(t, revision.Valid)
				require.JSONEq(t, `{}`, string(outputs))
			} else {
				require.Equal(t, appliedID, revision.UUID)
				require.JSONEq(t, string(state.Outputs), string(outputs))
				require.NotEqual(t, state.CurrentRevisionID.UUID, revision.UUID, "pin applied revision, not latest source")
			}
		})
	}
	for _, transition := range []database.WorkspaceTransition{database.WorkspaceTransitionStop, database.WorkspaceTransitionDelete} {
		t.Run(string(transition), func(t *testing.T) {
			t.Parallel()
			// No template reads are allowed for cleanup, including unknown/pending roots.
			db := dbmock.NewMockStore(gomock.NewController(t))
			b := New(database.Workspace{TemplateID: uuid.New()}, transition, NoopUsageChecker{})
			b.ctx, b.store = t.Context(), db
			b.lastBuild = &database.WorkspaceBuild{TemplateResourceRevisionID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, TemplateResourceOutputs: json.RawMessage(`{"old":true}`)}
			revision, outputs, err := b.templateResourceSnapshot()
			require.NoError(t, err)
			require.Equal(t, b.lastBuild.TemplateResourceRevisionID, revision)
			require.JSONEq(t, string(b.lastBuild.TemplateResourceOutputs), string(outputs))
		})
	}
}
