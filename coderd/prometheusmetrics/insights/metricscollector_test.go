package insights_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/prometheusmetrics/insights"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestCollectInsights(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	db, ps := dbtestutil.NewDB(t)

	options := &coderdtest.Options{
		IncludeProvisionerDaemon:  true,
		AgentStatsRefreshInterval: time.Millisecond * 100,
		Database:                  db,
		Pubsub:                    ps,
	}
	client := coderdtest.New(t, options)
	client.SetLogger(logger.Named("client").Leveled(slog.LevelDebug))

	// Given
	// Initialize metrics collector
	mc, err := insights.NewMetricsCollector(db, logger, 0, time.Second)
	require.NoError(t, err)

	registry := prometheus.NewRegistry()
	registry.Register(mc)

	// Create two users, one that will appear in the report and another that
	// won't (due to not having/using a workspace).
	user := coderdtest.CreateFirstUser(t, client)
	_, _ = coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  provisionPlanWithParameters(),
		ProvisionApply: provisionApplyWithAgentAndApp(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
		ctr.Name = "golden-template"
	})
	require.Empty(t, template.BuildTimeStats[codersdk.WorkspaceTransitionStart])

	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.RichParameterValues = []codersdk.WorkspaceBuildParameter{
			{Name: "first_parameter", Value: "Foobar"},
			{Name: "second_parameter", Value: "true"},
			{Name: "third_parameter", Value: "789"},
		}
	})
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	// Start an agent so that we can generate stats.
	_ = agenttest.New(t, client.URL, authToken)
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	// Fake app usage
	reporter := workspaceapps.NewStatsDBReporter(db, workspaceapps.DefaultStatsDBReporterBatchSize)
	//nolint:gocritic // This is a test.
	err = reporter.Report(dbauthz.AsSystemRestricted(context.Background()), []workspaceapps.StatsReport{
		{
			UserID:           user.UserID,
			WorkspaceID:      workspace.ID,
			AgentID:          resources[0].Agents[0].ID,
			AccessMethod:     "terminal",
			SlugOrPort:       "golden-slug",
			SessionID:        uuid.New(),
			SessionStartedAt: time.Now().Add(-3 * time.Minute),
			SessionEndedAt:   time.Now().Add(-time.Minute).Add(-time.Second),
			Requests:         1,
		},
	})
	require.NoError(t, err, "want no error inserting app stats")

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Run metrics collector
	closeFunc, err := mc.Run(ctx)
	require.NoError(t, err)
	defer closeFunc()

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

	defer func() {
		_ = sess.Close()
		_ = sshConn.Close()
	}()

	goldenFile, err := os.ReadFile("testdata/insights-metrics.json")
	require.NoError(t, err)
	golden := map[string]int{}
	err = json.Unmarshal(goldenFile, &golden)
	require.NoError(t, err)

	collected := map[string]int{}
	assert.Eventuallyf(t, func() bool {
		// When
		metrics, err := registry.Gather()
		require.NoError(t, err)

		// Then
		for _, metric := range metrics {
			switch metric.GetName() {
			case "coderd_insights_applications_usage_seconds", "coderd_insights_templates_active_users", "coderd_insights_parameters":
				for _, m := range metric.Metric {
					key := metric.GetName()
					if len(m.Label) > 0 {
						key = key + "[" + metricLabelAsString(m) + "]"
					}
					collected[key] = int(m.Gauge.GetValue())
				}
			default:
				require.FailNowf(t, "unexpected metric collected", "metric: %s", metric.GetName())
			}
		}

		return insightsMetricsAreEqual(golden, collected)
	}, testutil.WaitMedium, testutil.IntervalFast, "template insights are inconsistent with golden files, got: %v", collected)
}

func metricLabelAsString(m *io_prometheus_client.Metric) string {
	var labels []string
	for _, labelPair := range m.Label {
		labels = append(labels, labelPair.GetName()+"="+labelPair.GetValue())
	}
	return strings.Join(labels, ",")
}

func provisionPlanWithParameters() []*proto.Response {
	return []*proto.Response{
		{
			Type: &proto.Response_Plan{
				Plan: &proto.PlanComplete{
					Parameters: []*proto.RichParameter{
						{Name: "first_parameter", Type: "string", Mutable: true},
						{Name: "second_parameter", Type: "bool", Mutable: true},
						{Name: "third_parameter", Type: "number", Mutable: true},
					},
				},
			},
		},
	}
}

func provisionApplyWithAgentAndApp(authToken string) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Apply{
			Apply: &proto.ApplyComplete{
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "aws_instance",
					Agents: []*proto.Agent{{
						Id:   uuid.NewString(),
						Name: "example",
						Auth: &proto.Agent_Token{
							Token: authToken,
						},
						Apps: []*proto.App{
							{
								Slug:         "golden-slug",
								DisplayName:  "Golden Slug",
								SharingLevel: proto.AppSharingLevel_OWNER,
								Url:          "http://localhost:1234",
							},
						},
					}},
				}},
			},
		},
	}}
}

// insightsMetricsAreEqual patches collected metrics to be used
// in comparison with golden metrics using `assert.ObjectsAreEqualValues`.
// Collected metrics must be patched as sometimes they may slip
// due to timestamp truncation.
// See:
// https://github.com/coder/coder/blob/92ef0baff3b632c52c2335aae1d643a3cc49e26a/coderd/database/dbmem/dbmem.go#L2463
// https://github.com/coder/coder/blob/9b6433e3a7c788b7e87b7d8f539ea111957a0cf1/coderd/database/queries/insights.sql#L246
func insightsMetricsAreEqual(golden, collected map[string]int) bool {
	greaterOrEqualKeys := []string{
		"coderd_insights_applications_usage_seconds[application_name=Golden Slug,slug=golden-slug,template_name=golden-template]",
		"coderd_insights_applications_usage_seconds[application_name=SSH,slug=,template_name=golden-template]",
	}
	for _, key := range greaterOrEqualKeys {
		if v, ok := collected[key]; ok && v > golden[key] {
			collected[key] = golden[key]
		}
	}
	return assert.ObjectsAreEqualValues(golden, collected)
}
