package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"

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

// execProjectDir returns a canonical directory an agent manifest can point
// at. The shared project path policy resolves both the manifest directory
// and the declared shared path, so a fixture has to be a real directory that
// is already free of symlinked components.
func execProjectDir(t *testing.T) string {
	t.Helper()

	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	project := filepath.Join(base, "project")
	require.NoError(t, os.MkdirAll(project, 0o700))
	return project
}

func testSubagentExecution(sharedHostPath string) agentsdk.SubagentExecution {
	return agentsdk.SubagentExecution{
		ExecutionID:     uuid.New(),
		Generation:      uuid.New(),
		Name:            "sandbox",
		Driver:          "bubblewrap",
		DriverProtocol:  1,
		SharedHostPath:  sharedHostPath,
		SharedChildPath: "/workspace/project",
		StartupTimeout:  time.Minute,
		RestartPolicy:   "never",
	}
}

// startExecAgent brings up an agent against the fake agent API with the
// given manifest, without the full tailnet fixture: manifest handling and
// the subagent execution manager do not need a reachable network.
func startExecAgent(t *testing.T, manifest agentsdk.Manifest, driver subagentexec.Driver, wrap func(agent.Client) agent.Client, mutate ...func(*agent.Options)) (*agenttest.Client, agent.Agent) {
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
	options := agent.Options{
		Client:             agentClient,
		Logger:             logger.Named("agent"),
		SubagentExecDriver: driver,
	}
	for _, mutate := range mutate {
		mutate(&options)
	}
	agnt := agent.New(options)
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

	// The manifest directory is the project root the declared shared path
	// must live under, and the declaration shares the project root itself.
	project := execProjectDir(t)
	decl := testSubagentExecution(project)
	childAgentID := uuid.New()
	driver := newFakeExecDriver()
	client, _ := startExecAgent(t, agentsdk.Manifest{
		Directory:          project,
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
	// The driver receives the canonical shared path the agent validated.
	require.Equal(t, project, launch.SharedHostPath)

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
	project := execProjectDir(t)
	client, _ := startExecAgent(t, agentsdk.Manifest{
		ParentID:           uuid.New(),
		Directory:          project,
		SubagentExecutions: []agentsdk.SubagentExecution{testSubagentExecution(project)},
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

// TestAgent_SubagentExecution_DefaultDriverLaunches covers the wiring an
// ordinary deployment uses: no injected driver, only the agent's driver
// configuration, so the concrete driver launches the declaration.
func TestAgent_SubagentExecution_DefaultDriverLaunches(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the concrete subagent execution driver targets unix platforms")
	}

	stateRoot := filepath.Join(t.TempDir(), "subagent-exec")
	markerDir := t.TempDir()
	const childToken = "child-auth-token"

	// The declared shared path is the project root the manifest names: the
	// launcher resolves both before it writes any private state.
	project := execProjectDir(t)
	decl := testSubagentExecution(project)
	decl.Driver = fmt.Sprintf(`#!/bin/sh
if [ "$1" = cleanup ]; then exit 0; fi
cp "$2" %[1]q/input.json
touch %[1]q/ready
while true; do sleep 0.05; done
`, markerDir)

	client, agnt := startExecAgent(t, agentsdk.Manifest{
		Directory:          project,
		SubagentExecutions: []agentsdk.SubagentExecution{decl},
	}, nil, nil, func(options *agent.Options) {
		options.SubagentExecDriverConfig = subagentexec.ScriptDriverConfig{
			StateRoot:       stateRoot,
			CoderURL:        "https://coder.example.com",
			CoderBinaryPath: "/opt/coder/bin/coder",
		}
	})
	childAgentID := uuid.New()
	client.SetAcquireSubagentExecutionFunc(func(_ context.Context, _ *agentproto.AcquireSubagentExecutionRequest) (*agentproto.AcquireSubagentExecutionResponse, error) {
		return &agentproto.AcquireSubagentExecutionResponse{
			ChildAgentId:       childAgentID[:],
			AuthToken:          childToken,
			AcquisitionVersion: 4,
		}, nil
	})

	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(markerDir, "ready"))
		return err == nil
	}, testutil.WaitLong, testutil.IntervalFast)

	require.Eventually(t, func() bool {
		reports := client.SubagentExecutionReports()
		return len(reports) == 1 &&
			reports[0].GetStatus() == agentproto.ReportSubagentExecutionStatusRequest_RUNNING
	}, testutil.WaitLong, testutil.IntervalFast)

	// The private state uses UUID paths and holds the token in a 0600 file
	// that the protocol document only points at.
	// The driver writes to the canonical state root, which is what the
	// configured one resolves to once it exists.
	canonicalStateRoot, err := filepath.EvalSymlinks(stateRoot)
	require.NoError(t, err)
	executionDir := filepath.Join(canonicalStateRoot, "agent", decl.ExecutionID.String())
	tokenPath := filepath.Join(executionDir, "token")
	tokenContent, err := os.ReadFile(tokenPath)
	require.NoError(t, err)
	require.Equal(t, childToken, string(tokenContent))
	info, err := os.Stat(tokenPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	document, err := os.ReadFile(filepath.Join(markerDir, "input.json"))
	require.NoError(t, err)
	require.NotContains(t, string(document), childToken)
	var input subagentexec.DriverInput
	require.NoError(t, json.Unmarshal(document, &input))
	require.Equal(t, subagentexec.OperationRun, input.Operation)
	require.Equal(t, decl.ExecutionID, input.ExecutionID)
	require.Equal(t, childAgentID, input.ChildAgentID)
	require.Equal(t, tokenPath, input.TokenFilePath)

	// Closing the agent stops the driver and reclaims the token file.
	require.NoError(t, agnt.Close())
	require.NoFileExists(t, tokenPath)
	require.NoDirExists(t, executionDir)

	reports := client.SubagentExecutionReports()
	require.Equal(t, agentproto.ReportSubagentExecutionStatusRequest_STOPPED, reports[len(reports)-1].GetStatus())
	for _, report := range reports {
		require.NotContains(t, report.GetError(), childToken)
	}
}

// TestAgent_SubagentExecution_UnconfiguredDriverFails pins the behavior of a
// deployment that declares an execution without configuring a driver: the
// declaration fails visibly rather than silently doing nothing.
func TestAgent_SubagentExecution_UnconfiguredDriverFails(t *testing.T) {
	t.Parallel()

	project := execProjectDir(t)
	client, _ := startExecAgent(t, agentsdk.Manifest{
		Directory:          project,
		SubagentExecutions: []agentsdk.SubagentExecution{testSubagentExecution(project)},
	}, nil, nil, func(options *agent.Options) {
		// A launch that cannot proceed is logged as an error by design.
		options.Logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
			Leveled(slog.LevelDebug).Named("agent")
	})
	childAgentID := uuid.New()
	client.SetAcquireSubagentExecutionFunc(func(_ context.Context, _ *agentproto.AcquireSubagentExecutionRequest) (*agentproto.AcquireSubagentExecutionResponse, error) {
		return &agentproto.AcquireSubagentExecutionResponse{
			ChildAgentId:       childAgentID[:],
			AuthToken:          "child-auth-token",
			AcquisitionVersion: 2,
		}, nil
	})

	// A declaration that cannot launch is retried on every manifest
	// refresh, so the assertion is on the reported failure, not a count.
	require.Eventually(t, func() bool {
		return slices.ContainsFunc(client.SubagentExecutionReports(), func(report *agentproto.ReportSubagentExecutionStatusRequest) bool {
			return report.GetStatus() == agentproto.ReportSubagentExecutionStatusRequest_FAILED &&
				strings.Contains(report.GetError(), "driver is not configured")
		})
	}, testutil.WaitLong, testutil.IntervalFast)
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
	project := execProjectDir(t)
	client, _ := startExecAgent(t, agentsdk.Manifest{
		Directory:          project,
		SubagentExecutions: []agentsdk.SubagentExecution{testSubagentExecution(project)},
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
