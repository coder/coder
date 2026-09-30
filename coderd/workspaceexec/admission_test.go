package workspaceexec_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/testutil"
)

func TestAdmissionSerializesWithPreservation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: org.ID, OwnerID: user.ID, TemplateID: template.ID})
	sessionID := uuid.New()
	_, err := sqlDB.ExecContext(ctx, `
  INSERT INTO workspace_execution_sessions (
   id, organization_id, owner_id, actor_id, request_id, input_digest,
   workspace_id, workspace_owner_id, created_at, updated_at, state,
   disposable, retained, lease_expires_at, declarations
  ) VALUES ($1,$2,$3,$3,$4,$5,$6,$3,NOW(),NOW(),'active',true,false,NOW(),'{}')
 `, sessionID, org.ID, user.ID, uuid.New(), make([]byte, 32), workspace.ID)
	require.NoError(t, err)
	require.NoError(t, db.InTx(func(tx database.Store) error {
		return workspaceexec.CheckAdmission(ctx, tx, workspace.ID)
	}, nil))

	preservation, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer preservation.Rollback()
	_, err = preservation.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", database.WorkspaceLifecycleLockID(workspace.ID))
	require.NoError(t, err)
	// The independent transaction cannot admit work until preservation commits.
	entered := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- db.InTx(func(tx database.Store) error {
			close(entered)
			return workspaceexec.CheckAdmission(ctx, tx, workspace.ID)
		}, nil)
	}()
	<-entered
	_, err = preservation.ExecContext(ctx, "UPDATE workspace_execution_sessions SET state='preserving', revision=revision+1 WHERE id=$1", sessionID)
	require.NoError(t, err)
	require.NoError(t, preservation.Commit())
	require.ErrorIs(t, <-result, workspaceexec.ErrAdmissionClosed)

	protected, err := db.HasProtectedWorkspaceExecutionSession(ctx, database.HasProtectedWorkspaceExecutionSessionParams{WorkspaceID: uuid.NullUUID{UUID: workspace.ID, Valid: true}, Now: time.Now()})
	require.NoError(t, err)
	require.True(t, protected, "legacy lifecycle cannot delete during preservation")
	_, err = sqlDB.ExecContext(ctx, "UPDATE workspace_execution_sessions SET state='retained', retained=true WHERE id=$1", sessionID)
	require.NoError(t, err)
	require.NoError(t, db.InTx(func(tx database.Store) error {
		return workspaceexec.CheckAdmission(ctx, tx, workspace.ID)
	}, nil), "explicit retention allows activity")
	protected, err = db.HasProtectedWorkspaceExecutionSession(ctx, database.HasProtectedWorkspaceExecutionSessionParams{WorkspaceID: uuid.NullUUID{UUID: workspace.ID, Valid: true}, Now: time.Now()})
	require.NoError(t, err)
	require.True(t, protected)
	_, err = sqlDB.ExecContext(ctx, "UPDATE workspace_execution_sessions SET state='completed' WHERE id=$1", sessionID)
	require.NoError(t, err)
	protected, err = db.HasProtectedWorkspaceExecutionSession(ctx, database.HasProtectedWorkspaceExecutionSessionParams{WorkspaceID: uuid.NullUUID{UUID: workspace.ID, Valid: true}, Now: time.Now()})
	require.NoError(t, err)
	require.True(t, protected, "explicit retention protects even an inconsistent completed state")
}

func TestAdmissionUnmanagedAndDeletedWorkspace(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	for _, deleted := range []bool{false, true} {
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: org.ID, OwnerID: user.ID, TemplateID: template.ID, Deleted: deleted})
		err := db.InTx(func(tx database.Store) error {
			return workspaceexec.CheckAdmission(context.Background(), tx, workspace.ID)
		}, nil)
		if deleted {
			require.ErrorIs(t, err, workspaceexec.ErrAdmissionClosed)
		} else {
			require.NoError(t, err)
		}
	}
}

// Successful adopted generations are snapshots, not permanent workspace locks.
