package agentapi_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestBatcher_DB(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	mClock := quartz.NewMock(t)

	// Trap timer resets so we can synchronize with flush completion.
	resetTrap := mClock.Trap().TimerReset("connectionBatcher", "scheduledFlush")
	defer resetTrap.Close()

	// Build the full fixture chain required by foreign key constraints.
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	_, err := db.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
		OrganizationID: org.ID,
		UserID:         user.ID,
		Roles:          []string{codersdk.RoleOrganizationMember},
	})
	require.NoError(t, err)
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tpl := dbgen.Template(t, db, database.Template{
		CreatedBy:       user.ID,
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
		OrganizationID: org.ID,
	})
	pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		InitiatorID:    user.ID,
		OrganizationID: org.ID,
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		TemplateVersionID: tv.ID,
		WorkspaceID:       ws.ID,
		JobID:             pj.ID,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	agent1 := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: res.ID,
	})
	agent2 := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: res.ID,
	})

	b := agentapi.NewHeartbeatBatcher(ctx, db,
		agentapi.WithHeartbeatInterval(time.Second),
		agentapi.WithHeartbeatClock(mClock),
	)
	t.Cleanup(b.Close)

	now := mClock.Now()

	// Add first update for agent1.
	b.Add(agentapi.HeartbeatUpdate{
		ID: agent1.ID,
		LastConnectedAt: sql.NullTime{
			Time:  now,
			Valid: true,
		},
		UpdatedAt: now,
	})

	// Add a second (later) update for agent1 to test deduplication —
	// only the latest should be persisted.
	later := now.Add(500 * time.Millisecond)
	b.Add(agentapi.HeartbeatUpdate{
		ID: agent1.ID,
		LastConnectedAt: sql.NullTime{
			Time:  later,
			Valid: true,
		},
		UpdatedAt: later,
	})

	// Add an update for agent2 to verify the batch query works for
	// n > 1 agents.
	b.Add(agentapi.HeartbeatUpdate{
		ID: agent2.ID,
		LastConnectedAt: sql.NullTime{
			Time:  now,
			Valid: true,
		},
		UpdatedAt: now,
	})

	// Advance past the flush interval to trigger a batch write.
	mClock.Advance(time.Second).MustWait(ctx)
	resetTrap.MustWait(ctx).MustRelease(ctx)

	// Verify agent1 was updated with the latest value (deduplication).
	got1, err := db.GetWorkspaceAgentByID(ctx, agent1.ID)
	require.NoError(t, err)
	require.True(t, got1.LastConnectedAt.Valid)
	require.WithinDuration(t, later, got1.LastConnectedAt.Time, time.Millisecond)

	// Verify agent2 was also updated in the same batch.
	got2, err := db.GetWorkspaceAgentByID(ctx, agent2.ID)
	require.NoError(t, err)
	require.True(t, got2.LastConnectedAt.Valid)
	require.WithinDuration(t, now, got2.LastConnectedAt.Time, time.Millisecond)
}
