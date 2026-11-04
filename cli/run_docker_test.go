package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/x/agentworkload"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestRunDocker exercises CLI, authorization, placement, real agent tailnet
// transport, and Docker. The image must be prepared before running this test.
func TestRunDocker(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_WORKLOAD_DOCKER_TEST") != "1" {
		t.Skip("set CODER_WORKLOAD_DOCKER_TEST=1 after building examples/workloads/Dockerfile")
	}
	// The Go test timeout bounds the full benchmark, which can launch hundreds
	// of CLI processes. Each invocation has a separate deadline below.
	ctx := t.Context()

	state := t.TempDir()
	token := uuid.NewString() + uuid.NewString()
	config := map[string]any{
		"token": token, "state_dir": filepath.Join(state, "state"),
		"image": "coder-workload-demo:coder", "workdir": "/work/coder", "shell": "/bin/bash",
		"repositories": map[string]string{"/work/coder": "e26c1934a5f96c5209e7b054419db5f6c5c12323"},
		"cpu":          int64(4), "memory_bytes": int64(8 << 30),
		"docker_host": os.Getenv("CODER_WORKLOAD_TEST_DOCKER_HOST"),
	}
	data, err := json.Marshal(config)
	require.NoError(t, err)
	configPath := filepath.Join(state, "host.json")
	require.NoError(t, os.WriteFile(configPath, data, 0o600))
	supervisor, err := agentworkload.NewFromFile(ctx, configPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, supervisor.Close()) })

	deployment := coderdtest.DeploymentValues(t)
	deployment.Experiments = []string{string(codersdk.ExperimentWorkloads)}
	client, store := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: deployment})
	first := coderdtest.CreateFirstUser(t, client)
	host := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{OrganizationID: first.OrganizationID, OwnerID: first.UserID}).WithAgent(func(agents []*sdkproto.Agent) []*sdkproto.Agent {
		agents[0].OperatingSystem = "linux"
		return agents
	}).Do()
	agenttest.New(t, client.URL, host.AgentToken, func(o *agent.Options) {
		o.WorkloadHandler = supervisor
	})
	coderdtest.AwaitWorkspaceAgents(t, client, host.Workspace.ID)
	registered, err := client.RegisterWorkloadHost(ctx, first.OrganizationID, codersdk.RegisterWorkloadHostRequest{AgentID: host.Agents[0].ID, Token: token})
	require.NoError(t, err)
	require.Equal(t, host.Workspace.TemplateID, registered.TemplateID)
	runner, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
	_, err = runner.Workspace(ctx, host.Workspace.ID)
	require.Error(t, err, "ordinary workload users must not have access to the service workspace")

	// Reading project source and loading installed dependencies precedes the
	// marker. Merely spawning a shell does not count as useful execution.
	command := `test -s codersdk/client.go && node -e 'if (require("./site/node_modules/lodash").sum([20,22]) !== 42) process.exit(1); console.log("PROJECT_READY")'`
	samples := 1
	if raw := os.Getenv("CODER_WORKLOAD_BENCHMARK_SAMPLES"); raw != "" {
		samples, err = strconv.Atoi(raw)
		require.NoError(t, err)
		require.Positive(t, samples)
		require.LessOrEqual(t, samples, 1000)
	}
	latencies := make([]time.Duration, 0, samples)
	for range samples {
		inv, root := clitest.New(t, "run", "--template", host.Workspace.TemplateID.String(), "--cpu", "1", "--ram", "512MiB", "--cmd", command)
		clitest.SetupConfig(t, runner, root)
		output := &workloadReadinessWriter{start: time.Now()}
		var stderr bytes.Buffer
		inv.Stdout, inv.Stderr = output, &stderr
		sampleCtx, sampleCancel := context.WithTimeout(ctx, testutil.WaitSuperLong)
		if binary := os.Getenv("CODER_WORKLOAD_TEST_BINARY"); binary != "" {
			cmd := exec.CommandContext(sampleCtx, binary, inv.Args...)
			// Match the in-process invocation's empty environment. Inherited
			// CODER_URL or credentials must never select a real deployment.
			cmd.Env = []string{}
			cmd.Stdout, cmd.Stderr = output, &stderr
			err = cmd.Run()
		} else {
			err = inv.WithContext(sampleCtx).Run()
		}
		sampleCancel()
		require.NoError(t, err, "%s", stderr.String())
		require.Equal(t, "PROJECT_READY\n", output.String())
		require.Positive(t, output.ready)
		require.Empty(t, stderr.String())
		latencies = append(latencies, output.ready)
	}
	slices.Sort(latencies)
	percentile := func(p int) time.Duration { return latencies[(len(latencies)*p+99)/100-1] }
	t.Logf("useful-project latency: samples=%d concurrency=1 prepared image warm local Docker; p50=%s p95=%s p99=%s max=%s; external CLI=%t", samples, percentile(50), percentile(95), percentile(99), latencies[len(latencies)-1], os.Getenv("CODER_WORKLOAD_TEST_BINARY") != "")

	// The private branch can be changed, but the next run must see the base.
	for _, tc := range []struct {
		command, ram, stdout, stderr string
		exitCode                     int
	}{
		{command: `printf private > /work/coder/private-marker; printf error-output >&2; exit 7`, ram: "512MiB", stderr: "error-output", exitCode: 7},
		{command: `test ! -e /work/coder/private-marker && GOPROXY=off GOTOOLCHAIN=local GOMAXPROCS=2 go test -count=1 -p=1 ./coderd/util/ptr ./coderd/util/slice && pnpm --dir site test src/utils/fileSize.test.ts --maxWorkers=1 && printf 'PROJECT_TESTS_PASSED\n'`, ram: "8GiB", stdout: "PROJECT_TESTS_PASSED"},
		{command: `python3 -c 'from pathlib import Path; import os; c=Path("/sys/fs/cgroup"); assert (c/"cpu.max").read_text().strip() == "100000 100000"; assert (c/"memory.max").read_text().strip() == "536870912"; assert (c/"memory.swap.max").read_text().strip() == "0"; assert (c/"pids.max").read_text().strip() == "256"; assert not Path("/var/run/docker.sock").exists(); assert not any(k.startswith("CODER_") for k in os.environ); assert set(p.name for p in Path("/sys/class/net").iterdir()) == {"lo"}; print("ISOLATED")'`, ram: "512MiB", stdout: "ISOLATED"},
		{command: `python3 -c 'data=bytearray(256*1024*1024)'`, ram: "32MiB", exitCode: 137},
	} {
		inv, root := clitest.New(t, "run", "--template", host.Workspace.TemplateID.String(), "--cpu", "1", "--ram", tc.ram, "--cmd", tc.command)
		clitest.SetupConfig(t, runner, root)
		var stdout, stderr bytes.Buffer
		inv.Stdout, inv.Stderr = &stdout, &stderr
		runCtx, runCancel := context.WithTimeout(ctx, testutil.WaitSuperLong)
		err := inv.WithContext(runCtx).Run()
		runCancel()
		if tc.exitCode != 0 {
			var exitErr interface{ ExitCode() int }
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, tc.exitCode, exitErr.ExitCode())
		} else {
			require.NoError(t, err, "%s", stderr.String())
		}
		require.Contains(t, stdout.String(), tc.stdout)
		require.Contains(t, stderr.String(), tc.stderr)
	}

	// A running command reserves host CPUs. Repeated API submission and
	// observation keep the same identity, and explicit cancellation frees it.
	request := codersdk.CreateWorkloadRequest{ID: uuid.New(), TemplateID: host.Workspace.TemplateID, CPU: 1, MemoryBytes: 512 << 20, Command: `printf RUNNING; exec sleep 300`}
	started, err := runner.CreateWorkload(ctx, first.OrganizationID, request)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		assert.NoError(t, runner.CancelWorkload(cleanupCtx, first.OrganizationID, request.ID))
	})
	require.Equal(t, request.ID, started.ID)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		output, err := runner.WorkloadOutput(ctx, first.OrganizationID, request.ID, 0)
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, "running", output.Workload.State)
		assert.NotEmpty(c, output.Events)
	}, testutil.WaitLong, testutil.IntervalFast)
	duplicate, err := runner.CreateWorkload(ctx, first.OrganizationID, request)
	require.NoError(t, err)
	require.Equal(t, started.ID, duplicate.ID)
	busy, err := runner.CreateWorkload(ctx, first.OrganizationID, codersdk.CreateWorkloadRequest{ID: uuid.New(), TemplateID: request.TemplateID, CPU: 4, MemoryBytes: 512 << 20, Command: "true"})
	require.NoError(t, err)
	require.Equal(t, "rejected", busy.State)
	require.NoError(t, runner.CancelWorkload(ctx, first.OrganizationID, request.ID))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		output, err := runner.WorkloadOutput(ctx, first.OrganizationID, request.ID, 0)
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, "canceled", output.Workload.State)
		assert.False(c, output.Workload.CleanupPending)
		var data []byte
		for _, event := range output.Events {
			data = append(data, event.Data...)
		}
		assert.Equal(c, "RUNNING", string(data))
	}, testutil.WaitLong, testutil.IntervalFast)
	duplicate, err = runner.CreateWorkload(ctx, first.OrganizationID, request)
	require.NoError(t, err)
	require.Equal(t, "canceled", duplicate.State)
	require.NoError(t, client.DrainWorkloadHost(ctx, first.OrganizationID, host.Agents[0].ID))
	rejected, err := client.CreateWorkload(ctx, first.OrganizationID, codersdk.CreateWorkloadRequest{ID: uuid.New(), TemplateID: host.Workspace.TemplateID, CPU: 1, MemoryBytes: 512 << 20, Command: "true"})
	require.NoError(t, err)
	require.Equal(t, "rejected", rejected.State)
}

type workloadReadinessWriter struct {
	buffer bytes.Buffer
	start  time.Time
	ready  time.Duration
}

func (w *workloadReadinessWriter) String() string { return w.buffer.String() }

func (w *workloadReadinessWriter) Write(p []byte) (int, error) {
	n, err := w.buffer.Write(p)
	if w.ready == 0 && bytes.Contains(w.buffer.Bytes(), []byte("PROJECT_READY\n")) {
		w.ready = time.Since(w.start)
	}
	return n, err
}

var _ io.Writer = (*workloadReadinessWriter)(nil)
