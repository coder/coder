package mcp_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMCPHTTP_ManagedWorkspaceRejectsUntrackedMutations(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{Database: db, Pubsub: ps, DeploymentValues: mcpDeploymentValues(t)})
	defer closer.Close()
	user := coderdtest.CreateFirstUser(t, client)
	build := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{OrganizationID: user.OrganizationID, OwnerID: user.UserID}).WithAgent().Do()
	fs := afero.NewMemMapFs()
	path := "/untracked-result.txt"
	require.NoError(t, afero.WriteFile(fs, path, []byte("original"), 0o600))
	agenttest.New(t, client.URL, build.AgentToken, func(options *agent.Options) { options.Filesystem = fs })
	coderdtest.NewWorkspaceAgentWaiter(t, client, build.Workspace.ID).Wait()
	session, err := newIsolatedMCPClient(ctx, client.URL.String()+mcpserver.MCPEndpoint, uuid.NewString(), map[string]string{"Authorization": "Bearer " + client.SessionToken()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameWorkspaceWriteFile, Arguments: toolsdk.WorkspaceWriteFileArgs{Workspace: build.Workspace.Name, Path: path, Content: []byte("original")}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	receipt, err := acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, toolsdk.AcquireWorkspaceExecutionArgs{OrganizationID: user.OrganizationID.String(), AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{
		RequestID: uuid.New(), OwnerID: user.UserID, WorkspaceID: build.Workspace.ID, LeaseExpiresAt: time.Now().Add(time.Hour), Retained: new(false),
	}})
	require.NoError(t, err)
	marker := filepath.Join(t.TempDir(), "must-not-run")

	checkMutations := func(allowed bool) {
		t.Helper()
		for _, request := range []*mcp.CallToolParams{
			{Name: toolsdk.ToolNameWorkspaceBash, Arguments: toolsdk.WorkspaceBashArgs{Workspace: build.Workspace.Name, Command: "touch " + marker}},
			{Name: toolsdk.ToolNameWorkspaceWriteFile, Arguments: toolsdk.WorkspaceWriteFileArgs{Workspace: build.Workspace.Name, Path: path, Content: []byte("changed")}},
			{Name: toolsdk.ToolNameWorkspaceEditFile, Arguments: map[string]any{"workspace": build.Workspace.Name, "path": path, "edits": []map[string]any{{"old_text": "original", "new_text": "changed"}}}},
			{Name: toolsdk.ToolNameWorkspaceEditFiles, Arguments: map[string]any{"workspace": build.Workspace.Name, "files": []map[string]any{{"path": path, "edits": []map[string]any{{"old_text": "original", "new_text": "changed"}}}}}},
		} {
			require.NoError(t, afero.WriteFile(fs, path, []byte("original"), 0o600))
			result, err := session.CallTool(ctx, request)
			require.NoError(t, err)
			require.Equal(t, !allowed, result.IsError, "%s: %+v", request.Name, result.Content)
			if !allowed {
				require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "tracked workspace command")
			}
			content, err := afero.ReadFile(fs, path)
			require.NoError(t, err)
			if allowed && request.Name != toolsdk.ToolNameWorkspaceBash {
				require.Equal(t, "changed", string(content))
			} else {
				require.Equal(t, "original", string(content))
			}
			_, err = os.Stat(marker)
			if allowed && request.Name == toolsdk.ToolNameWorkspaceBash {
				require.NoError(t, err)
				require.NoError(t, os.Remove(marker))
			} else {
				require.True(t, os.IsNotExist(err))
			}
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameWorkspaceReadFile, Arguments: toolsdk.WorkspaceReadFileArgs{Workspace: build.Workspace.Name, Path: path}})
		require.NoError(t, err)
		require.False(t, result.IsError)
	}
	checkMutations(true)
	_, err = sqlDB.ExecContext(ctx, "UPDATE workspace_execution_sessions SET state='preserving' WHERE id=$1", receipt.ID)
	require.NoError(t, err)
	checkMutations(false)
	_, err = sqlDB.ExecContext(ctx, "UPDATE workspace_execution_sessions SET state='active' WHERE id=$1", receipt.ID)
	require.NoError(t, err)
	exported, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameExportWorkspaceExecution, Arguments: toolsdk.WorkspaceExecutionControlArgs{OrganizationID: user.OrganizationID.String(), SessionID: receipt.ID, ExpectedRevision: receipt.Revision}})
	require.NoError(t, err)
	require.False(t, exported.IsError, "%+v", exported.Content)
	checkMutations(true)
	next, err := acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, toolsdk.AcquireWorkspaceExecutionArgs{OrganizationID: user.OrganizationID.String(), AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{
		RequestID: uuid.New(), OwnerID: user.UserID, WorkspaceID: build.Workspace.ID, LeaseExpiresAt: time.Now().Add(time.Hour), Retained: new(false),
	}})
	require.NoError(t, err)
	require.NotEqual(t, receipt.ID, next.ID)
	checkMutations(true)
	// Every session is inspected: a disposable sibling still fences this workspace.
	_, err = sqlDB.ExecContext(ctx, "UPDATE workspace_execution_sessions SET disposable=true, state='active' WHERE id=$1", receipt.ID)
	require.NoError(t, err)
	checkMutations(false)
}
