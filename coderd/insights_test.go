package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/coderd/batchstats"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
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

		testAppSlug = "test-app"
		testAppName = "Test App"
		testAppIcon = "/icon.png"
		testAppURL  = "http://127.1.0.1:65536" // Not used.
	)

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	opts := &coderdtest.Options{
		Logger:                    &logger,
		IncludeProvisionerDaemon:  true,
		AgentStatsRefreshInterval: time.Millisecond * 100,
	}
	client, _, coderdAPI := coderdtest.NewWithAPI(t, opts)

	user := coderdtest.CreateFirstUser(t, client)
	_, otherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
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
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "dev",
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
							Apps: []*proto.App{
								{
									Slug:         testAppSlug,
									DisplayName:  testAppName,
									Icon:         testAppIcon,
									SharingLevel: proto.AppSharingLevel_OWNER,
									Url:          testAppURL,
								},
							},
						}},
					}},
				},
			},
		}},
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
	requestStartTime := today
	requestEndTime := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// TODO(mafredri): We should prefer to set up an app and generate
	// data by accessing it.
	// Insert entries within and outside timeframe.
	reporter := workspaceapps.NewStatsDBReporter(coderdAPI.Database, workspaceapps.DefaultStatsDBReporterBatchSize)
	//nolint:gocritic // This is a test.
	err := reporter.Report(dbauthz.AsSystemRestricted(ctx), []workspaceapps.StatsReport{
		{
			UserID:       user.UserID,
			WorkspaceID:  workspace.ID,
			AgentID:      resources[0].Agents[0].ID,
			AccessMethod: workspaceapps.AccessMethodPath,
			SlugOrPort:   testAppSlug,
			SessionID:    uuid.New(),
			// Outside report range.
			SessionStartedAt: requestStartTime.Add(-1 * time.Minute),
			SessionEndedAt:   requestStartTime,
			Requests:         1,
		},
		{
			UserID:       user.UserID,
			WorkspaceID:  workspace.ID,
			AgentID:      resources[0].Agents[0].ID,
			AccessMethod: workspaceapps.AccessMethodPath,
			SlugOrPort:   testAppSlug,
			SessionID:    uuid.New(),
			// One minute of usage (rounded up to 5 due to query intervals).
			// TODO(mafredri): We'll fix this in a future refactor so that it's
			// 1 minute increments instead of 5.
			SessionStartedAt: requestStartTime,
			SessionEndedAt:   requestStartTime.Add(1 * time.Minute),
			Requests:         1,
		},
		{
			// Other use is using users workspace, this will result in an
			// additional active user and more time spent in app.
			UserID:       otherUser.ID,
			WorkspaceID:  workspace.ID,
			AgentID:      resources[0].Agents[0].ID,
			AccessMethod: workspaceapps.AccessMethodPath,
			SlugOrPort:   testAppSlug,
			SessionID:    uuid.New(),
			// One minute of usage (rounded up to 5 due to query intervals).
			SessionStartedAt: requestStartTime,
			SessionEndedAt:   requestStartTime.Add(1 * time.Minute),
			Requests:         1,
		},
		{
			UserID:       user.UserID,
			WorkspaceID:  workspace.ID,
			AgentID:      resources[0].Agents[0].ID,
			AccessMethod: workspaceapps.AccessMethodPath,
			SlugOrPort:   testAppSlug,
			SessionID:    uuid.New(),
			// Five additional minutes of usage.
			SessionStartedAt: requestStartTime.Add(10 * time.Minute),
			SessionEndedAt:   requestStartTime.Add(15 * time.Minute),
			Requests:         1,
		},
		{
			UserID:       user.UserID,
			WorkspaceID:  workspace.ID,
			AgentID:      resources[0].Agents[0].ID,
			AccessMethod: workspaceapps.AccessMethodPath,
			SlugOrPort:   testAppSlug,
			SessionID:    uuid.New(),
			// Outside report range.
			SessionStartedAt: requestEndTime,
			SessionEndedAt:   requestEndTime.Add(1 * time.Minute),
			Requests:         1,
		},
	})
	require.NoError(t, err, "want no error inserting stats")

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
				StartTime: requestStartTime,
				EndTime:   requestEndTime,
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
	assert.Equal(t, int64(2), resp.Report.ActiveUsers, "want two active users")
	var gotApps []codersdk.TemplateAppUsage
	// Check builtin apps usage.
	for _, app := range resp.Report.AppsUsage {
		if app.Type != codersdk.TemplateAppsTypeBuiltin {
			gotApps = append(gotApps, app)
			continue
		}
		if slices.Contains([]string{"reconnecting-pty", "ssh"}, app.Slug) {
			assert.Equal(t, app.Seconds, int64(300), "want app %q to have 5 minutes of usage", app.Slug)
		} else {
			assert.Equal(t, app.Seconds, int64(0), "want app %q to have 0 minutes of usage", app.Slug)
		}
	}
	// Check app usage.
	assert.Len(t, gotApps, 1, "want one app")
	assert.Equal(t, []codersdk.TemplateAppUsage{
		{
			TemplateIDs: []uuid.UUID{template.ID},
			Type:        codersdk.TemplateAppsTypeApp,
			Slug:        testAppSlug,
			DisplayName: testAppName,
			Icon:        testAppIcon,
			Seconds:     300 + 300 + 300, // Three times 5 minutes of usage (actually 1 + 1 + 5, but see TODO above).
		},
	}, gotApps, "want app usage to match")

	// The full timeframe is <= 24h, so the interval matches exactly.
	require.Len(t, resp.IntervalReports, 1, "want one interval report")
	assert.WithinDuration(t, req.StartTime, resp.IntervalReports[0].StartTime, 0)
	assert.WithinDuration(t, req.EndTime, resp.IntervalReports[0].EndTime, 0)
	assert.Equal(t, int64(2), resp.IntervalReports[0].ActiveUsers, "want two active users in the interval report")

	// The workspace uses 3 parameters
	require.Len(t, resp.Report.ParametersUsage, 3)
	assert.Equal(t, firstParameterName, resp.Report.ParametersUsage[0].Name)
	assert.Equal(t, firstParameterType, resp.Report.ParametersUsage[0].Type)
	assert.Equal(t, firstParameterDescription, resp.Report.ParametersUsage[0].Description)
	assert.Equal(t, firstParameterDisplayName, resp.Report.ParametersUsage[0].DisplayName)
	assert.Contains(t, resp.Report.ParametersUsage[0].Values, codersdk.TemplateParameterValue{
		Value: firstParameterValue,
		Count: 1,
	})
	assert.Contains(t, resp.Report.ParametersUsage[0].TemplateIDs, template.ID)
	assert.Empty(t, resp.Report.ParametersUsage[0].Options)

	assert.Equal(t, secondParameterName, resp.Report.ParametersUsage[1].Name)
	assert.Equal(t, secondParameterType, resp.Report.ParametersUsage[1].Type)
	assert.Equal(t, secondParameterDescription, resp.Report.ParametersUsage[1].Description)
	assert.Equal(t, secondParameterDisplayName, resp.Report.ParametersUsage[1].DisplayName)
	assert.Contains(t, resp.Report.ParametersUsage[1].Values, codersdk.TemplateParameterValue{
		Value: secondParameterValue,
		Count: 1,
	})
	assert.Contains(t, resp.Report.ParametersUsage[1].TemplateIDs, template.ID)
	assert.Empty(t, resp.Report.ParametersUsage[1].Options)

	assert.Equal(t, thirdParameterName, resp.Report.ParametersUsage[2].Name)
	assert.Equal(t, thirdParameterType, resp.Report.ParametersUsage[2].Type)
	assert.Equal(t, thirdParameterDescription, resp.Report.ParametersUsage[2].Description)
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

func TestTemplateInsights_Golden(t *testing.T) {
	t.Parallel()

	// Prepare test data types.
	type templateParameterOption struct {
		name  string
		value string
	}
	type templateParameter struct {
		name        string
		description string
		options     []templateParameterOption
	}
	type templateApp struct {
		name string
		icon string
	}
	type testTemplate struct {
		name       string
		parameters []*templateParameter
		apps       []templateApp

		// Filled later.
		id uuid.UUID
	}
	type buildParameter struct {
		templateParameter *templateParameter
		value             string
	}
	type workspaceApp templateApp
	type testWorkspace struct {
		name            string
		template        *testTemplate
		buildParameters []buildParameter

		// Filled later.
		id          uuid.UUID
		user        any // *testUser, but it's not available yet, defined below.
		agentID     uuid.UUID
		apps        []*workspaceApp
		agentClient *agentsdk.Client
	}
	type testUser struct {
		name       string
		workspaces []*testWorkspace

		client *codersdk.Client
		sdk    codersdk.User
	}

	// Represent agent stats, to be inserted via stats batcher.
	type agentStat struct {
		// Set a range via start/end, multiple stats will be generated
		// within the range.
		startedAt time.Time
		endedAt   time.Time

		sessionCountVSCode          int64
		sessionCountJetBrains       int64
		sessionCountReconnectingPTY int64
		sessionCountSSH             int64
		noConnections               bool
	}
	// Represent app usage stats, to be inserted via stats reporter.
	type appUsage struct {
		app       *workspaceApp
		startedAt time.Time
		endedAt   time.Time
		requests  int
	}

	// Represent actual data being generated on a per-workspace basis.
	type testDataGen struct {
		agentStats []agentStat
		appUsage   []appUsage
	}

	createFixture := func() ([]*testTemplate, []*testUser) {
		// Test templates and configuration to generate.
		templates := []*testTemplate{
			// Create two templates with near-identical apps and parameters
			// to allow testing for grouping similar data.
			{
				name: "template1",
				parameters: []*templateParameter{
					{name: "param1", description: "This is first parameter"},
					{name: "param2", description: "This is second parameter"},
					{name: "param3", description: "This is third parameter"},
					{
						name:        "param4",
						description: "This is fourth parameter",
						options: []templateParameterOption{
							{name: "option1", value: "option1"},
							{name: "option2", value: "option2"},
						},
					},
				},
				apps: []templateApp{
					{name: "app1", icon: "/icon1.png"},
					{name: "app2", icon: "/icon2.png"},
					{name: "app3", icon: "/icon2.png"},
				},
			},
			{
				name: "template2",
				parameters: []*templateParameter{
					{name: "param1", description: "This is first parameter"},
					{name: "param2", description: "This is second parameter"},
					{name: "param3", description: "This is third parameter"},
				},
				apps: []templateApp{
					{name: "app1", icon: "/icon1.png"},
					{name: "app2", icon: "/icon2.png"},
					{name: "app3", icon: "/icon2.png"},
				},
			},
			// Create another template with different parameters and apps.
			{
				name: "othertemplate",
				parameters: []*templateParameter{
					{name: "otherparam1", description: "This is another parameter"},
				},
				apps: []templateApp{
					{name: "otherapp1", icon: "/icon1.png"},
				},
			},
		}

		// Users and workspaces to generate.
		users := []*testUser{
			{
				name: "user1",
				workspaces: []*testWorkspace{
					{
						name:     "workspace1",
						template: templates[0],
						buildParameters: []buildParameter{
							{templateParameter: templates[0].parameters[0], value: "abc"},
							{templateParameter: templates[0].parameters[1], value: "123"},
							{templateParameter: templates[0].parameters[2], value: "bbb"},
							{templateParameter: templates[0].parameters[3], value: "option1"},
						},
					},
					{
						name:     "workspace2",
						template: templates[1],
						buildParameters: []buildParameter{
							{templateParameter: templates[0].parameters[0], value: "ABC"},
							{templateParameter: templates[0].parameters[1], value: "123"},
							{templateParameter: templates[0].parameters[2], value: "BBB"},
							{templateParameter: templates[0].parameters[3], value: "option2"},
						},
					},
					{
						name:     "otherworkspace1",
						template: templates[2],
					},
				},
			},
			{
				name: "user2",
				workspaces: []*testWorkspace{
					{
						name:     "workspace1",
						template: templates[0],
						buildParameters: []buildParameter{
							{templateParameter: templates[0].parameters[0], value: "abc"},
							{templateParameter: templates[0].parameters[1], value: "123"},
							{templateParameter: templates[0].parameters[2], value: "BBB"},
							{templateParameter: templates[0].parameters[3], value: "option1"},
						},
					},
				},
			},
			{
				name: "user3",
				workspaces: []*testWorkspace{
					{
						name:     "otherworkspace1",
						template: templates[2],
						buildParameters: []buildParameter{
							{templateParameter: templates[2].parameters[0], value: "xyz"},
						},
					},
				},
			},
		}

		// Post-process.
		var stableIDs []uuid.UUID
		newStableUUID := func() uuid.UUID {
			stableIDs = append(stableIDs, uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", len(stableIDs)+1)))
			stableID := stableIDs[len(stableIDs)-1]
			return stableID
		}

		for _, template := range templates {
			template.id = newStableUUID()
		}
		for _, user := range users {
			for _, workspace := range user.workspaces {
				workspace.user = user
				for _, app := range workspace.template.apps {
					app := workspaceApp(app)
					workspace.apps = append(workspace.apps, &app)
				}
			}
		}

		return templates, users
	}

	prepare := func(t *testing.T, templates []*testTemplate, users []*testUser, testData map[*testWorkspace]testDataGen) *codersdk.Client {
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{
			Database:                  db,
			Pubsub:                    pubsub,
			Logger:                    &logger,
			IncludeProvisionerDaemon:  true,
			AgentStatsRefreshInterval: time.Hour, // Not relevant for this test.
		})
		firstUser := coderdtest.CreateFirstUser(t, client)

		// Prepare all test users.
		for _, user := range users {
			user.client, user.sdk = coderdtest.CreateAnotherUserMutators(t, client, firstUser.OrganizationID, nil, func(r *codersdk.CreateUserRequest) {
				r.Username = user.name
			})
			user.client.SetLogger(logger.Named("user").With(slog.Field{Name: "name", Value: user.name}))
		}

		// Prepare all the templates.
		for _, template := range templates {
			template := template

			var parameters []*proto.RichParameter
			for _, parameter := range template.parameters {
				var options []*proto.RichParameterOption
				var defaultValue string
				for _, option := range parameter.options {
					if defaultValue == "" {
						defaultValue = option.value
					}
					options = append(options, &proto.RichParameterOption{
						Name:  option.name,
						Value: option.value,
					})
				}
				parameters = append(parameters, &proto.RichParameter{
					Name:         parameter.name,
					DisplayName:  parameter.name,
					Type:         "string",
					Description:  parameter.description,
					Options:      options,
					DefaultValue: defaultValue,
				})
			}

			// Prepare all workspace resources (agents and apps).
			var (
				createWorkspaces []func(uuid.UUID)
				waitWorkspaces   []func()
			)
			var resources []*proto.Resource
			for _, user := range users {
				user := user
				for _, workspace := range user.workspaces {
					workspace := workspace

					if workspace.template != template {
						continue
					}
					authToken := uuid.New()
					agentClient := agentsdk.New(client.URL)
					agentClient.SetSessionToken(authToken.String())
					workspace.agentClient = agentClient

					var apps []*proto.App
					for _, app := range workspace.apps {
						apps = append(apps, &proto.App{
							Slug:         app.name,
							DisplayName:  app.name,
							Icon:         app.icon,
							SharingLevel: proto.AppSharingLevel_OWNER,
							Url:          "http://",
						})
					}

					resources = append(resources, &proto.Resource{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(), // Doesn't matter, not used in DB.
							Name: "dev",
							Auth: &proto.Agent_Token{
								Token: authToken.String(),
							},
							Apps: apps,
						}},
					})

					var buildParameters []codersdk.WorkspaceBuildParameter
					for _, buildParameter := range workspace.buildParameters {
						buildParameters = append(buildParameters, codersdk.WorkspaceBuildParameter{
							Name:  buildParameter.templateParameter.name,
							Value: buildParameter.value,
						})
					}

					createWorkspaces = append(createWorkspaces, func(templateID uuid.UUID) {
						// Create workspace using the users client.
						createdWorkspace := coderdtest.CreateWorkspace(t, user.client, firstUser.OrganizationID, templateID, func(cwr *codersdk.CreateWorkspaceRequest) {
							cwr.RichParameterValues = buildParameters
						})
						workspace.id = createdWorkspace.ID
						waitWorkspaces = append(waitWorkspaces, func() {
							coderdtest.AwaitWorkspaceBuildJob(t, user.client, createdWorkspace.LatestBuild.ID)
							ctx := testutil.Context(t, testutil.WaitShort)
							ws, err := user.client.Workspace(ctx, workspace.id)
							require.NoError(t, err, "want no error getting workspace")

							workspace.agentID = ws.LatestBuild.Resources[0].Agents[0].ID
						})
					})
				}
			}

			// Create the template version and template.
			version := coderdtest.CreateTemplateVersion(t, client, firstUser.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete,
				ProvisionPlan: []*proto.Provision_Response{
					{
						Type: &proto.Provision_Response_Complete{
							Complete: &proto.Provision_Complete{
								Parameters: parameters,
							},
						},
					},
				},
				ProvisionApply: []*proto.Provision_Response{{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
							Resources: resources,
						},
					},
				}},
			})
			coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

			// Create template, essentially a modified version of CreateTemplate
			// where we can control the template ID.
			// 	createdTemplate := coderdtest.CreateTemplate(t, client, firstUser.OrganizationID, version.ID)
			createdTemplate := dbgen.Template(t, db, database.Template{
				ID:              template.id,
				ActiveVersionID: version.ID,
				OrganizationID:  firstUser.OrganizationID,
				CreatedBy:       firstUser.UserID,
				GroupACL: database.TemplateACL{
					firstUser.OrganizationID.String(): []rbac.Action{rbac.ActionRead},
				},
			})
			err := db.UpdateTemplateVersionByID(context.Background(), database.UpdateTemplateVersionByIDParams{
				ID: version.ID,
				TemplateID: uuid.NullUUID{
					UUID:  createdTemplate.ID,
					Valid: true,
				},
			})
			require.NoError(t, err, "want no error updating template version")

			// Create all workspaces and wait for them.
			for _, createWorkspace := range createWorkspaces {
				createWorkspace(template.id)
			}
			for _, waitWorkspace := range waitWorkspaces {
				waitWorkspace()
			}
		}

		ctx := testutil.Context(t, testutil.WaitSuperLong)

		// Use agent stats batcher to insert agent stats, similar to live system.
		// NOTE(mafredri): Ideally we would pass batcher as a coderd option and
		// insert using the agentClient, but we have a circular dependency on
		// the database.
		batcher, batcherCloser, err := batchstats.New(
			ctx,
			batchstats.WithStore(db),
			batchstats.WithLogger(logger.Named("batchstats")),
			batchstats.WithInterval(time.Hour),
		)
		require.NoError(t, err)
		defer batcherCloser() // Flushes the stats, this is to ensure they're written.

		for workspace, data := range testData {
			for _, stat := range data.agentStats {
				createdAt := stat.startedAt
				connectionCount := int64(1)
				if stat.noConnections {
					connectionCount = 0
				}
				for createdAt.Before(stat.endedAt) {
					err = batcher.Add(createdAt, workspace.agentID, workspace.template.id, workspace.user.(*testUser).sdk.ID, workspace.id, agentsdk.Stats{
						ConnectionCount:             connectionCount,
						SessionCountVSCode:          stat.sessionCountVSCode,
						SessionCountJetBrains:       stat.sessionCountJetBrains,
						SessionCountReconnectingPTY: stat.sessionCountReconnectingPTY,
						SessionCountSSH:             stat.sessionCountSSH,
					})
					require.NoError(t, err, "want no error inserting agent stats")
					createdAt = createdAt.Add(30 * time.Second)
				}
			}
		}

		// Insert app usage.
		var stats []workspaceapps.StatsReport
		for workspace, data := range testData {
			for _, usage := range data.appUsage {
				appName := usage.app.name
				accessMethod := workspaceapps.AccessMethodPath
				if usage.app.name == "terminal" {
					appName = ""
					accessMethod = workspaceapps.AccessMethodTerminal
				}
				stats = append(stats, workspaceapps.StatsReport{
					UserID:           workspace.user.(*testUser).sdk.ID,
					WorkspaceID:      workspace.id,
					AgentID:          workspace.agentID,
					AccessMethod:     accessMethod,
					SlugOrPort:       appName,
					SessionID:        uuid.New(),
					SessionStartedAt: usage.startedAt,
					SessionEndedAt:   usage.endedAt,
					Requests:         usage.requests,
				})
			}
		}
		reporter := workspaceapps.NewStatsDBReporter(db, workspaceapps.DefaultStatsDBReporterBatchSize)
		//nolint:gocritic // This is a test.
		err = reporter.Report(dbauthz.AsSystemRestricted(ctx), stats)
		require.NoError(t, err, "want no error inserting app stats")

		return client
	}

	// Time range for report, test data will be generated within and
	// outside this range, but only data within the range should be
	// included in the report.
	frozenLastNight := time.Date(2023, 8, 22, 0, 0, 0, 0, time.UTC)
	frozenWeekAgo := frozenLastNight.AddDate(0, 0, -7)

	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	require.NoError(t, err)
	frozenWeekAgoSaoPaulo, err := time.ParseInLocation(time.DateTime, frozenWeekAgo.Format(time.DateTime), saoPaulo)
	require.NoError(t, err)

	makeBaseTestData := func(templates []*testTemplate, users []*testUser) map[*testWorkspace]testDataGen {
		return map[*testWorkspace]testDataGen{
			users[0].workspaces[0]: {
				agentStats: []agentStat{
					{ // One hour of usage.
						startedAt:          frozenWeekAgo,
						endedAt:            frozenWeekAgo.Add(time.Hour),
						sessionCountVSCode: 1,
						sessionCountSSH:    1,
					},
					{ // 12 minutes of usage -> 15 minutes.
						startedAt:       frozenWeekAgo.AddDate(0, 0, 1),
						endedAt:         frozenWeekAgo.AddDate(0, 0, 1).Add(12 * time.Minute),
						sessionCountSSH: 1,
					},
					{ // 2 minutes of usage -> 10 minutes because it crosses the 5 minute interval boundary.
						startedAt:             frozenWeekAgo.AddDate(0, 0, 2).Add(4 * time.Minute),
						endedAt:               frozenWeekAgo.AddDate(0, 0, 2).Add(6 * time.Minute),
						sessionCountJetBrains: 1,
					},
				},
				appUsage: []appUsage{
					{ // One hour of usage.
						app:       users[0].workspaces[0].apps[0],
						startedAt: frozenWeekAgo,
						endedAt:   frozenWeekAgo.Add(time.Hour),
						requests:  1,
					},
					{ // used an app on the last day, counts as active user, 12m -> 15m rounded.
						app:       users[0].workspaces[0].apps[2],
						startedAt: frozenWeekAgo.AddDate(0, 0, 6),
						endedAt:   frozenWeekAgo.AddDate(0, 0, 6).Add(12 * time.Minute),
						requests:  1,
					},
				},
			},
			users[0].workspaces[1]: {
				agentStats: []agentStat{
					{
						// One hour of usage in second template at the same time
						// as in first template. When selecting both templates
						// this user and their app usage will only be counted
						// once but the template ID will show up in the data.
						startedAt:          frozenWeekAgo,
						endedAt:            frozenWeekAgo.Add(time.Hour),
						sessionCountVSCode: 1,
						sessionCountSSH:    1,
					},
				},
				appUsage: []appUsage{
					// TODO(mafredri): This doesn't behave correctly right now
					// and will add more usage to the app. This could be
					// considered both correct and incorrect behavior.
					// { // One hour of usage, but same user and same template app, only count once.
					// 	app:       users[0].workspaces[1].apps[0],
					// 	startedAt: weekAgo,
					// 	endedAt:   weekAgo.Add(time.Hour),
					// 	requests:  1,
					// },
					{
						// Different templates but identical apps, apps will be
						// combined and usage will be summed.
						app:       users[0].workspaces[1].apps[0],
						startedAt: frozenWeekAgo.AddDate(0, 0, 2),
						endedAt:   frozenWeekAgo.AddDate(0, 0, 2).Add(6 * time.Hour),
						requests:  1,
					},
				},
			},
			users[0].workspaces[2]: {
				agentStats: []agentStat{},
				appUsage:   []appUsage{},
			},
			users[1].workspaces[0]: {
				agentStats: []agentStat{},
				appUsage:   []appUsage{},
			},
			users[2].workspaces[0]: {
				agentStats: []agentStat{},
				appUsage:   []appUsage{},
			},
		}
	}
	type testRequest struct {
		name        string
		makeRequest func([]*testTemplate) codersdk.TemplateInsightsRequest
		ignoreTimes bool
	}
	tests := []struct {
		name         string
		makeTestData func([]*testTemplate, []*testUser) map[*testWorkspace]testDataGen
		requests     []testRequest
	}{
		{
			name:         "multiple users and workspaces",
			makeTestData: makeBaseTestData,
			requests: []testRequest{
				{
					name: "week deployment wide",
					makeRequest: func(templates []*testTemplate) codersdk.TemplateInsightsRequest {
						return codersdk.TemplateInsightsRequest{
							StartTime: frozenWeekAgo,
							EndTime:   frozenWeekAgo.AddDate(0, 0, 7),
							Interval:  codersdk.InsightsReportIntervalDay,
						}
					},
				},
				{
					name: "week all templates",
					makeRequest: func(templates []*testTemplate) codersdk.TemplateInsightsRequest {
						return codersdk.TemplateInsightsRequest{
							TemplateIDs: []uuid.UUID{templates[0].id, templates[1].id, templates[2].id},
							StartTime:   frozenWeekAgo,
							EndTime:     frozenWeekAgo.AddDate(0, 0, 7),
							Interval:    codersdk.InsightsReportIntervalDay,
						}
					},
				},
				{
					name: "week first template",
					makeRequest: func(templates []*testTemplate) codersdk.TemplateInsightsRequest {
						return codersdk.TemplateInsightsRequest{
							TemplateIDs: []uuid.UUID{templates[0].id},
							StartTime:   frozenWeekAgo,
							EndTime:     frozenWeekAgo.AddDate(0, 0, 7),
							Interval:    codersdk.InsightsReportIntervalDay,
						}
					},
				},
				{
					name: "week second template",
					makeRequest: func(templates []*testTemplate) codersdk.TemplateInsightsRequest {
						return codersdk.TemplateInsightsRequest{
							TemplateIDs: []uuid.UUID{templates[1].id},
							StartTime:   frozenWeekAgo,
							EndTime:     frozenWeekAgo.AddDate(0, 0, 7),
							Interval:    codersdk.InsightsReportIntervalDay,
						}
					},
				},
				{
					name: "week third template",
					makeRequest: func(templates []*testTemplate) codersdk.TemplateInsightsRequest {
						return codersdk.TemplateInsightsRequest{
							TemplateIDs: []uuid.UUID{templates[2].id},
							StartTime:   frozenWeekAgo,
							EndTime:     frozenWeekAgo.AddDate(0, 0, 7),
							Interval:    codersdk.InsightsReportIntervalDay,
						}
					},
				},
				{
					// São Paulo is three hours behind UTC, so we should not see
					// any data between weekAgo and weekAgo.Add(3 * time.Hour).
					name: "week other timezone (São Paulo)",
					makeRequest: func(templates []*testTemplate) codersdk.TemplateInsightsRequest {
						return codersdk.TemplateInsightsRequest{
							StartTime: frozenWeekAgoSaoPaulo,
							EndTime:   frozenWeekAgoSaoPaulo.AddDate(0, 0, 7),
							Interval:  codersdk.InsightsReportIntervalDay,
						}
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			templates, users := createFixture()
			testData := tt.makeTestData(templates, users)
			// Sanity check.
			for ws, data := range testData {
				for _, usage := range data.appUsage {
					found := false
					wrongWorkspace := false
					for _, app := range ws.apps {
						if usage.app == app { // Pointer equality
							found = true
							break
						}
						if *usage.app == *app {
							wrongWorkspace = true
						}
					}
					require.True(t, found, "test bug: app %q not in workspace %q [wrongWorkspace=%v]", usage.app.name, ws.name, wrongWorkspace)
				}
			}

			client := prepare(t, templates, users, testData)

			for _, req := range tt.requests {
				req := req
				t.Run(req.name, func(t *testing.T) {
					t.Parallel()

					ctx := testutil.Context(t, testutil.WaitMedium)

					report, err := client.TemplateInsights(ctx, req.makeRequest(templates))
					require.NoError(t, err, "want no error getting template insights")

					if req.ignoreTimes {
						// Ignore times, we're only interested in the data.
						report.Report.StartTime = time.Time{}
						report.Report.EndTime = time.Time{}
						for i := range report.IntervalReports {
							report.IntervalReports[i].StartTime = time.Time{}
							report.IntervalReports[i].EndTime = time.Time{}
						}
					}

					partialName := strings.Join(strings.Split(t.Name(), "/")[1:], "_")
					goldenFile := filepath.Join("testdata", "insights", partialName+".json.golden")
					if *updateGoldenFiles {
						err = os.MkdirAll(filepath.Dir(goldenFile), 0o755)
						require.NoError(t, err, "want no error creating golden file directory")
						f, err := os.Create(goldenFile)
						require.NoError(t, err, "want no error creating golden file")
						defer f.Close()
						enc := json.NewEncoder(f)
						enc.SetIndent("", "  ")
						enc.Encode(report)
						return
					}

					f, err := os.Open(goldenFile)
					require.NoError(t, err, "open golden file, run \"make update-golden-files\" and commit the changes")
					defer f.Close()
					var want codersdk.TemplateInsightsResponse
					err = json.NewDecoder(f).Decode(&want)
					require.NoError(t, err, "want no error decoding golden file")

					assert.Equal(t, want, report, "golden file mismatch: %s, run \"make update-golden-files\", verify and commit the changes", goldenFile)
				})
			}
		})
	}
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
