package mcp_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMCPHTTP_DisposableExecutionSurvivesDisconnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	clock := quartz.NewMock(t)
	clock.Set(dbtime.Time(time.Now())).MustWait(ctx)
	setHandler, cancelServer, serverURL, options := coderdtest.NewOptions(t, &coderdtest.Options{Database: db, Pubsub: ps, Clock: clock, WorkspaceExecutionTickerClock: clock, DeploymentValues: mcpDeploymentValues(t)})
	// This fixture explicitly selects policy; production defaults remain disabled.
	options.DeploymentValues.WorkspaceExecutionCleanup = true
	api := coderd.New(options)
	t.Cleanup(func() { cancelServer(); require.NoError(t, api.Close()) })
	setHandler(api.RootHandler)
	provisioner := coderdtest.NewProvisionerDaemon(t, api)
	t.Cleanup(func() { require.NoError(t, provisioner.Close()) })
	owner := codersdk.New(serverURL, codersdk.WithHTTPClient(coderdtest.NewIsolatedHTTPClient(serverURL)))
	t.Cleanup(owner.HTTPClient.CloseIdleConnections)
	user := coderdtest.CreateFirstUser(t, owner)
	agentToken := uuid.NewString()
	otherAgentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, owner, user.OrganizationID, &echo.Responses{Parse: echo.ParseComplete, ProvisionPlan: echo.PlanComplete, ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken, func(graph *proto.GraphComplete) {
		graph.Resources[0].Agents = append(graph.Resources[0].Agents, &proto.Agent{Id: uuid.NewString(), Name: "other", Auth: &proto.Agent_Token{Token: otherAgentToken}})
	})})
	coderdtest.AwaitTemplateVersionJobCompleted(t, owner, version.ID)
	template := coderdtest.CreateTemplate(t, owner, user.OrganizationID, version.ID)
	app, secret := oauth2providertest.CreateTestOAuth2App(t, owner)
	resource := owner.URL.String()
	tokenFor := func(scope string) string {
		verifier, challenge := oauth2providertest.GeneratePKCE(t)
		code := oauth2providertest.AuthorizeOAuth2App(t, owner, owner.URL.String(), oauth2providertest.AuthorizeParams{
			ClientID: app.ID.String(), ResponseType: "code", RedirectURI: oauth2providertest.TestRedirectURI, State: uuid.NewString(), CodeChallenge: challenge, CodeChallengeMethod: "S256", Resource: resource, Scope: scope,
		})
		token := oauth2providertest.ExchangeCodeForToken(t, owner.URL.String(), oauth2providertest.TokenExchangeParams{GrantType: "authorization_code", Code: code, ClientID: app.ID.String(), ClientSecret: secret, CodeVerifier: verifier, RedirectURI: oauth2providertest.TestRedirectURI, Resource: resource})
		return token.AccessToken
	}
	token := tokenFor("coder:workspaces.access coder:workspaces.create user:read organization:read workspace_execution:create workspace_execution:read workspace_execution:update workspace_execution:ssh")
	connect := func(c context.Context, token string) *mcp.ClientSession {
		session, err := newIsolatedMCPClient(c, resource+mcpserver.MCPEndpoint, uuid.NewString(), map[string]string{"Authorization": "Bearer " + token})
		require.NoError(t, err)
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	requestCtx, cancelRequest := context.WithCancel(ctx)
	session := connect(requestCtx, token)
	resultPath := filepath.Join(t.TempDir(), "binary-result")
	expires := clock.Now().Add(24 * time.Hour)
	acquire := toolsdk.AcquireWorkspaceExecutionArgs{OrganizationID: user.OrganizationID.String(), AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{
		RequestID: uuid.New(), OwnerID: user.UserID, Create: &codersdk.CreateWorkspaceRequest{Name: "disposable-" + uuid.NewString()[:8], TemplateID: template.ID},
		LeaseExpiresAt: clock.Now().Add(time.Minute), Disposable: true, Retained: new(false), Declarations: codersdk.WorkspaceExecutionDeclarations{ResultAgentName: "example", ResultPaths: []string{resultPath}, ArtifactExpiresAt: &expires},
	}}
	receipt, err := acquisitionCall(requestCtx, session, toolsdk.ToolNameAcquireWorkspaceExecution, acquire)
	require.NoError(t, err)
	require.NotNil(t, receipt.WorkspaceID)
	require.NotNil(t, receipt.AcquisitionBuildID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, owner, *receipt.AcquisitionBuildID)
	_ = agenttest.New(t, owner.URL, agentToken)
	_ = agenttest.New(t, owner.URL, otherAgentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, owner, *receipt.WorkspaceID).WaitFor(coderdtest.AgentsReady)
	resources := coderdtest.AwaitWorkspaceAgents(t, owner, *receipt.WorkspaceID)
	require.Len(t, resources[0].Agents, 2)
	var selectedAgent uuid.UUID
	for _, a := range resources[0].Agents {
		if a.Name == "example" {
			selectedAgent = a.ID
		}
	}
	require.NotEqual(t, uuid.Nil, selectedAgent)
	ready, err := session.CallTool(requestCtx, &mcp.CallToolParams{Name: toolsdk.ToolNameWorkspaceReadiness, Arguments: toolsdk.WorkspaceReadinessArgs{Workspace: receipt.WorkspaceID.String() + ".example"}})
	require.NoError(t, err)
	require.False(t, ready.IsError)
	var readiness toolsdk.WorkspaceReadinessResponse
	require.NoError(t, json.Unmarshal([]byte(ready.Content[0].(*mcp.TextContent).Text), &readiness))
	require.True(t, readiness.Ready)
	start := toolsdk.StartWorkspaceCommandArgs{OrganizationID: user.OrganizationID.String(), SessionID: receipt.ID, StartWorkspaceCommandRequest: codersdk.StartWorkspaceCommandRequest{RequestID: uuid.New(), AgentID: selectedAgent, Command: fmt.Sprintf("printf '\\000\\001\\377payload' > %q", resultPath)}}
	command, err := commandCall(requestCtx, session, toolsdk.ToolNameStartWorkspaceCommand, start)
	require.NoError(t, err)
	api.Entitlements.Modify(func(e *codersdk.Entitlements) {
		e.Features[codersdk.FeatureBrowserOnly] = codersdk.Feature{Enabled: true}
	})
	require.NoError(t, session.Close())
	cancelRequest()
	// Current administrator access restrictions veto autonomous collection.
	require.True(t, testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		row, e := db.GetWorkspaceExecutionSessionByID(ctx, receipt.ID)
		require.NoError(t, e)
		if row.State != "preservation_failed" {
			return false
		}
		require.Contains(t, row.Error, "Non-browser result collection")
		require.False(t, row.DeleteBuildID.Valid)
		artifacts, e := db.GetWorkspaceExecutionArtifactsBySessionID(ctx, receipt.ID)
		require.NoError(t, e)
		require.Empty(t, artifacts)
		return true
	}, testutil.IntervalFast))
	api.Entitlements.Modify(func(e *codersdk.Entitlements) {
		e.Features[codersdk.FeatureBrowserOnly] = codersdk.Feature{Enabled: false}
	})
	// No client request drives cleanup. Only the running server timer advances it.
	require.True(t, testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		row, e := db.GetWorkspaceExecutionSessionByID(ctx, receipt.ID)
		require.NoError(t, e)
		return row.State == "completed"
	}, testutil.IntervalFast))
	workspace, err := db.GetWorkspaceByID(ctx, *receipt.WorkspaceID)
	require.NoError(t, err)
	require.True(t, workspace.Deleted)
	fresh := connect(ctx, token)
	list, err := fresh.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameWorkspaceArtifactList, Arguments: toolsdk.WorkspaceArtifactListArgs{OrganizationID: user.OrganizationID.String(), SessionID: receipt.ID.String()}})
	require.NoError(t, err)
	require.False(t, list.IsError)
	var artifacts []codersdk.WorkspaceExecutionArtifact
	require.NoError(t, json.Unmarshal([]byte(list.Content[0].(*mcp.TextContent).Text), &artifacts))
	require.Len(t, artifacts, 1)
	readArgs := toolsdk.WorkspaceArtifactReadArgs{OrganizationID: user.OrganizationID.String(), SessionID: receipt.ID.String(), ArtifactID: artifacts[0].ID.String(), Limit: 1024}
	data, err := fresh.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameWorkspaceArtifactRead, Arguments: readArgs})
	require.NoError(t, err)
	require.False(t, data.IsError)
	var result toolsdk.WorkspaceArtifactReadResult
	require.NoError(t, json.Unmarshal([]byte(data.Content[0].(*mcp.TextContent).Text), &result))
	want := append([]byte{0, 1, 255}, []byte("payload")...)
	require.Equal(t, want, result.Content)
	require.True(t, result.EOF)
	digest := sha256.Sum256(want)
	require.Equal(t, hex.EncodeToString(digest[:]), artifacts[0].SHA256)
	replay, err := commandCall(ctx, fresh, toolsdk.ToolNameStartWorkspaceCommand, start)
	require.NoError(t, err)
	require.Equal(t, command.ID, replay.ID)
	require.Equal(t, "completed", replay.State)
	// A separately consented metadata-only OAuth token cannot read result bytes.
	readOnly := connect(ctx, tokenFor("organization:read workspace_execution:read"))
	denied, err := readOnly.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameWorkspaceArtifactRead, Arguments: readArgs})
	require.NoError(t, err)
	require.True(t, denied.IsError)
}
