package workspaceexec_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestControllerUncertainSubmissionDoesNotBlockCleanup(t *testing.T) {
	t.Parallel()
	f := newControllerFixture(t)
	session := f.session(t, nil, nil)
	submissionID := uuid.New()
	_, err := f.sqlDB.ExecContext(t.Context(), `INSERT INTO chat_submissions
 (id,organization_id,actor_id,owner_id,request_id,input_digest,kind,chat_id,state)
 VALUES($1,$2,$3,$3,$4,$5,'create',$6,'uncertain')`,
		submissionID, session.OrganizationID, session.OwnerID, uuid.New(), make([]byte, 32), uuid.New())
	require.NoError(t, err)
	f.controller.Start(t.Context())

	require.Eventually(t, func() bool { f.tick(t); return f.current(t, session.ID).State == "completed" }, testutil.WaitLong, testutil.IntervalFast)
	var journalState string
	require.NoError(t, f.sqlDB.QueryRowContext(t.Context(), "SELECT state FROM chat_submissions WHERE id=$1", submissionID).Scan(&journalState))
	require.Equal(t, "uncertain", journalState, "cleanup must not invent an admission outcome")
	workspace, err := f.db.GetWorkspaceByID(t.Context(), f.workspace.ID)
	require.NoError(t, err)
	require.True(t, workspace.Deleted)
}
