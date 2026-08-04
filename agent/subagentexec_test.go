package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/subagentexec"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

// fakeExecDriver records the launches the agent's manager hands it. It
// cannot read the child's auth token: Launch keeps it unexported, which is
// the point of the seam.
type fakeExecDriver struct {
	mu       sync.Mutex
	launches []subagentexec.Launch
	startCh  chan subagentexec.Launch
}

func newFakeExecDriver() *fakeExecDriver {
	return &fakeExecDriver{startCh: make(chan subagentexec.Launch, 8)}
}

func (d *fakeExecDriver) Start(_ context.Context, launch subagentexec.Launch) (subagentexec.Process, error) {
	d.mu.Lock()
	d.launches = append(d.launches, launch)
	d.mu.Unlock()

	d.startCh <- launch
	return &fakeExecProcess{exit: make(chan struct{})}, nil
}

func (d *fakeExecDriver) startCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.launches)
}

type fakeExecProcess struct {
	exit     chan struct{}
	stopOnce sync.Once
}

func (p *fakeExecProcess) Wait() error {
	<-p.exit
	return nil
}

func (p *fakeExecProcess) Stop(context.Context) error {
	p.stopOnce.Do(func() { close(p.exit) })
	return nil
}

func testSubagentExecution() agentsdk.SubagentExecution {
	return agentsdk.SubagentExecution{
		ExecutionID:     uuid.New(),
		Generation:      uuid.New(),
		Name:            "sandbox",
		Driver:          "bubblewrap",
		DriverProtocol:  1,
		SharedHostPath:  "/home/coder/project",
		SharedChildPath: "/workspace/project",
		StartupTimeout:  time.Minute,
		RestartPolicy:   "never",
	}
}

// startExecAgent brings up an agent against the fake agent API with the
// given manifest, without the full tailnet fixture: manifest handling and
// the subagent execution manager do not need a reachable network.
func startExecAgent(t *testing.T, manifest agentsdk.Manifest, driver subagentexec.Driver, wrap func(agent.Client) agent.Client) (*agenttest.Client, agent.Agent) {
	t.Helper()

	logger := testutil.Logger(t)
	coordinator := tailnettest.NewFakeCoordinator()
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	manifest.DERPMap = derpMap
	if manifest.AgentID == uuid.Nil {
		manifest.AgentID = uuid.New()
	}
	// A start script gives every case the same deterministic marker for
	// "the manifest has been handled".
	manifest.Scripts = append(manifest.Scripts, codersdk.WorkspaceAgentScript{
		ID:          uuid.New(),
		LogSourceID: uuid.New(),
		Script:      "echo subagentexec",
		Timeout:     testutil.WaitShort,
		RunOnStart:  true,
	})

	client := agenttest.NewClient(t, logger.Named("agenttest"), manifest.AgentID, manifest, make(chan *agentproto.Stats, 50), coordinator)
	t.Cleanup(client.Close)

	var agentClient agent.Client = client
	if wrap != nil {
		agentClient = wrap(agentClient)
	}
	agnt := agent.New(agent.Options{
		Client:             agentClient,
		Logger:             logger.Named("agent"),
		SubagentExecDriver: driver,
	})
	t.Cleanup(func() { _ = agnt.Close() })
	return client, agnt
}

func awaitManifestHandled(t *testing.T, client *agenttest.Client) {
	t.Helper()
	require.Eventually(t, func() bool {
		return slices.Contains(client.GetLifecycleStates(), codersdk.WorkspaceAgentLifecycleReady)
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestAgent_SubagentExecution_TopLevelManifestLaunches(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	decl := testSubagentExecution()
	childAgentID := uuid.New()
	driver := newFakeExecDriver()
	client, _ := startExecAgent(t, agentsdk.Manifest{
		SubagentExecutions: []agentsdk.SubagentExecution{decl},
	}, driver, nil)
	client.SetAcquireSubagentExecutionFunc(func(_ context.Context, _ *agentproto.AcquireSubagentExecutionRequest) (*agentproto.AcquireSubagentExecutionResponse, error) {
		return &agentproto.AcquireSubagentExecutionResponse{
			ChildAgentId:       childAgentID[:],
			AuthToken:          "child-auth-token",
			AcquisitionVersion: 3,
		}, nil
	})
	client.SetReportSubagentExecutionStatusFunc(func(_ context.Context, _ *agentproto.ReportSubagentExecutionStatusRequest) (*agentproto.ReportSubagentExecutionStatusResponse, error) {
		return &agentproto.ReportSubagentExecutionStatusResponse{}, nil
	})

	launch := testutil.RequireReceive(ctx, t, driver.startCh)
	require.Equal(t, decl.ExecutionID, launch.Declaration.ExecutionID)
	require.Equal(t, decl.Generation, launch.Declaration.Generation)
	require.Equal(t, childAgentID, launch.ChildAgentID)
	require.EqualValues(t, 3, launch.AcquisitionVersion)

	acquisitions := client.SubagentExecutionAcquisitions()
	require.Len(t, acquisitions, 1)
	require.Equal(t, decl.ExecutionID[:], acquisitions[0].GetExecutionId())
	require.Equal(t, decl.Generation[:], acquisitions[0].GetGeneration())

	require.Eventually(t, func() bool {
		reports := client.SubagentExecutionReports()
		return len(reports) == 1 &&
			reports[0].GetStatus() == agentproto.ReportSubagentExecutionStatusRequest_RUNNING &&
			reports[0].GetAcquisitionVersion() == 3
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestAgent_SubagentExecution_ChildManifestNeverAcquires(t *testing.T) {
	t.Parallel()

	// The declaration is present but the agent is a child, so it must not
	// even attempt an acquisition. The top-level case above is the control
	// that proves an identical manifest does launch.
	driver := newFakeExecDriver()
	client, _ := startExecAgent(t, agentsdk.Manifest{
		ParentID:           uuid.New(),
		SubagentExecutions: []agentsdk.SubagentExecution{testSubagentExecution()},
	}, driver, nil)

	awaitManifestHandled(t, client)

	require.Empty(t, client.SubagentExecutionAcquisitions())
	require.Empty(t, client.SubagentExecutionReports())
	require.Zero(t, driver.startCount())
}

func TestAgent_SubagentExecution_NoDeclarationsIsUnchanged(t *testing.T) {
	t.Parallel()

	driver := newFakeExecDriver()
	client, agnt := startExecAgent(t, agentsdk.Manifest{}, driver, nil)

	awaitManifestHandled(t, client)

	require.Empty(t, client.SubagentExecutionAcquisitions())
	require.Zero(t, driver.startCount())
	require.NoError(t, agnt.Close())
}

// versionCountingClient answers ConnectRPC211WithRole with a fixed error
// and counts how often each version is dialed.
type versionCountingClient struct {
	agent.Client

	err      error
	calls211 atomic.Int64
	calls210 atomic.Int64
}

func (c *versionCountingClient) ConnectRPC211WithRole(ctx context.Context, role string) (
	agentproto.DRPCAgentClient211, tailnetproto.DRPCTailnetClient28, error,
) {
	c.calls211.Add(1)
	if c.err != nil {
		return nil, nil, c.err
	}
	return c.Client.ConnectRPC211WithRole(ctx, role)
}

func (c *versionCountingClient) ConnectRPC210WithRole(ctx context.Context, role string) (
	agentproto.DRPCAgentClient210, tailnetproto.DRPCTailnetClient28, error,
) {
	c.calls210.Add(1)
	return c.Client.ConnectRPC210WithRole(ctx, role)
}

// apiVersionError synthesizes the answer coderd gives for an API version
// it does not implement. codersdk.Error's status code is unexported, so it
// has to come through ReadBodyAsError like a real response would.
func apiVersionError(t *testing.T, status int, message string) error {
	t.Helper()
	body, err := json.Marshal(codersdk.Response{Message: message})
	require.NoError(t, err)
	res := &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    httptest.NewRequest(http.MethodGet, "/api/v2/workspaceagents/me/rpc", nil),
	}
	return codersdk.ReadBodyAsError(res)
}

func TestAgent_ConnectRPC_FallsBackToV210(t *testing.T) {
	t.Parallel()

	var counting *versionCountingClient
	driver := newFakeExecDriver()
	client, _ := startExecAgent(t, agentsdk.Manifest{
		SubagentExecutions: []agentsdk.SubagentExecution{testSubagentExecution()},
	}, driver, func(inner agent.Client) agent.Client {
		counting = &versionCountingClient{
			Client: inner,
			err:    apiVersionError(t, http.StatusBadRequest, workspacesdk.AgentAPIMismatchMessage),
		}
		return counting
	})

	awaitManifestHandled(t, client)

	require.Positive(t, counting.calls211.Load())
	require.Positive(t, counting.calls210.Load())
	// With no v2.11 controller there is nothing to acquire through, so a
	// declaration must not launch.
	require.Empty(t, client.SubagentExecutionAcquisitions())
	require.Zero(t, driver.startCount())
}

func TestAgent_ConnectRPC_NoFallbackOnAuthError(t *testing.T) {
	t.Parallel()

	var counting *versionCountingClient
	_, _ = startExecAgent(t, agentsdk.Manifest{}, newFakeExecDriver(), func(inner agent.Client) agent.Client {
		counting = &versionCountingClient{
			Client: inner,
			err:    apiVersionError(t, http.StatusUnauthorized, "Invalid audience"),
		}
		return counting
	})

	// The agent must keep redialing v2.11 rather than quietly dropping to
	// a version the deployment never rejected.
	require.Eventually(t, func() bool {
		return counting.calls211.Load() >= 2
	}, testutil.WaitLong, testutil.IntervalFast)
	require.Zero(t, counting.calls210.Load())
}
