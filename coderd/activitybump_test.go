package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceActivityBump(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// deadline allows you to forcibly set a max_deadline on the build. This
	// doesn't use template autostop requirements and instead edits the
	// max_deadline on the build directly in the database.
	setupActivityTest := func(t *testing.T, deadline ...time.Duration) (client *codersdk.Client, workspace codersdk.Workspace, assertBumped func(want bool)) {
		t.Helper()
		const ttl = time.Hour

		db, pubsub := dbtestutil.NewDB(t)
		client = coderdtest.New(t, &coderdtest.Options{
			Database:                 db,
			Pubsub:                   pubsub,
			IncludeProvisionerDaemon: true,
			// Agent stats trigger the activity bump, so we want to report
			// very frequently in tests.
			AgentStatsRefreshInterval: time.Millisecond * 100,
			TemplateScheduleStore: schedule.MockTemplateScheduleStore{
				GetFn: func(ctx context.Context, db database.Store, templateID uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					return schedule.TemplateScheduleOptions{
						UserAutostopEnabled: true,
						DefaultTTL:          ttl,
						// We set max_deadline manually below.
						AutostopRequirement: schedule.TemplateAutostopRequirement{},
					}, nil
				},
			},
		})
		user := coderdtest.CreateFirstUser(t, client)

		ttlMillis := int64(ttl / time.Millisecond)
		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(agentToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace = coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = &ttlMillis
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		var maxDeadline time.Time
		// Update the max deadline.
		if len(deadline) > 0 {
			maxDeadline = dbtime.Now().Add(deadline[0])
		}

		err := db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
			ID:        workspace.LatestBuild.ID,
			UpdatedAt: dbtime.Now(),
			// Make the deadline really close so it needs to be bumped immediately.
			Deadline:    dbtime.Now().Add(time.Minute),
			MaxDeadline: maxDeadline,
		})
		require.NoError(t, err)

		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Sanity-check that deadline is nearing requiring a bump.
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.WithinDuration(t,
			time.Now().Add(time.Minute),
			workspace.LatestBuild.Deadline.Time,
			testutil.WaitMedium,
		)
		firstDeadline := workspace.LatestBuild.Deadline.Time

		if !maxDeadline.IsZero() {
			require.WithinDuration(t,
				maxDeadline,
				workspace.LatestBuild.MaxDeadline.Time,
				testutil.WaitMedium,
			)
		} else {
			require.True(t, workspace.LatestBuild.MaxDeadline.Time.IsZero())
		}

		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		return client, workspace, func(want bool) {
			t.Helper()
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

			// maxTimeDrift is how long we are willing wait for a deadline to
			// be increased. Since it could have been bumped at the initial
			maxTimeDrift := testutil.WaitMedium

			updatedAfter := dbtime.Now()
			// waitedFor is purely for debugging failed tests. If a test fails,
			// it helps to know how long it took for the deadline bump to be
			// detected. The longer this takes, the more likely time drift will
			// affect the results.
			waitedFor := time.Now()
			// lastChecked is for logging within the Eventually loop.
			// Debouncing log lines to every second to prevent spam.
			lastChecked := time.Time{}
			// checks is for keeping track of the average check time.
			// If CI is running slow, this could be useful to know checks
			// are taking longer than expected.
			checks := 0

			// The Deadline bump occurs asynchronously.
			require.Eventuallyf(t,
				func() bool {
					checks++
					workspace, err = client.Workspace(ctx, workspace.ID)
					require.NoError(t, err)

					hasBumped := !workspace.LatestBuild.Deadline.Time.Equal(firstDeadline)

					// Always make sure to log this information, even on the last check.
					// The last check is the most important, as if this loop is acting
					// slow, the last check could be the cause of the failure.
					if time.Since(lastChecked) > time.Second || hasBumped {
						avgCheckTime := time.Since(waitedFor) / time.Duration(checks)
						t.Logf("deadline detect: bumped=%t since_last_check=%s avg_check_dur=%s checks=%d deadline=%v",
							hasBumped, time.Since(updatedAfter), avgCheckTime, checks, workspace.LatestBuild.Deadline.Time)
						lastChecked = time.Now()
					}

					updatedAfter = dbtime.Now()
					return hasBumped
				},
				//nolint: gocritic // maxTimeDrift is a testutil time
				maxTimeDrift, testutil.IntervalFast,
				"deadline %v never updated", firstDeadline,
			)

			// This log line helps establish how long it took for the deadline
			// to be detected as bumped.
			t.Logf("deadline bump detected: %v, waited for %s",
				workspace.LatestBuild.Deadline.Time,
				time.Since(waitedFor),
			)

			require.Greater(t, workspace.LatestBuild.Deadline.Time, updatedAfter)

			// If the workspace has a max deadline, the deadline must not exceed
			// it.
			if workspace.LatestBuild.MaxDeadline.Valid {
				require.LessOrEqual(t, workspace.LatestBuild.Deadline.Time, workspace.LatestBuild.MaxDeadline.Time)
				return
			}
			now := dbtime.Now()
			zone, offset := time.Now().Zone()
			t.Logf("[Zone=%s %d] originDeadline: %s, deadline: %s, now %s, (now-deadline)=%s",
				zone, offset,
				firstDeadline, workspace.LatestBuild.Deadline.Time, now,
				now.Sub(workspace.LatestBuild.Deadline.Time),
			)
			require.WithinDuration(t, now.Add(ttl), workspace.LatestBuild.Deadline.Time, maxTimeDrift)
		}
	}

	t.Run("Dial", func(t *testing.T) {
		t.Parallel()

		client, workspace, assertBumped := setupActivityTest(t)

		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		conn, err := workspacesdk.New(client).
			DialAgent(ctx, resources[0].Agents[0].ID, &workspacesdk.DialAgentOptions{
				Logger: testutil.Logger(t),
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

		// Set the max deadline to be in 30min. We bump by 1 hour, so we
		// should expect the deadline to match the max deadline exactly.
		client, workspace, assertBumped := setupActivityTest(t, time.Minute*30)

		// Bump by dialing the workspace and sending traffic.
		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		conn, err := workspacesdk.New(client).
			DialAgent(ctx, resources[0].Agents[0].ID, &workspacesdk.DialAgentOptions{
				Logger: testutil.Logger(t),
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
}
