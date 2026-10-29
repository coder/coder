package cli_test

import (
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestExpRpty(t *testing.T) {
	t.Parallel()

	randstr := uuid.NewString()

	tests := []struct {
		name   string   // test name
		args   []string // args to append
		stdin  string   // stdin to write
		stdout string   // expected stdout
		id     string   // expected client session id
	}{
		{name: "DefaultCommand", stdin: "exit"},
		{name: "Command", args: []string{"echo", randstr}, stdout: randstr},
		{name: "ClientSessionID", stdin: "exit", id: "0123456789abcdef0123456789abcdef"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			connLogger := connectionlog.NewFake()
			client, store := coderdtest.NewWithDatabase(t, &coderdtest.Options{
				ConnectionLogger: connLogger,
			})
			client.SetLogger(testutil.Logger(t).Named("client"))
			first := coderdtest.CreateFirstUser(t, client)
			userClient, user := coderdtest.CreateAnotherUserMutators(t, client, first.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
				r.Username = "myuser"
			})
			r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
				Name:           "myworkspace",
				OrganizationID: first.OrganizationID,
				OwnerID:        user.ID,
			}).WithAgent().Do()

			args := []string{"exp", "rpty", r.Workspace.Name}
			args = append(args, tc.args...)
			inv, root := clitest.New(t, args...)

			clitest.SetupConfig(t, userClient, root)
			if tc.id != "" {
				inv.Environ.Set("CODER_TRACE_SESSION_ID", tc.id)
			}

			stdin := testutil.NewWriterAttachedToInvocation(t, testutil.Logger(t), inv)
			stdout := expecter.NewAttachedToInvocation(t, inv)

			_ = agenttest.New(t, client.URL, r.AgentToken)
			_ = coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).Wait()

			ctx := testutil.Context(t, testutil.WaitLong)
			cmdDone := tGo(t, func() {
				err := inv.WithContext(ctx).Run()
				assert.NoError(t, err)
			})
			if tc.stdin != "" {
				stdin.WriteLine(tc.stdin)
			}
			if tc.stdout != "" {
				stdout.ExpectMatch(ctx, tc.stdout)
			}
			<-cmdDone
			assertConnLog(t, connLogger, r.Workspace, database.ConnectionLogMethodReconnectingPTY, tc.id)
		})
	}

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

		logger := testutil.Logger(t)
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
		stdout := expecter.NewAttachedToInvocation(t, inv)
		stdin := testutil.NewWriterAttachedToInvocation(t, logger.Named("stdin"), inv)

		ctx := testutil.Context(t, testutil.WaitLong)
		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		stdout.ExpectMatch(ctx, " #")
		stdin.WriteLine("hostname")
		stdout.ExpectMatch(ctx, ct.Container.Config.Hostname)
		stdin.WriteLine("exit")
		<-cmdDone
	})
}
