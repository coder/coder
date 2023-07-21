package coderd_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/testutil"
)

func TestDeploymentInsights(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon:    true,
		AgentStatsRefreshInterval:   time.Millisecond * 100,
		MetricsCacheRefreshInterval: time.Millisecond * 100,
	})

	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.ProvisionComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	require.Empty(t, template.BuildTimeStats[codersdk.WorkspaceTransitionStart])

	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Logger: slogtest.Make(t, nil),
		Client: agentClient,
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	daus, err := client.DeploymentDAUs(context.Background(), codersdk.TimezoneOffsetHour(time.UTC))
	require.NoError(t, err)

	res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	assert.NotZero(t, res.Workspaces[0].LastUsedAt)

	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
		Logger: slogtest.Make(t, nil).Named("tailnet"),
	})
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	sshConn, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	_ = sshConn.Close()

	wantDAUs := &codersdk.DAUsResponse{
		Entries: []codersdk.DAUEntry{
			{
				Date:   time.Now().UTC().Truncate(time.Hour * 24),
				Amount: 1,
			},
		},
	}
	require.Eventuallyf(t, func() bool {
		daus, err = client.DeploymentDAUs(ctx, codersdk.TimezoneOffsetHour(time.UTC))
		require.NoError(t, err)
		return len(daus.Entries) > 0
	},
		testutil.WaitShort, testutil.IntervalFast,
		"deployment daus never loaded",
	)
	gotDAUs, err := client.DeploymentDAUs(ctx, codersdk.TimezoneOffsetHour(time.UTC))
	require.NoError(t, err)
	require.Equal(t, gotDAUs, wantDAUs)

	template, err = client.Template(ctx, template.ID)
	require.NoError(t, err)

	res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
}

func TestUserLatencyInsights(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon:  true,
		AgentStatsRefreshInterval: time.Millisecond * 100,
	})

	user := coderdtest.CreateFirstUser(t, client)
	_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.ProvisionComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	require.Empty(t, template.BuildTimeStats[codersdk.WorkspaceTransitionStart])

	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Logger: logger.Named("agent"),
		Client: agentClient,
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
		Logger: logger.Named("client"),
	})
	require.NoError(t, err)
	defer conn.Close()

	sshConn, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshConn.Close()

	// Create users that will not appear in the report.
	_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	_, user4 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	_, err = client.UpdateUserStatus(ctx, user3.Username, codersdk.UserStatusSuspended)
	require.NoError(t, err)
	err = client.DeleteUser(ctx, user4.ID)
	require.NoError(t, err)

	y, m, d := time.Now().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	_ = sshConn.Close()

	var userLatencies codersdk.UserLatencyInsightsResponse
	require.Eventuallyf(t, func() bool {
		userLatencies, err = client.UserLatencyInsights(ctx, codersdk.UserLatencyInsightsRequest{
			StartTime:   today,
			EndTime:     time.Now().UTC().Truncate(time.Hour).Add(time.Hour), // Round up to include the current hour.
			TemplateIDs: []uuid.UUID{template.ID},
		})
		if !assert.NoError(t, err) {
			return false
		}
		if userLatencies.Report.Users[0].UserID == user2.ID {
			userLatencies.Report.Users[0], userLatencies.Report.Users[1] = userLatencies.Report.Users[1], userLatencies.Report.Users[0]
		}
		return userLatencies.Report.Users[0].LatencyMS != nil
	}, testutil.WaitShort, testutil.IntervalFast, "user latency is missing")

	require.Len(t, userLatencies.Report.Users, 2, "want only 2 users")
	assert.Greater(t, userLatencies.Report.Users[0].LatencyMS.P50, float64(0), "want p50 to be greater than 0")
	assert.Greater(t, userLatencies.Report.Users[0].LatencyMS.P95, float64(0), "want p95 to be greater than 0")
	assert.Nil(t, userLatencies.Report.Users[1].LatencyMS, "want user 2 to have no latency")
}

func TestUserLatencyInsights_BadRequest(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{})
	_ = coderdtest.CreateFirstUser(t, client)

	y, m, d := time.Now().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	_, err := client.UserLatencyInsights(ctx, codersdk.UserLatencyInsightsRequest{
		StartTime: today,
		EndTime:   today.AddDate(0, 0, -1),
	})
	assert.Error(t, err, "want error for end time before start time")

	_, err = client.UserLatencyInsights(ctx, codersdk.UserLatencyInsightsRequest{
		StartTime: today.AddDate(0, 0, -7),
		EndTime:   today.Add(-time.Hour),
	})
	assert.Error(t, err, "want error for end time partial day when not today")
}

func TestTemplateInsights(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	opts := &coderdtest.Options{
		IncludeProvisionerDaemon:  true,
		AgentStatsRefreshInterval: time.Millisecond * 100,
	}
	client := coderdtest.New(t, opts)

	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.ProvisionComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	require.Empty(t, template.BuildTimeStats[codersdk.WorkspaceTransitionStart])

	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Logger: logger.Named("agent"),
		Client: agentClient,
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	y, m, d := time.Now().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
		Logger: logger.Named("client"),
	})
	require.NoError(t, err)
	defer conn.Close()

	sshConn, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshConn.Close()

	sess, err := sshConn.NewSession()
	require.NoError(t, err)
	defer sess.Close()

	// Keep SSH session open for long enough to generate insights.
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	sess.Stdin = r
	err = sess.Start("cat")
	require.NoError(t, err)

	rpty, err := client.WorkspaceAgentReconnectingPTY(ctx, codersdk.WorkspaceAgentReconnectingPTYOpts{
		AgentID:   resources[0].Agents[0].ID,
		Reconnect: uuid.New(),
		Width:     80,
		Height:    24,
	})
	require.NoError(t, err)
	defer rpty.Close()

	var resp codersdk.TemplateInsightsResponse
	var req codersdk.TemplateInsightsRequest
	waitForAppSeconds := func(slug string) func() bool {
		return func() bool {
			req = codersdk.TemplateInsightsRequest{
				StartTime: today,
				EndTime:   time.Now().Truncate(time.Hour).Add(time.Hour),
				Interval:  codersdk.InsightsReportIntervalDay,
			}
			resp, err = client.TemplateInsights(ctx, req)
			if !assert.NoError(t, err) {
				return false
			}

			if slices.IndexFunc(resp.Report.AppsUsage, func(au codersdk.TemplateAppUsage) bool {
				return au.Slug == slug && au.Seconds > 0
			}) != -1 {
				return true
			}
			return false
		}
	}
	require.Eventually(t, waitForAppSeconds("reconnecting-pty"), testutil.WaitShort, testutil.IntervalFast, "reconnecting-pty seconds missing")
	require.Eventually(t, waitForAppSeconds("ssh"), testutil.WaitShort, testutil.IntervalFast, "ssh seconds missing")

	_ = rpty.Close()
	_ = sess.Close()
	_ = sshConn.Close()

	assert.WithinDuration(t, req.StartTime, resp.Report.StartTime, 0)
	assert.WithinDuration(t, req.EndTime, resp.Report.EndTime, 0)
	assert.Equal(t, resp.Report.ActiveUsers, int64(1), "want one active user")
	for _, app := range resp.Report.AppsUsage {
		if slices.Contains([]string{"reconnecting-pty", "ssh"}, app.Slug) {
			assert.Equal(t, app.Seconds, int64(300), "want app %q to have 5 minutes of usage", app.Slug)
		} else {
			assert.Equal(t, app.Seconds, int64(0), "want app %q to have 0 minutes of usage", app.Slug)
		}
	}
	// The full timeframe is <= 24h, so the interval matches exactly.
	assert.Len(t, resp.IntervalReports, 1, "want one interval report")
	assert.WithinDuration(t, req.StartTime, resp.IntervalReports[0].StartTime, 0)
	assert.WithinDuration(t, req.EndTime, resp.IntervalReports[0].EndTime, 0)
	assert.Equal(t, resp.IntervalReports[0].ActiveUsers, int64(1), "want one active user in the interval report")
}

func TestTemplateInsights_BadRequest(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{})
	_ = coderdtest.CreateFirstUser(t, client)

	y, m, d := time.Now().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	_, err := client.TemplateInsights(ctx, codersdk.TemplateInsightsRequest{
		StartTime: today,
		EndTime:   today.AddDate(0, 0, -1),
	})
	assert.Error(t, err, "want error for end time before start time")

	_, err = client.TemplateInsights(ctx, codersdk.TemplateInsightsRequest{
		StartTime: today.AddDate(0, 0, -7),
		EndTime:   today.Add(-time.Hour),
	})
	assert.Error(t, err, "want error for end time partial day when not today")

	_, err = client.TemplateInsights(ctx, codersdk.TemplateInsightsRequest{
		StartTime: today.AddDate(0, 0, -1),
		EndTime:   today,
		Interval:  "invalid",
	})
	assert.Error(t, err, "want error for bad interval")
}
