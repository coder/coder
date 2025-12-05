package cli_test

import (
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpRpty(t *testing.T) {
	t.Parallel()

	t.Run("DefaultCommand", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "exp", "rpty", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitLong)

		_ = agenttest.New(t, client.URL, agentToken)
		_ = coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		pty.WriteLine("exit")
		<-cmdDone
	})

	t.Run("Command", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		randStr := uuid.NewString()
		inv, root := clitest.New(t, "exp", "rpty", workspace.Name, "echo", randStr)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitLong)

		_ = agenttest.New(t, client.URL, agentToken)
		_ = coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		pty.ExpectMatch(randStr)
		<-cmdDone
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		client, _, _ := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "exp", "rpty", "not-found")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitShort)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "not found")
	})

	t.Run("Container", func(t *testing.T) {
		t.Parallel()
		// Skip this test on non-Linux platforms since it requires Docker
		if runtime.GOOS != "linux" {
			t.Skip("Skipping test on non-Linux platform")
		}

		wantLabel := "coder.devcontainers.TestExpRpty.Container"

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		pool, err := dockertest.NewPool("")
		require.NoError(t, err, "Could not connect to docker")
		ct, err := pool.RunWithOptions(&dockertest.RunOptions{
			Repository: "busybox",
			Tag:        "latest",
			Cmd:        []string{"sleep", "infinity"},
			Labels: map[string]string{
				wantLabel: "true",
			},
		}, func(config *docker.HostConfig) {
			config.AutoRemove = true
			config.RestartPolicy = docker.RestartPolicy{Name: "no"}
		})
		require.NoError(t, err, "Could not start container")
		// Wait for container to start
		require.Eventually(t, func() bool {
			ct, ok := pool.ContainerByName(ct.Container.Name)
			return ok && ct.Container.State.Running
		}, testutil.WaitShort, testutil.IntervalSlow, "Container did not start in time")
		t.Cleanup(func() {
			err := pool.Purge(ct)
			require.NoError(t, err, "Could not stop container")
		})

		_ = agenttest.New(t, client.URL, agentToken, func(o *agent.Options) {
			o.Devcontainers = true
			o.DevcontainerAPIOptions = append(o.DevcontainerAPIOptions,
				agentcontainers.WithProjectDiscovery(false),
				agentcontainers.WithContainerLabelIncludeFilter(wantLabel, "true"),
			)
		})
		_ = coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

		inv, root := clitest.New(t, "exp", "rpty", workspace.Name, "-c", ct.Container.ID)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitLong)
		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		pty.ExpectMatchContext(ctx, " #")
		pty.WriteLine("hostname")
		pty.ExpectMatchContext(ctx, ct.Container.Config.Hostname)
		pty.WriteLine("exit")
		<-cmdDone
	})
}
