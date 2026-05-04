package agentfake_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/agentfake"
	"github.com/coder/coder/v2/testutil"
)

// Assert that our fake agent routine establishes the drpc connection and sets its lifecycle status to Ready.
func TestAgent_ConnectsAndReachesReady(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	a := agentfake.NewAgent(client.URL, r.AgentToken, logger)
	t.Cleanup(func() { _ = a.Close() })

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	runErr := make(chan error, 1)
	go func() {
		runErr <- a.Run(runCtx)
	}()

	coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
		WithContext(ctx).
		Wait()

	require.Eventually(t, func() bool {
		ws, err := client.Workspace(ctx, r.Workspace.ID)
		if err != nil {
			return false
		}
		for _, res := range ws.LatestBuild.Resources {
			for _, agent := range res.Agents {
				if agent.LifecycleState != codersdk.WorkspaceAgentLifecycleReady {
					return false
				}
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast,
		"agent never reached Lifecycle=ready in workspace %s", r.Workspace.ID)

	// Cancel Run and confirm a clean exit (nil error, not ctx error).
	cancel()
	select {
	case err := <-runErr:
		require.NoError(t, err, "Agent.Run returned unexpected error")
	case <-ctx.Done():
		t.Fatalf("timed out waiting for Agent.Run to return: %v", ctx.Err())
	}

	// Close is idempotent and safe to call after Run returns.
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
}
