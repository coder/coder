package workspaceexec_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceexec"
)

func TestControllerSchedulesActionableBeforeParked(t *testing.T) {
	t.Parallel()
	f := newControllerFixture(t)
	session := f.session(t, nil, nil)
	target := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
	_, err := f.sqlDB.ExecContext(t.Context(), "UPDATE workspace_execution_sessions SET id=$1, lease_expires_at=$2 WHERE id=$3", target, f.clock.Now().Add(time.Hour), session.ID)
	require.NoError(t, err)
	session = f.current(t, target)
	session, err = workspaceexec.Retain(t.Context(), f.db, target, session.Revision, f.clock.Now())
	require.NoError(t, err)
	for n := 1; n <= 101; n++ {
		_, err = f.sqlDB.ExecContext(t.Context(), `INSERT INTO workspace_execution_sessions
 (id,organization_id,owner_id,actor_id,request_id,input_digest,workspace_id,workspace_owner_id,created_at,updated_at,lease_expires_at,state,declarations,disposable,retained)
 VALUES($1,$2,$3,$3,$4,$5,$6,$3,$7,$7,$7,'preserved','{}',false,false)`,
			uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012x", n)), session.OrganizationID, session.OwnerID, uuid.New(), make([]byte, 32), f.workspace.ID, f.clock.Now())
		require.NoError(t, err)
	}
	receipt, err := workspaceexec.Start(t.Context(), f.db, f.agent, workspaceexec.StartInput{
		SessionID: target, ActorID: session.ActorID, RequestID: uuid.New(), AgentID: f.agentID, Command: "exec sleep 600",
	}, f.clock.Now())
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = workspaceexec.Cancel(context.Background(), f.db, f.agent, receipt, 1000, f.clock.Now()) })
	dial := f.controller.DialAgent
	f.controller.DialAgent = func(ctx context.Context, id uuid.UUID) (workspaceexec.ControllerAgent, func(), error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 5*time.Second, "offline agent observation must not consume the whole reconciliation budget")
		return dial(ctx, id)
	}
	f.controller.Start(t.Context())
	f.tick(t)
	observed, err := f.db.GetWorkspaceExecutionReceiptByID(t.Context(), receipt.ID)
	require.NoError(t, err)
	require.True(t, observed.UpdatedAt.After(receipt.UpdatedAt), "actionable work must not wait behind a full page of parked rows")
	require.Equal(t, "running", observed.State)
	parkedID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	parked := f.current(t, parkedID)
	require.True(t, parked.NextRetryAt.Valid)
	require.GreaterOrEqual(t, parked.NextRetryAt.Time.Sub(parked.UpdatedAt), 5*time.Minute)
	f.tick(t)
	require.Equal(t, parked, f.current(t, parkedID), "unchanged parked rows must not be rewritten every tick")
	last := f.current(t, uuid.MustParse("00000000-0000-0000-0000-000000000065"))
	require.Equal(t, "preserved", last.State)
	require.True(t, last.NextRetryAt.Valid, "quiet maintenance must still advance beyond its first page")
}
