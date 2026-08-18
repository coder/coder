package confine

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestValidateSandboxMicroVMPlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		goos   string
		goarch string
		want   string
	}{
		{name: "supported", goos: "linux", goarch: "amd64"},
		{name: "non-Linux", goos: "darwin", goarch: "amd64", want: "supported only on linux/amd64"},
		{name: "non-amd64", goos: "linux", goarch: "arm64", want: "supported only on linux/amd64"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateSandboxMicroVMPlatform(test.goos, test.goarch)
			if test.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestSandboxControllerMicroVM(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("embedded microVM controller requires linux/amd64, got %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	tempDir := t.TempDir()
	policyFile := filepath.Join(tempDir, "policy.yaml")
	destroyMarker := filepath.Join(tempDir, "destroyed")
	reloadMarker := filepath.Join(tempDir, "reloaded")
	client := newSandboxControllerTestClient()
	controller := newMicroVMTestController(t, client, SandboxDeclaration{
		Mode:               SandboxModeMicroVM,
		DestroyScript:      "touch " + destroyMarker,
		PolicyFile:         policyFile,
		PolicyReloadScript: "touch " + reloadMarker,
		Name:               "microvm",
		EgressEnforcement:  codersdk.AISandboxEgressEnforcementForced,
		MicroVMImage:       "ubuntu:24.04",
		MicroVMMemoryMiB:   1536,
		MicroVMCPUs:        2,
	})

	started := make(chan MicroVMOptions, 1)
	running := &testRunningMicroVM{closed: make(chan struct{})}
	controller.startMicroVM = func(_ context.Context, options MicroVMOptions) (runningMicroVMSandbox, error) {
		started <- options
		return running, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- controller.Run(ctx)
	}()

	var options MicroVMOptions
	select {
	case options = <-started:
	case <-testutil.Context(t, testutil.WaitLong).Done():
		t.Fatal("timed out waiting for embedded microVM start")
	}
	require.Equal(t, "ubuntu:24.04", options.Image)
	require.Equal(t, "microvm", options.Name)
	require.Equal(t, 2, options.CPUs)
	require.Equal(t, 1536, options.MemoryMiB)
	require.Equal(t, "https://coder.example.com", options.AgentURL)
	require.Equal(t, client.response.AgentToken, options.AgentToken)
	require.Equal(t, client.response.SessionToken, options.SessionToken)
	require.Equal(t, "coder.example.com", options.Destination.AllowPrivateHost)
	require.NotNil(t, options.Policy)
	require.NotNil(t, options.Event)

	executable, err := os.Executable()
	require.NoError(t, err)
	executable, err = filepath.Abs(executable)
	require.NoError(t, err)
	executable, err = filepath.EvalSymlinks(executable)
	require.NoError(t, err)
	require.Equal(t, executable, options.CoderBinaryPath)
	configDir, err := os.UserConfigDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(configDir, "coder-ai", "microvm", "cache"), options.CacheDir)
	require.Equal(t, filepath.Join(configDir, "coder-ai", "microvm", "state"), options.StateDir)

	readyCtx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, controller.WaitForProxy(readyCtx))
	extraEnv := strings.Join(controller.ScriptExtraEnv(), "\n")
	require.Contains(t, extraEnv, EnvSandboxID+"="+client.response.ID.String())
	require.NotContains(t, extraEnv, EnvEgressProxy+"=")
	require.NoFileExists(t, policyFile)
	require.NoFileExists(t, destroyMarker)
	require.NoFileExists(t, reloadMarker)

	client.policyUpdates <- codersdk.AIEgressPolicy{Revision: 12}
	require.True(t, testutil.Eventually(readyCtx, t, func(context.Context) bool {
		return options.Policy.Decide("denied.example", 443).Revision == 12
	}, testutil.IntervalFast))
	options.Event(NetworkEvent{
		Protocol:       agentsdk.AISandboxNetworkProtocolConnect,
		Host:           "denied.example",
		Port:           443,
		Action:         agentsdk.AISandboxNetworkEventActionDenied,
		PolicyRevision: 12,
	})

	cancel()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-readyCtx.Done():
		t.Fatal("timed out waiting for controller shutdown")
	}
	select {
	case <-running.closed:
	default:
		t.Fatal("embedded microVM was not closed")
	}
	require.NoFileExists(t, policyFile)
	require.NoFileExists(t, destroyMarker)
	require.NoFileExists(t, reloadMarker)

	client.mu.Lock()
	defer client.mu.Unlock()
	require.Len(t, client.sessions, 2)
	require.NotNil(t, client.sessions[1].EndedAt)
	require.Len(t, client.eventBatches, 1)
	require.Equal(t, "denied.example", client.eventBatches[0].Events[0].Host)
}

func TestSandboxControllerMicroVMBootFailureDegrades(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("embedded microVM controller requires linux/amd64, got %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	client := newSandboxControllerTestClient()
	controller := newMicroVMTestController(t, client, SandboxDeclaration{
		Mode:              SandboxModeMicroVM,
		Name:              "microvm",
		EgressEnforcement: codersdk.AISandboxEgressEnforcementNone,
		MicroVMImage:      "ubuntu:24.04",
		MicroVMMemoryMiB:  1024,
		MicroVMCPUs:       1,
	})
	controller.startMicroVM = func(context.Context, MicroVMOptions) (runningMicroVMSandbox, error) {
		return nil, xerrors.New("KVM unavailable")
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- controller.Run(ctx)
	}()

	readyCtx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, controller.WaitForProxy(readyCtx))
	require.True(t, testutil.Eventually(readyCtx, t, func(context.Context) bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return len(client.logs) > 0 && strings.Contains(
			client.logs[len(client.logs)-1].Logs[0].Output,
			"microVM boot failed; sandbox remains active (degraded)",
		)
	}, testutil.IntervalFast))

	cancel()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-readyCtx.Done():
		t.Fatal("timed out waiting for controller shutdown")
	}
}

func newMicroVMTestController(
	t *testing.T,
	client *sandboxControllerTestClient,
	declaration SandboxDeclaration,
) *SandboxController {
	t.Helper()
	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	controller, err := NewSandboxController(SandboxControllerOptions{
		Declaration: declaration,
		Client:      client,
		Logger:      slog.Make(),
		LogDir:      t.TempDir(),
		AccessURL:   accessURL,
	})
	require.NoError(t, err)
	return controller
}

type testRunningMicroVM struct {
	closed chan struct{}
}

func (sandbox *testRunningMicroVM) Close(context.Context) error {
	close(sandbox.closed)
	return nil
}

type sandboxControllerTestClient struct {
	mu sync.Mutex

	response      agentsdk.CreateAISandboxResponse
	policyUpdates chan codersdk.AIEgressPolicy
	sessions      []agentsdk.PostAISandboxSessionRequest
	eventBatches  []agentsdk.PatchAISandboxNetworkEventsRequest
	logs          []agentsdk.PatchLogs
}

func newSandboxControllerTestClient() *sandboxControllerTestClient {
	return &sandboxControllerTestClient{
		response: agentsdk.CreateAISandboxResponse{
			ID:           uuid.New(),
			ChildAgentID: uuid.New(),
			AIAgentID:    uuid.New(),
			AgentToken:   "child-agent-token",
			SessionToken: "session-token",
		},
		policyUpdates: make(chan codersdk.AIEgressPolicy, 1),
	}
}

func (*sandboxControllerTestClient) AIEgressPolicy(context.Context) (codersdk.AIEgressPolicy, error) {
	return codersdk.AIEgressPolicy{Revision: 11}, nil
}

func (client *sandboxControllerTestClient) WatchAIEgressPolicy(ctx context.Context) (<-chan codersdk.AIEgressPolicy, io.Closer, error) {
	updates := make(chan codersdk.AIEgressPolicy)
	go func() {
		defer close(updates)
		for {
			select {
			case <-ctx.Done():
				return
			case policy := <-client.policyUpdates:
				select {
				case <-ctx.Done():
					return
				case updates <- policy:
				}
			}
		}
	}()
	return updates, nil, nil
}

func (client *sandboxControllerTestClient) PostAISandboxSession(_ context.Context, request agentsdk.PostAISandboxSessionRequest) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.sessions = append(client.sessions, request)
	return nil
}

func (client *sandboxControllerTestClient) PatchAISandboxNetworkEvents(_ context.Context, request agentsdk.PatchAISandboxNetworkEventsRequest) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.eventBatches = append(client.eventBatches, request)
	return nil
}

func (client *sandboxControllerTestClient) PatchLogs(_ context.Context, logs agentsdk.PatchLogs) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.logs = append(client.logs, logs)
	return nil
}

func (client *sandboxControllerTestClient) CreateAISandbox(context.Context, agentsdk.CreateAISandboxRequest) (agentsdk.CreateAISandboxResponse, error) {
	return client.response, nil
}

func (*sandboxControllerTestClient) AISandboxes(context.Context) ([]agentsdk.AISandbox, error) {
	return nil, nil
}

func (*sandboxControllerTestClient) DeleteAISandbox(context.Context, uuid.UUID) error {
	return nil
}
