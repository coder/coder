package mcp_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestMCPHTTP_ExportRetainedAdoptedWorkspace(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	provider := chattest.OpenAI(t)
	keys := coderdtest.OpenAICompatProviderAPIKeys(provider)
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: mcpDeploymentValues(t), ChatProviderAPIKeys: &keys})
	defer closer.Close()
	user := coderdtest.CreateFirstUser(t, client)
	model := coderdtest.CreateOpenAICompatChatModel(t, codersdk.NewExperimentalClient(client), provider)
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	coderdtest.NewProvisionerDaemon(t, api)
	token := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{Parse: echo.ParseComplete, ProvisionPlan: echo.PlanComplete, ProvisionGraph: echo.ProvisionGraphWithAgent(token)})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	_ = agenttest.New(t, client.URL, token)
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).WaitFor(coderdtest.AgentsReady)
	session := acquisitionClient(ctx, t, client)
	output := filepath.Join(t.TempDir(), "retained.bin")
	payload := []byte{0, 255, 1, 3}
	require.NoError(t, os.WriteFile(output, payload, 0o600))
	acquireArgs := toolsdk.AcquireWorkspaceExecutionArgs{
		OrganizationID:                   user.OrganizationID.String(),
		AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{RequestID: uuid.New(), OwnerID: user.UserID, WorkspaceID: workspace.ID, Retained: new(true), LeaseExpiresAt: time.Now().Add(time.Hour), Declarations: codersdk.WorkspaceExecutionDeclarations{ResultPaths: []string{output}}},
	}
	acquired, err := acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, acquireArgs)
	require.NoError(t, err)
	siblingRequest := codersdk.AcquireWorkspaceExecutionRequest{RequestID: uuid.New(), OwnerID: user.UserID, WorkspaceID: workspace.ID, Retained: new(true), LeaseExpiresAt: time.Now().Add(time.Hour), Declarations: codersdk.WorkspaceExecutionDeclarations{ResultPaths: []string{}}}
	sibling, err := client.AcquireWorkspaceExecution(ctx, user.OrganizationID, siblingRequest)
	require.NoError(t, err)
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	var agentID uuid.UUID
	for _, resource := range resources {
		for _, agent := range resource.Agents {
			agentID = agent.ID
		}
	}
	require.NotEqual(t, uuid.Nil, agentID)
	start := func(id uuid.UUID, command string) (codersdk.WorkspaceCommand, error) {
		return commandCall(ctx, session, toolsdk.ToolNameStartWorkspaceCommand, toolsdk.StartWorkspaceCommandArgs{OrganizationID: user.OrganizationID.String(), SessionID: id, StartWorkspaceCommandRequest: codersdk.StartWorkspaceCommandRequest{RequestID: uuid.New(), AgentID: agentID, Command: command}})
	}
	exportArgs := toolsdk.WorkspaceExecutionControlArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID, ExpectedRevision: acquired.Revision}
	require.NoError(t, os.Remove(output))
	failed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameExportWorkspaceExecution, Arguments: exportArgs})
	require.NoError(t, err)
	require.True(t, failed.IsError)

	repairRequest := siblingRequest
	repairRequest.RequestID = uuid.New()
	repair, err := client.AcquireWorkspaceExecution(ctx, user.OrganizationID, repairRequest)
	require.NoError(t, err, "failed adopted export must allow an authorized repair session")
	require.NotEqual(t, acquired.ID, repair.ID)
	_, err = start(acquired.ID, "printf must-not-run")
	require.Error(t, err, "the failed session itself remains closed")
	written, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameWorkspaceWriteFile, Arguments: toolsdk.WorkspaceWriteFileArgs{Workspace: workspace.ID.String(), Path: output, Content: payload}})
	require.NoError(t, err)
	require.False(t, written.IsError, "%+v", written.Content)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameExportWorkspaceExecution, Arguments: exportArgs})
	require.NoError(t, err)
	require.False(t, result.IsError, "%+v", result.Content)
	var artifacts []codersdk.WorkspaceExecutionArtifact
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &artifacts))
	require.Len(t, artifacts, 1)
	status, err := client.WorkspaceExecutionSession(ctx, user.OrganizationID, acquired.ID)
	require.NoError(t, err)
	require.Equal(t, "preserved", status.State)
	require.True(t, status.Retained)
	retained, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameRetainWorkspaceExecutionSession, Arguments: toolsdk.WorkspaceExecutionControlArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID, ExpectedRevision: status.Revision}})
	require.NoError(t, err)
	require.False(t, retained.IsError, "%+v", retained.Content)
	afterRetain, err := client.WorkspaceExecutionSession(ctx, user.OrganizationID, acquired.ID)
	require.NoError(t, err)
	require.Equal(t, status.Revision, afterRetain.Revision, "retention must preserve the artifact generation")
	require.Equal(t, "preserved", afterRetain.State)

	// The exported identity remains immutable while the adopted workspace is reusable.
	_, err = start(acquired.ID, "printf must-not-run")
	require.Error(t, err, "the exported session itself must not admit another command")
	original, err := acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, acquireArgs)
	require.NoError(t, err)
	require.Equal(t, acquired.ID, original.ID)
	require.Equal(t, "preserved", original.State)
	run := func(id uuid.UUID, command string) {
		t.Helper()
		started, err := start(id, command)
		require.NoError(t, err)
		terminal, err := commandCall(ctx, session, toolsdk.ToolNameGetWorkspaceCommand, toolsdk.WorkspaceCommandArgs{OrganizationID: user.OrganizationID.String(), SessionID: id, ExecutionID: started.ID, WaitMillis: 30000})
		require.NoError(t, err)
		require.Equal(t, "completed", terminal.State)
		require.NotNil(t, terminal.ExitCode)
		require.Zero(t, *terminal.ExitCode)
	}
	run(sibling.ID, fmt.Sprintf("printf changed > %q", output))
	siblingRequest.RequestID = uuid.New()
	next, err := acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, toolsdk.AcquireWorkspaceExecutionArgs{OrganizationID: user.OrganizationID.String(), AcquireWorkspaceExecutionRequest: siblingRequest})
	require.NoError(t, err)
	require.NotEqual(t, acquired.ID, next.ID)
	run(next.ID, "printf new-session")
	chat := callChatTool[toolsdk.ChatToolStatus](ctx, t, session, toolsdk.ToolNameCreateChat, toolsdk.CreateChatArgs{Prompt: "Say hello.", OrganizationID: user.OrganizationID.String(), WorkspaceID: workspace.ID.String(), ModelConfigID: model.ID.String()})
	settled := callChatTool[toolsdk.AwaitChatResponse](ctx, t, session, toolsdk.ToolNameAwaitChat, toolsdk.AwaitChatArgs{ChatID: chat.ID, WaitSecs: 10})
	require.False(t, settled.TimedOut)
	require.Equal(t, codersdk.ChatStatusWaiting, settled.Chat.Status)
	stored := callChatTool[toolsdk.WorkspaceArtifactReadResult](ctx, t, session, toolsdk.ToolNameWorkspaceArtifactRead, toolsdk.WorkspaceArtifactReadArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID.String(), ArtifactID: artifacts[0].ID.String(), Limit: 1024})
	require.Equal(t, payload, stored.Content, "later workspace mutations must not change stored bytes")
	require.NoError(t, os.WriteFile(output, payload, 0o600))

	_, err = client.ExportWorkspaceExecution(ctx, user.OrganizationID, sibling.ID, codersdk.ExportWorkspaceExecutionRequest{ExpectedRevision: sibling.Revision})
	require.NoError(t, err, "already admitted siblings must still export after the workspace fence closes")
	unchanged, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.LatestBuild.ID, unchanged.LatestBuild.ID)
	require.NoError(t, os.Remove(output))
	replay, err := client.ExportWorkspaceExecution(ctx, user.OrganizationID, acquired.ID, codersdk.ExportWorkspaceExecutionRequest{ExpectedRevision: acquired.Revision})
	require.NoError(t, err)
	require.Equal(t, artifacts, replay, "recovery must use durable results after source removal")

	// Independently authorized explicit deletion is separate from export.
	build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{Transition: codersdk.WorkspaceTransitionDelete})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
	fresh := acquisitionClient(ctx, t, client)
	read, err := fresh.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameWorkspaceArtifactRead, Arguments: toolsdk.WorkspaceArtifactReadArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID.String(), ArtifactID: artifacts[0].ID.String(), Limit: 1024}})
	require.NoError(t, err)
	require.False(t, read.IsError, "%+v", read.Content)
	var bytes toolsdk.WorkspaceArtifactReadResult
	require.NoError(t, json.Unmarshal([]byte(read.Content[0].(*mcp.TextContent).Text), &bytes))
	require.Equal(t, payload, bytes.Content)
	require.True(t, bytes.EOF)
}
