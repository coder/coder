package agentworkload_test

import (
	"context"
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

// fakeDocker writes a docker stand-in that records its arguments.
func fakeDocker(t *testing.T) (bin, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "docker")
	argsFile = filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + argsFile + "\necho ---- >> " + argsFile + "\n" +
		"case \"$1\" in run) echo container-1;; wait) echo 7;; esac\n"
	//nolint:gosec // The test executes this script.
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o700))
	return bin, argsFile
}

func TestDockerRuntime(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("the Docker runtime is Linux only")
	}

	docker, argsFile := fakeDocker(t)
	stateDir := filepath.Join(t.TempDir(), "state")
	rt := &agentworkload.DockerRuntime{Docker: docker, StateDir: stateDir, CLIPath: "/opt/coder"}
	ctx := testutil.Context(t, testutil.WaitShort)

	id := uuid.New()
	channel := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(rw, "the workload's channel")
	})
	sandbox, err := rt.Start(ctx, agentworkload.Spec{
		ID: id, Name: "alpha", Command: "echo hi; coder identity", Image: "alpine:3.22",
		Network: codersdk.WorkloadNetworkNone,
		Mounts:  []codersdk.WorkloadMount{{Path: "/projects/repo"}, {Path: "/projects/scratch", Writable: true}},
	}, channel)
	require.NoError(t, err)
	require.Equal(t, "container-1", sandbox.ID())

	raw, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	args := strings.Split(strings.TrimSpace(strings.Split(string(raw), "----")[0]), "\n")
	channelDir := filepath.Join(stateDir, id.String())
	// The sandbox gets no privileges, no network, and nothing of the
	// workspace except its own channel and the CLI, both read-only. The
	// CLI starts the command.
	require.Equal(t, []string{
		"run", "--detach",
		"--name", "coder-workload-" + id.String(),
		"--label", "com.coder.workload=" + id.String(),
		"--init",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--user", "1000:1000",
		"--pids-limit", "512",
		"--network", "none",
		"--volume", channelDir + ":/run/coder:ro",
		"--env", codersdk.WorkloadSocketEnv + "=/run/coder/workload.sock",
		"--env", "HOME=/tmp",
		"--env", "CODER_WORKLOAD_NAME=alpha",
		"--volume", "/projects/repo:/projects/repo:ro",
		"--volume", "/projects/scratch:/projects/scratch:rw",
		"--volume", "/opt/coder:/usr/local/bin/coder:ro",
		"alpine:3.22",
		"/usr/local/bin/coder", "exp", "workload-init", "--",
		"sh", "-c", "echo hi; coder identity",
	}, args)

	// The channel is served on the socket in the mounted directory.
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(channelDir, "workload.sock"))
		},
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://coder.workload/", nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	_ = res.Body.Close()
	require.Equal(t, "the workload's channel", string(body))

	code, err := sandbox.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, 7, code)

	// Closing removes the container and the channel.
	require.NoError(t, sandbox.Close(ctx))
	_, err = os.Stat(channelDir)
	require.ErrorIs(t, err, os.ErrNotExist)
	client.CloseIdleConnections()
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://coder.workload/", nil)
	require.NoError(t, err)
	//nolint:bodyclose // The request must fail.
	_, err = client.Do(req)
	require.Error(t, err)
}
