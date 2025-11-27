package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	agentapisdk "github.com/coder/agentapi-sdk-go"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// This test performs an integration-style test for tasks functionality.
//
//nolint:tparallel // The sub-tests of this test must be run sequentially.
func Test_Tasks(t *testing.T) {
	t.Parallel()

	// Given: a template configured for tasks
	var (
		ctx           = testutil.Context(t, testutil.WaitLong)
		client        = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner         = coderdtest.CreateFirstUser(t, client)
		userClient, _ = coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		initMsg       = agentapisdk.Message{
			Content: "test task input for " + t.Name(),
			Id:      0,
			Role:    "user",
			Time:    time.Now().UTC(),
		}
		authToken    = uuid.NewString()
		echoAgentAPI = startFakeAgentAPI(t, fakeAgentAPIEcho(ctx, t, initMsg, "hello"))
		taskTpl      = createAITaskTemplate(t, client, owner.OrganizationID, withAgentToken(authToken), withSidebarURL(echoAgentAPI.URL()))
		taskName     = strings.ReplaceAll(testutil.GetRandomName(t), "_", "-")
	)

	for _, tc := range []struct {
		name     string
		cmdArgs  []string
		assertFn func(stdout string, userClient *codersdk.Client)
	}{
		{
			name:    "create task",
			cmdArgs: []string{"task", "create", "test task input for " + t.Name(), "--name", taskName, "--template", taskTpl.Name},
			assertFn: func(stdout string, userClient *codersdk.Client) {
				require.Contains(t, stdout, taskName, "task name should be in output")
			},
		},
		{
			name:    "list tasks after create",
			cmdArgs: []string{"task", "list", "--output", "json"},
			assertFn: func(stdout string, userClient *codersdk.Client) {
				var tasks []codersdk.Task
				err := json.NewDecoder(strings.NewReader(stdout)).Decode(&tasks)
				require.NoError(t, err, "list output should unmarshal properly")
				require.Len(t, tasks, 1, "expected one task")
				require.Equal(t, taskName, tasks[0].Name, "task name should match")
				require.Equal(t, initMsg.Content, tasks[0].InitialPrompt, "initial prompt should match")
				require.True(t, tasks[0].WorkspaceID.Valid, "workspace should be created")
				// For the next test, we need to wait for the workspace to be healthy
				ws := coderdtest.MustWorkspace(t, userClient, tasks[0].WorkspaceID.UUID)
				coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
				agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))
				_ = agenttest.New(t, client.URL, authToken, func(o *agent.Options) {
					o.Client = agentClient
				})
				coderdtest.NewWorkspaceAgentWaiter(t, userClient, tasks[0].WorkspaceID.UUID).WithContext(ctx).WaitFor(coderdtest.AgentsReady)
			},
		},
		{
			name:    "get task status after create",
			cmdArgs: []string{"task", "status", taskName, "--output", "json"},
			assertFn: func(stdout string, userClient *codersdk.Client) {
				var task codersdk.Task
				require.NoError(t, json.NewDecoder(strings.NewReader(stdout)).Decode(&task), "should unmarshal task status")
				require.Equal(t, task.Name, taskName, "task name should match")
				require.Equal(t, codersdk.TaskStatusActive, task.Status, "task should be active")
			},
		},
		{
			name:    "send task message",
			cmdArgs: []string{"task", "send", taskName, "hello"},
			// Assertions for this happen in the fake agent API handler.
		},
		{
			name:    "read task logs",
			cmdArgs: []string{"task", "logs", taskName, "--output", "json"},
			assertFn: func(stdout string, userClient *codersdk.Client) {
				var logs []codersdk.TaskLogEntry
				require.NoError(t, json.NewDecoder(strings.NewReader(stdout)).Decode(&logs), "should unmarshal task logs")
				require.Len(t, logs, 3, "should have 3 logs")
				require.Equal(t, logs[0].Content, initMsg.Content, "first message should be the init message")
				require.Equal(t, logs[0].Type, codersdk.TaskLogTypeInput, "first message should be an input")
				require.Equal(t, logs[1].Content, "hello", "second message should be the sent message")
				require.Equal(t, logs[1].Type, codersdk.TaskLogTypeInput, "second message should be an input")
				require.Equal(t, logs[2].Content, "hello", "third message should be the echoed message")
				require.Equal(t, logs[2].Type, codersdk.TaskLogTypeOutput, "third message should be an output")
			},
		},
		{
			name:    "delete task",
			cmdArgs: []string{"task", "delete", taskName, "--yes"},
			assertFn: func(stdout string, userClient *codersdk.Client) {
				// The task should eventually no longer show up in the list of tasks
				testutil.Eventually(ctx, t, func(ctx context.Context) bool {
					tasks, err := userClient.Tasks(ctx, &codersdk.TasksFilter{})
					if !assert.NoError(t, err) {
						return false
					}
					return slices.IndexFunc(tasks, func(task codersdk.Task) bool {
						return task.Name == taskName
					}) == -1
				}, testutil.IntervalMedium)
			},
		},
	} {
		t.Logf("test case: %q", tc.name)
		var stdout strings.Builder
		inv, root := clitest.New(t, tc.cmdArgs...)
		inv.Stdout = &stdout
		clitest.SetupConfig(t, userClient, root)
		require.NoError(t, inv.WithContext(ctx).Run(), tc.name)
		if tc.assertFn != nil {
			tc.assertFn(stdout.String(), userClient)
		}
	}
}

func fakeAgentAPIEcho(ctx context.Context, t testing.TB, initMsg agentapisdk.Message, want ...string) map[string]http.HandlerFunc {
	t.Helper()
	var mmu sync.RWMutex
	msgs := []agentapisdk.Message{initMsg}
	wantCpy := make([]string, len(want))
	copy(wantCpy, want)
	t.Cleanup(func() {
		mmu.Lock()
		defer mmu.Unlock()
		if !t.Failed() {
			assert.Empty(t, wantCpy, "not all expected messages received: missing %v", wantCpy)
		}
	})
	writeAgentAPIError := func(w http.ResponseWriter, err error, status int) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(agentapisdk.ErrorModel{
			Errors: ptr.Ref([]agentapisdk.ErrorDetail{
				{
					Message: ptr.Ref(err.Error()),
				},
			}),
		})
	}
	return map[string]http.HandlerFunc{
		"/status": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(agentapisdk.GetStatusResponse{
				Status: "stable",
			})
		},
		"/messages": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			mmu.RLock()
			defer mmu.RUnlock()
			bs, err := json.Marshal(agentapisdk.GetMessagesResponse{
				Messages: msgs,
			})
			if err != nil {
				writeAgentAPIError(w, err, http.StatusBadRequest)
				return
			}
			_, _ = w.Write(bs)
		},
		"/message": func(w http.ResponseWriter, r *http.Request) {
			mmu.Lock()
			defer mmu.Unlock()
			var params agentapisdk.PostMessageParams
			w.Header().Set("Content-Type", "application/json")
			err := json.NewDecoder(r.Body).Decode(&params)
			if !assert.NoError(t, err, "decode message") {
				writeAgentAPIError(w, err, http.StatusBadRequest)
				return
			}

			if len(wantCpy) == 0 {
				assert.Fail(t, "unexpected message", "received message %v, but no more expected messages", params)
				writeAgentAPIError(w, xerrors.New("no more expected messages"), http.StatusBadRequest)
				return
			}
			exp := wantCpy[0]
			wantCpy = wantCpy[1:]

			if !assert.Equal(t, exp, params.Content, "message content mismatch") {
				writeAgentAPIError(w, xerrors.New("unexpected message content: expected "+exp+", got "+params.Content), http.StatusBadRequest)
				return
			}

			msgs = append(msgs, agentapisdk.Message{
				Id:      int64(len(msgs) + 1),
				Content: params.Content,
				Role:    agentapisdk.RoleUser,
				Time:    time.Now().UTC(),
			})
			msgs = append(msgs, agentapisdk.Message{
				Id:      int64(len(msgs) + 1),
				Content: params.Content,
				Role:    agentapisdk.RoleAgent,
				Time:    time.Now().UTC(),
			})
			assert.NoError(t, json.NewEncoder(w).Encode(agentapisdk.PostMessageResponse{
				Ok: true,
			}))
		},
	}
}

// setupCLITaskTest creates a test workspace with an AI task template and agent,
// with a fake agent API configured with the provided set of handlers.
// Returns the user client and workspace.
func setupCLITaskTest(ctx context.Context, t *testing.T, agentAPIHandlers map[string]http.HandlerFunc) (*codersdk.Client, codersdk.Task) {
	t.Helper()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	owner := coderdtest.CreateFirstUser(t, client)
	userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	fakeAPI := startFakeAgentAPI(t, agentAPIHandlers)

	authToken := uuid.NewString()
	template := createAITaskTemplate(t, client, owner.OrganizationID, withSidebarURL(fakeAPI.URL()), withAgentToken(authToken))

	wantPrompt := "test prompt"
	task, err := userClient.CreateTask(ctx, codersdk.Me, codersdk.CreateTaskRequest{
		TemplateVersionID: template.ActiveVersionID,
		Input:             wantPrompt,
		Name:              "test-task",
	})
	require.NoError(t, err)

	// Wait for the task's underlying workspace to be built
	require.True(t, task.WorkspaceID.Valid, "task should have a workspace ID")
	workspace, err := userClient.Workspace(ctx, task.WorkspaceID.UUID)
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))
	_ = agenttest.New(t, client.URL, authToken, func(o *agent.Options) {
		o.Client = agentClient
	})

	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).
		WaitFor(coderdtest.AgentsReady)

	return userClient, task
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
								AppId: taskAppID.String(),
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
