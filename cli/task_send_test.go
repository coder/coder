package cli_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentapisdk "github.com/coder/agentapi-sdk-go"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskSend(t *testing.T) {
	t.Parallel()

	t.Run("ByTaskName_WithArgument", func(t *testing.T) {
		t.Parallel()

		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, fakeAgentAPITaskSendOK(t, "carry on with the task", "you got it"))

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", setup.task.Name, "carry on with the task")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, setup.userClient, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("ByTaskID_WithArgument", func(t *testing.T) {
		t.Parallel()

		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, fakeAgentAPITaskSendOK(t, "carry on with the task", "you got it"))

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", setup.task.ID.String(), "carry on with the task")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, setup.userClient, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("ByTaskName_WithStdin", func(t *testing.T) {
		t.Parallel()

		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, fakeAgentAPITaskSendOK(t, "carry on with the task", "you got it"))

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", setup.task.Name, "--stdin")
		inv.Stdout = &stdout
		inv.Stdin = strings.NewReader("carry on with the task")
		clitest.SetupConfig(t, setup.userClient, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("TaskNotFound_ByName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", "doesnotexist", "some task input")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, httpapi.ResourceNotFoundResponse.Message)
	})

	t.Run("TaskNotFound_ByID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", uuid.Nil.String(), "some task input")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, httpapi.ResourceNotFoundResponse.Message)
	})

	t.Run("SendError", func(t *testing.T) {
		t.Parallel()

		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, fakeAgentAPITaskSendErr(assert.AnError))

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", setup.task.Name, "some task input")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, setup.userClient, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, assert.AnError.Error())
	})

	t.Run("WaitsForInitializingTask", func(t *testing.T) {
		t.Parallel()

		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, fakeAgentAPITaskSendOK(t, "some task input", "some task response"))

		// Close the first agent, pause, then resume the task so the
		// workspace is started but no agent is connected.
		// This puts the task in "initializing" state.
		require.NoError(t, setup.agent.Close())
		pauseTask(setupCtx, t, setup.userClient, setup.task)
		resumeTask(setupCtx, t, setup.userClient, setup.task)

		// When: We attempt to send input to the initializing task.
		inv, root := clitest.New(t, "task", "send", setup.task.Name, "some task input")
		clitest.SetupConfig(t, setup.userClient, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		inv = inv.WithContext(ctx)

		// Use a pty so we can wait for the command to produce build
		// output, confirming it has entered the initializing code
		// path before we connect the agent.
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)

		// Wait for the command to observe the initializing state and
		// start watching the workspace build. This ensures the command
		// has entered the waiting code path.
		pty.ExpectMatchContext(ctx, "Queued")

		// Connect a new agent so the task can transition to active.
		agentClient := agentsdk.New(setup.userClient.URL, agentsdk.WithFixedToken(setup.agentToken))
		setup.agent = agenttest.New(t, setup.userClient.URL, setup.agentToken, func(o *agent.Options) {
			o.Client = agentClient
		})
		coderdtest.NewWorkspaceAgentWaiter(t, setup.userClient, setup.task.WorkspaceID.UUID).
			WaitFor(coderdtest.AgentsReady)

		// Then: The command should complete successfully.
		require.NoError(t, w.Wait())

		updated, err := setup.userClient.TaskByIdentifier(ctx, setup.task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusActive, updated.Status)
	})

	t.Run("ResumesPausedTask", func(t *testing.T) {
		t.Parallel()

		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, fakeAgentAPITaskSendOK(t, "some task input", "some task response"))

		// Close the first agent before pausing so it does not conflict
		// with the agent we reconnect after the workspace is resumed.
		require.NoError(t, setup.agent.Close())
		pauseTask(setupCtx, t, setup.userClient, setup.task)

		// When: We attempt to send input to the paused task.
		inv, root := clitest.New(t, "task", "send", setup.task.Name, "some task input")
		clitest.SetupConfig(t, setup.userClient, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		inv = inv.WithContext(ctx)

		// Use a pty so we can wait for the command to produce build
		// output, confirming it has entered the paused code path and
		// triggered a resume before we connect the agent.
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)

		// Wait for the command to observe the paused state, trigger
		// a resume, and start watching the workspace build.
		pty.ExpectMatchContext(ctx, "Queued")

		// Connect a new agent so the task can transition to active.
		agentClient := agentsdk.New(setup.userClient.URL, agentsdk.WithFixedToken(setup.agentToken))
		setup.agent = agenttest.New(t, setup.userClient.URL, setup.agentToken, func(o *agent.Options) {
			o.Client = agentClient
		})
		coderdtest.NewWorkspaceAgentWaiter(t, setup.userClient, setup.task.WorkspaceID.UUID).
			WaitFor(coderdtest.AgentsReady)

		// Then: The command should complete successfully.
		require.NoError(t, w.Wait())

		updated, err := setup.userClient.TaskByIdentifier(ctx, setup.task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusActive, updated.Status)
	})
}

func fakeAgentAPITaskSendOK(t *testing.T, expectMessage, returnMessage string) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/status": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "stable",
			})
		},
		"/message": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			var msg agentapisdk.PostMessageParams
			if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			assert.Equal(t, expectMessage, msg.Content)
			message := agentapisdk.Message{
				Id:      999,
				Role:    agentapisdk.RoleAgent,
				Content: returnMessage,
				Time:    time.Now(),
			}
			_ = json.NewEncoder(w).Encode(message)
		},
	}
}

func fakeAgentAPITaskSendErr(returnErr error) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/status": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "stable",
			})
		},
		"/message": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(returnErr.Error()))
		},
	}
}
