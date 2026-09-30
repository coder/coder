package autobuild_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/testutil"
)

func TestExecutionProtectionPreservesAutostopAndDormancy(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"active", "preserving", "preservation_failed", "retained", "adopted_active", "adopted_preserving", "adopted_preservation_failed", "adopted_retained"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			disposable := !strings.HasPrefix(state, "adopted_")
			state = strings.TrimPrefix(state, "adopted_")
			db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
			tickCh, statsCh := make(chan time.Time), make(chan autobuild.Stats)
			client := coderdtest.New(t, &coderdtest.Options{
				Database: db, Pubsub: ps, AutobuildTicker: tickCh, AutobuildStats: statsCh, IncludeProvisionerDaemon: true,
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					GetFn: func(context.Context, database.Store, uuid.UUID) (schedule.TemplateScheduleOptions, error) {
						return schedule.TemplateScheduleOptions{UserAutostopEnabled: true, DefaultTTL: time.Hour, TimeTilDormant: 2 * time.Hour, TimeTilDormantAutoDelete: time.Hour}, nil
					},
				},
			})
			workspace := mustProvisionWorkspace(t, client)
			_, err := sqlDB.ExecContext(t.Context(), `INSERT INTO workspace_execution_sessions
 (id,organization_id,owner_id,actor_id,request_id,input_digest,workspace_id,workspace_owner_id,created_at,updated_at,lease_expires_at,state,declarations,disposable,retained)
 VALUES($1,$2,$3,$3,$4,$5,$6,$3,now(),now(),now(),$7,'{"result_paths":["/required"]}',$9,$8)`,
				uuid.New(), workspace.OrganizationID, workspace.OwnerID, uuid.New(), make([]byte, 32), workspace.ID, state, state == "retained", disposable)
			require.NoError(t, err)
			p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
			require.NoError(t, err)
			tick := workspace.LatestBuild.Deadline.Time.Add(time.Minute)
			coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tick)
			tickCh <- tick
			stats := testutil.RequireReceive(t.Context(), t, statsCh)
			require.Empty(t, stats.Errors)
			require.Equal(t, database.WorkspaceTransitionStop, stats.Transitions[workspace.ID])
			stopped := coderdtest.MustWorkspace(t, client, workspace.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, stopped.LatestBuild.ID)
			// Existing dormancy still runs, while its later destructive transition is fenced.
			tick = workspace.LastUsedAt.Add(3 * time.Hour)
			coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tick)
			tickCh <- tick
			stats = testutil.RequireReceive(t.Context(), t, statsCh)
			require.Empty(t, stats.Errors)
			dormant, err := db.GetWorkspaceByID(t.Context(), workspace.ID)
			require.NoError(t, err)
			require.True(t, dormant.DormantAt.Valid)
			_, err = sqlDB.ExecContext(t.Context(), "UPDATE workspaces SET deleting_at=$1 WHERE id=$2", tick.Add(-time.Minute), workspace.ID)
			require.NoError(t, err)
			tickCh <- tick.Add(time.Minute)
			stats = testutil.RequireReceive(t.Context(), t, statsCh)
			require.Empty(t, stats.Errors)
			require.Empty(t, stats.Transitions)
			current, err := db.GetWorkspaceByID(t.Context(), workspace.ID)
			require.NoError(t, err)
			require.False(t, current.Deleted)
		})
	}
}
