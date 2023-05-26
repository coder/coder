package workspacetraffic_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/scaletest/workspacetraffic"
	"github.com/coder/coder/testutil"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Parallel()

	// We need to stand up an in-memory coderd and run a fake workspace.
	var (
		client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		firstUser = coderdtest.CreateFirstUser(t, client)
		authToken = uuid.NewString()
		agentName = "agent"
		version   = coderdtest.CreateTemplateVersion(t, client, firstUser.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.ProvisionComplete,
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agents: []*proto.Agent{{
								// Agent ID gets generated no matter what we say ¯\_(ツ)_/¯
								Name: agentName,
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
								Apps: []*proto.App{},
							}},
						}},
					},
				},
			}},
		})
		template = coderdtest.CreateTemplate(t, client, firstUser.OrganizationID, version.ID)
		_        = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		// In order to be picked up as a scaletest workspace, the workspace must be named specifically
		ws = coderdtest.CreateWorkspace(t, client, firstUser.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.Name = "scaletest-test"
		})
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
	)

	// We also need a running agent to run this test.
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	// We actually need to know the full user and not just the UserID / OrgID
	user, err := client.User(ctx, firstUser.UserID.String())
	require.NoError(t, err, "get first user")

	// Make sure the agent is connected before we go any further.
	resources := coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)
	var agentID uuid.UUID
	for _, res := range resources {
		for _, agt := range res.Agents {
			agentID = agt.ID
		}
	}
	require.NotEqual(t, uuid.Nil, agentID, "did not expect agentID to be nil")

	// Now we can start the runner.
	var (
		bytesPerTick = 1024
		tickInterval = 1000 * time.Millisecond
		cancelAfter  = 1500 * time.Millisecond
		fudgeWrite   = 12 // The ReconnectingPTY payload incurs some overhead
	)
	reg := prometheus.NewRegistry()
	metrics := workspacetraffic.NewMetrics(reg, "username", "workspace_name", "agent_name")
	runner := workspacetraffic.NewRunner(client, workspacetraffic.Config{
		AgentID:        agentID,
		AgentName:      agentName,
		WorkspaceName:  ws.Name,
		WorkspaceOwner: ws.OwnerName,
		BytesPerTick:   int64(bytesPerTick),
		TickInterval:   tickInterval,
		Duration:       testutil.WaitLong,
		Registry:       reg,
	}, metrics)

	var logs strings.Builder
	// Stop the test after one 'tick'. This will cause an EOF.
	go func() {
		<-time.After(cancelAfter)
		cancel()
	}()
	require.NoError(t, runner.Run(ctx, "", &logs), "unexpected error calling Run()")

	// We want to ensure the metrics are somewhat accurate.
	lvs := []string{user.Username, ws.Name, agentName}
	assert.InDelta(t, bytesPerTick+fudgeWrite, toFloat64(t, metrics.BytesWrittenTotal.WithLabelValues(lvs...)), 0.1)
	// Read is highly variable, depending on how far we read before stopping.
	// Just ensure it's not zero.
	assert.NotZero(t, bytesPerTick, toFloat64(t, metrics.BytesReadTotal.WithLabelValues(lvs...)))
	// Latency should report non-zero values.
	assert.NotZero(t, toFloat64(t, metrics.ReadLatencySeconds))
	assert.NotZero(t, toFloat64(t, metrics.WriteLatencySeconds))
	// Should not report any errors!
	assert.Zero(t, toFloat64(t, metrics.ReadErrorsTotal.WithLabelValues(lvs...)))
	assert.Zero(t, toFloat64(t, metrics.ReadErrorsTotal.WithLabelValues(lvs...)))
}

// toFloat64 version of Prometheus' testutil.ToFloat64 that integrates with
// github.com/stretchr/testify/require and handles histograms (somewhat)
func toFloat64(t testing.TB, c prometheus.Collector) float64 {
	var (
		m      prometheus.Metric
		mCount int
		mChan  = make(chan prometheus.Metric)
		done   = make(chan struct{})
	)

	go func() {
		for m = range mChan {
			mCount++
		}
		close(done)
	}()

	c.Collect(mChan)
	close(mChan)
	<-done

	require.Equal(t, 1, mCount, "expected exactly 1 metric but got %d", mCount)

	pb := &dto.Metric{}
	require.NoError(t, m.Write(pb), "unexpected error collecting metrics")

	if pb.Gauge != nil {
		return pb.Gauge.GetValue()
	}
	if pb.Counter != nil {
		return pb.Counter.GetValue()
	}
	if pb.Untyped != nil {
		return pb.Untyped.GetValue()
	}
	if pb.Histogram != nil {
		// If no samples, just return zero.
		if pb.Histogram.GetSampleCount() == 0 {
			return 0
		}
		// Average is sufficient for testing purposes.
		return pb.Histogram.GetSampleSum() / pb.Histogram.GetSampleCountFloat()
	}
	require.Fail(t, "collected a non-gauge/counter/untyped/histogram metric: %s", pb)
	return 0
}
