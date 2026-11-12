package agentworkload_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentworkload"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// unixClient returns an HTTP client that talks to a unix socket.
func unixClient(socket string) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
}

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
	rt := &agentworkload.PodRuntime{SandboxID: "pod-1", StateDir: t.TempDir(), Logger: testutil.Logger(t)}
	require.Equal(t, "pod", rt.Name())
	channel := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = rw.Write([]byte("channel saw " + r.URL.Path))
	})

	// A workload that has a pod to itself is given no part of anything else.
	_, err := rt.Start(ctx, agentworkload.Spec{
		ID: uuid.New(), Name: "mounts", Command: "true", Mounts: []codersdk.WorkloadMount{{Path: "/etc"}},
	}, channel)
	require.ErrorContains(t, err, "cannot be shared")

	id := uuid.New()
	sandbox, err := rt.Start(ctx, agentworkload.Spec{
		ID: id, Name: "alpha",
		Command: `echo "socket=$CODER_WORKLOAD_SOCKET name=$CODER_WORKLOAD_NAME token=[$CODER_AGENT_TOKEN] url=[$CODER_AGENT_URL]"; echo "$CODER_URL" > "$HOME/coder-url"; echo to-stderr >&2; sleep 0.3; exit 3`,
	}, channel)
	require.NoError(t, err)
	require.Equal(t, "pod-1", sandbox.ID())

	// The channel is served for as long as the workload's sandbox exists.
	socket := filepath.Join(rt.StateDir, id.String(), "workload.sock")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://coder.workload/hello", nil)
	require.NoError(t, err)
	res, err := unixClient(socket).Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	require.NoError(t, err)
	require.Equal(t, "channel saw /hello", string(body))
	// Programs that cannot use a unix socket get the same channel on a
	// loopback address.
	var loopback []byte
	require.Eventually(t, func() bool {
		loopback, err = os.ReadFile(filepath.Join(home, "coder-url"))
		return err == nil && len(loopback) > 0
	}, testutil.WaitShort, testutil.IntervalFast)
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(string(loopback))+"/again", nil)
	require.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err = io.ReadAll(res.Body)
	_ = res.Body.Close()
	require.NoError(t, err)
	require.Equal(t, "channel saw /again", string(body))

	// Output is followed to the end, and can be read again afterwards.
	var followed bytes.Buffer
	require.NoError(t, sandbox.Logs(ctx, &followed))
	code, err := sandbox.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, code)
	require.Equal(t, "socket="+socket+" name=alpha token=[] url=[]\nto-stderr\n", followed.String())
	var again bytes.Buffer
	require.NoError(t, sandbox.Logs(ctx, &again))
	require.Equal(t, followed.String(), again.String())

	require.NoError(t, sandbox.Close(ctx))
	require.NoFileExists(t, socket)

	// Removing a workload stops its command.
	sandbox, err = rt.Start(ctx, agentworkload.Spec{ID: uuid.New(), Name: "sleeper", Command: "sleep 600"}, channel)
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
	sandbox, err := rt.Start(ctx, agentworkload.Spec{ID: uuid.New(), Name: "shell", Command: "sh", Interactive: true},
		http.NotFoundHandler())
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
