package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

func commandCall(ctx context.Context, client *mcp.ClientSession, name string, args any) (codersdk.WorkspaceCommand, error) {
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return codersdk.WorkspaceCommand{}, err
	}
	if result.IsError {
		return codersdk.WorkspaceCommand{}, xerrors.Errorf("command tool: %v", result.Content)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return codersdk.WorkspaceCommand{}, err
	}
	var receipt codersdk.WorkspaceCommand
	err = json.Unmarshal(raw, &receipt)
	return receipt, err
}

type commandLostResponse struct {
	base     http.RoundTripper
	lost     atomic.Bool
	accepted atomic.Pointer[mcp.CallToolResult]
}

func (d *commandLostResponse) RoundTrip(req *http.Request) (*http.Response, error) {
	isCall := false
	if req.Body != nil {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(raw))
		isCall = bytes.Contains(raw, []byte(toolsdk.ToolNameStartWorkspaceCommand))
	}
	res, err := d.base.RoundTrip(req)
	if err == nil && isCall && res.StatusCode == http.StatusOK && d.lost.CompareAndSwap(false, true) {
		var envelope struct {
			Result *mcp.CallToolResult `json:"result"`
		}
		decodeErr := json.NewDecoder(res.Body).Decode(&envelope)
		_ = res.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}
		d.accepted.Store(envelope.Result)
		return nil, io.ErrUnexpectedEOF
	}
	return res, err
}

func TestMCPHTTP_DurableWorkspaceCommands(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: mcpDeploymentValues(t)})
	defer closer.Close()
	user := coderdtest.CreateFirstUser(t, client)
	fixture := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{Name: "command-" + uuid.NewString()[:8], OrganizationID: user.OrganizationID, OwnerID: user.UserID}).WithAgent().Do()
	workspaceAgent := agenttest.New(t, client.URL, fixture.AgentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, fixture.Workspace.ID).Wait()
	actorCtx := dbauthz.As(ctx, rbac.Subject{ID: user.UserID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleOwner()}, Scope: rbac.ScopeAll})
	session, err := api.Database.InsertWorkspaceExecutionSession(actorCtx, database.InsertWorkspaceExecutionSessionParams{
		ID: uuid.New(), OrganizationID: user.OrganizationID, OwnerID: user.UserID, ActorID: user.UserID, RequestID: uuid.New(), InputDigest: make([]byte, 32),
		WorkspaceID: uuid.NullUUID{UUID: fixture.Workspace.ID, Valid: true}, WorkspaceOwnerID: uuid.NullUUID{UUID: user.UserID, Valid: true},
		CreatedAt: dbtime.Now(), State: "active", Retained: true, LeaseExpiresAt: time.Now().Add(time.Hour), Declarations: []byte("{}"),
	})
	require.NoError(t, err)
	connect := func() *mcp.ClientSession {
		c, e := newIsolatedMCPClient(ctx, client.URL.String()+mcpserver.MCPEndpoint, uuid.NewString(), map[string]string{"Authorization": "Bearer " + client.SessionToken()})
		require.NoError(t, e)
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	first, second := connect(), connect()
	marker := filepath.Join(t.TempDir(), "executions")
	args := toolsdk.StartWorkspaceCommandArgs{OrganizationID: user.OrganizationID.String(), SessionID: session.ID, StartWorkspaceCommandRequest: codersdk.StartWorkspaceCommandRequest{
		RequestID: uuid.New(), AgentID: fixture.Agents[0].ID, Command: fmt.Sprintf("printf x >> '%s'; printf 'finished\\n'", marker),
	}}
	var results [2]codersdk.WorkspaceCommand
	var errors [2]error
	var wg sync.WaitGroup
	for i, c := range []*mcp.ClientSession{first, second} {
		wg.Go(func() { results[i], errors[i] = commandCall(ctx, c, toolsdk.ToolNameStartWorkspaceCommand, args) })
	}
	wg.Wait()
	for _, err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, results[0].ID, results[1].ID)
	require.Equal(t, results[0].ProcessID, results[1].ProcessID)
	get := toolsdk.WorkspaceCommandArgs{OrganizationID: user.OrganizationID.String(), SessionID: session.ID, ExecutionID: results[0].ID, WaitMillis: 1000}
	completed, err := commandCall(ctx, first, toolsdk.ToolNameGetWorkspaceCommand, get)
	require.NoError(t, err)
	require.Equal(t, "completed", completed.State)
	require.NotNil(t, completed.ExitCode)
	require.Zero(t, *completed.ExitCode)
	require.NotNil(t, completed.Output)
	require.Contains(t, completed.Output.Text, "finished")
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "x", string(contents))
	changed := args
	changed.Command = "printf changed"
	_, err = commandCall(ctx, first, toolsdk.ToolNameStartWorkspaceCommand, changed)
	require.Error(t, err)
	// Consume an accepted MCP response, then recover through a fresh MCP session.
	isolated := coderdtest.NewIsolatedHTTPClient(nil)
	drop := &commandLostResponse{base: &headerRoundTripper{base: isolated.Transport, headers: map[string]string{"Authorization": "Bearer " + client.SessionToken()}}}
	isolated.Transport = drop
	lost, err := mcp.NewClient(&mcp.Implementation{Name: uuid.NewString(), Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: client.URL.String() + mcpserver.MCPEndpoint, HTTPClient: isolated}, nil)
	require.NoError(t, err)
	defer lost.Close()
	args.RequestID = uuid.New()
	args.Command = fmt.Sprintf("printf x >> '%s'; sleep 60", marker)
	_, err = commandCall(ctx, lost, toolsdk.ToolNameStartWorkspaceCommand, args)
	require.Error(t, err)
	require.True(t, drop.lost.Load())
	accepted := drop.accepted.Load()
	require.NotNil(t, accepted)
	require.False(t, accepted.IsError, "%+v", accepted.Content)
	acceptedJSON, err := json.Marshal(accepted.StructuredContent)
	require.NoError(t, err)
	var beforeLoss codersdk.WorkspaceCommand
	require.NoError(t, json.Unmarshal(acceptedJSON, &beforeLoss))
	require.NotEqual(t, uuid.Nil, beforeLoss.ID)
	require.NoError(t, lost.Close())
	recovered, err := commandCall(ctx, connect(), toolsdk.ToolNameStartWorkspaceCommand, args)
	require.NoError(t, err)
	require.Equal(t, beforeLoss.ID, recovered.ID)
	require.Equal(t, beforeLoss.ProcessID, recovered.ProcessID)
	require.Equal(t, beforeLoss.AgentInstanceID, recovered.AgentInstanceID)
	get.ExecutionID = recovered.ID
	get.WaitMillis = 10
	observed, err := commandCall(ctx, connect(), toolsdk.ToolNameGetWorkspaceCommand, get)
	require.NoError(t, err)
	require.Equal(t, "running", observed.State)
	require.Nil(t, observed.ExitCode)
	replayed, err := commandCall(ctx, connect(), toolsdk.ToolNameStartWorkspaceCommand, args)
	require.NoError(t, err)
	require.Equal(t, recovered.ID, replayed.ID)
	require.Equal(t, recovered.ProcessID, replayed.ProcessID)
	get.WaitMillis = 1000
	completed, err = commandCall(ctx, connect(), toolsdk.ToolNameCancelWorkspaceCommand, get)
	require.NoError(t, err)
	require.Equal(t, "completed", completed.State)
	contents, err = os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "xx", string(contents))
	// A bounded observation returns running without canceling the real process.
	args.RequestID = uuid.New()
	args.Command = "printf ready; sleep 60"
	running, err := commandCall(ctx, first, toolsdk.ToolNameStartWorkspaceCommand, args)
	require.NoError(t, err)
	get.ExecutionID = running.ID
	get.WaitMillis = 10
	running, err = commandCall(ctx, first, toolsdk.ToolNameGetWorkspaceCommand, get)
	require.NoError(t, err)
	require.Equal(t, "running", running.State)
	require.Nil(t, running.ExitCode)
	get.WaitMillis = 1000
	canceled, err := commandCall(ctx, first, toolsdk.ToolNameCancelWorkspaceCommand, get)
	require.NoError(t, err)
	require.Equal(t, "completed", canceled.State)
	require.NotNil(t, canceled.ExitCode)
	again, err := commandCall(ctx, first, toolsdk.ToolNameCancelWorkspaceCommand, get)
	require.NoError(t, err)
	require.Equal(t, canceled.ExitCode, again.ExitCode)
	// Output metadata describes actual retained bytes, including truncation markers.
	args.RequestID = uuid.New()
	args.Command = "head -c 262144 /dev/zero | tr '\\000' x"
	large, err := commandCall(ctx, first, toolsdk.ToolNameStartWorkspaceCommand, args)
	require.NoError(t, err)
	get.ExecutionID = large.ID
	large, err = commandCall(ctx, first, toolsdk.ToolNameGetWorkspaceCommand, get)
	require.NoError(t, err)
	require.Equal(t, "completed", large.State)
	require.NotNil(t, large.Output)
	require.NotNil(t, large.Output.Truncated)
	require.Equal(t, len(large.Output.Text), large.Output.Truncated.RetainedBytes)
	require.Equal(t, 262144, large.Output.Truncated.OriginalBytes)
	require.Positive(t, large.Output.Truncated.OmittedBytes)
	api.Entitlements.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureBrowserOnly] = codersdk.Feature{Enabled: true}
	})
	blocked := args
	blocked.RequestID = uuid.New()
	blocked.Command = fmt.Sprintf("printf forbidden >> '%s'", marker)
	_, err = commandCall(ctx, first, toolsdk.ToolNameStartWorkspaceCommand, blocked)
	require.Error(t, err)
	_, err = client.StartWorkspaceCommand(ctx, user.OrganizationID, session.ID, blocked.StartWorkspaceCommandRequest)
	require.Error(t, err)
	blockedOutput, err := client.WorkspaceCommand(ctx, user.OrganizationID, session.ID, large.ID, 0)
	require.NoError(t, err)
	require.True(t, blockedOutput.OutputUnavailable)
	require.Nil(t, blockedOutput.Output)
	recovered, err = commandCall(ctx, first, toolsdk.ToolNameStartWorkspaceCommand, args)
	require.NoError(t, err, "identity recovery must not execute or require a live connection")
	require.Equal(t, large.ID, recovered.ID)
	contents, err = os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "xx", string(contents))

	api.Entitlements.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureBrowserOnly] = codersdk.Feature{Enabled: false}
	})

	// Restart loses the agent's ephemeral history, not the durable request identity.
	args.RequestID = uuid.New()
	args.Command = fmt.Sprintf("printf y >> '%s'; sleep 60", marker)
	interrupted, err := commandCall(ctx, first, toolsdk.ToolNameStartWorkspaceCommand, args)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		data, readErr := os.ReadFile(marker)
		return readErr == nil && string(data) == "xxy"
	}, testutil.WaitShort, testutil.IntervalFast)
	require.NoError(t, workspaceAgent.Close())
	_ = agenttest.New(t, client.URL, fixture.AgentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, fixture.Workspace.ID).Wait()
	get.ExecutionID = interrupted.ID
	get.WaitMillis = 0
	unknown, err := commandCall(ctx, connect(), toolsdk.ToolNameGetWorkspaceCommand, get)
	require.NoError(t, err)
	require.Equal(t, "unknown", unknown.State)
	require.True(t, unknown.OutputUnavailable)
	require.Nil(t, unknown.ExitCode)
	recovered, err = commandCall(ctx, connect(), toolsdk.ToolNameStartWorkspaceCommand, args)
	require.NoError(t, err)
	require.Equal(t, interrupted.ID, recovered.ID)
	require.Equal(t, interrupted.AgentInstanceID, recovered.AgentInstanceID)
	contents, err = os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "xxy", string(contents))
	get.ExecutionID = completed.ID
	known, err := commandCall(ctx, first, toolsdk.ToolNameGetWorkspaceCommand, get)
	require.NoError(t, err)
	require.Equal(t, "completed", known.State)
	require.True(t, known.OutputUnavailable)
	require.NotNil(t, known.ExitCode)

	other, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	_, err = other.WorkspaceCommand(ctx, user.OrganizationID, session.ID, large.ID, 0)
	require.Error(t, err)
	wrongOrg := uuid.New()
	_, err = client.WorkspaceCommand(ctx, wrongOrg, session.ID, large.ID, 0)
	require.Error(t, err)
}
