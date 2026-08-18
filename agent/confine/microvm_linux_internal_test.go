//go:build linux

package confine

import (
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	sandboxpolicy "github.com/coder/coder/coder-sandbox/policy"
	sandboxproxy "github.com/coder/coder/coder-sandbox/proxy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func writeExecutable(t *testing.T, path string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, content, 0o600))
	require.NoError(t, os.Chmod(path, 0o700))
}

func TestEmbeddedMicroVMOptionValidation(t *testing.T) {
	t.Parallel()

	binaryPath := filepath.Join(t.TempDir(), "coder")
	writeExecutable(t, binaryPath, []byte("binary"))
	base := MicroVMOptions{
		Image:           "alpine:latest",
		Name:            "embedded-test",
		CacheDir:        t.TempDir(),
		StateDir:        t.TempDir(),
		MemoryMiB:       512,
		CoderBinaryPath: binaryPath,
		AgentURL:        "https://coder.example.com",
		AgentToken:      "agent-token",
		Policy:          NewPolicyEngine("coder.example.com", 443),
	}
	tests := []struct {
		name   string
		mutate func(*MicroVMOptions)
		want   string
	}{
		{name: "name", mutate: func(options *MicroVMOptions) { options.Name = "../bad" }, want: "invalid embedded microVM name"},
		{name: "image", mutate: func(options *MicroVMOptions) { options.Image = "" }, want: "image is required"},
		{name: "cache", mutate: func(options *MicroVMOptions) { options.CacheDir = "" }, want: "cache directory is required"},
		{name: "state", mutate: func(options *MicroVMOptions) { options.StateDir = "" }, want: "state directory is required"},
		{name: "CPUs", mutate: func(options *MicroVMOptions) { options.CPUs = -1 }, want: "CPU count cannot be negative"},
		{name: "memory", mutate: func(options *MicroVMOptions) { options.MemoryMiB = 0 }, want: "memory must be positive"},
		{name: "policy", mutate: func(options *MicroVMOptions) { options.Policy = nil }, want: "policy engine is required"},
		{name: "agent token", mutate: func(options *MicroVMOptions) { options.AgentToken = "" }, want: "agent token is required"},
		{name: "binary path", mutate: func(options *MicroVMOptions) { options.CoderBinaryPath = "" }, want: "binary path is required"},
		{name: "missing binary", mutate: func(options *MicroVMOptions) { options.CoderBinaryPath = filepath.Join(t.TempDir(), "missing") }, want: "resolve Coder binary symlinks"},
		{name: "agent URL", mutate: func(options *MicroVMOptions) { options.AgentURL = "://bad" }, want: "invalid embedded microVM agent URL"},
		{name: "agent URL scheme", mutate: func(options *MicroVMOptions) { options.AgentURL = "ftp://coder.example.com" }, want: "agent URL scheme"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			options := base
			test.mutate(&options)
			_, _, err := validateMicroVMOptions(options)
			require.ErrorContains(t, err, test.want)
		})
	}
	_, _, err := validateMicroVMOptions(base)
	require.NoError(t, err)
}

func TestEmbeddedMicroVMConfigWiresEvaluatorRecorderAndAgent(t *testing.T) {
	t.Parallel()

	binaryPath := filepath.Join(t.TempDir(), "coder-real")
	writeExecutable(t, binaryPath, []byte("binary"))
	binaryLink := filepath.Join(t.TempDir(), "coder-link")
	require.NoError(t, os.Symlink(binaryPath, binaryLink))

	engine := NewPolicyEngine("", 0)
	engine.Update(codersdk.AIEgressPolicy{Revision: 11})
	var events []NetworkEvent
	config, err := newEmbeddedMicroVMConfig(MicroVMOptions{
		Image:           "alpine:latest",
		Name:            "embedded-test",
		CacheDir:        t.TempDir(),
		StateDir:        t.TempDir(),
		CPUs:            2,
		MemoryMiB:       768,
		CoderBinaryPath: binaryLink,
		AgentURL:        "https://coder.example.com",
		AgentToken:      "agent'token",
		SessionToken:    "session-token",
		Policy:          engine,
		Event: func(event NetworkEvent) {
			events = append(events, event)
		},
	})
	require.NoError(t, err)
	require.Equal(t, "embedded-test", config.hostOptions.Name)
	require.Equal(t, 2, config.hostOptions.CPUs)
	require.Equal(t, 768, config.hostOptions.MemoryMiB)
	require.False(t, config.hostOptions.DNS)
	require.NotNil(t, config.hostOptions.Proxy)
	require.NotNil(t, config.hostOptions.Subject)
	require.Same(t, config.evaluator, config.hostOptions.Subject.Policy)
	require.Equal(t, "embedded-test", config.hostOptions.Subject.ID)
	require.Len(t, config.hostOptions.Mounts, 1)
	require.Equal(t, filepath.Dir(binaryPath), config.hostOptions.Mounts[0].Source)
	require.Equal(t, embeddedCoderGuestDir, config.hostOptions.Mounts[0].Target)
	require.True(t, config.hostOptions.Mounts[0].ReadOnly)
	require.True(t, config.hostOptions.Mounts[0].Nosuid)
	require.True(t, config.hostOptions.Mounts[0].Nodev)
	require.False(t, config.hostOptions.Mounts[0].Noexec)
	require.Contains(t, config.agentCommand, "setsid sh -c")
	require.Contains(t, config.agentCommand, "CODER_AGENT_URL=")
	require.Contains(t, config.agentCommand, "CODER_AGENT_TOKEN=")
	require.Contains(t, config.agentCommand, "CODER_SESSION_TOKEN=")
	require.Contains(t, config.agentCommand, "exec '\\''/opt/coder-bin/coder-real'\\'' agent")
	//nolint:gosec // The generated command is the value under test.
	require.NoError(t, exec.Command("sh", "-n", "-c", config.agentCommand).Run())

	decision, err := config.evaluator.EvaluateResolvedIP(
		t.Context(), "CODER.Example.COM.", 443, netip.MustParseAddr("127.0.0.1"),
	)
	require.NoError(t, err)
	require.Equal(t, sandboxpolicy.ActionAllow, decision.Action)
	decision, err = config.evaluator.EvaluateResolvedIP(
		t.Context(), "coder.example.com", 8443, netip.MustParseAddr("203.0.113.10"),
	)
	require.NoError(t, err)
	require.Equal(t, sandboxpolicy.ActionDeny, decision.Action)

	engine.Update(codersdk.AIEgressPolicy{Revision: 12})
	require.EqualValues(t, 12, config.evaluator.Generation())

	config.recorder.Record(sandboxproxy.Event{
		Kind:       sandboxproxy.KindConnect,
		Action:     sandboxpolicy.ActionDeny,
		Host:       "denied.example",
		Port:       "443",
		Generation: 12,
	})
	require.Equal(t, []NetworkEvent{{
		Protocol: agentsdk.AISandboxNetworkProtocolConnect, Host: "denied.example", Port: 443,
		Action: agentsdk.AISandboxNetworkEventActionDenied, PolicyRevision: 12,
	}}, events)
}

func TestEmbeddedAgentCommandWithoutSessionToken(t *testing.T) {
	t.Parallel()

	command := embeddedAgentCommand(embeddedCoderGuestDir+"/coder", "https://coder.example.com", "agent-token", "")
	require.NotContains(t, command, "CODER_SESSION_TOKEN=")
	//nolint:gosec // The generated command is the value under test.
	require.NoError(t, exec.Command("sh", "-n", "-c", command).Run())
}

func TestEmbeddedAgentCommandShellQuoting(t *testing.T) {
	t.Parallel()

	const value = "a'b c"
	quoted := shellQuote(value)
	require.Equal(t, `'a'\''b c'`, quoted)
	//nolint:gosec // quoted is the shell-escaping result under test.
	output, err := exec.Command("sh", "-c", "printf %s "+quoted).Output()
	require.NoError(t, err)
	require.Equal(t, value, string(output))

	command := embeddedAgentCommand(
		embeddedCoderGuestDir+"/coder name", "https://coder.example.com/a path", "agent'token", "session'token",
	)
	//nolint:gosec // The generated command is the value under test.
	require.NoError(t, exec.Command("sh", "-n", "-c", command).Run())
}

func TestStartEmbeddedMicroVMSmoke(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.Skipf("embedded microVM smoke test requires Linux KVM, got %s/%s", runtime.GOOS, runtime.GOARCH)
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
	writeExecutable(t, binaryPath, []byte(fakeAgent))
	image := os.Getenv("CODER_EMBEDDED_MICROVM_TEST_IMAGE")
	if image == "" {
		image = "alpine:latest"
	}
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	sandbox, err := StartEmbeddedMicroVM(ctx, MicroVMOptions{
		Image:           image,
		Name:            "confine-microvm-smoke",
		CacheDir:        filepath.Join(os.TempDir(), "coder-confine-microvm-cache"),
		StateDir:        t.TempDir(),
		MemoryMiB:       512,
		CoderBinaryPath: binaryPath,
		AgentURL:        "https://coder.example.com",
		AgentToken:      "smoke-token",
		Policy:          NewPolicyEngine("coder.example.com", 443),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		closeCtx := testutil.Context(t, testutil.WaitLong)
		require.NoError(t, sandbox.Close(closeCtx))
	})
	status, err := sandbox.Exec(ctx, "test -x /opt/coder")
	require.NoError(t, err)
	require.Zero(t, status)
}

func TestEmbeddedSandboxNil(t *testing.T) {
	t.Parallel()

	var sandbox *EmbeddedSandbox
	status, err := sandbox.Exec(t.Context(), "true")
	require.Error(t, err)
	require.Zero(t, status)
	require.NoError(t, sandbox.Close(t.Context()))
}
