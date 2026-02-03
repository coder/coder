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
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskSend(t *testing.T) {
	t.Parallel()

	t.Run("ByTaskName_WithArgument", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTest(ctx, t, fakeAgentAPITaskSendOK(t, "carry on with the task", "you got it"))
		userClient := client

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", task.Name, "carry on with the task")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("ByTaskID_WithArgument", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTest(ctx, t, fakeAgentAPITaskSendOK(t, "carry on with the task", "you got it"))
		userClient := client

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", task.ID.String(), "carry on with the task")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("ByTaskName_WithStdin", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTest(ctx, t, fakeAgentAPITaskSendOK(t, "carry on with the task", "you got it"))
		userClient := client

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", task.Name, "--stdin")
		inv.Stdout = &stdout
		inv.Stdin = strings.NewReader("carry on with the task")
		clitest.SetupConfig(t, userClient, root)

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
		ctx := testutil.Context(t, testutil.WaitLong)

		userClient, task := setupCLITaskTest(ctx, t, fakeAgentAPITaskSendErr(t, assert.AnError))

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "send", task.Name, "some task input")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, assert.AnError.Error())
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

func fakeAgentAPITaskSendErr(t *testing.T, returnErr error) map[string]http.HandlerFunc {
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
