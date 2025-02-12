package insights_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/prometheusmetrics/insights"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCollectInsights(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())

	options := &coderdtest.Options{
		IncludeProvisionerDaemon:  true,
		AgentStatsRefreshInterval: time.Millisecond * 100,
		Database:                  db,
		Pubsub:                    ps,
	}
	ownerClient := coderdtest.New(t, options)
	ownerClient.SetLogger(logger.Named("ownerClient").Leveled(slog.LevelDebug))
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	client, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	// Given
	// Initialize metrics collector
	mc, err := insights.NewMetricsCollector(db, logger, 0, time.Second)
	require.NoError(t, err)

	registry := prometheus.NewRegistry()
	registry.Register(mc)

	var (
		orgID      = owner.OrganizationID
		tpl        = dbgen.Template(t, db, database.Template{OrganizationID: orgID, CreatedBy: user.ID, Name: "golden-template"})
		ver        = dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: orgID, CreatedBy: user.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		param1     = dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{TemplateVersionID: ver.ID, Name: "first_parameter"})
		param2     = dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{TemplateVersionID: ver.ID, Name: "second_parameter", Type: "bool"})
		param3     = dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{TemplateVersionID: ver.ID, Name: "third_parameter", Type: "number"})
		workspace1 = dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: orgID, TemplateID: tpl.ID, OwnerID: user.ID})
		workspace2 = dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: orgID, TemplateID: tpl.ID, OwnerID: user.ID})
		job1       = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: orgID})
		job2       = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: orgID})
		build1     = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{TemplateVersionID: ver.ID, WorkspaceID: workspace1.ID, JobID: job1.ID})
		build2     = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{TemplateVersionID: ver.ID, WorkspaceID: workspace2.ID, JobID: job2.ID})
		res1       = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build1.JobID})
		res2       = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build2.JobID})
		agent1     = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res1.ID})
		agent2     = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res2.ID})
		app1       = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: agent1.ID, Slug: "golden-slug", DisplayName: "Golden Slug"})
		app2       = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: agent2.ID, Slug: "golden-slug", DisplayName: "Golden Slug"})
		_          = dbgen.WorkspaceBuildParameters(t, db, []database.WorkspaceBuildParameter{
			{WorkspaceBuildID: build1.ID, Name: param1.Name, Value: "Foobar"},
			{WorkspaceBuildID: build1.ID, Name: param2.Name, Value: "true"},
			{WorkspaceBuildID: build1.ID, Name: param3.Name, Value: "789"},
		})
		_ = dbgen.WorkspaceBuildParameters(t, db, []database.WorkspaceBuildParameter{
			{WorkspaceBuildID: build2.ID, Name: param1.Name, Value: "Baz"},
			{WorkspaceBuildID: build2.ID, Name: param2.Name, Value: "true"},
			{WorkspaceBuildID: build2.ID, Name: param3.Name, Value: "999"},
		})
	)

	// Start an agent so that we can generate stats.
	var agentClients []agentproto.DRPCAgentClient
	for i, agent := range []database.WorkspaceAgent{agent1, agent2} {
		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(agent.AuthToken.String())
		agentClient.SDK.SetLogger(logger.Leveled(slog.LevelDebug).Named(fmt.Sprintf("agent%d", i+1)))
		conn, err := agentClient.ConnectRPC(context.Background())
		require.NoError(t, err)
		agentAPI := agentproto.NewDRPCAgentClient(conn)
		agentClients = append(agentClients, agentAPI)
	}

	defer func() {
		for a := range agentClients {
			err := agentClients[a].DRPCConn().Close()
			require.NoError(t, err)
		}
	}()

	// Fake app stats
	_, err = agentClients[0].UpdateStats(context.Background(), &agentproto.UpdateStatsRequest{
		Stats: &agentproto.Stats{
			// ConnectionCount must be positive as database query ignores stats with no active connections at the time frame
			ConnectionsByProto:        map[string]int64{"TCP": 1},
			ConnectionCount:           1,
			ConnectionMedianLatencyMs: 15,
			// Session counts must be positive, but the exact value is ignored.
			// Database query approximates it to 60s of usage.
			SessionCountSsh:       99,
			SessionCountJetbrains: 47,
			SessionCountVscode:    34,
		},
	})
	require.NoError(t, err, "unable to post fake stats")

	// Fake app usage
	reporter := workspacestats.NewReporter(workspacestats.ReporterOptions{
		Database:         db,
		AppStatBatchSize: workspaceapps.DefaultStatsDBReporterBatchSize,
	})
	refTime := time.Now().Add(-3 * time.Minute).Truncate(time.Minute)
	//nolint:gocritic // This is a test.
	err = reporter.ReportAppStats(dbauthz.AsSystemRestricted(context.Background()), []workspaceapps.StatsReport{
		{
			UserID:           user.ID,
			WorkspaceID:      workspace1.ID,
			AgentID:          agent1.ID,
			AccessMethod:     "path",
			SlugOrPort:       app1.Slug,
			SessionID:        uuid.New(),
			SessionStartedAt: refTime,
			SessionEndedAt:   refTime.Add(2 * time.Minute).Add(-time.Second),
			Requests:         1,
		},
		// Same usage on differrent workspace/agent in same template,
		// should not be counted as extra.
		{
			UserID:           user.ID,
			WorkspaceID:      workspace2.ID,
			AgentID:          agent2.ID,
			AccessMethod:     "path",
			SlugOrPort:       app2.Slug,
			SessionID:        uuid.New(),
			SessionStartedAt: refTime,
			SessionEndedAt:   refTime.Add(2 * time.Minute).Add(-time.Second),
			Requests:         1,
		},
		{
			UserID:           user.ID,
			WorkspaceID:      workspace2.ID,
			AgentID:          agent2.ID,
			AccessMethod:     "path",
			SlugOrPort:       app2.Slug,
			SessionID:        uuid.New(),
			SessionStartedAt: refTime.Add(2 * time.Minute),
			SessionEndedAt:   refTime.Add(2 * time.Minute).Add(30 * time.Second),
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

	goldenFile, err := os.ReadFile("testdata/insights-metrics.json")
	require.NoError(t, err)
	golden := map[string]int{}
	err = json.Unmarshal(goldenFile, &golden)
	require.NoError(t, err)

	collected := map[string]int{}
	ok := assert.Eventuallyf(t, func() bool {
		// When
		metrics, err := registry.Gather()
		if !assert.NoError(t, err) {
			return false
		}

		// Then
		for _, metric := range metrics {
			t.Logf("metric: %s: %#v", metric.GetName(), metric)
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
				assert.Failf(t, "unexpected metric collected", "metric: %s", metric.GetName())
			}
		}

		return assert.ObjectsAreEqualValues(golden, collected)
	}, testutil.WaitMedium, testutil.IntervalFast, "template insights are inconsistent with golden files")
	if !ok {
		diff := cmp.Diff(golden, collected)
		assert.Empty(t, diff, "template insights are inconsistent with golden files (-golden +collected)")
	}
}

func metricLabelAsString(m *io_prometheus_client.Metric) string {
	var labels []string
	for _, labelPair := range m.Label {
		labels = append(labels, labelPair.GetName()+"="+labelPair.GetValue())
	}
	return strings.Join(labels, ",")
}
