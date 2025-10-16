package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aiagentapi "github.com/coder/agentapi-sdk-go"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskSend(t *testing.T) {
	t.Parallel()

	t.Run("ByWorkspaceName_WithArgument", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, workspace := setupTaskSendTest(ctx, t, "carry on with the task")
		userClient := client

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "send", workspace.Name, "carry on with the task")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("ByWorkspaceID_WithArgument", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, workspace := setupTaskSendTest(ctx, t, "carry on with the task")
		userClient := client

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "send", workspace.ID.String(), "carry on with the task")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("ByWorkspaceName_WithStdin", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, workspace := setupTaskSendTest(ctx, t, "carry on with the task")
		userClient := client

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "send", workspace.Name, "--stdin")
		inv.Stdout = &stdout
		inv.Stdin = strings.NewReader("carry on with the task")
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("WorkspaceNotFound_ByName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "send", "doesnotexist", "some task input")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, httpapi.ResourceNotFoundResponse.Message)
	})

	t.Run("WorkspaceNotFound_ByID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "send", uuid.Nil.String(), "some task input")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, httpapi.ResourceNotFoundResponse.Message)
	})
}

// setupTaskSendTest creates a test workspace with an AI task template and agent,
// configured to expect a specific message input for testing task send functionality.
func setupTaskSendTest(ctx context.Context, t *testing.T, expectedInput string) (*codersdk.Client, codersdk.Workspace) {
	t.Helper()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	owner := coderdtest.CreateFirstUser(t, client)
	userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	fakeAPI := startFakeAgentAPI(t, map[string]http.HandlerFunc{
		"/status": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "stable",
			})
		},
		"/message": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			var msg aiagentapi.PostMessageParams
			if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			assert.Equal(t, expectedInput, msg.Content)
			message := aiagentapi.Message{
				Id:      999,
				Role:    aiagentapi.RoleAgent,
				Content: "Task received",
				Time:    time.Now(),
			}
			_ = json.NewEncoder(w).Encode(message)
		},
	})

	authToken := uuid.NewString()
	template := createAITaskTemplate(t, client, owner.OrganizationID, withSidebarURL(fakeAPI.URL()), withAgentToken(authToken))

	wantPrompt := "test prompt"
	workspace := coderdtest.CreateWorkspace(t, userClient, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
		req.RichParameterValues = []codersdk.WorkspaceBuildParameter{
			{Name: codersdk.AITaskPromptParameterName, Value: wantPrompt},
		}
	})
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))
	_ = agenttest.New(t, client.URL, authToken, func(o *agent.Options) {
		o.Client = agentClient
	})

	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).
		WaitFor(coderdtest.AgentsReady)

	return userClient, workspace
}
