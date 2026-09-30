package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
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
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMCPHTTP_ArtifactExpiryRecovery(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	clock := quartz.NewMock(t)
	clock.Set(dbtime.Time(time.Now())).MustWait(ctx)
	setHandler, cancelServer, serverURL, options := coderdtest.NewOptions(t, &coderdtest.Options{Database: db, Pubsub: ps, Clock: clock, WorkspaceExecutionTickerClock: clock, DeploymentValues: mcpDeploymentValues(t)})
	// Explicit fixture policy, never a production default.
	options.DeploymentValues.WorkspaceExecutionCleanup = true
	api := coderd.New(options)
	t.Cleanup(func() { cancelServer(); require.NoError(t, api.Close()) })
	setHandler(api.RootHandler)
	token := uuid.NewString()
	worker := startRecoveryProvisioner(t, api, token)
	owner := codersdk.New(serverURL)
	user := coderdtest.CreateFirstUser(t, owner)
	stranger, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID)
	version := coderdtest.CreateTemplateVersion(t, owner, user.OrganizationID, &echo.Responses{Parse: echo.ParseComplete, ProvisionPlan: echo.PlanComplete, ProvisionGraph: echo.ProvisionGraphWithAgent(token)})
	coderdtest.AwaitTemplateVersionJobCompleted(t, owner, version.ID)
	template := coderdtest.CreateTemplate(t, owner, user.OrganizationID, version.ID)
	transport := acquisitionClient(ctx, t, owner)
	path := filepath.Join(t.TempDir(), "result")
	secondPath := path + "-second"
	payload := []byte{0, 1, 255, 4}
	require.NoError(t, os.WriteFile(path, payload, 0o600))
	require.NoError(t, os.WriteFile(secondPath, []byte("second"), 0o600))
	expires := clock.Now().Add(2 * time.Minute)
	acquired, err := acquisitionCall(ctx, transport, toolsdk.ToolNameAcquireWorkspaceExecution, toolsdk.AcquireWorkspaceExecutionArgs{
		OrganizationID:                   user.OrganizationID.String(),
		AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{RequestID: uuid.New(), OwnerID: user.UserID, Create: &codersdk.CreateWorkspaceRequest{Name: "recover-" + uuid.NewString()[:8], TemplateID: template.ID}, Disposable: true, Retained: new(false), LeaseExpiresAt: clock.Now().Add(4 * time.Minute), Declarations: codersdk.WorkspaceExecutionDeclarations{ResultPaths: []string{path, secondPath}, ArtifactExpiresAt: &expires}},
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, owner, *acquired.AcquisitionBuildID)
	_ = agenttest.New(t, owner.URL, token)
	coderdtest.NewWorkspaceAgentWaiter(t, owner, *acquired.WorkspaceID).WaitFor(coderdtest.AgentsReady)
	original, err := db.GetWorkspaceExecutionSessionByID(ctx, acquired.ID)
	require.NoError(t, err)
	old, err := owner.ExportWorkspaceExecution(ctx, user.OrganizationID, acquired.ID, codersdk.ExportWorkspaceExecutionRequest{ExpectedRevision: acquired.Revision})
	require.NoError(t, err)
	require.Len(t, old, 2)
	preserved, err := owner.WorkspaceExecutionSession(ctx, user.OrganizationID, acquired.ID)
	require.NoError(t, err)
	status := func(err error, want int) {
		t.Helper()
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, want, apiErr.StatusCode())
	}
	future := clock.Now().Add(time.Hour)
	_, err = owner.RetryWorkspaceExecutionSession(ctx, user.OrganizationID, acquired.ID, codersdk.WorkspaceExecutionControlRequest{ExpectedRevision: preserved.Revision, ArtifactExpiresAt: &future})
	status(err, http.StatusConflict) // A valid generation cannot be replaced.
	for !clock.Now().After(expires) {
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
	}
	_, err = owner.RetryWorkspaceExecutionSession(ctx, user.OrganizationID, acquired.ID, codersdk.WorkspaceExecutionControlRequest{ExpectedRevision: preserved.Revision})
	status(err, http.StatusBadRequest)

	_, err = owner.ReadWorkspaceExecutionArtifact(ctx, user.OrganizationID, acquired.ID, old[0].ID, 0, 1024)
	status(err, http.StatusGone)
	tooSoon := clock.Now().Add(30 * time.Second)
	_, err = owner.RetryWorkspaceExecutionSession(ctx, user.OrganizationID, acquired.ID, codersdk.WorkspaceExecutionControlRequest{ExpectedRevision: preserved.Revision, ArtifactExpiresAt: &tooSoon})
	status(err, http.StatusBadRequest)
	_, err = stranger.RetryWorkspaceExecutionSession(ctx, user.OrganizationID, acquired.ID, codersdk.WorkspaceExecutionControlRequest{ExpectedRevision: preserved.Revision, ArtifactExpiresAt: &future})
	status(err, http.StatusNotFound)
	future = clock.Now().Add(time.Hour)
	retryArgs := toolsdk.WorkspaceExecutionControlArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID, ExpectedRevision: preserved.Revision, ArtifactExpiresAt: &future}
	result, err := transport.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameRetryWorkspaceExecutionSession, Arguments: retryArgs})
	require.NoError(t, err)
	require.False(t, result.IsError, "%+v", result.Content)
	var retried codersdk.WorkspaceExecutionControlReceipt
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &retried))
	require.Equal(t, preserved.Revision+1, retried.Revision)
	require.Equal(t, "preservation_failed", retried.State)
	require.True(t, future.Equal(*retried.EffectiveArtifactExpiresAt))
	replay, err := transport.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameRetryWorkspaceExecutionSession, Arguments: retryArgs})
	require.NoError(t, err)
	require.True(t, replay.IsError, "stale recovery must not increment again")
	current, err := db.GetWorkspaceExecutionSessionByID(ctx, acquired.ID)
	require.NoError(t, err)
	require.JSONEq(t, string(original.Declarations), string(current.Declarations))
	require.Equal(t, original.InputDigest, current.InputDigest)
	require.ErrorIs(t, workspaceexec.CheckAdmission(ctx, db, *acquired.WorkspaceID), workspaceexec.ErrAdmissionClosed)

	// An incomplete new generation is never committed or used for deletion.
	require.NoError(t, os.Remove(secondPath))
	_, err = owner.ExportWorkspaceExecution(ctx, user.OrganizationID, acquired.ID, codersdk.ExportWorkspaceExecutionRequest{ExpectedRevision: retried.Revision})
	require.Error(t, err)
	rows, err := db.GetWorkspaceExecutionArtifactsBySessionID(ctx, acquired.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	manualFailure, err := db.GetWorkspaceExecutionSessionByID(ctx, acquired.ID)
	require.NoError(t, err)
	// The autonomous controller must also refuse the incomplete source.
	require.True(t, testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		row, e := db.GetWorkspaceExecutionSessionByID(ctx, acquired.ID)
		require.NoError(t, e)
		require.False(t, row.DeleteBuildID.Valid)
		return row.State == "preservation_failed" && row.AttemptCount > manualFailure.AttemptCount
	}, testutil.IntervalFast))
	refused, err := acquisitionCall(ctx, transport, toolsdk.ToolNameGetWorkspaceExecutionSession, toolsdk.GetWorkspaceExecutionSessionArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID})
	require.NoError(t, err)
	require.Equal(t, "preservation_failed", refused.State)
	require.NotEmpty(t, refused.Error)
	require.NoError(t, os.WriteFile(secondPath, []byte("second"), 0o600))
	retryCollection, err := transport.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameRetryWorkspaceExecutionSession, Arguments: toolsdk.WorkspaceExecutionControlArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID, ExpectedRevision: refused.Revision}})
	require.NoError(t, err)
	require.False(t, retryCollection.IsError, "%+v", retryCollection.Content)
	require.NoError(t, transport.Close())
	observer := acquisitionClient(ctx, t, owner)
	var failed codersdk.WorkspaceExecutionSession
	require.True(t, testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		failed, err = acquisitionCall(ctx, observer, toolsdk.ToolNameGetWorkspaceExecutionSession, toolsdk.GetWorkspaceExecutionSessionArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID})
		require.NoError(t, err)
		return failed.State == "deletion_failed"
	}, testutil.IntervalFast))
	require.Contains(t, failed.Error, "injected delete apply failure")
	require.EqualValues(t, 1, worker.deletes.Load())
	failedBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, *acquired.WorkspaceID)
	require.NoError(t, err)
	allArtifacts, err := owner.WorkspaceExecutionArtifacts(ctx, user.OrganizationID, acquired.ID)
	require.NoError(t, err)
	var recovered []codersdk.WorkspaceExecutionArtifact
	for _, artifact := range allArtifacts {
		if artifact.PreservationRevision == retried.Revision {
			recovered = append(recovered, artifact)
		}
	}
	require.Len(t, allArtifacts, 4, "old metadata is retained")
	require.Len(t, recovered, 2)
	retry, err := observer.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameRetryWorkspaceExecutionSession, Arguments: toolsdk.WorkspaceExecutionControlArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID, ExpectedRevision: failed.Revision}})
	require.NoError(t, err)
	require.False(t, retry.IsError, "%+v", retry.Content)
	require.NoError(t, observer.Close())
	require.True(t, testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		row, e := db.GetWorkspaceExecutionSessionByID(ctx, acquired.ID)
		require.NoError(t, e)
		return row.State == "completed"
	}, testutil.IntervalFast))
	workspace, err := db.GetWorkspaceByID(ctx, *acquired.WorkspaceID)
	require.NoError(t, err)
	require.True(t, workspace.Deleted)
	require.EqualValues(t, 2, worker.deletes.Load())
	completed, err := db.GetWorkspaceExecutionSessionByID(ctx, acquired.ID)
	require.NoError(t, err)
	require.NotEqual(t, failedBuild.ID, completed.DeleteBuildID.UUID)
	replayed, err := owner.ExportWorkspaceExecution(ctx, user.OrganizationID, acquired.ID, codersdk.ExportWorkspaceExecutionRequest{ExpectedRevision: retried.Revision})
	require.NoError(t, err)
	require.Equal(t, recovered, replayed)
	fresh := acquisitionClient(ctx, t, owner)
	for _, artifact := range recovered {
		want := payload
		if artifact.SourcePath == secondPath {
			want = []byte("second")
		}
		require.Equal(t, retried.Revision, artifact.PreservationRevision)
		require.True(t, future.Equal(*artifact.ExpiresAt))
		bytes, err := owner.ReadWorkspaceExecutionArtifact(ctx, user.OrganizationID, acquired.ID, artifact.ID, 0, 1024)
		require.NoError(t, err)
		require.Equal(t, want, bytes.Content)
		require.EqualValues(t, len(want), bytes.SizeBytes)
		read, err := fresh.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameWorkspaceArtifactRead, Arguments: toolsdk.WorkspaceArtifactReadArgs{OrganizationID: user.OrganizationID.String(), SessionID: acquired.ID.String(), ArtifactID: artifact.ID.String(), Limit: 1024}})
		require.NoError(t, err)
		require.False(t, read.IsError, "%+v", read.Content)
		var contents toolsdk.WorkspaceArtifactReadResult
		require.NoError(t, json.Unmarshal([]byte(read.Content[0].(*mcp.TextContent).Text), &contents))
		require.Equal(t, want, contents.Content)
	}
	later := future.Add(time.Hour)
	_, err = owner.RetryWorkspaceExecutionSession(ctx, user.OrganizationID, acquired.ID, codersdk.WorkspaceExecutionControlRequest{ExpectedRevision: retried.Revision, ArtifactExpiresAt: &later})
	status(err, http.StatusConflict)
}
