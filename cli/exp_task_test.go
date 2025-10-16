package cli_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

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
