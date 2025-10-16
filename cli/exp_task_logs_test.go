package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
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

	var (
		clock = time.Date(2025, 8, 26, 12, 34, 56, 0, time.UTC)

		taskID   = uuid.MustParse("11111111-1111-1111-1111-111111111111")
		taskName = "task-workspace"

		taskLogs = []codersdk.TaskLogEntry{
			{
				ID:      0,
				Content: "What is 1 + 1?",
				Type:    codersdk.TaskLogTypeInput,
				Time:    clock,
			},
			{
				ID:      1,
				Content: "2",
				Type:    codersdk.TaskLogTypeOutput,
				Time:    clock.Add(1 * time.Second),
			},
		}
	)

	tests := []struct {
		args        []string
		expectTable string
		expectLogs  []codersdk.TaskLogEntry
		expectError string
		handler     func(t *testing.T, ctx context.Context) http.HandlerFunc
	}{
		{
			args:       []string{taskName, "--output", "json"},
			expectLogs: taskLogs,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/users/me/workspace/%s", taskName):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Workspace{
							ID: taskID,
						})
					case fmt.Sprintf("/api/experimental/tasks/me/%s/logs", taskID.String()):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.TaskLogsResponse{
							Logs: taskLogs,
						})
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:       []string{taskID.String(), "--output", "json"},
			expectLogs: taskLogs,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/experimental/tasks/me/%s/logs", taskID.String()):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.TaskLogsResponse{
							Logs: taskLogs,
						})
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args: []string{taskID.String()},
			expectTable: `
TYPE    CONTENT
input   What is 1 + 1?
output  2`,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/experimental/tasks/me/%s/logs", taskID.String()):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.TaskLogsResponse{
							Logs: taskLogs,
						})
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{"doesnotexist"},
			expectError: httpapi.ResourceNotFoundResponse.Message,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/workspace/doesnotexist":
						httpapi.ResourceNotFound(w)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{uuid.Nil.String()}, // uuid does not exist
			expectError: httpapi.ResourceNotFoundResponse.Message,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/experimental/tasks/me/%s/logs", uuid.Nil.String()):
						httpapi.ResourceNotFound(w)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{"err-fetching-logs"},
			expectError: assert.AnError.Error(),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/workspace/err-fetching-logs":
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Workspace{
							ID: taskID,
						})
					case fmt.Sprintf("/api/experimental/tasks/me/%s/logs", taskID.String()):
						httpapi.InternalServerError(w, assert.AnError)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, ","), func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				srv    = httptest.NewServer(tt.handler(t, ctx))
				client = codersdk.New(testutil.MustURL(t, srv.URL))
				args   = []string{"exp", "task", "logs"}
				stdout strings.Builder
				err    error
			)

			t.Cleanup(srv.Close)

			inv, root := clitest.New(t, append(args, tt.args...)...)
			inv.Stdout = &stdout
			inv.Stderr = &stdout
			clitest.SetupConfig(t, client, root)

			err = inv.WithContext(ctx).Run()
			if tt.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.expectError)
			}

			if tt.expectTable != "" {
				if diff := tableDiff(tt.expectTable, stdout.String()); diff != "" {
					t.Errorf("unexpected output diff (-want +got):\n%s", diff)
				}
			}

			if tt.expectLogs != nil {
				var logs []codersdk.TaskLogEntry
				err = json.NewDecoder(strings.NewReader(stdout.String())).Decode(&logs)
				require.NoError(t, err)

				assert.Equal(t, tt.expectLogs, logs)
			}
		})
	}
}

func Test_TaskLogs_HappyPath(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	owner := coderdtest.CreateFirstUser(t, client)
	userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	testMessages := []aiagentapi.Message{
		{
			Id:      0,
			Role:    aiagentapi.RoleUser,
			Content: "What is 2 + 2?",
			Time:    time.Now().Add(-2 * time.Minute),
		},
		{
			Id:      1,
			Role:    aiagentapi.RoleAgent,
			Content: "The answer is 4",
			Time:    time.Now().Add(-1 * time.Minute),
		},
		{
			Id:      2,
			Role:    aiagentapi.RoleUser,
			Content: "Thanks!",
			Time:    time.Now(),
		},
	}

	fakeAPI := startFakeAgentAPI(t, map[string]http.HandlerFunc{
		"/messages": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": testMessages,
			})
		},
	})
	authToken := uuid.NewString()
	template := createAITaskTemplate(t, client, owner.OrganizationID, withSidebarURL(fakeAPI.URL()), withAgentToken(authToken))

	wantPrompt := "build me a calculator"
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

	ws := coderdtest.MustWorkspace(t, client, workspace.ID)
	resources := ws.LatestBuild.Resources

	require.Len(t, resources, 1)
	require.Len(t, resources[0].Agents, 1)
	agentResource := resources[0].Agents[0]
	require.Len(t, agentResource.Apps, 1)
	// App health may be disabled since we're using a fake API without proper health checks
	require.NotEqual(t, codersdk.WorkspaceAppHealthUnhealthy, agentResource.Apps[0].Health)

	t.Run("WithoutFollow_JSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "logs", workspace.Name, "--output", "json")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var logs []codersdk.TaskLogEntry
		err = json.NewDecoder(strings.NewReader(stdout.String())).Decode(&logs)
		require.NoError(t, err)

		require.Len(t, logs, 3)
		require.Equal(t, "What is 2 + 2?", logs[0].Content)
		require.Equal(t, codersdk.TaskLogTypeInput, logs[0].Type)
		require.Equal(t, "The answer is 4", logs[1].Content)
		require.Equal(t, codersdk.TaskLogTypeOutput, logs[1].Type)
		require.Equal(t, "Thanks!", logs[2].Content)
		require.Equal(t, codersdk.TaskLogTypeInput, logs[2].Type)
	})

	t.Run("WithoutFollow_Table", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		var stdout strings.Builder
		inv, root := clitest.New(t, "exp", "task", "logs", workspace.Name)
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		output := stdout.String()
		require.Contains(t, output, "What is 2 + 2?")
		require.Contains(t, output, "The answer is 4")
		require.Contains(t, output, "Thanks!")
		require.Contains(t, output, "input")
		require.Contains(t, output, "output")
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
