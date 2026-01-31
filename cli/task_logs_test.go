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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskLogs_Golden(t *testing.T) {
	t.Parallel()

	testMessages := []agentapisdk.Message{
		{
			Id:      0,
			Role:    agentapisdk.RoleUser,
			Content: "What is 1 + 1?",
			Time:    time.Now().Add(-2 * time.Minute),
		},
		{
			Id:      1,
			Role:    agentapisdk.RoleAgent,
			Content: "2",
			Time:    time.Now().Add(-1 * time.Minute),
		},
	}

	t.Run("ByTaskName_JSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTest(ctx, t, fakeAgentAPITaskLogsOK(testMessages))
		userClient := client // user already has access to their own workspace

		inv, root := clitest.New(t, "task", "logs", task.Name, "--output", "json")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify JSON is valid.
		var logs []codersdk.TaskLogEntry
		err = json.NewDecoder(strings.NewReader(output.Stdout())).Decode(&logs)
		require.NoError(t, err)

		// Verify output format with golden file.
		clitest.TestGoldenFile(t, t.Name(), output.Golden(), nil)
	})

	t.Run("ByTaskID_JSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTest(ctx, t, fakeAgentAPITaskLogsOK(testMessages))
		userClient := client

		inv, root := clitest.New(t, "task", "logs", task.ID.String(), "--output", "json")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify JSON is valid.
		var logs []codersdk.TaskLogEntry
		err = json.NewDecoder(strings.NewReader(output.Stdout())).Decode(&logs)
		require.NoError(t, err)

		// Verify output format with golden file.
		clitest.TestGoldenFile(t, t.Name(), output.Golden(), nil)
	})

	t.Run("ByTaskID_Table", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTest(ctx, t, fakeAgentAPITaskLogsOK(testMessages))
		userClient := client

		inv, root := clitest.New(t, "task", "logs", task.ID.String())
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify output format with golden file.
		clitest.TestGoldenFile(t, t.Name(), output.Golden(), nil)
	})

	t.Run("TaskNotFound_ByName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "logs", "doesnotexist")
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
		inv, root := clitest.New(t, "task", "logs", uuid.Nil.String())
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, httpapi.ResourceNotFoundResponse.Message)
	})

	t.Run("ErrorFetchingLogs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTest(ctx, t, fakeAgentAPITaskLogsErr(assert.AnError))
		userClient := client

		inv, root := clitest.New(t, "task", "logs", task.ID.String())
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, assert.AnError.Error())
	})

	t.Run("SnapshotWithLogs_Table", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTestWithSnapshot(ctx, t, codersdk.TaskStatusPaused, testMessages)
		userClient := client

		inv, root := clitest.New(t, "task", "logs", task.Name)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify output format with golden file.
		clitest.TestGoldenFile(t, t.Name(), output.Golden(), nil)
	})

	t.Run("SnapshotWithLogs_JSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTestWithSnapshot(ctx, t, codersdk.TaskStatusPaused, testMessages)
		userClient := client

		inv, root := clitest.New(t, "task", "logs", task.Name, "--output", "json")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify JSON is valid.
		var logs []codersdk.TaskLogEntry
		err = json.NewDecoder(strings.NewReader(output.Stdout())).Decode(&logs)
		require.NoError(t, err)

		// Verify output format with golden file.
		clitest.TestGoldenFile(t, t.Name(), output.Golden(), nil)
	})

	t.Run("SnapshotWithoutLogs_NoSnapshotCaptured", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTestWithoutSnapshot(t, codersdk.TaskStatusPaused)
		userClient := client

		inv, root := clitest.New(t, "task", "logs", task.Name)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify output format with golden file.
		clitest.TestGoldenFile(t, t.Name(), output.Golden(), nil)
	})

	t.Run("SnapshotWithSingleMessage", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		singleMessage := []agentapisdk.Message{
			{
				Id:      0,
				Role:    agentapisdk.RoleUser,
				Content: "Single message",
				Time:    time.Now(),
			},
		}

		client, task := setupCLITaskTestWithSnapshot(ctx, t, codersdk.TaskStatusPending, singleMessage)
		userClient := client

		inv, root := clitest.New(t, "task", "logs", task.Name)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify output format with golden file.
		clitest.TestGoldenFile(t, t.Name(), output.Golden(), nil)
	})

	t.Run("SnapshotEmptyLogs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTestWithSnapshot(ctx, t, codersdk.TaskStatusInitializing, []agentapisdk.Message{})
		userClient := client

		inv, root := clitest.New(t, "task", "logs", task.Name)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify output format with golden file.
		clitest.TestGoldenFile(t, t.Name(), output.Golden(), nil)
	})

	t.Run("InitializingTaskSnapshot", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, task := setupCLITaskTestWithSnapshot(ctx, t, codersdk.TaskStatusInitializing, testMessages)
		userClient := client

		inv, root := clitest.New(t, "task", "logs", task.Name)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify output format with golden file.
		clitest.TestGoldenFile(t, t.Name(), output.Golden(), nil)
	})
}

func fakeAgentAPITaskLogsOK(messages []agentapisdk.Message) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/messages": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": messages,
			})
		},
	}
}

func fakeAgentAPITaskLogsErr(err error) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/messages": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
		},
	}
}
