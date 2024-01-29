package workspacetraffic_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/exp/slices"
	"nhooyr.io/websocket"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/scaletest/workspacetraffic"
	"github.com/coder/coder/v2/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Test not supported on windows.")
	}
	if testutil.RaceEnabled() {
		t.Skip("Race detector enabled, skipping time-sensitive test.")
	}

	//nolint:dupl
	t.Run("RPTY", func(t *testing.T) {
		t.Parallel()
		// We need to stand up an in-memory coderd and run a fake workspace.
		var (
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			firstUser = coderdtest.CreateFirstUser(t, client)
			authToken = uuid.NewString()
			agentName = "agent"
			version   = coderdtest.CreateTemplateVersion(t, client, firstUser.OrganizationID, &echo.Responses{
				Parse:         echo.ParseComplete,
				ProvisionPlan: echo.PlanComplete,
				ProvisionApply: []*proto.Response{{
					Type: &proto.Response_Apply{
						Apply: &proto.ApplyComplete{
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
			_        = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			// In order to be picked up as a scaletest workspace, the workspace must be named specifically
			ws = coderdtest.CreateWorkspace(t, client, firstUser.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.Name = "scaletest-test"
			})
			_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		)

		// We also need a running agent to run this test.
		_ = agenttest.New(t, client.URL, authToken)
		resources := coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Make sure the agent is connected before we go any further.
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
			readMetrics  = &testMetrics{}
			writeMetrics = &testMetrics{}
		)
		runner := workspacetraffic.NewRunner(client, workspacetraffic.Config{
			AgentID:      agentID,
			BytesPerTick: int64(bytesPerTick),
			TickInterval: tickInterval,
			Duration:     testutil.WaitLong,
			ReadMetrics:  readMetrics,
			WriteMetrics: writeMetrics,
			SSH:          false,
			Echo:         false,
		})

		var logs strings.Builder

		runDone := make(chan struct{})
		go func() {
			defer close(runDone)
			err := runner.Run(ctx, "", &logs)
			assert.NoError(t, err, "unexpected error calling Run()")
		}()

		gotMetrics := make(chan struct{})
		go func() {
			defer close(gotMetrics)
			// Wait until we get some non-zero metrics before canceling.
			assert.Eventually(t, func() bool {
				readLatencies := readMetrics.Latencies()
				writeLatencies := writeMetrics.Latencies()
				return len(readLatencies) > 0 &&
					len(writeLatencies) > 0 &&
					slices.ContainsFunc(readLatencies, func(f float64) bool { return f > 0.0 }) &&
					slices.ContainsFunc(writeLatencies, func(f float64) bool { return f > 0.0 })
			}, testutil.WaitLong, testutil.IntervalMedium, "expected non-zero metrics")
		}()

		// Stop the test after we get some non-zero metrics.
		<-gotMetrics
		cancel()
		<-runDone

		t.Logf("read errors: %.0f\n", readMetrics.Errors())
		t.Logf("write errors: %.0f\n", writeMetrics.Errors())
		t.Logf("bytes read total: %.0f\n", readMetrics.Total())
		t.Logf("bytes written total: %.0f\n", writeMetrics.Total())

		// Ensure something was both read and written.
		assert.NotZero(t, readMetrics.Total())
		assert.NotZero(t, writeMetrics.Total())
		// We want to ensure the metrics are somewhat accurate.
		assert.InDelta(t, writeMetrics.Total(), readMetrics.Total(), float64(bytesPerTick)*10)
		// Latency should report non-zero values.
		assert.NotEmpty(t, readMetrics.Latencies())
		assert.NotEmpty(t, writeMetrics.Latencies())
		// Should not report any errors!
		assert.Zero(t, readMetrics.Errors())
		assert.Zero(t, writeMetrics.Errors())
	})

	//nolint:dupl
	t.Run("SSH", func(t *testing.T) {
		t.Parallel()
		// We need to stand up an in-memory coderd and run a fake workspace.
		var (
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			firstUser = coderdtest.CreateFirstUser(t, client)
			authToken = uuid.NewString()
			agentName = "agent"
			version   = coderdtest.CreateTemplateVersion(t, client, firstUser.OrganizationID, &echo.Responses{
				Parse:         echo.ParseComplete,
				ProvisionPlan: echo.PlanComplete,
				ProvisionApply: []*proto.Response{{
					Type: &proto.Response_Apply{
						Apply: &proto.ApplyComplete{
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
			_        = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			// In order to be picked up as a scaletest workspace, the workspace must be named specifically
			ws = coderdtest.CreateWorkspace(t, client, firstUser.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.Name = "scaletest-test"
			})
			_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		)

		// We also need a running agent to run this test.
		_ = agenttest.New(t, client.URL, authToken)
		resources := coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Make sure the agent is connected before we go any further.
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
			readMetrics  = &testMetrics{}
			writeMetrics = &testMetrics{}
		)
		runner := workspacetraffic.NewRunner(client, workspacetraffic.Config{
			AgentID:      agentID,
			BytesPerTick: int64(bytesPerTick),
			TickInterval: tickInterval,
			Duration:     testutil.WaitLong,
			ReadMetrics:  readMetrics,
			WriteMetrics: writeMetrics,
			SSH:          true,
			Echo:         true,
		})

		var logs strings.Builder

		runDone := make(chan struct{})
		go func() {
			defer close(runDone)
			err := runner.Run(ctx, "", &logs)
			assert.NoError(t, err, "unexpected error calling Run()")
		}()

		gotMetrics := make(chan struct{})
		go func() {
			defer close(gotMetrics)
			// Wait until we get some non-zero metrics before canceling.
			assert.Eventually(t, func() bool {
				readLatencies := readMetrics.Latencies()
				writeLatencies := writeMetrics.Latencies()
				return len(readLatencies) > 0 &&
					len(writeLatencies) > 0 &&
					slices.ContainsFunc(readLatencies, func(f float64) bool { return f > 0.0 }) &&
					slices.ContainsFunc(writeLatencies, func(f float64) bool { return f > 0.0 })
			}, testutil.WaitLong, testutil.IntervalMedium, "expected non-zero metrics")
		}()

		// Stop the test after we get some non-zero metrics.
		<-gotMetrics
		cancel()
		<-runDone

		t.Logf("read errors: %.0f\n", readMetrics.Errors())
		t.Logf("write errors: %.0f\n", writeMetrics.Errors())
		t.Logf("bytes read total: %.0f\n", readMetrics.Total())
		t.Logf("bytes written total: %.0f\n", writeMetrics.Total())

		// Ensure something was both read and written.
		assert.NotZero(t, readMetrics.Total())
		assert.NotZero(t, writeMetrics.Total())
		// We want to ensure the metrics are somewhat accurate.
		assert.InDelta(t, writeMetrics.Total(), readMetrics.Total(), float64(bytesPerTick)*10)
		// Latency should report non-zero values.
		assert.NotEmpty(t, readMetrics.Latencies())
		assert.NotEmpty(t, writeMetrics.Latencies())
		// Should not report any errors!
		assert.Zero(t, readMetrics.Errors())
		assert.Zero(t, writeMetrics.Errors())
	})

	t.Run("App", func(t *testing.T) {
		t.Parallel()

		// Start a test server that will echo back the request body, this skips
		// the roundtrip to coderd/agent and simply tests the http request conn
		// directly.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
			if err != nil {
				t.Error(err)
				return
			}

			nc := websocket.NetConn(context.Background(), c, websocket.MessageBinary)
			defer nc.Close()

			_, err = io.Copy(nc, nc)
			if err == nil || errors.Is(err, io.EOF) {
				return
			}
			// Ignore policy violations, we expect these, e.g.:
			//
			// 	failed to get reader: received close frame: status = StatusPolicyViolation and reason = "timed out"
			if websocket.CloseStatus(err) == websocket.StatusPolicyViolation {
				return
			}

			t.Error(err)
		}))
		defer srv.Close()

		// Now we can start the runner.
		var (
			bytesPerTick = 1024
			tickInterval = 1000 * time.Millisecond
			readMetrics  = &testMetrics{}
			writeMetrics = &testMetrics{}
		)
		client := &codersdk.Client{
			HTTPClient: &http.Client{},
		}
		runner := workspacetraffic.NewRunner(client, workspacetraffic.Config{
			BytesPerTick: int64(bytesPerTick),
			TickInterval: tickInterval,
			Duration:     testutil.WaitLong,
			ReadMetrics:  readMetrics,
			WriteMetrics: writeMetrics,
			App: workspacetraffic.AppConfig{
				Name: "echo",
				URL:  srv.URL,
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var logs strings.Builder

		runDone := make(chan struct{})
		go func() {
			defer close(runDone)
			err := runner.Run(ctx, "", &logs)
			assert.NoError(t, err, "unexpected error calling Run()")
		}()

		gotMetrics := make(chan struct{})
		go func() {
			defer close(gotMetrics)
			// Wait until we get some non-zero metrics before canceling.
			assert.Eventually(t, func() bool {
				readLatencies := readMetrics.Latencies()
				writeLatencies := writeMetrics.Latencies()
				return len(readLatencies) > 0 &&
					len(writeLatencies) > 0 &&
					slices.ContainsFunc(readLatencies, func(f float64) bool { return f > 0.0 }) &&
					slices.ContainsFunc(writeLatencies, func(f float64) bool { return f > 0.0 })
			}, testutil.WaitLong, testutil.IntervalMedium, "expected non-zero metrics")
		}()

		// Stop the test after we get some non-zero metrics.
		<-gotMetrics
		cancel()
		<-runDone

		t.Logf("read errors: %.0f\n", readMetrics.Errors())
		t.Logf("write errors: %.0f\n", writeMetrics.Errors())
		t.Logf("bytes read total: %.0f\n", readMetrics.Total())
		t.Logf("bytes written total: %.0f\n", writeMetrics.Total())

		// Ensure something was both read and written.
		assert.NotZero(t, readMetrics.Total())
		assert.NotZero(t, writeMetrics.Total())
		// We want to ensure the metrics are somewhat accurate.
		assert.InDelta(t, writeMetrics.Total(), readMetrics.Total(), float64(bytesPerTick)*10)
		// Latency should report non-zero values.
		assert.NotEmpty(t, readMetrics.Latencies())
		assert.NotEmpty(t, writeMetrics.Latencies())
		// Should not report any errors!
		assert.Zero(t, readMetrics.Errors())
		assert.Zero(t, writeMetrics.Errors())
	})
}

type testMetrics struct {
	sync.Mutex
	errors    float64
	latencies []float64
	total     float64
}

var _ workspacetraffic.ConnMetrics = (*testMetrics)(nil)

func (m *testMetrics) AddError(f float64) {
	m.Lock()
	defer m.Unlock()
	m.errors += f
}

func (m *testMetrics) ObserveLatency(f float64) {
	m.Lock()
	defer m.Unlock()
	m.latencies = append(m.latencies, f)
}

func (m *testMetrics) AddTotal(f float64) {
	m.Lock()
	defer m.Unlock()
	m.total += f
}

func (m *testMetrics) Total() float64 {
	m.Lock()
	defer m.Unlock()
	return m.total
}

func (m *testMetrics) Errors() float64 {
	m.Lock()
	defer m.Unlock()
	return m.errors
}

func (m *testMetrics) Latencies() []float64 {
	m.Lock()
	defer m.Unlock()
	return m.latencies
}
