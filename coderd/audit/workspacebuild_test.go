package audit_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

// TestWorkspaceBuildFields checks that an entry whose secrets could not be
// listed is marked, so it is not mistaken for a build without secrets.
func TestWorkspaceBuildFields(t *testing.T) {
	t.Parallel()

	workspace := database.Workspace{ID: uuid.New(), Name: "ws"}
	build := database.WorkspaceBuild{ID: uuid.New(), WorkspaceID: workspace.ID, BuildNumber: 3}

	t.Run("NoSecrets", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetWorkspaceSecretsHistory(gomock.Any(), gomock.Any()).Return(nil, nil)

		fields, err := audit.WorkspaceBuildFields(ctx, db, workspace, build)
		require.NoError(t, err)
		require.Empty(t, fields.WorkspaceSecrets)
		require.Empty(t, fields.WorkspaceSecretsError)
	})

	t.Run("Unavailable", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetWorkspaceSecretsHistory(gomock.Any(), gomock.Any()).Return(nil, xerrors.New("boom"))

		fields, err := audit.WorkspaceBuildFields(ctx, db, workspace, build)
		require.Error(t, err)
		require.Equal(t, "3", fields.BuildNumber, "other fields are still returned")
		require.Empty(t, fields.WorkspaceSecrets)
		require.Equal(t, audit.WorkspaceSecretsUnavailable, fields.WorkspaceSecretsError)
	})
}
