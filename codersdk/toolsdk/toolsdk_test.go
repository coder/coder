package toolsdk_test

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kylecarbs/aisdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// These tests are dependent on the state of the coder server.
// Running them in parallel is prone to racy behavior.
// nolint:tparallel,paralleltest
func TestTools(t *testing.T) {
	// Given: a running coderd instance
	setupCtx := testutil.Context(t, testutil.WaitShort)
	client, store := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	// Given: a member user with which to test the tools.
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	// Given: a workspace with an agent.
	// nolint:gocritic // This is in a test package and does not end up in the build
	r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        member.ID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Apps = []*proto.App{
			{
				Slug: "some-agent-app",
			},
		}
		return agents
	}).Do()

	// Given: a client configured with the agent token.
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(r.AgentToken)
	// Get the agent ID from the API. Overriding it in dbfake doesn't work.
	ws, err := client.Workspace(setupCtx, r.Workspace.ID)
	require.NoError(t, err)
	require.NotEmpty(t, ws.LatestBuild.Resources)
	require.NotEmpty(t, ws.LatestBuild.Resources[0].Agents)
	agentID := ws.LatestBuild.Resources[0].Agents[0].ID

	// Given: the workspace agent has written logs.
	agentClient.PatchLogs(setupCtx, agentsdk.PatchLogs{
		Logs: []agentsdk.Log{
			{
				CreatedAt: time.Now(),
				Level:     codersdk.LogLevelInfo,
				Output:    "test log message",
			},
		},
	})

	t.Run("ReportTask", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient, toolsdk.WithAgentClient(agentClient), toolsdk.WithAppStatusSlug("some-agent-app"))
		require.NoError(t, err)
		_, err = testTool(t, toolsdk.ReportTask, tb, toolsdk.ReportTaskArgs{
			Summary: "test summary",
			State:   "complete",
			Link:    "https://example.com",
		})
		require.NoError(t, err)
	})

	t.Run("GetWorkspace", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.GetWorkspace, tb, toolsdk.GetWorkspaceArgs{
			WorkspaceID: r.Workspace.ID.String(),
		})

		require.NoError(t, err)
		require.Equal(t, r.Workspace.ID, result.ID, "expected the workspace ID to match")
	})

	t.Run("ListTemplates", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		// Get the templates directly for comparison
		expected, err := memberClient.Templates(context.Background(), codersdk.TemplateFilter{})
		require.NoError(t, err)

		result, err := testTool(t, toolsdk.ListTemplates, tb, toolsdk.NoArgs{})

		require.NoError(t, err)
		require.Len(t, result, len(expected))

		// Sort the results by name to ensure the order is consistent
		sort.Slice(expected, func(a, b int) bool {
			return expected[a].Name < expected[b].Name
		})
		sort.Slice(result, func(a, b int) bool {
			return result[a].Name < result[b].Name
		})
		for i, template := range result {
			require.Equal(t, expected[i].ID.String(), template.ID)
		}
	})

	t.Run("Whoami", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.GetAuthenticatedUser, tb, toolsdk.NoArgs{})

		require.NoError(t, err)
		require.Equal(t, member.ID, result.ID)
		require.Equal(t, member.Username, result.Username)
	})

	t.Run("ListWorkspaces", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.ListWorkspaces, tb, toolsdk.ListWorkspacesArgs{})

		require.NoError(t, err)
		require.Len(t, result, 1, "expected 1 workspace")
		workspace := result[0]
		require.Equal(t, r.Workspace.ID.String(), workspace.ID, "expected the workspace to match the one we created")
	})

	t.Run("CreateWorkspaceBuild", func(t *testing.T) {
		t.Run("Stop", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)
			tb, err := toolsdk.NewDeps(memberClient)
			require.NoError(t, err)
			result, err := testTool(t, toolsdk.CreateWorkspaceBuild, tb, toolsdk.CreateWorkspaceBuildArgs{
				WorkspaceID: r.Workspace.ID.String(),
				Transition:  "stop",
			})

			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStop, result.Transition)
			require.Equal(t, r.Workspace.ID, result.WorkspaceID)
			require.Equal(t, r.TemplateVersion.ID, result.TemplateVersionID)
			require.Equal(t, codersdk.WorkspaceTransitionStop, result.Transition)

			// Important: cancel the build. We don't run any provisioners, so this
			// will remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, result.ID))
		})

		t.Run("Start", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)
			tb, err := toolsdk.NewDeps(memberClient)
			require.NoError(t, err)
			result, err := testTool(t, toolsdk.CreateWorkspaceBuild, tb, toolsdk.CreateWorkspaceBuildArgs{
				WorkspaceID: r.Workspace.ID.String(),
				Transition:  "start",
			})

			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStart, result.Transition)
			require.Equal(t, r.Workspace.ID, result.WorkspaceID)
			require.Equal(t, r.TemplateVersion.ID, result.TemplateVersionID)
			require.Equal(t, codersdk.WorkspaceTransitionStart, result.Transition)

			// Important: cancel the build. We don't run any provisioners, so this
			// will remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, result.ID))
		})

		t.Run("TemplateVersionChange", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)
			tb, err := toolsdk.NewDeps(memberClient)
			require.NoError(t, err)
			// Get the current template version ID before updating
			workspace, err := memberClient.Workspace(ctx, r.Workspace.ID)
			require.NoError(t, err)
			originalVersionID := workspace.LatestBuild.TemplateVersionID

			// Create a new template version to update to
			newVersion := dbfake.TemplateVersion(t, store).
				// nolint:gocritic // This is in a test package and does not end up in the build
				Seed(database.TemplateVersion{
					OrganizationID: owner.OrganizationID,
					CreatedBy:      owner.UserID,
					TemplateID:     uuid.NullUUID{UUID: r.Template.ID, Valid: true},
				}).Do()

			// Update to new version
			updateBuild, err := testTool(t, toolsdk.CreateWorkspaceBuild, tb, toolsdk.CreateWorkspaceBuildArgs{
				WorkspaceID:       r.Workspace.ID.String(),
				Transition:        "start",
				TemplateVersionID: newVersion.TemplateVersion.ID.String(),
			})
			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStart, updateBuild.Transition)
			require.Equal(t, r.Workspace.ID.String(), updateBuild.WorkspaceID.String())
			require.Equal(t, newVersion.TemplateVersion.ID.String(), updateBuild.TemplateVersionID.String())
			// Cancel the build so it doesn't remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, updateBuild.ID))

			// Roll back to the original version
			rollbackBuild, err := testTool(t, toolsdk.CreateWorkspaceBuild, tb, toolsdk.CreateWorkspaceBuildArgs{
				WorkspaceID:       r.Workspace.ID.String(),
				Transition:        "start",
				TemplateVersionID: originalVersionID.String(),
			})
			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStart, rollbackBuild.Transition)
			require.Equal(t, r.Workspace.ID.String(), rollbackBuild.WorkspaceID.String())
			require.Equal(t, originalVersionID.String(), rollbackBuild.TemplateVersionID.String())
			// Cancel the build so it doesn't remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, rollbackBuild.ID))
		})
	})

	t.Run("ListTemplateVersionParameters", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		params, err := testTool(t, toolsdk.ListTemplateVersionParameters, tb, toolsdk.ListTemplateVersionParametersArgs{
			TemplateVersionID: r.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
		require.Empty(t, params)
	})

	t.Run("GetWorkspaceAgentLogs", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		logs, err := testTool(t, toolsdk.GetWorkspaceAgentLogs, tb, toolsdk.GetWorkspaceAgentLogsArgs{
			WorkspaceAgentID: agentID.String(),
		})

		require.NoError(t, err)
		require.NotEmpty(t, logs)
	})

	t.Run("GetWorkspaceBuildLogs", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		logs, err := testTool(t, toolsdk.GetWorkspaceBuildLogs, tb, toolsdk.GetWorkspaceBuildLogsArgs{
			WorkspaceBuildID: r.Build.ID.String(),
		})

		require.NoError(t, err)
		_ = logs // The build may not have any logs yet, so we just check that the function returns successfully
	})

	t.Run("GetTemplateVersionLogs", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		logs, err := testTool(t, toolsdk.GetTemplateVersionLogs, tb, toolsdk.GetTemplateVersionLogsArgs{
			TemplateVersionID: r.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
		_ = logs // Just ensuring the call succeeds
	})

	t.Run("UpdateTemplateActiveVersion", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.UpdateTemplateActiveVersion, tb, toolsdk.UpdateTemplateActiveVersionArgs{
			TemplateID:        r.Template.ID.String(),
			TemplateVersionID: r.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
		require.Contains(t, result, "Successfully updated")
	})

	t.Run("DeleteTemplate", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		_, err = testTool(t, toolsdk.DeleteTemplate, tb, toolsdk.DeleteTemplateArgs{
			TemplateID: r.Template.ID.String(),
		})

		// This will fail with because there already exists a workspace.
		require.ErrorContains(t, err, "All workspaces must be deleted before a template can be removed")
	})

	t.Run("UploadTarFile", func(t *testing.T) {
		files := map[string]string{
			"main.tf": `resource "null_resource" "example" {}`,
		}
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)

		result, err := testTool(t, toolsdk.UploadTarFile, tb, toolsdk.UploadTarFileArgs{
			Files: files,
		})

		require.NoError(t, err)
		require.NotEmpty(t, result.ID)
	})

	t.Run("CreateTemplateVersion", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		// nolint:gocritic // This is in a test package and does not end up in the build
		file := dbgen.File(t, store, database.File{})
		t.Run("WithoutTemplateID", func(t *testing.T) {
			tv, err := testTool(t, toolsdk.CreateTemplateVersion, tb, toolsdk.CreateTemplateVersionArgs{
				FileID: file.ID.String(),
			})
			require.NoError(t, err)
			require.NotEmpty(t, tv)
		})
		t.Run("WithTemplateID", func(t *testing.T) {
			tv, err := testTool(t, toolsdk.CreateTemplateVersion, tb, toolsdk.CreateTemplateVersionArgs{
				FileID:     file.ID.String(),
				TemplateID: r.Template.ID.String(),
			})
			require.NoError(t, err)
			require.NotEmpty(t, tv)
		})
	})

	t.Run("CreateTemplate", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		// Create a new template version for use here.
		tv := dbfake.TemplateVersion(t, store).
			// nolint:gocritic // This is in a test package and does not end up in the build
			Seed(database.TemplateVersion{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID}).
			SkipCreateTemplate().Do()

		// We're going to re-use the pre-existing template version
		_, err = testTool(t, toolsdk.CreateTemplate, tb, toolsdk.CreateTemplateArgs{
			Name:        testutil.GetRandomNameHyphenated(t),
			DisplayName: "Test Template",
			Description: "This is a test template",
			VersionID:   tv.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
	})

	t.Run("CreateWorkspace", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		// We need a template version ID to create a workspace
		res, err := testTool(t, toolsdk.CreateWorkspace, tb, toolsdk.CreateWorkspaceArgs{
			User:              "me",
			TemplateVersionID: r.TemplateVersion.ID.String(),
			Name:              testutil.GetRandomNameHyphenated(t),
			RichParameters:    map[string]string{},
		})

		// The creation might fail for various reasons, but the important thing is
		// to mark it as tested
		require.NoError(t, err)
		require.NotEmpty(t, res.ID, "expected a workspace ID")
	})
}

// TestedTools keeps track of which tools have been tested.
var testedTools sync.Map

// testTool is a helper function to test a tool and mark it as tested.
// Note that we test the _generic_ version of the tool and not the typed one.
// This is to mimic how we expect external callers to use the tool.
func testTool[Arg, Ret any](t *testing.T, tool toolsdk.Tool[Arg, Ret], tb toolsdk.Deps, args Arg) (Ret, error) {
	t.Helper()
	defer func() { testedTools.Store(tool.Tool.Name, true) }()
	toolArgs, err := json.Marshal(args)
	require.NoError(t, err, "failed to marshal args")
	result, err := tool.Generic().Handler(context.Background(), tb, toolArgs)
	var ret Ret
	require.NoError(t, json.Unmarshal(result, &ret), "failed to unmarshal result %q", string(result))
	return ret, err
}

func TestWithRecovery(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		fakeTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "echo",
				Description: "Echoes the input.",
			},
			Handler: func(ctx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				return args, nil
			},
		}

		wrapped := toolsdk.WithRecover(fakeTool.Handler)
		v, err := wrapped(context.Background(), toolsdk.Deps{}, []byte(`{}`))
		require.NoError(t, err)
		require.JSONEq(t, `{}`, string(v))
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		fakeTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "fake_tool",
				Description: "Returns an error for testing.",
			},
			Handler: func(ctx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				return nil, assert.AnError
			},
		}
		wrapped := toolsdk.WithRecover(fakeTool.Handler)
		v, err := wrapped(context.Background(), toolsdk.Deps{}, []byte(`{}`))
		require.Nil(t, v)
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("Panic", func(t *testing.T) {
		t.Parallel()
		panicTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "panic_tool",
				Description: "Panics for testing.",
			},
			Handler: func(ctx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				panic("you can't sweat this fever out")
			},
		}

		wrapped := toolsdk.WithRecover(panicTool.Handler)
		v, err := wrapped(context.Background(), toolsdk.Deps{}, []byte("disco"))
		require.Empty(t, v)
		require.ErrorContains(t, err, "you can't sweat this fever out")
	})
}

type testContextKey struct{}

func TestWithCleanContext(t *testing.T) {
	t.Parallel()

	t.Run("NoContextKeys", func(t *testing.T) {
		t.Parallel()

		// This test is to ensure that the context values are not set in the
		// toolsdk package.
		ctxTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "context_tool",
				Description: "Returns the context value for testing.",
			},
			Handler: func(toolCtx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				v := toolCtx.Value(testContextKey{})
				assert.Nil(t, v, "expected the context value to be nil")
				return nil, nil
			},
		}

		wrapped := toolsdk.WithCleanContext(ctxTool.Handler)
		ctx := context.WithValue(context.Background(), testContextKey{}, "test")
		_, _ = wrapped(ctx, toolsdk.Deps{}, []byte(`{}`))
	})

	t.Run("PropagateCancel", func(t *testing.T) {
		t.Parallel()

		// This test is to ensure that the context is canceled properly.
		callCh := make(chan struct{})
		ctxTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "context_tool",
				Description: "Returns the context value for testing.",
			},
			Handler: func(toolCtx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				defer close(callCh)
				// Wait for the context to be canceled
				<-toolCtx.Done()
				return nil, toolCtx.Err()
			},
		}
		wrapped := toolsdk.WithCleanContext(ctxTool.Handler)
		errCh := make(chan error, 1)

		tCtx := testutil.Context(t, testutil.WaitShort)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go func() {
			_, err := wrapped(ctx, toolsdk.Deps{}, []byte(`{}`))
			errCh <- err
		}()

		cancel()

		// Ensure the tool is called
		select {
		case <-callCh:
		case <-tCtx.Done():
			require.Fail(t, "test timed out before handler was called")
		}

		// Ensure the correct error is returned
		select {
		case <-tCtx.Done():
			require.Fail(t, "test timed out")
		case err := <-errCh:
			// Context was canceled and the done channel was closed
			require.ErrorIs(t, err, context.Canceled)
		}
	})

	t.Run("PropagateDeadline", func(t *testing.T) {
		t.Parallel()

		// This test ensures that the context deadline is propagated to the child
		// from the parent.
		ctxTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "context_tool_deadline",
				Description: "Checks if context has deadline.",
			},
			Handler: func(toolCtx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				_, ok := toolCtx.Deadline()
				assert.True(t, ok, "expected deadline to be set on the child context")
				return nil, nil
			},
		}

		wrapped := toolsdk.WithCleanContext(ctxTool.Handler)
		parent, cancel := context.WithTimeout(context.Background(), testutil.IntervalFast)
		t.Cleanup(cancel)
		_, err := wrapped(parent, toolsdk.Deps{}, []byte(`{}`))
		require.NoError(t, err)
	})
}

func TestToolSchemaFields(t *testing.T) {
	t.Parallel()

	// Test that all tools have the required Schema fields (Properties and Required)
	for _, tool := range toolsdk.All {
		t.Run(tool.Tool.Name, func(t *testing.T) {
			t.Parallel()

			// Check that Properties is not nil
			require.NotNil(t, tool.Tool.Schema.Properties,
				"Tool %q missing Schema.Properties", tool.Tool.Name)

			// Check that Required is not nil
			require.NotNil(t, tool.Tool.Schema.Required,
				"Tool %q missing Schema.Required", tool.Tool.Name)

			// Ensure Properties has entries for all required fields
			for _, requiredField := range tool.Tool.Schema.Required {
				_, exists := tool.Tool.Schema.Properties[requiredField]
				require.True(t, exists,
					"Tool %q requires field %q but it is not defined in Properties",
					tool.Tool.Name, requiredField)
			}
		})
	}
}

// TestMain runs after all tests to ensure that all tools in this package have
// been tested once.
func TestMain(m *testing.M) {
	// Initialize testedTools
	for _, tool := range toolsdk.All {
		testedTools.Store(tool.Tool.Name, false)
	}

	code := m.Run()

	// Ensure all tools have been tested
	var untested []string
	for _, tool := range toolsdk.All {
		if tested, ok := testedTools.Load(tool.Tool.Name); !ok || !tested.(bool) {
			untested = append(untested, tool.Tool.Name)
		}
	}

	if len(untested) > 0 && code == 0 {
		code = 1
		println("The following tools were not tested:")
		for _, tool := range untested {
			println(" - " + tool)
		}
		println("Please ensure that all tools are tested using testTool().")
		println("If you just added a new tool, please add a test for it.")
		println("NOTE: if you just ran an individual test, this is expected.")
	}

	// Check for goroutine leaks. Below is adapted from goleak.VerifyTestMain:
	if code == 0 {
		if err := goleak.Find(testutil.GoleakOptions...); err != nil {
			println("goleak: Errors on successful test run: ", err.Error())
			code = 1
		}
	}

	os.Exit(code)
}
