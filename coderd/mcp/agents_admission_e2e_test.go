package mcp_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

func TestMCPHTTP_AgentsProductionAdmissionFence(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	clock := quartz.NewMock(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var hooks atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hooks.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer consumer.Close()
	releaseHook := sync.OnceFunc(func() { close(release) })
	defer releaseHook()
	values := mcpDeploymentValues(t)
	values.Experiments = append(values.Experiments, string(codersdk.ExperimentAgentLifecycleHooks))
	require.NoError(t, values.AI.Chat.HookURL.Set(consumer.URL))
	values.AI.Chat.HookEnabled = serpent.Bool(true)
	values.AI.Chat.HookAllowInsecure = serpent.Bool(true)
	values.AI.Chat.HookSecret = serpent.String("test-hook-secret-32-bytes-minimum!!")
	values.AI.Chat.HookTimeout = serpent.Duration(testutil.WaitLong)
	providerURL := chattest.OpenAI(t)
	keys := coderdtest.OpenAICompatProviderAPIKeys(providerURL)
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{Database: db, Pubsub: ps, ChatWorkerDisabled: true, DeploymentValues: values, ChatProviderAPIKeys: &keys})
	defer closer.Close()
	defer releaseHook()
	user := coderdtest.CreateFirstUser(t, client)
	exp := codersdk.NewExperimentalClient(client)
	model := coderdtest.CreateOpenAICompatChatModel(t, exp, providerURL)
	session, err := newIsolatedMCPClient(ctx, api.AccessURL.String()+mcpserver.MCPEndpoint, "admission-fence", map[string]string{"Authorization": "Bearer " + client.SessionToken()})
	require.NoError(t, err)
	defer session.Close()
	seed := func(state string) (database.WorkspaceExecutionSession, uuid.UUID) {
		build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{OwnerID: user.UserID, OrganizationID: user.OrganizationID}).Do()
		receipt, err := db.InsertWorkspaceExecutionSession(ctx, database.InsertWorkspaceExecutionSessionParams{ID: uuid.New(), OrganizationID: user.OrganizationID, OwnerID: user.UserID, ActorID: user.UserID, RequestID: uuid.New(), InputDigest: make([]byte, 32), WorkspaceID: uuid.NullUUID{UUID: build.Workspace.ID, Valid: true}, WorkspaceOwnerID: uuid.NullUUID{UUID: user.UserID, Valid: true}, CreatedAt: clock.Now(), State: state, Disposable: true, LeaseExpiresAt: clock.Now().Add(-time.Second), Declarations: []byte(`{"result_paths":[]}`)})
		require.NoError(t, err)
		return receipt, build.Workspace.ID
	}
	for _, state := range []string{"preserving"} {
		_, workspaceID := seed(state)
		create := toolsdk.CreateChatArgs{RequestID: uuid.NewString(), OrganizationID: user.OrganizationID.String(), WorkspaceID: workspaceID.String(), ModelConfigID: model.ID.String(), Prompt: "must reject"}
		response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameCreateChat, Arguments: create})
		require.NoError(t, err)
		require.True(t, response.IsError, "%+v", response.Content)
		require.Contains(t, response.Content[0].(*mcp.TextContent).Text, "admission is closed")
		// The real production callback must prevent reservation, not merely execution.
		_, err = api.Database.GetChatSubmission(dbauthz.AsSystemRestricted(ctx), database.GetChatSubmissionParams{OrganizationID: user.OrganizationID, ActorID: user.UserID, RequestID: uuid.MustParse(create.RequestID)})
		require.ErrorIs(t, err, sql.ErrNoRows)
		chat := dbgen.Chat(t, db, database.Chat{OrganizationID: user.OrganizationID, OwnerID: user.UserID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}, LastModelConfigID: model.ID})
		response, err = session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameSendChatMessage, Arguments: toolsdk.SendChatMessageArgs{RequestID: uuid.NewString(), ChatID: chat.ID.String(), Text: "must reject"}})
		require.NoError(t, err)
		require.True(t, response.IsError, "%+v", response.Content)
		require.Contains(t, response.Content[0].(*mcp.TextContent).Text, "admission is closed")
		messages, err := db.GetChatMessagesByChatIDAscPaginated(ctx, database.GetChatMessagesByChatIDAscPaginatedParams{ChatID: chat.ID, LimitVal: 10})
		require.NoError(t, err)
		require.Empty(t, messages)
	}
	require.Zero(t, hooks.Load())
	active, workspaceID := seed("active")
	request := toolsdk.CreateChatArgs{RequestID: uuid.NewString(), OrganizationID: user.OrganizationID.String(), WorkspaceID: workspaceID.String(), ModelConfigID: model.ID.String(), Prompt: "late admission"}
	done := make(chan *mcp.CallToolResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameCreateChat, Arguments: request})
		done <- result
		errs <- err
	}()
	testutil.RequireReceive(ctx, t, entered)

	controller := workspaceexec.NewController(workspaceexec.ControllerOptions{
		Database: db, Clock: clock, Logger: testutil.Logger(t), AllowDeletion: true,
		FileCache: api.FileCache, UsageChecker: func() wsbuilder.UsageChecker { return *api.BuildUsageChecker.Load() },
		Pubsub: ps, DeploymentValues: api.DeploymentValues, Experiments: api.Experiments,
	})
	require.NoError(t, controller.Reconcile(ctx, active.ID))
	current, err := db.GetWorkspaceExecutionSessionByID(ctx, active.ID)
	require.NoError(t, err)
	require.Equal(t, "deleting", current.State)
	require.True(t, current.DeleteBuildID.Valid)
	// Advance the durable deletion state before releasing the pending create.
	require.NoError(t, db.UpdateWorkspaceDeletedByID(ctx, database.UpdateWorkspaceDeletedByIDParams{ID: workspaceID, Deleted: true}))
	_, err = sqlDB.ExecContext(ctx, "UPDATE workspace_execution_sessions SET state = 'completed' WHERE id = $1", active.ID)
	require.NoError(t, err)
	// The journal is not activity. A hook returning after cleanup committed
	// must fail admission instead of creating work in the closing workspace.
	releaseHook()
	result := testutil.RequireReceive(ctx, t, done)
	require.NoError(t, testutil.RequireReceive(ctx, t, errs))
	require.True(t, result.IsError, "%+v", result.Content)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "admission is closed")
	journal, err := db.GetChatSubmission(ctx, database.GetChatSubmissionParams{OrganizationID: user.OrganizationID, ActorID: user.UserID, RequestID: uuid.MustParse(request.RequestID)})
	require.NoError(t, err)
	require.Equal(t, "rejected", journal.State)
	_, err = db.GetChatByID(ctx, journal.ChatID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	beforeReplay := hooks.Load()
	replayed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameCreateChat, Arguments: request})
	require.NoError(t, err)
	require.True(t, replayed.IsError)
	require.Equal(t, beforeReplay, hooks.Load(), "replay must not invoke the hook again")

	// Existing chats may recover from the same deleted workspace even though
	// new admission remains closed by the completed execution session.
	legacy := dbgen.Chat(t, db, database.Chat{OrganizationID: user.OrganizationID, OwnerID: user.UserID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}, LastModelConfigID: model.ID})
	recoveryRequestID := uuid.New()
	_, err = exp.CreateChatMessage(ctx, legacy.ID, codersdk.CreateChatMessageRequest{RequestID: &recoveryRequestID, Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "recover my workspace"}}})
	require.NoError(t, err)
	admitted, err := db.GetChatByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, admitted.Status)
}
