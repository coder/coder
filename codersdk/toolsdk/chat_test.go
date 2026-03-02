package toolsdk_test

import (
	"context"
	"encoding/json"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// testedChatTools keeps track of which chat tools have been tested.
var testedChatTools sync.Map

// testChatTool is a helper function to test a chat tool and mark it
// as tested. Like testTool, it exercises the generic (JSON) version
// of the tool to mimic how external callers use it.
func testChatTool[Arg, Ret any](t *testing.T, tool toolsdk.Tool[Arg, Ret], deps toolsdk.Deps, args Arg) (Ret, error) {
	t.Helper()
	defer func() { testedChatTools.Store(tool.Name, true) }()
	toolArgs, err := json.Marshal(args)
	require.NoError(t, err, "failed to marshal args")
	result, err := tool.Generic().Handler(t.Context(), deps, toolArgs)
	var ret Ret
	require.NoError(t, json.Unmarshal(result, &ret), "failed to unmarshal result %q", string(result))
	return ret, err
}

func intPtr(v int) *int { return &v }

// setupChatAgentWorkspace creates a workspace with an agent and
// returns the client, workspace, and a connected agent conn. The
// caller can use the agent conn to interact with the workspace
// through the chat tools.
func setupChatAgentWorkspace(t *testing.T) (*codersdk.Client, database.WorkspaceTable, workspacesdk.AgentConn) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: chat agent tools rely on Unix-like shell semantics.")
	}

	client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
	_ = agenttest.New(t, client.URL, agentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

	ctx := testutil.Context(t, testutil.WaitShort)
	ws, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.NotEmpty(t, ws.LatestBuild.Resources)
	require.NotEmpty(t, ws.LatestBuild.Resources[0].Agents)
	agentID := ws.LatestBuild.Resources[0].Agents[0].ID

	wsClient := workspacesdk.New(client)
	agentConn, err := wsClient.DialAgent(ctx, agentID, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = agentConn.Close() })

	return client, workspace, agentConn
}

// TestChatListTemplates exercises the ChatListTemplates tool that
// provides paginated, filterable template listings for the chat UI.
func TestChatListTemplates(t *testing.T) {
	t.Parallel()

	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()

		client, store := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)

		// Create 15 templates so we can verify 10/page pagination.
		for i := 0; i < 15; i++ {
			dbfake.TemplateVersion(t, store).
				Seed(database.TemplateVersion{
					OrganizationID: owner.OrganizationID,
					CreatedBy:      owner.UserID,
				}).Do()
		}

		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		// Page 1 should have 10 templates.
		result, err := testChatTool(t, toolsdk.ChatListTemplates, deps, toolsdk.ChatListTemplatesArgs{
			Page: 1,
		})
		require.NoError(t, err)
		require.Len(t, result.Templates, 10)
		require.Equal(t, 1, result.Page)
		require.Equal(t, 2, result.TotalPages)
		require.Equal(t, 15, result.TotalCount)
		require.Equal(t, 10, result.Count)

		// Page 2 should have the remaining 5.
		result2, err := testChatTool(t, toolsdk.ChatListTemplates, deps, toolsdk.ChatListTemplatesArgs{
			Page: 2,
		})
		require.NoError(t, err)
		require.Len(t, result2.Templates, 5)
		require.Equal(t, 2, result2.Page)
	})

	t.Run("QueryFilter", func(t *testing.T) {
		t.Parallel()

		client, store := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)

		// Create two templates with distinct names.
		dbfake.TemplateVersion(t, store).
			Seed(database.TemplateVersion{
				OrganizationID: owner.OrganizationID,
				CreatedBy:      owner.UserID,
			}).Do()
		dbfake.TemplateVersion(t, store).
			Seed(database.TemplateVersion{
				OrganizationID: owner.OrganizationID,
				CreatedBy:      owner.UserID,
			}).Do()

		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		// Query with a filter that doesn't match anything.
		result, err := testChatTool(t, toolsdk.ChatListTemplates, deps, toolsdk.ChatListTemplatesArgs{
			Query: "nonexistent-template-name-xyz",
		})
		require.NoError(t, err)
		require.Empty(t, result.Templates)
		require.Equal(t, 0, result.Count)
	})

	t.Run("PopularitySorting", func(t *testing.T) {
		t.Parallel()

		client, store := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)

		// Create a couple of templates.
		dbfake.TemplateVersion(t, store).
			Seed(database.TemplateVersion{
				OrganizationID: owner.OrganizationID,
				CreatedBy:      owner.UserID,
			}).Do()

		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		result, err := testChatTool(t, toolsdk.ChatListTemplates, deps, toolsdk.ChatListTemplatesArgs{})
		require.NoError(t, err)
		require.NotEmpty(t, result.Templates)
		// Templates should be sorted by active_user_count (popularity).
		for i := 1; i < len(result.Templates); i++ {
			require.GreaterOrEqual(t,
				result.Templates[i-1].ActiveUserCount,
				result.Templates[i].ActiveUserCount,
				"templates should be sorted by popularity (descending)",
			)
		}
	})
}

// TestChatReadTemplate exercises the ChatReadTemplate tool that
// returns template details including rich parameters.
func TestChatReadTemplate(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)

			// Create a template with a rich parameter.
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete,
				ProvisionGraph: []*proto.Response{{
					Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
						Parameters: []*proto.RichParameter{
							{
								Name:         "region",
								DisplayName:  "Region",
								Description:  "The deployment region",
								Type:         "string",
								DefaultValue: "us-east-1",
								Required:     true,
							},
						},
					}},
				}},
				ProvisionApply: echo.ApplyComplete,
			})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

			deps, err := toolsdk.NewDeps(client)
			require.NoError(t, err)

			result, err := testChatTool(t, toolsdk.ChatReadTemplate, deps, toolsdk.ChatReadTemplateArgs{
				TemplateID: template.ID.String(),
			})
			require.NoError(t, err)
			require.Equal(t, template.ID.String(), result.ID)
			require.Equal(t, template.Name, result.Name)
			require.Equal(t, template.DisplayName, result.DisplayName)
			require.NotEmpty(t, result.Parameters, "expected at least one parameter")
			require.Equal(t, "region", result.Parameters[0].Name)	})

	t.Run("InvalidID", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.NewWithDatabase(t, nil)
		coderdtest.CreateFirstUser(t, client)

		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		_, err = testChatTool(t, toolsdk.ChatReadTemplate, deps, toolsdk.ChatReadTemplateArgs{
			TemplateID: "not-a-uuid",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid")
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.NewWithDatabase(t, nil)
		coderdtest.CreateFirstUser(t, client)

		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		_, err = testChatTool(t, toolsdk.ChatReadTemplate, deps, toolsdk.ChatReadTemplateArgs{
			TemplateID: uuid.NewString(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})
}

// TestChatCreateWorkspace exercises the ChatCreateWorkspace tool that
// creates a new workspace from a template.
func TestChatCreateWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		result, err := testChatTool(t, toolsdk.ChatCreateWorkspace, deps, toolsdk.ChatCreateWorkspaceArgs{
			TemplateID: template.ID.String(),
			Name:       testutil.GetRandomNameHyphenated(t),
		})
		require.NoError(t, err)
		require.True(t, result.Created, "expected workspace to be created")
		require.NotEmpty(t, result.WorkspaceName, "expected a workspace name")
	})

	t.Run("WithParameters", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		result, err := testChatTool(t, toolsdk.ChatCreateWorkspace, deps, toolsdk.ChatCreateWorkspaceArgs{
			TemplateID: template.ID.String(),
			Name:       testutil.GetRandomNameHyphenated(t),
			Parameters: map[string]string{"region": "us-west-2"},
		})
		require.NoError(t, err)
		require.True(t, result.Created)
	})

	t.Run("AutoGeneratedName", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		// Omit name to test auto-generation.
		result, err := testChatTool(t, toolsdk.ChatCreateWorkspace, deps, toolsdk.ChatCreateWorkspaceArgs{
			TemplateID: template.ID.String(),
		})
		require.NoError(t, err)
		require.True(t, result.Created)
		require.NotEmpty(t, result.WorkspaceName, "expected an auto-generated name")
	})
}

// TestChatReadFile exercises the ChatReadFile tool that reads files
// from a workspace via the agent connection.
func TestChatReadFile(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, _, agentConn := setupChatAgentWorkspace(t)
		deps, err := toolsdk.NewDeps(client, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return agentConn
		}))
		require.NoError(t, err)

		// Write a file via the agent so we have something to read.
		_, err = testChatTool(t, toolsdk.ChatWriteFile, deps, toolsdk.ChatWriteFileArgs{
			Path:    "/tmp/chat-read-test.txt",
			Content: "line one\nline two\nline three\n",
		})
		require.NoError(t, err)

		result, err := testChatTool(t, toolsdk.ChatReadFile, deps, toolsdk.ChatReadFileArgs{
			Path: "/tmp/chat-read-test.txt",
		})
		require.NoError(t, err)
		require.Contains(t, result.Content, "line one")
		require.Contains(t, result.Content, "line two")
			require.Equal(t, int64(4), result.TotalLines)
			require.Greater(t, result.FileSize, int64(0))	})

	t.Run("OffsetAndLimit", func(t *testing.T) {
		t.Parallel()

		client, _, agentConn := setupChatAgentWorkspace(t)
		deps, err := toolsdk.NewDeps(client, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return agentConn
		}))
		require.NoError(t, err)

		// Write a multi-line file.
		lines := "alpha\nbeta\ngamma\ndelta\nepsilon\n"
		_, err = testChatTool(t, toolsdk.ChatWriteFile, deps, toolsdk.ChatWriteFileArgs{
			Path:    "/tmp/chat-offset-test.txt",
			Content: lines,
		})
		require.NoError(t, err)

		offset := int64(2)
		limit := int64(2)
		result, err := testChatTool(t, toolsdk.ChatReadFile, deps, toolsdk.ChatReadFileArgs{
			Path:   "/tmp/chat-offset-test.txt",
			Offset: &offset,
			Limit:  &limit,
		})
		require.NoError(t, err)
		require.Equal(t, int64(2), result.LinesRead)
		require.Contains(t, result.Content, "beta")
		require.Contains(t, result.Content, "gamma")
		require.NotContains(t, result.Content, "alpha")
	})
}

// TestChatWriteFile exercises the ChatWriteFile tool that writes
// files to a workspace via the agent connection.
func TestChatWriteFile(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, _, agentConn := setupChatAgentWorkspace(t)
		deps, err := toolsdk.NewDeps(client, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return agentConn
		}))
		require.NoError(t, err)

		result, err := testChatTool(t, toolsdk.ChatWriteFile, deps, toolsdk.ChatWriteFileArgs{
			Path:    "/tmp/chat-write-test.txt",
			Content: "hello from chat",
		})
		require.NoError(t, err)
		require.True(t, result.OK)

		// Read it back to verify.
		readResult, err := testChatTool(t, toolsdk.ChatReadFile, deps, toolsdk.ChatReadFileArgs{
			Path: "/tmp/chat-write-test.txt",
		})
		require.NoError(t, err)
		require.Contains(t, readResult.Content, "hello from chat")
	})
}

// TestChatEditFiles exercises the ChatEditFiles tool that performs
// search-and-replace edits across files via the agent connection.
func TestChatEditFiles(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, _, agentConn := setupChatAgentWorkspace(t)
		deps, err := toolsdk.NewDeps(client, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return agentConn
		}))
		require.NoError(t, err)

		// Write a file to edit.
		_, err = testChatTool(t, toolsdk.ChatWriteFile, deps, toolsdk.ChatWriteFileArgs{
			Path:    "/tmp/chat-edit-test.txt",
			Content: "foo bar baz",
		})
		require.NoError(t, err)

		// Edit the file using search/replace.
		result, err := testChatTool(t, toolsdk.ChatEditFiles, deps, toolsdk.ChatEditFilesArgs{
			Files: []workspacesdk.FileEdits{
				{
					Path: "/tmp/chat-edit-test.txt",
					Edits: []workspacesdk.FileEdit{
						{Search: "foo", Replace: "qux"},
					},
				},
			},
		})
		require.NoError(t, err)
		require.True(t, result.OK)

		// Read back and verify.
		readResult, err := testChatTool(t, toolsdk.ChatReadFile, deps, toolsdk.ChatReadFileArgs{
			Path: "/tmp/chat-edit-test.txt",
		})
		require.NoError(t, err)
		require.Contains(t, readResult.Content, "qux bar baz")
		require.NotContains(t, readResult.Content, "foo")
	})
}

// TestChatExecute exercises the ChatExecute tool that runs commands
// in a workspace via the agent connection.
func TestChatExecute(t *testing.T) {
	t.Parallel()

	t.Run("BasicCommand", func(t *testing.T) {
		t.Parallel()

		client, _, agentConn := setupChatAgentWorkspace(t)
		deps, err := toolsdk.NewDeps(client, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return agentConn
		}))
		require.NoError(t, err)

		result, err := testChatTool(t, toolsdk.ChatExecute, deps, toolsdk.ChatExecuteArgs{
			Command: "echo hello",
		})
		require.NoError(t, err)
		require.True(t, result.Success)
		require.Contains(t, result.Output, "hello")
		require.Equal(t, 0, result.ExitCode)
		require.Greater(t, result.WallDurationMs, int64(0))
	})

	t.Run("BackgroundMode", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				require.True(t, req.Background)
				return workspacesdk.StartProcessResponse{ID: "bg-proc-123"}, nil
			},
		)

		deps, err := toolsdk.NewDeps(nil, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return mockConn
		}))
		require.NoError(t, err)

		bg := true
		result, err := testChatTool(t, toolsdk.ChatExecute, deps, toolsdk.ChatExecuteArgs{
			Command:         "sleep 120",
			RunInBackground: &bg,
		})
		require.NoError(t, err)
		require.Equal(t, "bg-proc-123", result.BackgroundProcessID)
	})

	t.Run("WithTimeout", func(t *testing.T) {
		t.Parallel()

		client, _, agentConn := setupChatAgentWorkspace(t)
		deps, err := toolsdk.NewDeps(client, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return agentConn
		}))
		require.NoError(t, err)

		timeout := "2s"
		result, err := testChatTool(t, toolsdk.ChatExecute, deps, toolsdk.ChatExecuteArgs{
			Command: `echo "fast"`,
			Timeout: &timeout,
		})
		require.NoError(t, err)
		require.True(t, result.Success)
		require.Contains(t, result.Output, "fast")
	})

	t.Run("WithWorkDir", func(t *testing.T) {
		t.Parallel()

		client, _, agentConn := setupChatAgentWorkspace(t)
		deps, err := toolsdk.NewDeps(client, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return agentConn
		}))
		require.NoError(t, err)

		workdir := "/tmp"
		result, err := testChatTool(t, toolsdk.ChatExecute, deps, toolsdk.ChatExecuteArgs{
			Command: "pwd",
			WorkDir: &workdir,
		})
		require.NoError(t, err)
		require.True(t, result.Success)
		require.Contains(t, result.Output, "/tmp")
	})
}

// TestChatProcessOutput exercises the ChatProcessOutput tool that
// retrieves output from a background process by ID.
func TestChatProcessOutput(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).Return(
			workspacesdk.StartProcessResponse{ID: "proc-out-123"}, nil,
		)
		mockConn.EXPECT().ProcessOutput(gomock.Any(), "proc-out-123").Return(
			workspacesdk.ProcessOutputResponse{
				Output:   "background output\n",
				Running:  false,
				ExitCode: intPtr(0),
			}, nil,
		)

		deps, err := toolsdk.NewDeps(nil, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return mockConn
		}))
		require.NoError(t, err)

		// Start a background process.
		bg := true
		execResult, err := testChatTool(t, toolsdk.ChatExecute, deps, toolsdk.ChatExecuteArgs{
			Command:         `echo "background output" && sleep 120`,
			RunInBackground: &bg,
		})
		require.NoError(t, err)
		require.Equal(t, "proc-out-123", execResult.BackgroundProcessID)

		// Retrieve the output by process ID.
		result, err := testChatTool(t, toolsdk.ChatProcessOutput, deps, toolsdk.ChatProcessOutputArgs{
			ProcessID: execResult.BackgroundProcessID,
		})
		require.NoError(t, err)
		require.Contains(t, result.Output, "background output")
	})
}

// TestChatProcessList exercises the ChatProcessList tool that lists
// tracked processes on the workspace agent.
func TestChatProcessList(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).Return(
			workspacesdk.StartProcessResponse{ID: "proc-list-123"}, nil,
		)
		mockConn.EXPECT().ListProcesses(gomock.Any()).Return(
			workspacesdk.ListProcessesResponse{
				Processes: []workspacesdk.ProcessInfo{
					{ID: "proc-list-123", Command: "sleep 300", Running: true},
				},
			}, nil,
		)

		deps, err := toolsdk.NewDeps(nil, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return mockConn
		}))
		require.NoError(t, err)

		// Start a background process so we have something to list.
		bg := true
		execResult, err := testChatTool(t, toolsdk.ChatExecute, deps, toolsdk.ChatExecuteArgs{
			Command:         "sleep 300",
			RunInBackground: &bg,
		})
		require.NoError(t, err)
		require.Equal(t, "proc-list-123", execResult.BackgroundProcessID)

		// List processes and verify ours appears.
		result, err := testChatTool(t, toolsdk.ChatProcessList, deps, toolsdk.ChatProcessListArgs{})
		require.NoError(t, err)
		require.NotEmpty(t, result.Processes)

		found := false
		for _, proc := range result.Processes {
			if proc.ID == execResult.BackgroundProcessID {
				found = true
				require.True(t, proc.Running, "background process should still be running")
				break
			}
		}
		require.True(t, found, "expected background process %s in process list", execResult.BackgroundProcessID)
	})
}

// TestChatProcessSignal exercises the ChatProcessSignal tool that
// sends signals to tracked processes on the workspace agent.
func TestChatProcessSignal(t *testing.T) {
	t.Parallel()

	t.Run("Terminate", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).Return(
			workspacesdk.StartProcessResponse{ID: "proc-sig-123"}, nil,
		)
		mockConn.EXPECT().SignalProcess(gomock.Any(), "proc-sig-123", "terminate").Return(nil)

		deps, err := toolsdk.NewDeps(nil, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return mockConn
		}))
		require.NoError(t, err)

		// Start a background process to signal.
		bg := true
		execResult, err := testChatTool(t, toolsdk.ChatExecute, deps, toolsdk.ChatExecuteArgs{
			Command:         "sleep 600",
			RunInBackground: &bg,
		})
		require.NoError(t, err)
		require.Equal(t, "proc-sig-123", execResult.BackgroundProcessID)

		// Terminate the process.
		result, err := testChatTool(t, toolsdk.ChatProcessSignal, deps, toolsdk.ChatProcessSignalArgs{
			ProcessID: execResult.BackgroundProcessID,
			Signal:    "terminate",
		})
		require.NoError(t, err)
		require.True(t, result.Success)
		require.NotEmpty(t, result.Message)
	})

	t.Run("InvalidSignal", func(t *testing.T) {
		t.Parallel()

		// Signal validation happens before the conn is used, so
		// no mock expectations for SignalProcess are needed.
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).Return(
			workspacesdk.StartProcessResponse{ID: "proc-sig-invalid"}, nil,
		)

		deps, err := toolsdk.NewDeps(nil, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
			return mockConn
		}))
		require.NoError(t, err)

		// Start a background process to attempt signaling.
		bg := true
		execResult, err := testChatTool(t, toolsdk.ChatExecute, deps, toolsdk.ChatExecuteArgs{
			Command:         "sleep 600",
			RunInBackground: &bg,
		})
		require.NoError(t, err)

		_, err = testChatTool(t, toolsdk.ChatProcessSignal, deps, toolsdk.ChatProcessSignalArgs{
			ProcessID: execResult.BackgroundProcessID,
			Signal:    "not-a-real-signal",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid signal")
	})
}
