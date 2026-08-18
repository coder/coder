package cli

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/confine"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

type agentSandboxPolicyClient struct {
	policy   codersdk.AIEgressPolicy
	fetchErr error
}

func (c *agentSandboxPolicyClient) AIEgressPolicy(context.Context) (codersdk.AIEgressPolicy, error) {
	return c.policy, c.fetchErr
}

func (*agentSandboxPolicyClient) WatchAIEgressPolicy(ctx context.Context) (<-chan codersdk.AIEgressPolicy, io.Closer, error) {
	updates := make(chan codersdk.AIEgressPolicy)
	go func() {
		<-ctx.Done()
		close(updates)
	}()
	return updates, nil, nil
}

type agentSandboxTestInstance struct {
	closed bool
}

func (*agentSandboxTestInstance) AgentLog(context.Context) (string, error) {
	return "", nil
}

func (s *agentSandboxTestInstance) Close(context.Context) error {
	s.closed = true
	return nil
}

func TestAgentSandboxOptions(t *testing.T) {
	t.Parallel()

	t.Run("Flags", func(t *testing.T) {
		t.Parallel()

		var (
			gotOptions     confine.MicroVMOptions
			gotPolicyToken string
		)
		instance := &agentSandboxTestInstance{}
		deps := agentSandboxTestDeps()
		deps.newPolicyClient = func(_ *url.URL, token string, _ slog.Logger) confine.PolicyClient {
			gotPolicyToken = token
			return &agentSandboxPolicyClient{}
		}
		deps.startMicroVM = func(_ context.Context, options confine.MicroVMOptions) (agentSandboxInstance, error) {
			gotOptions = options
			return instance, nil
		}
		inv := canceledAgentSandboxInvocation(t, agentSandboxWithDeps(&AgentAuth{}, deps), []string{
			"--agent-token", "guest-token",
			"--policy-token", "host-token",
			"--agent-url", "https://coder.example.com",
			"--image", "alpine:latest",
			"--name", "flag-sandbox",
			"--cpus", "2",
			"--memory-mib", "2048",
			"--cache-dir", "/cache",
			"--state-dir", "/state",
		}, nil)
		require.NoError(t, inv.Run())
		require.Equal(t, "host-token", gotPolicyToken)
		require.Equal(t, "guest-token", gotOptions.AgentToken)
		require.Equal(t, "https://coder.example.com", gotOptions.AgentURL)
		require.Equal(t, "alpine:latest", gotOptions.Image)
		require.Equal(t, "flag-sandbox", gotOptions.Name)
		require.Equal(t, 2, gotOptions.CPUs)
		require.Equal(t, 2048, gotOptions.MemoryMiB)
		require.Equal(t, "/cache", gotOptions.CacheDir)
		require.Equal(t, "/state", gotOptions.StateDir)
		require.True(t, instance.closed)
	})

	t.Run("Environment", func(t *testing.T) {
		t.Parallel()

		var (
			gotOptions     confine.MicroVMOptions
			gotPolicyToken string
		)
		deps := agentSandboxTestDeps()
		deps.newPolicyClient = func(_ *url.URL, token string, _ slog.Logger) confine.PolicyClient {
			gotPolicyToken = token
			return &agentSandboxPolicyClient{}
		}
		deps.startMicroVM = func(_ context.Context, options confine.MicroVMOptions) (agentSandboxInstance, error) {
			gotOptions = options
			return &agentSandboxTestInstance{}, nil
		}
		inv := canceledAgentSandboxInvocation(t, agentSandboxWithDeps(&AgentAuth{}, deps), nil, []string{
			envSandboxAgentToken + "=env-guest-token",
			envSandboxPolicyToken + "=env-host-token",
			envAgentURL + "=https://coder.example.com",
			envSandboxImage + "=ubuntu:24.10",
			envSandboxName + "=env-sandbox",
			envSandboxCPUs + "=3",
			envSandboxMemoryMiB + "=3072",
			envSandboxCacheDir + "=/env-cache",
			envSandboxStateDir + "=/env-state",
		})
		require.NoError(t, inv.Run())
		require.Equal(t, "env-host-token", gotPolicyToken)
		require.Equal(t, "env-guest-token", gotOptions.AgentToken)
		require.Equal(t, "https://coder.example.com", gotOptions.AgentURL)
		require.Equal(t, "ubuntu:24.10", gotOptions.Image)
		require.Equal(t, "env-sandbox", gotOptions.Name)
		require.Equal(t, 3, gotOptions.CPUs)
		require.Equal(t, 3072, gotOptions.MemoryMiB)
		require.Equal(t, "/env-cache", gotOptions.CacheDir)
		require.Equal(t, "/env-state", gotOptions.StateDir)
	})
}

func TestAgentSandboxControlChannelAllowance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		agentURL string
		wantHost string
		wantPort int
	}{
		{name: "HTTPSDefault", agentURL: "https://CODER.Example.COM.", wantHost: "coder.example.com", wantPort: 443},
		{name: "HTTPSExplicit", agentURL: "https://coder.example.com:8443", wantHost: "coder.example.com", wantPort: 8443},
		{name: "HTTPDefault", agentURL: "http://coder.example.com", wantHost: "coder.example.com", wantPort: 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotOptions confine.MicroVMOptions
			deps := agentSandboxTestDeps()
			deps.startMicroVM = func(_ context.Context, options confine.MicroVMOptions) (agentSandboxInstance, error) {
				gotOptions = options
				return &agentSandboxTestInstance{}, nil
			}
			inv := canceledAgentSandboxInvocation(t, agentSandboxWithDeps(&AgentAuth{}, deps), []string{
				"--agent-token", "guest-token",
				"--policy-token", "host-token",
				"--agent-url", tt.agentURL,
			}, nil)
			require.NoError(t, inv.Run())
			require.Equal(t, tt.wantHost, gotOptions.Destination.AlwaysAllowHost)
			require.Equal(t, tt.wantPort, gotOptions.Destination.AlwaysAllowPort)
			require.Equal(t, gotOptions.Destination.AlwaysAllowHost, gotOptions.Destination.AllowPrivateHost)
		})
	}
}

func TestValidateAgentSandboxOptions(t *testing.T) {
	t.Parallel()

	agentURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	tests := []struct {
		name        string
		agentToken  string
		policyToken string
		agentURL    *url.URL
		cpus        int64
		memoryMiB   int64
		wantErr     string
	}{
		{name: "Valid", agentToken: "guest", policyToken: "host", agentURL: agentURL, cpus: 1, memoryMiB: 1024},
		{name: "AgentTokenWithoutURL", agentToken: "guest", policyToken: "host", cpus: 1, memoryMiB: 1024, wantErr: "required together"},
		{name: "URLWithoutAgentToken", policyToken: "host", agentURL: agentURL, cpus: 1, memoryMiB: 1024, wantErr: "required together"},
		{name: "MissingPolicyToken", agentToken: "guest", agentURL: agentURL, cpus: 1, memoryMiB: 1024, wantErr: "--policy-token"},
		{name: "InvalidCPUs", agentToken: "guest", policyToken: "host", agentURL: agentURL, memoryMiB: 1024, wantErr: "--cpus"},
		{name: "InvalidMemory", agentToken: "guest", policyToken: "host", agentURL: agentURL, cpus: 1, wantErr: "--memory-mib"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateAgentSandboxOptions(tt.agentToken, tt.policyToken, tt.agentURL, tt.cpus, tt.memoryMiB)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestAgentSandboxPolicyFetchFailureStaysFailClosed(t *testing.T) {
	t.Parallel()

	deps := agentSandboxTestDeps()
	deps.newPolicyClient = func(*url.URL, string, slog.Logger) confine.PolicyClient {
		return &agentSandboxPolicyClient{fetchErr: xerrors.New("fetch failed")}
	}
	deps.startMicroVM = func(_ context.Context, options confine.MicroVMOptions) (agentSandboxInstance, error) {
		require.True(t, options.Policy.Decide("coder.example.com", 443).Allowed)
		require.False(t, options.Policy.Decide("example.com", 443).Allowed)
		return &agentSandboxTestInstance{}, nil
	}
	inv := canceledAgentSandboxInvocation(t, agentSandboxWithDeps(&AgentAuth{}, deps), []string{
		"--agent-token", "guest-token",
		"--policy-token", "host-token",
		"--agent-url", "https://coder.example.com",
	}, nil)
	require.NoError(t, inv.Run())
}

func TestAgentSandboxUnsupportedPlatform(t *testing.T) {
	t.Parallel()

	deps := agentSandboxTestDeps()
	deps.goos = "darwin"
	deps.goarch = "arm64"
	inv := agentSandboxWithDeps(&AgentAuth{}, deps).Invoke()
	err := inv.Run()
	require.ErrorContains(t, err, "supported only on linux/amd64, got darwin/arm64")
}

func TestAgentSandboxKVM(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("agent sandbox smoke test requires linux/amd64 KVM, got %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	kvm, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no usable /dev/kvm, cannot boot a microVM: %v", err)
	}
	require.NoError(t, kvm.Close())

	binaryPath := filepath.Join(t.TempDir(), "coder")
	const fakeAgent = `#!/bin/sh
[ "$1" = agent ] || exit 2
[ -n "$CODER_AGENT_URL" ] || exit 3
[ -n "$CODER_AGENT_TOKEN" ] || exit 4
while :; do sleep 60; done
`
	require.NoError(t, os.WriteFile(binaryPath, []byte(fakeAgent), 0o600))
	require.NoError(t, os.Chmod(binaryPath, 0o700))

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitSuperLong))
	defer cancel()
	deps := agentSandboxTestDeps()
	deps.executablePath = func() (string, error) { return binaryPath, nil }
	deps.newPolicyClient = func(*url.URL, string, slog.Logger) confine.PolicyClient {
		return &agentSandboxPolicyClient{}
	}
	deps.startMicroVM = func(ctx context.Context, options confine.MicroVMOptions) (agentSandboxInstance, error) {
		sandbox, err := confine.StartEmbeddedMicroVM(ctx, options)
		if err == nil {
			cancel()
		}
		return sandbox, err
	}
	image := os.Getenv("CODER_EMBEDDED_MICROVM_TEST_IMAGE")
	if image == "" {
		image = "alpine:latest"
	}
	cmd := agentSandboxWithDeps(&AgentAuth{}, deps)
	inv := cmd.Invoke(
		"--agent-token", "guest-token",
		"--policy-token", "host-token",
		"--agent-url", "https://coder.example.com",
		"--image", image,
		"--name", "cli-microvm-smoke",
		"--memory-mib", "512",
		"--cache-dir", filepath.Join(os.TempDir(), "coder-cli-microvm-cache"),
		"--state-dir", t.TempDir(),
	).WithContext(ctx)
	require.NoError(t, inv.Run())
}

func agentSandboxTestDeps() agentSandboxDeps {
	return agentSandboxDeps{
		goos:           "linux",
		goarch:         "amd64",
		executablePath: func() (string, error) { return "/tmp/coder", nil },
		userConfigDir:  func() (string, error) { return "/config", nil },
		newPolicyClient: func(*url.URL, string, slog.Logger) confine.PolicyClient {
			return &agentSandboxPolicyClient{}
		},
		startMicroVM: func(context.Context, confine.MicroVMOptions) (agentSandboxInstance, error) {
			return &agentSandboxTestInstance{}, nil
		},
		shutdownTimeout: time.Second,
	}
}

func canceledAgentSandboxInvocation(t *testing.T, cmd *serpent.Command, args, env []string) *serpent.Invocation {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	inv := cmd.Invoke(args...).WithContext(ctx)
	inv.Environ = serpent.ParseEnviron(env, "")
	return inv
}
