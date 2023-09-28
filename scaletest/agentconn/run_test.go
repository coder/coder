package agentconn_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/scaletest/agentconn"
	"github.com/coder/coder/v2/testutil"
)

func Test_Runner(t *testing.T) {
	t.Parallel()

	t.Run("Derp+Simple", func(t *testing.T) {
		t.Parallel()

		client, agentID := setupRunnerTest(t)

		runner := agentconn.NewRunner(client, agentconn.Config{
			AgentID:        agentID,
			ConnectionMode: agentconn.ConnectionModeDerp,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logStr := logs.String()
		t.Log("Runner logs:\n\n" + logStr)
		require.NoError(t, err)

		require.Contains(t, logStr, "Opening connection to workspace agent")
		require.Contains(t, logStr, "Using proxied DERP connection")
		require.Contains(t, logStr, "Disco ping attempt 1/10...")
		require.Contains(t, logStr, "Connection established")
		require.Contains(t, logStr, "Verify connection attempt 1/30...")
		require.Contains(t, logStr, "Connection verified")
		require.NotContains(t, logStr, "Performing initial service connections")
		require.NotContains(t, logStr, "Starting connection loops")
		require.NotContains(t, logStr, "Waiting for ")
	})

	t.Run("Derp+ServicesNoHold", func(t *testing.T) {
		t.Parallel()

		client, agentID := setupRunnerTest(t)
		service1URL, service1Count := testServer(t)
		service2URL, service2Count := testServer(t)

		runner := agentconn.NewRunner(client, agentconn.Config{
			AgentID:        agentID,
			ConnectionMode: agentconn.ConnectionModeDerp,
			HoldDuration:   0,
			Connections: []agentconn.Connection{
				{
					URL:     service1URL,
					Timeout: httpapi.Duration(time.Second),
				},
				{
					URL:     service2URL,
					Timeout: httpapi.Duration(time.Second),
				},
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logStr := logs.String()
		t.Log("Runner logs:\n\n" + logStr)
		require.NoError(t, err)

		require.Contains(t, logStr, "Opening connection to workspace agent")
		require.Contains(t, logStr, "Using proxied DERP connection")
		require.Contains(t, logStr, "Disco ping attempt 1/10...")
		require.Contains(t, logStr, "Connection established")
		require.Contains(t, logStr, "Verify connection attempt 1/30...")
		require.Contains(t, logStr, "Connection verified")
		require.Contains(t, logStr, "Performing initial service connections")
		require.Contains(t, logStr, "0. "+service1URL)
		require.Contains(t, logStr, "1. "+service2URL)
		require.NotContains(t, logStr, "Starting connection loops")
		require.NotContains(t, logStr, "Waiting for ")

		require.EqualValues(t, 1, service1Count())
		require.EqualValues(t, 1, service2Count())
	})
}

//nolint:paralleltest // Measures timing as part of the test.
func Test_Runner_Timing(t *testing.T) {
	testutil.SkipIfNotTiming(t)
	//nolint:paralleltest
	t.Run("Direct+Hold", func(t *testing.T) {
		client, agentID := setupRunnerTest(t)

		runner := agentconn.NewRunner(client, agentconn.Config{
			AgentID:        agentID,
			ConnectionMode: agentconn.ConnectionModeDirect,
			HoldDuration:   httpapi.Duration(testutil.WaitShort),
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		logs := bytes.NewBuffer(nil)
		start := time.Now()
		err := runner.Run(ctx, "1", logs)
		logStr := logs.String()
		t.Log("Runner logs:\n\n" + logStr)
		require.NoError(t, err)

		require.WithinRange(t,
			time.Now(),
			start.Add(testutil.WaitShort-time.Second),
			start.Add(testutil.WaitShort+5*time.Second),
		)

		require.Contains(t, logStr, "Opening connection to workspace agent")
		require.Contains(t, logStr, "Using direct connection")
		require.Contains(t, logStr, "Disco ping attempt 1/10...")
		require.Contains(t, logStr, "Direct connection check 1/30...")
		require.Contains(t, logStr, "Connection established")
		require.Contains(t, logStr, "Verify connection attempt 1/30...")
		require.Contains(t, logStr, "Connection verified")
		require.NotContains(t, logStr, "Performing initial service connections")
		require.NotContains(t, logStr, "Starting connection loops")
		require.Contains(t, logStr, fmt.Sprintf("Waiting for %s", testutil.WaitShort))
	})

	//nolint:paralleltest
	t.Run("Derp+Hold+Services", func(t *testing.T) {
		client, agentID := setupRunnerTest(t)
		service1URL, service1Count := testServer(t)
		service2URL, service2Count := testServer(t)
		service3URL, service3Count := testServer(t)

		runner := agentconn.NewRunner(client, agentconn.Config{
			AgentID:        agentID,
			ConnectionMode: agentconn.ConnectionModeDerp,
			HoldDuration:   httpapi.Duration(testutil.WaitShort),
			Connections: []agentconn.Connection{
				{
					URL: service1URL,
					// No interval.
					Timeout: httpapi.Duration(time.Second),
				},
				{
					URL:      service2URL,
					Interval: httpapi.Duration(1 * time.Second),
					Timeout:  httpapi.Duration(time.Second),
				},
				{
					URL:      service3URL,
					Interval: httpapi.Duration(500 * time.Millisecond),
					Timeout:  httpapi.Duration(time.Second),
				},
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		logs := bytes.NewBuffer(nil)
		start := time.Now()
		err := runner.Run(ctx, "1", logs)
		logStr := logs.String()
		t.Log("Runner logs:\n\n" + logStr)
		require.NoError(t, err)

		require.WithinRange(t,
			time.Now(),
			start.Add(testutil.WaitShort-time.Second),
			start.Add(testutil.WaitShort+10*time.Second),
		)

		require.Contains(t, logStr, "Opening connection to workspace agent")
		require.Contains(t, logStr, "Using proxied DERP connection")
		require.Contains(t, logStr, "Disco ping attempt 1/10...")
		require.Contains(t, logStr, "Connection established")
		require.Contains(t, logStr, "Verify connection attempt 1/30...")
		require.Contains(t, logStr, "Connection verified")
		require.Contains(t, logStr, "Performing initial service connections")
		require.Contains(t, logStr, "0. "+service1URL)
		require.Contains(t, logStr, "1. "+service2URL)
		require.Contains(t, logStr, "Starting connection loops")
		require.NotContains(t, logStr, fmt.Sprintf("OK: %s (0)", service1URL))
		require.Contains(t, logStr, fmt.Sprintf("OK: %s (1)", service2URL))
		require.Contains(t, logStr, fmt.Sprintf("OK: %s (2)", service3URL))
		require.Contains(t, logStr, fmt.Sprintf("Waiting for %s", testutil.WaitShort))

		t.Logf("service 1 called %d times", service1Count())
		t.Logf("service 2 called %d times", service2Count())
		t.Logf("service 3 called %d times", service3Count())
		require.EqualValues(t, 1, service1Count())
		require.NotEqualValues(t, 1, service2Count())
		require.NotEqualValues(t, 1, service3Count())
		// service 3 should've been called way more times than service 2
		require.True(t, service3Count() > service2Count()+2)
	})
}

func setupRunnerTest(t *testing.T) (client *codersdk.Client, agentID uuid.UUID) {
	t.Helper()

	client = coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)

	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.PlanComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "agent",
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

	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	_ = agenttest.New(t, client.URL, authToken)
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	return client, resources[0].Agents[0].ID
}

func testServer(t *testing.T) (string, func() int64) {
	t.Helper()

	var count int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&count, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	return srv.URL, func() int64 {
		return atomic.LoadInt64(&count)
	}
}
