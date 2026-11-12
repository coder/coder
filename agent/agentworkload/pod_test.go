package agentworkload_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentworkload"
	"github.com/coder/coder/v2/testutil"
)

//nolint:paralleltest // Sets environment variables.
func TestPodRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the pod runtime runs shell commands")
	}
	// What configures the daemon is not for the workload to see.
	t.Setenv("CODER_AGENT_TOKEN", "the-daemons-secret")
	t.Setenv("CODER_AGENT_URL", "http://coder.example.com")
	home := t.TempDir()
	t.Setenv("HOME", home)

	ctx := testutil.Context(t, testutil.WaitMedium)
	rt := &agentworkload.PodRuntime{
		CoderURL: "http://coder.example.com", SandboxID: "pod-1", StateDir: t.TempDir(), Logger: testutil.Logger(t),
	}
	require.Equal(t, "pod", rt.Name())

	// The command sees its own credential and where Coder is, and nothing
	// that configures the daemon.
	id := uuid.New()
	sandbox, err := rt.Start(ctx, agentworkload.Spec{
		ID: id, Name: "alpha", SessionToken: "the-workloads-own",
		Command: `echo "name=$CODER_WORKLOAD_NAME url=$CODER_URL token=$CODER_SESSION_TOKEN daemon=[$CODER_AGENT_TOKEN] [$CODER_AGENT_URL]"; echo to-stderr >&2; sleep 0.3; exit 3`,
	})
	require.NoError(t, err)
	require.Equal(t, "pod-1", sandbox.ID())

	// Output is followed to the end, and can be read again afterwards.
	var followed bytes.Buffer
	require.NoError(t, sandbox.Logs(ctx, &followed))
	code, err := sandbox.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, code)
	require.Equal(t, "name=alpha url=http://coder.example.com token=the-workloads-own daemon=[] []\nto-stderr\n", followed.String())
	var again bytes.Buffer
	require.NoError(t, sandbox.Logs(ctx, &again))
	require.Equal(t, followed.String(), again.String())

	require.NoError(t, sandbox.Close(ctx))
	require.NoDirExists(t, filepath.Join(rt.StateDir, id.String()))

	// Removing a workload stops its command.
	sandbox, err = rt.Start(ctx, agentworkload.Spec{ID: uuid.New(), Name: "sleeper", Command: "sleep 600"})
	require.NoError(t, err)
	require.NoError(t, sandbox.Close(ctx))
	_, err = sandbox.Wait(ctx)
	require.Error(t, err)
}

func TestPodRuntimeTerminal(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the pod runtime runs shell commands")
	}
	ctx := testutil.Context(t, testutil.WaitMedium)
	rt := &agentworkload.PodRuntime{SandboxID: "pod-1", StateDir: t.TempDir(), Logger: testutil.Logger(t)}
	sandbox, err := rt.Start(ctx, agentworkload.Spec{ID: uuid.New(), Name: "shell", Command: "sh", Interactive: true})
	require.NoError(t, err)

	// The command runs in the terminal of whoever attaches.
	client, server := net.Pipe()
	attached := make(chan error, 1)
	go func() { attached <- sandbox.Attach(ctx, server, 24, 80) }()
	var seen strings.Builder
	read := make(chan struct{})
	go func() {
		defer close(read)
		_, _ = io.Copy(&seen, client)
	}()
	encoder := json.NewEncoder(client)
	require.NoError(t, encoder.Encode(map[string]string{"data": "echo typed-$((6*7))\r"}))
	require.NoError(t, encoder.Encode(map[string]string{"data": "exit 4\r"}))

	code, err := sandbox.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, code)
	require.NoError(t, testutil.TryReceive(ctx, t, attached))
	_ = client.Close()
	testutil.TryReceive(ctx, t, read)
	require.Contains(t, seen.String(), "typed-42")

	// What the terminal showed is the workload's output.
	var output bytes.Buffer
	require.NoError(t, sandbox.Logs(ctx, &output))
	require.Contains(t, output.String(), "typed-42")
	// It can be attached to once.
	require.Error(t, sandbox.Attach(ctx, server, 24, 80))
	require.NoError(t, sandbox.Close(ctx))
}
