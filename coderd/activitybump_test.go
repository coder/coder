package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestWorkspaceActivityBump(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	const ttl = time.Minute

	setupActivityTest := func(t *testing.T) (client *codersdk.Client, workspace codersdk.Workspace, assertBumped func(want bool)) {
		ttlMillis := int64(ttl / time.Millisecond)

		client = coderdtest.New(t, &coderdtest.Options{
			AppHostname:              proxyTestSubdomainRaw,
			IncludeProvisionerDaemon: true,
			// Agent stats trigger the activity bump, so we want to report
			// very frequently in tests.
			AgentStatsRefreshInterval: time.Millisecond * 100,
		})
		user := coderdtest.CreateFirstUser(t, client)

		workspace = createWorkspaceWithApps(t, client, user.OrganizationID, "", 1234, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = &ttlMillis
		})

		// Sanity-check that deadline is near.
		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.WithinDuration(t,
			time.Now().Add(time.Duration(ttlMillis)*time.Millisecond),
			workspace.LatestBuild.Deadline.Time, testutil.WaitMedium,
		)
		firstDeadline := workspace.LatestBuild.Deadline.Time

		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		return client, workspace, func(want bool) {
			if !want {
				// It is difficult to test the absence of a call in a non-racey
				// way. In general, it is difficult for the API to generate
				// false positive activity since Agent networking event
				// is required. The Activity Bump behavior is also coupled with
				// Last Used, so it would be obvious to the user if we
				// are falsely recognizing activity.
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
				testutil.WaitLong, testutil.IntervalFast,
				"deadline %v never updated", firstDeadline,
			)

			require.WithinDuration(t, database.Now().Add(ttl), workspace.LatestBuild.Deadline.Time, 3*time.Second)
		}
	}

	t.Run("Dial", func(t *testing.T) {
		t.Parallel()

		client, workspace, assertBumped := setupActivityTest(t)

		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
			Logger: slogtest.Make(t, nil),
		})
		require.NoError(t, err)
		defer conn.Close()

		// Must send network traffic after a few seconds to surpass bump threshold.
		time.Sleep(time.Second * 3)
		sshConn, err := conn.SSHClient(ctx)
		require.NoError(t, err)
		_ = sshConn.Close()

		assertBumped(true)
	})

	t.Run("NoBump", func(t *testing.T) {
		t.Parallel()

		client, workspace, assertBumped := setupActivityTest(t)

		// Benign operations like retrieving workspace must not
		// bump the deadline.
		_, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)

		assertBumped(false)
	})
}
