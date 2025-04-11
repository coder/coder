package toolsdk_test

import (
	"context"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithAgentClient(ctx, agentClient)
		ctx = toolsdk.WithWorkspaceAppStatusSlug(ctx, "some-agent-app")
		_, err := testTool(ctx, t, toolsdk.ReportTask, map[string]any{
			"summary": "test summary",
			"state":   "complete",
			"link":    "https://example.com",
			"emoji":   "✅",
		})
		require.NoError(t, err)
	})

	t.Run("ListTemplates", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, memberClient)

		// Get the templates directly for comparison
		expected, err := memberClient.Templates(context.Background(), codersdk.TemplateFilter{})
		require.NoError(t, err)

		result, err := testTool(ctx, t, toolsdk.ListTemplates, map[string]any{})

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
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, memberClient)

		result, err := testTool(ctx, t, toolsdk.GetAuthenticatedUser, map[string]any{})

		require.NoError(t, err)
		require.Equal(t, member.ID, result.ID)
		require.Equal(t, member.Username, result.Username)
	})

	t.Run("ListWorkspaces", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, memberClient)

		result, err := testTool(ctx, t, toolsdk.ListWorkspaces, map[string]any{
			"owner": "me",
		})

		require.NoError(t, err)
		require.Len(t, result, 1, "expected 1 workspace")
		workspace := result[0]
		require.Equal(t, r.Workspace.ID.String(), workspace.ID, "expected the workspace to match the one we created")
	})

	t.Run("GetWorkspace", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, memberClient)

		result, err := testTool(ctx, t, toolsdk.GetWorkspace, map[string]any{
			"workspace_id": r.Workspace.ID.String(),
		})

		require.NoError(t, err)
		require.Equal(t, r.Workspace.ID, result.ID, "expected the workspace ID to match")
	})

	t.Run("CreateWorkspaceBuild", func(t *testing.T) {
		t.Run("Stop", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)
			ctx = toolsdk.WithClient(ctx, memberClient)

			result, err := testTool(ctx, t, toolsdk.CreateWorkspaceBuild, map[string]any{
				"workspace_id": r.Workspace.ID.String(),
				"transition":   "stop",
			})

			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStop, result.Transition)
			require.Equal(t, r.Workspace.ID, result.WorkspaceID)

			// Important: cancel the build. We don't run any provisioners, so this
			// will remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, result.ID))
		})

		t.Run("Start", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)
			ctx = toolsdk.WithClient(ctx, memberClient)

			result, err := testTool(ctx, t, toolsdk.CreateWorkspaceBuild, map[string]any{
				"workspace_id": r.Workspace.ID.String(),
				"transition":   "start",
			})

			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStart, result.Transition)
			require.Equal(t, r.Workspace.ID, result.WorkspaceID)

			// Important: cancel the build. We don't run any provisioners, so this
			// will remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, result.ID))
		})
	})

	t.Run("ListTemplateVersionParameters", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, memberClient)

		params, err := testTool(ctx, t, toolsdk.ListTemplateVersionParameters, map[string]any{
			"template_version_id": r.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
		require.Empty(t, params)
	})

	t.Run("GetWorkspaceAgentLogs", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, client)

		logs, err := testTool(ctx, t, toolsdk.GetWorkspaceAgentLogs, map[string]any{
			"workspace_agent_id": agentID.String(),
		})

		require.NoError(t, err)
		require.NotEmpty(t, logs)
	})

	t.Run("GetWorkspaceBuildLogs", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, memberClient)

		logs, err := testTool(ctx, t, toolsdk.GetWorkspaceBuildLogs, map[string]any{
			"workspace_build_id": r.Build.ID.String(),
		})

		require.NoError(t, err)
		_ = logs // The build may not have any logs yet, so we just check that the function returns successfully
	})

	t.Run("GetTemplateVersionLogs", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, memberClient)

		logs, err := testTool(ctx, t, toolsdk.GetTemplateVersionLogs, map[string]any{
			"template_version_id": r.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
		_ = logs // Just ensuring the call succeeds
	})

	t.Run("UpdateTemplateActiveVersion", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, client) // Use owner client for permission

		result, err := testTool(ctx, t, toolsdk.UpdateTemplateActiveVersion, map[string]any{
			"template_id":         r.Template.ID.String(),
			"template_version_id": r.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
		require.Contains(t, result, "Successfully updated")
	})

	t.Run("DeleteTemplate", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, client)

		_, err := testTool(ctx, t, toolsdk.DeleteTemplate, map[string]any{
			"template_id": r.Template.ID.String(),
		})

		// This will fail with because there already exists a workspace.
		require.ErrorContains(t, err, "All workspaces must be deleted before a template can be removed")
	})

	t.Run("UploadTarFile", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, client)

		files := map[string]any{
			"main.tf": "resource \"null_resource\" \"example\" {}",
		}

		result, err := testTool(ctx, t, toolsdk.UploadTarFile, map[string]any{
			"mime_type": string(codersdk.ContentTypeTar),
			"files":     files,
		})

		require.NoError(t, err)
		require.NotEmpty(t, result.ID)
	})

	t.Run("CreateTemplateVersion", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, client)

		// nolint:gocritic // This is in a test package and does not end up in the build
		file := dbgen.File(t, store, database.File{})

		tv, err := testTool(ctx, t, toolsdk.CreateTemplateVersion, map[string]any{
			"file_id": file.ID.String(),
		})
		require.NoError(t, err)
		require.NotEmpty(t, tv)
	})

	t.Run("CreateTemplate", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, client)

		// Create a new template version for use here.
		tv := dbfake.TemplateVersion(t, store).SkipCreateTemplate().Do()

		// We're going to re-use the pre-existing template version
		_, err := testTool(ctx, t, toolsdk.CreateTemplate, map[string]any{
			"name":         testutil.GetRandomNameHyphenated(t),
			"display_name": "Test Template",
			"description":  "This is a test template",
			"version_id":   tv.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
	})

	t.Run("CreateWorkspace", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		ctx = toolsdk.WithClient(ctx, memberClient)

		// We need a template version ID to create a workspace
		res, err := testTool(ctx, t, toolsdk.CreateWorkspace, map[string]any{
			"user":                "me",
			"template_version_id": r.TemplateVersion.ID.String(),
			"name":                testutil.GetRandomNameHyphenated(t),
			"rich_parameters":     map[string]any{},
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
func testTool[T any](ctx context.Context, t *testing.T, tool toolsdk.Tool[T], args map[string]any) (T, error) {
	t.Helper()
	testedTools.Store(tool.Tool.Name, struct{}{})
	result, err := tool.Handler(ctx, args)
	return result, err
}

// TestMain runs after all tests to ensure that all tools in this package have
// been tested once.
func TestMain(m *testing.M) {
	code := m.Run()
	var untested []string
	for _, tool := range toolsdk.All {
		if _, ok := testedTools.Load(tool.Tool.Name); !ok {
			untested = append(untested, tool.Tool.Name)
		}
	}

	if len(untested) > 0 {
		println("The following tools were not tested:")
		for _, tool := range untested {
			println(" - " + tool)
		}
		println("Please ensure that all tools are tested using testTool().")
		println("If you just added a new tool, please add a test for it.")
		println("NOTE: if you just ran an individual test, this is expected.")
		os.Exit(1)
	}

	os.Exit(code)
}
