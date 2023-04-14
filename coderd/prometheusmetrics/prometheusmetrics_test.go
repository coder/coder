package prometheusmetrics_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/prometheusmetrics"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/tailnet/tailnettest"
	"github.com/coder/coder/testutil"
)

func TestActiveUsers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name     string
		Database func(t *testing.T) database.Store
		Count    int
	}{{
		Name: "None",
		Database: func(t *testing.T) database.Store {
			return dbfake.New()
		},
		Count: 0,
	}, {
		Name: "One",
		Database: func(t *testing.T) database.Store {
			db := dbfake.New()
			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now(),
			})
			return db
		},
		Count: 1,
	}, {
		Name: "OneWithExpired",
		Database: func(t *testing.T) database.Store {
			db := dbfake.New()

			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now(),
			})

			// Because this API key hasn't been used in the past hour, this shouldn't
			// add to the user count.
			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now().Add(-2 * time.Hour),
			})
			return db
		},
		Count: 1,
	}, {
		Name: "Multiple",
		Database: func(t *testing.T) database.Store {
			db := dbfake.New()
			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now(),
			})
			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now(),
			})
			return db
		},
		Count: 2,
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			closeFunc, err := prometheusmetrics.ActiveUsers(context.Background(), registry, tc.Database(t), time.Millisecond)
			require.NoError(t, err)
			t.Cleanup(closeFunc)

			require.Eventually(t, func() bool {
				metrics, err := registry.Gather()
				assert.NoError(t, err)
				result := int(*metrics[0].Metric[0].Gauge.Value)
				return result == tc.Count
			}, testutil.WaitShort, testutil.IntervalFast)
		})
	}
}

func TestWorkspaces(t *testing.T) {
	t.Parallel()

	insertRunning := func(db database.Store) database.ProvisionerJob {
		job, err := db.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			CreatedAt:     database.Now(),
			UpdatedAt:     database.Now(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
		})
		require.NoError(t, err)
		_, err = db.InsertWorkspaceBuild(context.Background(), database.InsertWorkspaceBuildParams{
			ID:          uuid.New(),
			WorkspaceID: uuid.New(),
			JobID:       job.ID,
			BuildNumber: 1,
			Transition:  database.WorkspaceTransitionStart,
			Reason:      database.BuildReasonInitiator,
		})
		require.NoError(t, err)
		// This marks the job as started.
		_, err = db.AcquireProvisionerJob(context.Background(), database.AcquireProvisionerJobParams{
			StartedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		return job
	}

	insertCanceled := func(db database.Store) {
		job := insertRunning(db)
		err := db.UpdateProvisionerJobWithCancelByID(context.Background(), database.UpdateProvisionerJobWithCancelByIDParams{
			ID: job.ID,
			CanceledAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
		})
		require.NoError(t, err)
		err = db.UpdateProvisionerJobWithCompleteByID(context.Background(), database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
		})
		require.NoError(t, err)
	}

	insertFailed := func(db database.Store) {
		job := insertRunning(db)
		err := db.UpdateProvisionerJobWithCompleteByID(context.Background(), database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
			Error: sql.NullString{
				String: "failed",
				Valid:  true,
			},
		})
		require.NoError(t, err)
	}

	insertSuccess := func(db database.Store) {
		job := insertRunning(db)
		err := db.UpdateProvisionerJobWithCompleteByID(context.Background(), database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
		})
		require.NoError(t, err)
	}

	for _, tc := range []struct {
		Name     string
		Database func() database.Store
		Total    int
		Status   map[codersdk.ProvisionerJobStatus]int
	}{{
		Name: "None",
		Database: func() database.Store {
			return dbfake.New()
		},
		Total: 0,
	}, {
		Name: "Multiple",
		Database: func() database.Store {
			db := dbfake.New()
			insertCanceled(db)
			insertFailed(db)
			insertFailed(db)
			insertSuccess(db)
			insertSuccess(db)
			insertSuccess(db)
			insertRunning(db)
			return db
		},
		Total: 7,
		Status: map[codersdk.ProvisionerJobStatus]int{
			codersdk.ProvisionerJobCanceled:  1,
			codersdk.ProvisionerJobFailed:    2,
			codersdk.ProvisionerJobSucceeded: 3,
			codersdk.ProvisionerJobRunning:   1,
		},
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			closeFunc, err := prometheusmetrics.Workspaces(context.Background(), registry, tc.Database(), time.Millisecond)
			require.NoError(t, err)
			t.Cleanup(closeFunc)

			require.Eventually(t, func() bool {
				metrics, err := registry.Gather()
				assert.NoError(t, err)
				if len(metrics) < 1 {
					return false
				}
				sum := 0
				for _, metric := range metrics[0].Metric {
					count, ok := tc.Status[codersdk.ProvisionerJobStatus(metric.Label[0].GetValue())]
					if metric.Gauge.GetValue() == 0 {
						continue
					}
					if !ok {
						t.Fail()
					}
					if metric.Gauge.GetValue() != float64(count) {
						return false
					}
					sum += int(metric.Gauge.GetValue())
				}
				t.Logf("sum %d == total %d", sum, tc.Total)
				return sum == tc.Total
			}, testutil.WaitShort, testutil.IntervalFast)
		})
	}
}

func TestAgents(t *testing.T) {
	t.Parallel()

	// Build a sample workspace with test agent and fake application
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	db := api.Database

	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.ProvisionComplete,
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:        uuid.NewString(),
							Name:      "testagent",
							Directory: t.TempDir(),
							Auth: &proto.Agent_Token{
								Token: uuid.NewString(),
							},
							Apps: []*proto.App{
								{
									Slug:         "fake-app",
									DisplayName:  "Fake application",
									SharingLevel: proto.AppSharingLevel_OWNER,
									// Hopefully this IP and port doesn't exist.
									Url: "http://127.1.0.1:65535",
								},
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	// given
	coordinator := tailnet.NewCoordinator()
	coordinatorPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordinatorPtr.Store(&coordinator)
	derpMap := tailnettest.RunDERPAndSTUN(t)
	agentInactiveDisconnectTimeout := 1 * time.Hour // don't need to focus on this value in tests
	registry := prometheus.NewRegistry()

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	// when
	closeFunc, err := prometheusmetrics.Agents(ctx, slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}), registry, db, &coordinatorPtr, derpMap, agentInactiveDisconnectTimeout, time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(closeFunc)

	// then
	var agentsUp bool
	var agentsConnections bool
	var agentsApps bool
	var agentsExecutionInSeconds bool
	require.Eventually(t, func() bool {
		metrics, err := registry.Gather()
		assert.NoError(t, err)

		if len(metrics) < 1 {
			return false
		}

		for _, metric := range metrics {
			switch metric.GetName() {
			case "coderd_agents_up":
				assert.Equal(t, "testuser", metric.Metric[0].Label[0].GetValue())     // Username
				assert.Equal(t, workspace.Name, metric.Metric[0].Label[1].GetValue()) // Workspace name
				assert.Equal(t, 1, int(metric.Metric[0].Gauge.GetValue()))            // Metric value
				agentsUp = true
			case "coderd_agents_connections":
				assert.Equal(t, "testagent", metric.Metric[0].Label[0].GetValue())    // Agent name
				assert.Equal(t, "created", metric.Metric[0].Label[1].GetValue())      // Lifecycle state
				assert.Equal(t, "connecting", metric.Metric[0].Label[2].GetValue())   // Status
				assert.Equal(t, "unknown", metric.Metric[0].Label[3].GetValue())      // Tailnet node
				assert.Equal(t, "testuser", metric.Metric[0].Label[4].GetValue())     // Username
				assert.Equal(t, workspace.Name, metric.Metric[0].Label[5].GetValue()) // Workspace name
				assert.Equal(t, 1, int(metric.Metric[0].Gauge.GetValue()))            // Metric value
				agentsConnections = true
			case "coderd_agents_apps":
				assert.Equal(t, "testagent", metric.Metric[0].Label[0].GetValue())        // Agent name
				assert.Equal(t, "Fake application", metric.Metric[0].Label[1].GetValue()) // App name
				assert.Equal(t, "disabled", metric.Metric[0].Label[2].GetValue())         // Health
				assert.Equal(t, "testuser", metric.Metric[0].Label[3].GetValue())         // Username
				assert.Equal(t, workspace.Name, metric.Metric[0].Label[4].GetValue())     // Workspace name
				assert.Equal(t, 1, int(metric.Metric[0].Gauge.GetValue()))                // Metric value
				agentsApps = true
			case "coderd_prometheusmetrics_agents_execution_seconds":
				agentsExecutionInSeconds = true
			default:
				require.FailNowf(t, "unexpected metric collected", "metric: %s", metric.GetName())
			}
		}
		return agentsUp && agentsConnections && agentsApps && agentsExecutionInSeconds
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestAgentStats(t *testing.T) {
	t.Parallel()

	// Build sample workspaces with test agents and fake agent client
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	db := api.Database

	user := coderdtest.CreateFirstUser(t, client)

	agent1 := prepareWorkspaceAndAgent(t, client, user, 1)
	agent2 := prepareWorkspaceAndAgent(t, client, user, 2)
	agent3 := prepareWorkspaceAndAgent(t, client, user, 3)

	registry := prometheus.NewRegistry()

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	// given
	var err error
	var i int64
	for i = 0; i < 3; i++ {
		_, err = agent1.PostStats(ctx, &agentsdk.Stats{
			TxBytes: 1 + i, RxBytes: 2 + i,
			SessionCountVSCode: 3 + i, SessionCountJetBrains: 4 + i, SessionCountReconnectingPTY: 5 + i, SessionCountSSH: 6 + i,
			ConnectionCount: 7 + i, ConnectionMedianLatencyMS: 8000,
			ConnectionsByProto: map[string]int64{"TCP": 1},
		})
		require.NoError(t, err)

		_, err = agent2.PostStats(ctx, &agentsdk.Stats{
			TxBytes: 2 + i, RxBytes: 4 + i,
			SessionCountVSCode: 6 + i, SessionCountJetBrains: 8 + i, SessionCountReconnectingPTY: 10 + i, SessionCountSSH: 12 + i,
			ConnectionCount: 8 + i, ConnectionMedianLatencyMS: 10000,
			ConnectionsByProto: map[string]int64{"TCP": 1},
		})
		require.NoError(t, err)

		_, err = agent3.PostStats(ctx, &agentsdk.Stats{
			TxBytes: 3 + i, RxBytes: 6 + i,
			SessionCountVSCode: 12 + i, SessionCountJetBrains: 14 + i, SessionCountReconnectingPTY: 16 + i, SessionCountSSH: 18 + i,
			ConnectionCount: 9 + i, ConnectionMedianLatencyMS: 12000,
			ConnectionsByProto: map[string]int64{"TCP": 1},
		})
		require.NoError(t, err)
	}

	// when
	//
	// Set initialCreateAfter to some time in the past, so that AgentStats would include all above PostStats,
	// and it doesn't depend on the real time.
	closeFunc, err := prometheusmetrics.AgentStats(ctx, slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}), registry, db, time.Now().Add(-time.Minute), time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(closeFunc)

	// then
	goldenFile, err := os.ReadFile("testdata/agent-stats.json")
	require.NoError(t, err)
	golden := map[string]int{}
	err = json.Unmarshal(goldenFile, &golden)
	require.NoError(t, err)

	collected := map[string]int{}
	var executionSeconds bool
	assert.Eventually(t, func() bool {
		metrics, err := registry.Gather()
		assert.NoError(t, err)

		if len(metrics) < 1 {
			return false
		}

		for _, metric := range metrics {
			switch metric.GetName() {
			case "coderd_prometheusmetrics_agentstats_execution_seconds":
				executionSeconds = true
			case "coderd_agentstats_connection_count",
				"coderd_agentstats_connection_median_latency_seconds",
				"coderd_agentstats_rx_bytes",
				"coderd_agentstats_tx_bytes",
				"coderd_agentstats_session_count_jetbrains",
				"coderd_agentstats_session_count_reconnecting_pty",
				"coderd_agentstats_session_count_ssh",
				"coderd_agentstats_session_count_vscode":
				for _, m := range metric.Metric {
					// username:workspace:agent:metric = value
					collected[m.Label[1].GetValue()+":"+m.Label[2].GetValue()+":"+m.Label[0].GetValue()+":"+metric.GetName()] = int(m.Gauge.GetValue())
				}
			default:
				require.FailNowf(t, "unexpected metric collected", "metric: %s", metric.GetName())
			}
		}
		return executionSeconds && reflect.DeepEqual(golden, collected)
	}, testutil.WaitShort, testutil.IntervalFast)

	// Keep this assertion, so that "go test" can print differences instead of "Condition never satisfied"
	assert.EqualValues(t, golden, collected)
}

func prepareWorkspaceAndAgent(t *testing.T, client *codersdk.Client, user codersdk.CreateFirstUserResponse, workspaceNum int) *agentsdk.Client {
	authToken := uuid.NewString()

	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.ProvisionComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.Name = fmt.Sprintf("workspace-%d", workspaceNum)
	})
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	return agentClient
}
