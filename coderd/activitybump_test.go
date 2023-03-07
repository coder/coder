package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/schedule"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

type mockTemplateScheduleStore struct {
	getFn func(ctx context.Context, db database.Store, templateID uuid.UUID) (schedule.TemplateScheduleOptions, error)
	setFn func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error)
}

var _ schedule.TemplateScheduleStore = mockTemplateScheduleStore{}

func (m mockTemplateScheduleStore) GetTemplateScheduleOptions(ctx context.Context, db database.Store, templateID uuid.UUID) (schedule.TemplateScheduleOptions, error) {
	if m.getFn != nil {
		return m.getFn(ctx, db, templateID)
	}

	return schedule.NewAGPLTemplateScheduleStore().GetTemplateScheduleOptions(ctx, db, templateID)
}

func (m mockTemplateScheduleStore) SetTemplateScheduleOptions(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
	if m.setFn != nil {
		return m.setFn(ctx, db, template, options)
	}

	return schedule.NewAGPLTemplateScheduleStore().SetTemplateScheduleOptions(ctx, db, template, options)
}

func TestWorkspaceActivityBump(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	setupActivityTest := func(t *testing.T, maxDeadline ...time.Duration) (client *codersdk.Client, workspace codersdk.Workspace, assertBumped func(want bool)) {
		const ttl = time.Minute
		maxTTL := time.Duration(0)
		if len(maxDeadline) > 0 {
			maxTTL = maxDeadline[0]
		}

		client = coderdtest.New(t, &coderdtest.Options{
			AppHostname:              proxyTestSubdomainRaw,
			IncludeProvisionerDaemon: true,
			// Agent stats trigger the activity bump, so we want to report
			// very frequently in tests.
			AgentStatsRefreshInterval: time.Millisecond * 100,
			TemplateScheduleStore: mockTemplateScheduleStore{
				getFn: func(ctx context.Context, db database.Store, templateID uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					return schedule.TemplateScheduleOptions{
						UserSchedulingEnabled: true,
						DefaultTTL:            ttl,
						MaxTTL:                maxTTL,
					}, nil
				},
			},
		})
		user := coderdtest.CreateFirstUser(t, client)

		ttlMillis := int64(ttl / time.Millisecond)
		workspace = createWorkspaceWithApps(t, client, user.OrganizationID, "", 1234, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = &ttlMillis
		})

		// Sanity-check that deadline is near.
		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.WithinDuration(t,
			time.Now().Add(time.Duration(ttlMillis)*time.Millisecond),
			workspace.LatestBuild.Deadline.Time,
			testutil.WaitMedium,
		)
		firstDeadline := workspace.LatestBuild.Deadline.Time

		if maxTTL != 0 {
			require.WithinDuration(t,
				time.Now().Add(maxTTL),
				workspace.LatestBuild.MaxDeadline.Time,
				testutil.WaitMedium,
			)
		} else {
			require.True(t, workspace.LatestBuild.MaxDeadline.Time.IsZero())
		}

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

			// If the workspace has a max deadline, the deadline must not exceed
			// it.
			if maxTTL != 0 && database.Now().Add(ttl).After(workspace.LatestBuild.MaxDeadline.Time) {
				require.Equal(t, workspace.LatestBuild.Deadline.Time, workspace.LatestBuild.MaxDeadline.Time)
				return
			}
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

	t.Run("NotExceedMaxDeadline", func(t *testing.T) {
		t.Parallel()

		// Set the max deadline to be in 61 seconds. We bump by 1 minute, so we
		// should expect the deadline to match the max deadline exactly.
		client, workspace, assertBumped := setupActivityTest(t, 61*time.Second)

		// Bump by dialing the workspace and sending traffic.
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

		// Double check that the workspace build's deadline is equal to the
		// max deadline.
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.Deadline.Time, workspace.LatestBuild.MaxDeadline.Time)
	})
}
