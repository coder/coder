package mcptools_test

import (
	"context"
	"encoding/json"
	"io"
	"runtime"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	mcptools "github.com/coder/coder/v2/mcp/tools"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

// These tests are dependent on the state of the coder server.
// Running them in parallel is prone to racy behavior.
// nolint:tparallel,paralleltest
func TestCoderTools(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux due to pty issues")
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	// Given: a coder server, workspace, and agent.
	client, store := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	// Given: a member user with which to test the tools.
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	// Given: a workspace with an agent.
	r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        member.ID,
	}).WithAgent().Do()

	// Note: we want to test the list_workspaces tool before starting the
	// workspace agent. Starting the workspace agent will modify the workspace
	// state, which will affect the results of the list_workspaces tool.
	listWorkspacesDone := make(chan struct{})
	agentStarted := make(chan struct{})
	go func() {
		defer close(agentStarted)
		<-listWorkspacesDone
		agt := agenttest.New(t, client.URL, r.AgentToken)
		t.Cleanup(func() {
			_ = agt.Close()
		})
		_ = coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).Wait()
	}()

	// Given: a MCP server listening on a pty.
	pty := ptytest.New(t)
	mcpSrv, closeSrv := startTestMCPServer(ctx, t, pty.Input(), pty.Output())
	t.Cleanup(func() {
		_ = closeSrv()
	})

	// Register tools using our registry
	logger := slogtest.Make(t, nil)
	mcptools.AllTools().Register(mcpSrv, mcptools.ToolDeps{
		Client: memberClient,
		Logger: &logger,
	})

	t.Run("coder_list_templates", func(t *testing.T) {
		// When: the coder_list_templates tool is called
		ctr := makeJSONRPCRequest(t, "tools/call", "coder_list_templates", map[string]any{})

		pty.WriteLine(ctr)
		_ = pty.ReadLine(ctx) // skip the echo

		templates, err := memberClient.Templates(ctx, codersdk.TemplateFilter{})
		require.NoError(t, err)
		templatesJSON, err := json.Marshal(templates)
		require.NoError(t, err)

		// Then: the response is a list of templates visible to the user.
		expected := makeJSONRPCTextResponse(t, string(templatesJSON))
		actual := pty.ReadLine(ctx)
		testutil.RequireJSONEq(t, expected, actual)
	})

	t.Run("coder_report_task", func(t *testing.T) {
		// When: the coder_report_task tool is called
		ctr := makeJSONRPCRequest(t, "tools/call", "coder_report_task", map[string]any{
			"summary":             "Test summary",
			"link":                "https://example.com",
			"emoji":               "🔍",
			"done":                false,
			"coder_url":           client.URL.String(),
			"coder_session_token": client.SessionToken(),
		})

		pty.WriteLine(ctr)
		_ = pty.ReadLine(ctx) // skip the echo

		// Then: the response is a success message.
		// TODO: check the task was created. This functionality is not yet implemented.
		expected := makeJSONRPCTextResponse(t, "Thanks for reporting!")
		actual := pty.ReadLine(ctx)
		testutil.RequireJSONEq(t, expected, actual)
	})

	t.Run("coder_whoami", func(t *testing.T) {
		// When: the coder_whoami tool is called
		me, err := memberClient.User(ctx, codersdk.Me)
		require.NoError(t, err)
		meJSON, err := json.Marshal(me)
		require.NoError(t, err)

		ctr := makeJSONRPCRequest(t, "tools/call", "coder_whoami", map[string]any{})

		pty.WriteLine(ctr)
		_ = pty.ReadLine(ctx) // skip the echo

		// Then: the response is a valid JSON respresentation of the calling user.
		expected := makeJSONRPCTextResponse(t, string(meJSON))
		actual := pty.ReadLine(ctx)
		testutil.RequireJSONEq(t, expected, actual)
	})

	t.Run("coder_list_workspaces", func(t *testing.T) {
		defer close(listWorkspacesDone)
		// When: the coder_list_workspaces tool is called
		ctr := makeJSONRPCRequest(t, "tools/call", "coder_list_workspaces", map[string]any{
			"coder_url":           client.URL.String(),
			"coder_session_token": client.SessionToken(),
		})

		pty.WriteLine(ctr)
		_ = pty.ReadLine(ctx) // skip the echo

		ws, err := memberClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		wsJSON, err := json.Marshal(ws)
		require.NoError(t, err)

		// Then: the response is a valid JSON respresentation of the calling user's workspaces.
		expected := makeJSONRPCTextResponse(t, string(wsJSON))
		actual := pty.ReadLine(ctx)
		testutil.RequireJSONEq(t, expected, actual)
	})

	t.Run("coder_get_workspace", func(t *testing.T) {
		// Given: the workspace agent is connected.
		// The act of starting the agent will modify the workspace state.
		<-agentStarted
		// When: the coder_get_workspace tool is called
		ctr := makeJSONRPCRequest(t, "tools/call", "coder_get_workspace", map[string]any{
			"workspace": r.Workspace.ID.String(),
		})

		pty.WriteLine(ctr)
		_ = pty.ReadLine(ctx) // skip the echo

		ws, err := memberClient.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		wsJSON, err := json.Marshal(ws)
		require.NoError(t, err)

		// Then: the response is a valid JSON respresentation of the workspace.
		expected := makeJSONRPCTextResponse(t, string(wsJSON))
		actual := pty.ReadLine(ctx)
		testutil.RequireJSONEq(t, expected, actual)
	})

	// NOTE: this test runs after the list_workspaces tool is called.
	t.Run("coder_workspace_exec", func(t *testing.T) {
		// Given: the workspace agent is connected
		<-agentStarted

		// When: the coder_workspace_exec tools is called with a command
		randString := testutil.GetRandomName(t)
		ctr := makeJSONRPCRequest(t, "tools/call", "coder_workspace_exec", map[string]any{
			"workspace":           r.Workspace.ID.String(),
			"command":             "echo " + randString,
			"coder_url":           client.URL.String(),
			"coder_session_token": client.SessionToken(),
		})

		pty.WriteLine(ctr)
		_ = pty.ReadLine(ctx) // skip the echo

		// Then: the response is the output of the command.
		expected := makeJSONRPCTextResponse(t, randString)
		actual := pty.ReadLine(ctx)
		testutil.RequireJSONEq(t, expected, actual)
	})

	t.Run("coder_stop_workspace", func(t *testing.T) {
		// Given: a separate workspace in the running state
		stopWs := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
		}).WithAgent().Do()

		// When: the coder_stop_workspace tool is called
		ctr := makeJSONRPCRequest(t, "tools/call", "coder_stop_workspace", map[string]any{
			"workspace": stopWs.Workspace.ID.String(),
		})

		pty.WriteLine(ctr)
		_ = pty.ReadLine(ctx) // skip the echo

		// Then: the response is as expected.
		expected := makeJSONRPCTextResponse(t, `{"status":"pending","transition":"stop"}`) // no provisionerd yet
		actual := pty.ReadLine(ctx)
		testutil.RequireJSONEq(t, expected, actual)
	})

	// NOTE: this test runs after the list_workspaces tool is called.
	t.Run("tool_restrictions", func(t *testing.T) {
		// Given: the workspace agent is connected
		<-agentStarted

		// Given: a restricted MCP server with only allowed tools and commands
		restrictedPty := ptytest.New(t)
		allowedTools := []string{"coder_workspace_exec"}
		restrictedMCPSrv, closeRestrictedSrv := startTestMCPServer(ctx, t, restrictedPty.Input(), restrictedPty.Output())
		t.Cleanup(func() {
			_ = closeRestrictedSrv()
		})
		mcptools.AllTools().
			WithOnlyAllowed(allowedTools...).
			Register(restrictedMCPSrv, mcptools.ToolDeps{
				Client: memberClient,
				Logger: &logger,
			})

		// When: the tools/list command is called
		toolsListCmd := makeJSONRPCRequest(t, "tools/list", "", nil)
		restrictedPty.WriteLine(toolsListCmd)
		_ = restrictedPty.ReadLine(ctx) // skip the echo

		// Then: the response is a list of only the allowed tools.
		toolsListResponse := restrictedPty.ReadLine(ctx)
		require.Contains(t, toolsListResponse, "coder_workspace_exec")
		require.NotContains(t, toolsListResponse, "coder_whoami")

		// When: a disallowed tool is called
		disallowedToolCmd := makeJSONRPCRequest(t, "tools/call", "coder_whoami", map[string]any{})
		restrictedPty.WriteLine(disallowedToolCmd)
		_ = restrictedPty.ReadLine(ctx) // skip the echo

		// Then: the response is an error indicating the tool is not available.
		disallowedToolResponse := restrictedPty.ReadLine(ctx)
		require.Contains(t, disallowedToolResponse, "error")
		require.Contains(t, disallowedToolResponse, "not found")
	})

	t.Run("coder_start_workspace", func(t *testing.T) {
		// Given: a separate workspace in the stopped state
		stopWs := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStop,
		}).Do()

		// When: the coder_start_workspace tool is called
		ctr := makeJSONRPCRequest(t, "tools/call", "coder_start_workspace", map[string]any{
			"workspace": stopWs.Workspace.ID.String(),
		})

		pty.WriteLine(ctr)
		_ = pty.ReadLine(ctx) // skip the echo

		// Then: the response is as expected
		expected := makeJSONRPCTextResponse(t, `{"status":"pending","transition":"start"}`) // no provisionerd yet
		actual := pty.ReadLine(ctx)
		testutil.RequireJSONEq(t, expected, actual)
	})
}

// makeJSONRPCRequest is a helper function that makes a JSON RPC request.
func makeJSONRPCRequest(t *testing.T, method, name string, args map[string]any) string {
	t.Helper()
	req := mcp.JSONRPCRequest{
		ID:      "1",
		JSONRPC: "2.0",
		Request: mcp.Request{Method: method},
		Params: struct { // Unfortunately, there is no type for this yet.
			Name      string         "json:\"name\""
			Arguments map[string]any "json:\"arguments,omitempty\""
			Meta      *struct {
				ProgressToken mcp.ProgressToken "json:\"progressToken,omitempty\""
			} "json:\"_meta,omitempty\""
		}{
			Name:      name,
			Arguments: args,
		},
	}
	bs, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal JSON RPC request")
	return string(bs)
}

// makeJSONRPCTextResponse is a helper function that makes a JSON RPC text response
func makeJSONRPCTextResponse(t *testing.T, text string) string {
	t.Helper()

	resp := mcp.JSONRPCResponse{
		ID:      "1",
		JSONRPC: "2.0",
		Result: mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(text),
			},
		},
	}
	bs, err := json.Marshal(resp)
	require.NoError(t, err, "failed to marshal JSON RPC response")
	return string(bs)
}

// startTestMCPServer is a helper function that starts a MCP server listening on
// a pty. It is the responsibility of the caller to close the server.
func startTestMCPServer(ctx context.Context, t testing.TB, stdin io.Reader, stdout io.Writer) (*server.MCPServer, func() error) {
	t.Helper()

	mcpSrv := server.NewMCPServer(
		"Test Server",
		"0.0.0",
		server.WithInstructions(""),
		server.WithLogging(),
	)

	stdioSrv := server.NewStdioServer(mcpSrv)

	cancelCtx, cancel := context.WithCancel(ctx)
	closeCh := make(chan struct{})
	done := make(chan error)
	go func() {
		defer close(done)
		srvErr := stdioSrv.Listen(cancelCtx, stdin, stdout)
		done <- srvErr
	}()

	go func() {
		select {
		case <-closeCh:
			cancel()
		case <-done:
			cancel()
		}
	}()

	return mcpSrv, func() error {
		close(closeCh)
		return <-done
	}
}
