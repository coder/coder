package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func acquisitionCall(ctx context.Context, client *mcp.ClientSession, name string, args any) (codersdk.WorkspaceExecutionSession, error) {
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return codersdk.WorkspaceExecutionSession{}, err
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return codersdk.WorkspaceExecutionSession{}, err
	}
	if result.IsError {
		return codersdk.WorkspaceExecutionSession{}, &acquisitionToolError{result: result}
	}
	var receipt codersdk.WorkspaceExecutionSession
	err = json.Unmarshal(raw, &receipt)
	return receipt, err
}

type acquisitionToolError struct{ result *mcp.CallToolResult }

func (e *acquisitionToolError) Error() string {
	raw, _ := json.Marshal(e.result.Content)
	return string(raw)
}

func acquisitionClient(ctx context.Context, t *testing.T, client *codersdk.Client) *mcp.ClientSession {
	t.Helper()
	session, err := newIsolatedMCPClient(ctx, client.URL.String()+mcpserver.MCPEndpoint, uuid.NewString(), map[string]string{"Authorization": "Bearer " + client.SessionToken()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// acquisitionLostResponse consumes a successful server response before simulating
// a broken connection. The next request must recover from the durable receipt.
type acquisitionLostResponse struct {
	base     http.RoundTripper
	lost     atomic.Bool
	accepted atomic.Pointer[mcp.CallToolResult]
}

func (d *acquisitionLostResponse) RoundTrip(req *http.Request) (*http.Response, error) {
	isCall := false
	if req.Body != nil {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(raw))
		isCall = bytes.Contains(raw, []byte(`"tools/call"`))
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

func TestMCPHTTP_WorkspaceAcquisition(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: mcpDeploymentValues(t)})
	defer closer.Close()
	user := coderdtest.CreateFirstUser(t, client)
	coderdtest.NewProvisionerDaemon(t, api)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	first, second := acquisitionClient(ctx, t, client), acquisitionClient(ctx, t, client)
	args := toolsdk.AcquireWorkspaceExecutionArgs{OrganizationID: user.OrganizationID.String(), AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{
		RequestID: uuid.New(), OwnerID: user.UserID, Create: &codersdk.CreateWorkspaceRequest{Name: "acquire-" + uuid.NewString()[:8], TemplateID: template.ID},
		LeaseExpiresAt: time.Now().Add(time.Hour).UTC(), Disposable: true, Retained: new(false), Declarations: codersdk.WorkspaceExecutionDeclarations{ResultPaths: []string{"/tmp/result"}},
	}}
	var receipts [2]codersdk.WorkspaceExecutionSession
	var errs [2]error
	var wg sync.WaitGroup
	for i, session := range []*mcp.ClientSession{first, second} {
		wg.Go(func() {
			receipts[i], errs[i] = acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, args)
		})
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, receipts[0].ID, receipts[1].ID)
	require.Equal(t, receipts[0].WorkspaceID, receipts[1].WorkspaceID)
	require.Equal(t, receipts[0].AcquisitionBuildID, receipts[1].AcquisitionBuildID)
	require.True(t, receipts[0].Disposable)
	require.False(t, receipts[0].Retained)
	require.Equal(t, args.Declarations, receipts[0].Declarations)
	require.NotNil(t, receipts[0].AcquisitionBuildID)
	workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{Owner: codersdk.Me})
	require.NoError(t, err)
	require.Len(t, workspaces.Workspaces, 1)
	builds, err := client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{WorkspaceID: *receipts[0].WorkspaceID})
	require.NoError(t, err)
	require.Len(t, builds, 1)
	changed := args.AcquireWorkspaceExecutionRequest
	changed.Disposable = false
	_, err = client.AcquireWorkspaceExecution(ctx, user.OrganizationID, changed)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
	fresh := acquisitionClient(ctx, t, client)
	recovered, err := acquisitionCall(ctx, fresh, toolsdk.ToolNameAcquireWorkspaceExecution, args)
	require.NoError(t, err)
	require.Equal(t, receipts[0].ID, recovered.ID)

	// Lose a separate create response, then recover using a completely new MCP client.
	isolated := coderdtest.NewIsolatedHTTPClient(nil)
	drop := &acquisitionLostResponse{base: &headerRoundTripper{base: isolated.Transport, headers: map[string]string{"Authorization": "Bearer " + client.SessionToken()}}}
	isolated.Transport = drop
	lost, err := mcp.NewClient(&mcp.Implementation{Name: uuid.NewString(), Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: client.URL.String() + mcpserver.MCPEndpoint, HTTPClient: isolated}, nil)
	require.NoError(t, err)
	lostArgs := args
	lostArgs.RequestID = uuid.New()
	lostArgs.Create = &codersdk.CreateWorkspaceRequest{Name: "lost-" + uuid.NewString()[:8], TemplateID: template.ID}
	_, err = acquisitionCall(ctx, lost, toolsdk.ToolNameAcquireWorkspaceExecution, lostArgs)
	require.Error(t, err)
	require.True(t, drop.lost.Load())
	accepted := drop.accepted.Load()
	require.NotNil(t, accepted)
	require.False(t, accepted.IsError, "%+v", accepted.Content)
	acceptedJSON, err := json.Marshal(accepted.StructuredContent)
	require.NoError(t, err)
	var beforeLoss codersdk.WorkspaceExecutionSession
	require.NoError(t, json.Unmarshal(acceptedJSON, &beforeLoss))
	require.NotEqual(t, uuid.Nil, beforeLoss.ID)
	_ = lost.Close()
	recovered, err = acquisitionCall(ctx, acquisitionClient(ctx, t, client), toolsdk.ToolNameAcquireWorkspaceExecution, lostArgs)
	require.NoError(t, err)
	require.Equal(t, beforeLoss.ID, recovered.ID)
	require.Equal(t, beforeLoss.WorkspaceID, recovered.WorkspaceID)
	require.Equal(t, beforeLoss.AcquisitionBuildID, recovered.AcquisitionBuildID)
	workspaces, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{Owner: codersdk.Me})
	require.NoError(t, err)
	require.Len(t, workspaces.Workspaces, 2)
	builds, err = client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{WorkspaceID: *recovered.WorkspaceID})
	require.NoError(t, err)
	require.Len(t, builds, 1)

	other, otherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	_, err = acquisitionCall(ctx, acquisitionClient(ctx, t, other), toolsdk.ToolNameGetWorkspaceExecutionSession, toolsdk.GetWorkspaceExecutionSessionArgs{OrganizationID: user.OrganizationID.String(), SessionID: recovered.ID})
	require.Error(t, err)
	_, err = acquisitionCall(ctx, acquisitionClient(ctx, t, other), toolsdk.ToolNameAcquireWorkspaceExecution, lostArgs)
	require.Error(t, err)
	reuse := args
	reuse.RequestID = uuid.New()
	reuse.Create = nil
	reuse.WorkspaceID = *recovered.WorkspaceID
	_, err = acquisitionCall(ctx, first, toolsdk.ToolNameAcquireWorkspaceExecution, reuse)
	require.ErrorContains(t, err, "cannot be made disposable")
	reuse.Disposable = false
	reuse.Retained = new(true)
	reused, err := acquisitionCall(ctx, first, toolsdk.ToolNameAcquireWorkspaceExecution, reuse)
	require.NoError(t, err)
	require.True(t, reused.Retained)
	require.False(t, reused.Disposable)
	reuse.RequestID = uuid.New()
	reuse.OwnerID = otherUser.ID
	_, err = acquisitionCall(ctx, first, toolsdk.ToolNameAcquireWorkspaceExecution, reuse)
	require.Error(t, err)
	wrongOrg := toolsdk.GetWorkspaceExecutionSessionArgs{OrganizationID: uuid.NewString(), SessionID: recovered.ID}
	_, err = acquisitionCall(ctx, first, toolsdk.ToolNameGetWorkspaceExecutionSession, wrongOrg)
	require.Error(t, err)
}

func TestMCPHTTP_WorkspaceAcquisitionExpiredReplayAndProvisioningFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	clock := quartz.NewMock(t)
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{Clock: clock, DeploymentValues: mcpDeploymentValues(t)})
	defer closer.Close()
	user := coderdtest.CreateFirstUser(t, client)
	coderdtest.NewProvisionerDaemon(t, api)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{Parse: echo.ParseComplete, ProvisionApply: []*proto.Response{{Type: &proto.Response_Apply{Apply: &proto.ApplyComplete{Error: "explicit provision failure"}}}}})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	session := acquisitionClient(ctx, t, client)
	args := toolsdk.AcquireWorkspaceExecutionArgs{OrganizationID: user.OrganizationID.String(), AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{RequestID: uuid.New(), OwnerID: user.UserID, Create: &codersdk.CreateWorkspaceRequest{Name: "failed-" + uuid.NewString()[:8], TemplateID: template.ID}, LeaseExpiresAt: clock.Now().Add(time.Millisecond), Retained: new(true)}}
	receipt, err := acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, args)
	require.NoError(t, err)
	build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, *receipt.AcquisitionBuildID)
	require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
	require.Contains(t, build.Job.Error, "explicit provision failure")
	clock.Advance(2 * time.Millisecond).MustWait(ctx)
	replay, err := acquisitionCall(ctx, acquisitionClient(ctx, t, client), toolsdk.ToolNameAcquireWorkspaceExecution, args)
	require.NoError(t, err)
	require.Equal(t, receipt.ID, replay.ID)
	require.Equal(t, receipt.LeaseExpiresAt, replay.LeaseExpiresAt)
	require.Equal(t, build.Job.Error, replay.AcquisitionBuild.Error)
	require.Equal(t, codersdk.WorkspaceStatusFailed, replay.AcquisitionBuild.Status)
	args.RequestID = uuid.New()
	args.Create = &codersdk.CreateWorkspaceRequest{Name: "expired-" + uuid.NewString()[:8], TemplateID: template.ID}
	_, err = acquisitionCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, args)
	require.ErrorContains(t, err, "lease_expires_at")
	workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{Owner: codersdk.Me})
	require.NoError(t, err)
	require.Len(t, workspaces.Workspaces, 1)
}
