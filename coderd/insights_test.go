package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
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

	// Create two users, one that will appear in the report and another that
	// won't (due to not having/using a workspace).
	user := coderdtest.CreateFirstUser(t, client)
	_, _ = coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
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

	// Start an agent so that we can generate stats.
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

	// Start must be at the beginning of the day, initialize it early in case
	// the day changes so that we get the relevant stats faster.
	y, m, d := time.Now().UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Connect to the agent to generate usage/latency stats.
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

	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	sess.Stdin = r
	sess.Stdout = io.Discard
	err = sess.Start("cat")
	require.NoError(t, err)

	var userLatencies codersdk.UserLatencyInsightsResponse
	require.Eventuallyf(t, func() bool {
		// Keep connection active.
		_, err := w.Write([]byte("hello world\n"))
		if !assert.NoError(t, err) {
			return false
		}
		userLatencies, err = client.UserLatencyInsights(ctx, codersdk.UserLatencyInsightsRequest{
			StartTime:   today,
			EndTime:     time.Now().UTC().Truncate(time.Hour).Add(time.Hour), // Round up to include the current hour.
			TemplateIDs: []uuid.UUID{template.ID},
		})
		if !assert.NoError(t, err) {
			return false
		}
		return len(userLatencies.Report.Users) > 0 && userLatencies.Report.Users[0].LatencyMS.P50 > 0
	}, testutil.WaitMedium, testutil.IntervalFast, "user latency is missing")

	// We got our latency data, close the connection.
	_ = sess.Close()
	_ = sshConn.Close()

	require.Len(t, userLatencies.Report.Users, 1, "want only 1 user")
	require.Equal(t, userLatencies.Report.Users[0].UserID, user.UserID, "want user id to match")
	assert.Greater(t, userLatencies.Report.Users[0].LatencyMS.P50, float64(0), "want p50 to be greater than 0")
	assert.Greater(t, userLatencies.Report.Users[0].LatencyMS.P95, float64(0), "want p95 to be greater than 0")
}

func TestUserLatencyInsights_BadRequest(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{})
	_ = coderdtest.CreateFirstUser(t, client)

	y, m, d := time.Now().UTC().Date()
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

	const (
		firstParameterName        = "first_parameter"
		firstParameterDisplayName = "First PARAMETER"
		firstParameterType        = "string"
		firstParameterDescription = "This is first parameter"
		firstParameterValue       = "abc"

		secondParameterName        = "second_parameter"
		secondParameterDisplayName = "Second PARAMETER"
		secondParameterType        = "number"
		secondParameterDescription = "This is second parameter"
		secondParameterValue       = "123"

		thirdParameterName         = "third_parameter"
		thirdParameterDisplayName  = "Third PARAMETER"
		thirdParameterType         = "string"
		thirdParameterDescription  = "This is third parameter"
		thirdParameterValue        = "bbb"
		thirdParameterOptionName1  = "This is AAA"
		thirdParameterOptionValue1 = "aaa"
		thirdParameterOptionName2  = "This is BBB"
		thirdParameterOptionValue2 = "bbb"
		thirdParameterOptionName3  = "This is CCC"
		thirdParameterOptionValue3 = "ccc"
	)

	logger := slogtest.Make(t, nil)
	opts := &coderdtest.Options{
		IncludeProvisionerDaemon:  true,
		AgentStatsRefreshInterval: time.Millisecond * 100,
	}
	client := coderdtest.New(t, opts)

	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Provision_Response{
			{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Parameters: []*proto.RichParameter{
							{Name: firstParameterName, DisplayName: firstParameterDisplayName, Type: firstParameterType, Description: firstParameterDescription, Required: true},
							{Name: secondParameterName, DisplayName: secondParameterDisplayName, Type: secondParameterType, Description: secondParameterDescription, Required: true},
							{Name: thirdParameterName, DisplayName: thirdParameterDisplayName, Type: thirdParameterType, Description: thirdParameterDescription, Required: true, Options: []*proto.RichParameterOption{
								{Name: thirdParameterOptionName1, Value: thirdParameterOptionValue1},
								{Name: thirdParameterOptionName2, Value: thirdParameterOptionValue2},
								{Name: thirdParameterOptionName3, Value: thirdParameterOptionValue3},
							}},
						},
					},
				},
			},
		},
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	require.Empty(t, template.BuildTimeStats[codersdk.WorkspaceTransitionStart])
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

	buildParameters := []codersdk.WorkspaceBuildParameter{
		{Name: firstParameterName, Value: firstParameterValue},
		{Name: secondParameterName, Value: secondParameterValue},
		{Name: thirdParameterName, Value: thirdParameterValue},
	}

	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.RichParameterValues = buildParameters
	})
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	// Start an agent so that we can generate stats.
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

	// Start must be at the beginning of the day, initialize it early in case
	// the day changes so that we get the relevant stats faster.
	y, m, d := time.Now().UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Connect to the agent to generate usage/latency stats.
	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
		Logger: logger.Named("client"),
	})
	require.NoError(t, err)
	defer conn.Close()

	sshConn, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshConn.Close()

	// Start an SSH session to generate SSH usage stats.
	sess, err := sshConn.NewSession()
	require.NoError(t, err)
	defer sess.Close()

	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	sess.Stdin = r
	err = sess.Start("cat")
	require.NoError(t, err)

	// Start an rpty session to generate rpty usage stats.
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
				EndTime:   time.Now().UTC().Truncate(time.Hour).Add(time.Hour),
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
	require.Eventually(t, waitForAppSeconds("reconnecting-pty"), testutil.WaitMedium, testutil.IntervalFast, "reconnecting-pty seconds missing")
	require.Eventually(t, waitForAppSeconds("ssh"), testutil.WaitMedium, testutil.IntervalFast, "ssh seconds missing")

	// We got our data, close down sessions and connections.
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
	require.Len(t, resp.IntervalReports, 1, "want one interval report")
	assert.WithinDuration(t, req.StartTime, resp.IntervalReports[0].StartTime, 0)
	assert.WithinDuration(t, req.EndTime, resp.IntervalReports[0].EndTime, 0)
	assert.Equal(t, resp.IntervalReports[0].ActiveUsers, int64(1), "want one active user in the interval report")

	// The workspace uses 3 parameters
	require.Len(t, resp.Report.ParametersUsage, 3)
	assert.Equal(t, firstParameterName, resp.Report.ParametersUsage[0].Name)
	assert.Equal(t, firstParameterDisplayName, resp.Report.ParametersUsage[0].DisplayName)
	assert.Contains(t, resp.Report.ParametersUsage[0].Values, codersdk.TemplateParameterValue{
		Value: firstParameterValue,
		Count: 1,
	})
	assert.Contains(t, resp.Report.ParametersUsage[0].TemplateIDs, template.ID)
	assert.Empty(t, resp.Report.ParametersUsage[0].Options)

	assert.Equal(t, secondParameterName, resp.Report.ParametersUsage[1].Name)
	assert.Equal(t, secondParameterDisplayName, resp.Report.ParametersUsage[1].DisplayName)
	assert.Contains(t, resp.Report.ParametersUsage[1].Values, codersdk.TemplateParameterValue{
		Value: secondParameterValue,
		Count: 1,
	})
	assert.Contains(t, resp.Report.ParametersUsage[1].TemplateIDs, template.ID)
	assert.Empty(t, resp.Report.ParametersUsage[1].Options)

	assert.Equal(t, thirdParameterName, resp.Report.ParametersUsage[2].Name)
	assert.Equal(t, thirdParameterDisplayName, resp.Report.ParametersUsage[2].DisplayName)
	assert.Contains(t, resp.Report.ParametersUsage[2].Values, codersdk.TemplateParameterValue{
		Value: thirdParameterValue,
		Count: 1,
	})
	assert.Contains(t, resp.Report.ParametersUsage[2].TemplateIDs, template.ID)
	assert.Equal(t, []codersdk.TemplateVersionParameterOption{
		{Name: thirdParameterOptionName1, Value: thirdParameterOptionValue1},
		{Name: thirdParameterOptionName2, Value: thirdParameterOptionValue2},
		{Name: thirdParameterOptionName3, Value: thirdParameterOptionValue3},
	}, resp.Report.ParametersUsage[2].Options)
}

func TestTemplateInsights_BadRequest(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{})
	_ = coderdtest.CreateFirstUser(t, client)

	y, m, d := time.Now().UTC().Date()
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

func TestTemplateInsights_RBAC(t *testing.T) {
	t.Parallel()

	y, m, d := time.Now().UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	type test struct {
		interval     codersdk.InsightsReportInterval
		withTemplate bool
	}

	tests := []test{
		{codersdk.InsightsReportIntervalDay, true},
		{codersdk.InsightsReportIntervalDay, false},
		{"", true},
		{"", false},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(fmt.Sprintf("with interval=%q", tt.interval), func(t *testing.T) {
			t.Parallel()

			t.Run("AsOwner", func(t *testing.T) {
				t.Parallel()

				client := coderdtest.New(t, &coderdtest.Options{})
				admin := coderdtest.CreateFirstUser(t, client)

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cancel()

				var templateIDs []uuid.UUID
				if tt.withTemplate {
					version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
					template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
					templateIDs = append(templateIDs, template.ID)
				}

				_, err := client.TemplateInsights(ctx, codersdk.TemplateInsightsRequest{
					StartTime:   today.AddDate(0, 0, -1),
					EndTime:     today,
					Interval:    tt.interval,
					TemplateIDs: templateIDs,
				})
				require.NoError(t, err)
			})
			t.Run("AsTemplateAdmin", func(t *testing.T) {
				t.Parallel()

				client := coderdtest.New(t, &coderdtest.Options{})
				admin := coderdtest.CreateFirstUser(t, client)

				templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleTemplateAdmin())

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cancel()

				var templateIDs []uuid.UUID
				if tt.withTemplate {
					version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
					template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
					templateIDs = append(templateIDs, template.ID)
				}

				_, err := templateAdmin.TemplateInsights(ctx, codersdk.TemplateInsightsRequest{
					StartTime:   today.AddDate(0, 0, -1),
					EndTime:     today,
					Interval:    tt.interval,
					TemplateIDs: templateIDs,
				})
				require.NoError(t, err)
			})
			t.Run("AsRegularUser", func(t *testing.T) {
				t.Parallel()

				client := coderdtest.New(t, &coderdtest.Options{})
				admin := coderdtest.CreateFirstUser(t, client)

				regular, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cancel()

				var templateIDs []uuid.UUID
				if tt.withTemplate {
					version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
					template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
					templateIDs = append(templateIDs, template.ID)
				}

				_, err := regular.TemplateInsights(ctx, codersdk.TemplateInsightsRequest{
					StartTime:   today.AddDate(0, 0, -1),
					EndTime:     today,
					Interval:    tt.interval,
					TemplateIDs: templateIDs,
				})
				require.Error(t, err)
				var apiErr *codersdk.Error
				require.ErrorAs(t, err, &apiErr)
				require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
			})
		})
	}
}

func TestUserLatencyInsights_RBAC(t *testing.T) {
	t.Parallel()

	y, m, d := time.Now().UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	type test struct {
		interval     codersdk.InsightsReportInterval
		withTemplate bool
	}

	tests := []test{
		{codersdk.InsightsReportIntervalDay, true},
		{codersdk.InsightsReportIntervalDay, false},
		{"", true},
		{"", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("with interval=%q", tt.interval), func(t *testing.T) {
			t.Parallel()

			t.Run("AsOwner", func(t *testing.T) {
				t.Parallel()

				client := coderdtest.New(t, &coderdtest.Options{})
				admin := coderdtest.CreateFirstUser(t, client)

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cancel()

				var templateIDs []uuid.UUID
				if tt.withTemplate {
					version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
					template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
					templateIDs = append(templateIDs, template.ID)
				}

				_, err := client.UserLatencyInsights(ctx, codersdk.UserLatencyInsightsRequest{
					StartTime:   today,
					EndTime:     time.Now().UTC().Truncate(time.Hour).Add(time.Hour), // Round up to include the current hour.
					TemplateIDs: templateIDs,
				})
				require.NoError(t, err)
			})
			t.Run("AsTemplateAdmin", func(t *testing.T) {
				t.Parallel()

				client := coderdtest.New(t, &coderdtest.Options{})
				admin := coderdtest.CreateFirstUser(t, client)

				templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleTemplateAdmin())

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cancel()

				var templateIDs []uuid.UUID
				if tt.withTemplate {
					version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
					template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
					templateIDs = append(templateIDs, template.ID)
				}

				_, err := templateAdmin.UserLatencyInsights(ctx, codersdk.UserLatencyInsightsRequest{
					StartTime:   today,
					EndTime:     time.Now().UTC().Truncate(time.Hour).Add(time.Hour), // Round up to include the current hour.
					TemplateIDs: templateIDs,
				})
				require.NoError(t, err)
			})
			t.Run("AsRegularUser", func(t *testing.T) {
				t.Parallel()

				client := coderdtest.New(t, &coderdtest.Options{})
				admin := coderdtest.CreateFirstUser(t, client)

				regular, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cancel()

				var templateIDs []uuid.UUID
				if tt.withTemplate {
					version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
					template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
					templateIDs = append(templateIDs, template.ID)
				}

				_, err := regular.UserLatencyInsights(ctx, codersdk.UserLatencyInsightsRequest{
					StartTime:   today,
					EndTime:     time.Now().UTC().Truncate(time.Hour).Add(time.Hour), // Round up to include the current hour.
					TemplateIDs: templateIDs,
				})
				require.Error(t, err)
				var apiErr *codersdk.Error
				require.ErrorAs(t, err, &apiErr)
				require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
			})
		})
	}
}
