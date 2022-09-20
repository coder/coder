package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestWorkspaceActivityBump(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	setupActivityTest := func(t *testing.T) (client *codersdk.Client, workspace codersdk.Workspace, assertBumped func(want bool)) {
		var ttlMillis int64 = 60 * 1000

		client, _, workspace, _ = setupProxyTest(t, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = &ttlMillis
		})

		// Sanity-check that deadline is near.
		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.WithinDuration(t,
			time.Now().Add(time.Duration(ttlMillis)*time.Millisecond),
			workspace.LatestBuild.Deadline.Time, testutil.WaitShort,
		)
		firstDeadline := workspace.LatestBuild.Deadline.Time

		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)

		return client, workspace, func(want bool) {
			if !want {
				time.Sleep(testutil.IntervalMedium)
				workspace, err = client.Workspace(ctx, workspace.ID)
				require.NoError(t, err)
				require.Equal(t, workspace.LatestBuild.Deadline.Time, firstDeadline)
				return
			}

			// The Deadline bump occurs asynchronously.
			require.Eventuallyf(t,
				func() bool {
					workspace, err = client.Workspace(ctx, workspace.ID)
					require.NoError(t, err)
					return workspace.LatestBuild.Deadline.Time != firstDeadline
				},
				testutil.WaitShort, testutil.IntervalFast,
				"deadline %v never updated", firstDeadline,
			)

			require.WithinDuration(t, time.Now().Add(time.Hour), workspace.LatestBuild.Deadline.Time, time.Second)
		}
	}

	t.Run("Dial", func(t *testing.T) {
		t.Parallel()

		client, workspace, assertBumped := setupActivityTest(t)

		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
		conn, err := client.DialWorkspaceAgentTailnet(ctx, slogtest.Make(t, nil), resources[0].Agents[0].ID)
		require.NoError(t, err)
		defer conn.Close()

		sshConn, err := conn.SSHClient()
		require.NoError(t, err)
		_ = sshConn.Close()

		assertBumped(true)
	})

	t.Run("NoBump", func(t *testing.T) {
		t.Parallel()

		client, workspace, assertBumped := setupActivityTest(t)

		// Doing some inactive operation like retrieving resources must not
		// bump the deadline.
		_, err := client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)

		assertBumped(false)
	})
}
