package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskLogs(t *testing.T) {
	t.Parallel()

	testMessages := []aiagentapi.Message{
		{
			Id:      0,
			Role:    aiagentapi.RoleUser,
			Content: "What is 1 + 1?",
			Time:    time.Now().Add(-2 * time.Minute),
		},
		{
			Id:      1,
			Role:    aiagentapi.RoleAgent,
			Content: "2",
			Time:    time.Now().Add(-1 * time.Minute),
		},
	}

	t.Run("ByWorkspaceName_JSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, workspace := setupCLITaskTest(ctx, t, fakeAgentAPITaskLogsOK(testMessages))
		userClient := client // user already has access to their own workspace

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "logs", workspace.Name, "--output", "json")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var logs []codersdk.TaskLogEntry
		err = json.NewDecoder(strings.NewReader(stdout.String())).Decode(&logs)
		require.NoError(t, err)

		require.Len(t, logs, 2)
		require.Equal(t, "What is 1 + 1?", logs[0].Content)
		require.Equal(t, codersdk.TaskLogTypeInput, logs[0].Type)
		require.Equal(t, "2", logs[1].Content)
		require.Equal(t, codersdk.TaskLogTypeOutput, logs[1].Type)
	})

	t.Run("ByWorkspaceID_JSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, workspace := setupCLITaskTest(ctx, t, fakeAgentAPITaskLogsOK(testMessages))
		userClient := client

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "logs", workspace.ID.String(), "--output", "json")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var logs []codersdk.TaskLogEntry
		err = json.NewDecoder(strings.NewReader(stdout.String())).Decode(&logs)
		require.NoError(t, err)

		require.Len(t, logs, 2)
		require.Equal(t, "What is 1 + 1?", logs[0].Content)
		require.Equal(t, codersdk.TaskLogTypeInput, logs[0].Type)
		require.Equal(t, "2", logs[1].Content)
		require.Equal(t, codersdk.TaskLogTypeOutput, logs[1].Type)
	})

	t.Run("ByWorkspaceID_Table", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, workspace := setupCLITaskTest(ctx, t, fakeAgentAPITaskLogsOK(testMessages))
		userClient := client

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "logs", workspace.ID.String())
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		output := stdout.String()
		require.Contains(t, output, "What is 1 + 1?")
		require.Contains(t, output, "2")
		require.Contains(t, output, "input")
		require.Contains(t, output, "output")
	})

	t.Run("WorkspaceNotFound_ByName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "logs", "doesnotexist")
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
		inv, root := clitest.New(t, "exp", "task", "logs", uuid.Nil.String())
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, httpapi.ResourceNotFoundResponse.Message)
	})

	t.Run("ErrorFetchingLogs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client, workspace := setupCLITaskTest(ctx, t, fakeAgentAPITaskLogsErr(assert.AnError))
		userClient := client

		inv, root := clitest.New(t, "exp", "task", "logs", workspace.ID.String())
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, assert.AnError.Error())
	})
}

// createAITaskTemplate creates a template configured for AI tasks with a sidebar app.
func createAITaskTemplate(t *testing.T, client *codersdk.Client, orgID uuid.UUID, opts ...aiTemplateOpt) codersdk.Template {
	t.Helper()

	opt := aiTemplateOpts{
		authToken: uuid.NewString(),
	}
	for _, o := range opts {
		o(&opt)
	}

	taskAppID := uuid.New()
	version := coderdtest.CreateTemplateVersion(t, client, orgID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
						HasAiTasks: true,
					},
				},
			},
		},
		ProvisionApply: []*proto.Response{
			{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{
							{
								Name: "example",
								Type: "aws_instance",
								Agents: []*proto.Agent{
									{
										Id:   uuid.NewString(),
										Name: "example",
										Auth: &proto.Agent_Token{
											Token: opt.authToken,
										},
										Apps: []*proto.App{
											{
												Id:          taskAppID.String(),
												Slug:        "task-sidebar",
												DisplayName: "Task Sidebar",
												Url:         opt.appURL,
											},
										},
									},
								},
							},
						},
						AiTasks: []*proto.AITask{
							{
								SidebarApp: &proto.AITaskSidebarApp{
									Id: taskAppID.String(),
								},
							},
						},
					},
				},
			},
		},
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, orgID, version.ID)

	return template
}

// fakeAgentAPI implements a fake AgentAPI HTTP server for testing.
type fakeAgentAPI struct {
	t        *testing.T
	server   *httptest.Server
	handlers map[string]http.HandlerFunc
	called   map[string]bool
	mu       sync.Mutex
}

// startFakeAgentAPI starts an HTTP server that implements the AgentAPI endpoints.
// handlers is a map of path -> handler function.
func startFakeAgentAPI(t *testing.T, handlers map[string]http.HandlerFunc) *fakeAgentAPI {
	t.Helper()

	fake := &fakeAgentAPI{
		t:        t,
		handlers: handlers,
		called:   make(map[string]bool),
	}

	mux := http.NewServeMux()

	// Register all provided handlers with call tracking
	for path, handler := range handlers {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			fake.mu.Lock()
			fake.called[path] = true
			fake.mu.Unlock()
			handler(w, r)
		})
	}

	knownEndpoints := []string{"/status", "/messages", "/message"}
	for _, endpoint := range knownEndpoints {
		if handlers[endpoint] == nil {
			endpoint := endpoint // capture loop variable
			mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("unexpected call to %s %s - no handler defined", r.Method, endpoint)
			})
		}
	}
	// Default handler for unknown endpoints should cause the test to fail.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected call to %s %s - no handler defined", r.Method, r.URL.Path)
	})

	fake.server = httptest.NewServer(mux)

	// Register cleanup to check that all defined handlers were called
	t.Cleanup(func() {
		fake.server.Close()
		fake.mu.Lock()
		for path := range handlers {
			if !fake.called[path] {
				t.Errorf("handler for %s was defined but never called", path)
			}
		}
	})
	return fake
}

func (f *fakeAgentAPI) URL() string {
	return f.server.URL
}

type aiTemplateOpts struct {
	appURL    string
	authToken string
}

type aiTemplateOpt func(*aiTemplateOpts)

func withSidebarURL(url string) aiTemplateOpt {
	return func(o *aiTemplateOpts) { o.appURL = url }
}

func withAgentToken(token string) aiTemplateOpt {
	return func(o *aiTemplateOpts) { o.authToken = token }
}

func fakeAgentAPITaskLogsOK(messages []aiagentapi.Message) map[string]http.HandlerFunc {
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

// setupCLITaskTest creates a test workspace with an AI task template and agent,
// with a fake agent API configured with the provided set of handlers.
// Returns the user client and workspace.
func setupCLITaskTest(ctx context.Context, t *testing.T, agentAPIHandlers map[string]http.HandlerFunc) (*codersdk.Client, codersdk.Workspace) {
	t.Helper()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	owner := coderdtest.CreateFirstUser(t, client)
	userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	fakeAPI := startFakeAgentAPI(t, agentAPIHandlers)

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
