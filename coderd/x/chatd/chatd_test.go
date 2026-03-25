package chatd_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/provisioner/echo"
	proto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestInterruptChatBroadcastsStatusAcrossInstances(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replicaA := newTestServer(t, db, ps, uuid.New())
	replicaB := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replicaA.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "interrupt-me",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	runningWorker := uuid.New()
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: runningWorker, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	_, events, cancel, ok := replicaB.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	updated := replicaA.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)
	require.False(t, updated.WorkerID.Valid)

	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type == codersdk.ChatStreamEventTypeStatus && event.Status != nil {
				return event.Status.Status == codersdk.ChatStatusWaiting
			}
			t.Logf("skipping unexpected event: type=%s", event.Type)
			return false
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestSubagentChatExcludesWorkspaceProvisioningTools(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	_ = agenttest.New(t, client.URL, agentToken)

	// Track tools sent in LLM requests. The first call is for the
	// root chat which spawns a subagent; the second call is for the
	// subagent itself.
	var toolsMu sync.Mutex
	toolsByCall := make([][]string, 0, 2)

	var callCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}

		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		toolsMu.Lock()
		toolsByCall = append(toolsByCall, names)
		toolsMu.Unlock()

		if callCount.Add(1) == 1 {
			// Root chat: model calls spawn_agent.
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("spawn_agent", `{"prompt":"do the thing","title":"sub"}`),
			)
		}
		// Subsequent calls (including the subagent): just reply.
		// Include literal \u0000 in the response text, which is
		// what a real LLM writes when explaining binary output.
		// json.Marshal encodes the backslash as \\, producing
		// \\u0000 in the JSON bytes. The sanitizer must not
		// corrupt this into invalid JSON.
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("The file contains \\u0000 null bytes.")...,
		)
	})

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

	// Create a root chat whose first model call will spawn a subagent.
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Spawn a subagent to do the thing.",
			},
		},
	})
	require.NoError(t, err)

	// Wait for the root chat AND the subagent to finish.
	// The root chat finishes first, then the chatd server
	// picks up and runs the child (subagent) chat.
	require.Eventually(t, func() bool {
		got, getErr := expClient.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		if got.Status != codersdk.ChatStatusWaiting && got.Status != codersdk.ChatStatusError {
			return false
		}
		// Also ensure the subagent LLM call has been made.
		toolsMu.Lock()
		n := len(toolsByCall)
		toolsMu.Unlock()
		// Expect at least 3 calls: root-1 (spawn_agent), child-1, root-2.
		return n >= 3
	}, testutil.WaitLong, testutil.IntervalFast)

	// There should be at least two streamed calls: one for the root
	// chat and one for the subagent child chat.
	toolsMu.Lock()
	recorded := append([][]string(nil), toolsByCall...)
	toolsMu.Unlock()

	require.GreaterOrEqual(t, len(recorded), 2,
		"expected at least 2 streamed LLM calls (root + subagent)")

	workspaceTools := []string{"propose_plan", "list_templates", "read_template", "create_workspace"}
	subagentTools := []string{"spawn_agent", "wait_agent", "message_agent", "close_agent"}

	// Identify root and subagent calls. Root chat calls include
	// spawn_agent; the subagent call does not. Because the root chat
	// makes multiple LLM calls (before and after spawn_agent), we
	// find exactly one call that lacks spawn_agent — that's the
	// subagent.
	var rootCalls, childCalls [][]string
	for _, tools := range recorded {
		hasSpawnAgent := slice.Contains(tools, "spawn_agent")
		if hasSpawnAgent {
			rootCalls = append(rootCalls, tools)
		} else {
			childCalls = append(childCalls, tools)
		}
	}

	require.NotEmpty(t, rootCalls, "expected at least one root chat LLM call")
	require.NotEmpty(t, childCalls, "expected at least one subagent LLM call")

	// Root chat calls must include workspace and subagent tools.
	for _, tool := range workspaceTools {
		require.Contains(t, rootCalls[0], tool,
			"root chat should have workspace tool %q", tool)
	}
	for _, tool := range subagentTools {
		require.Contains(t, rootCalls[0], tool,
			"root chat should have subagent tool %q", tool)
	}

	// Subagent calls must NOT include workspace or subagent tools.
	for _, tool := range workspaceTools {
		require.NotContains(t, childCalls[0], tool,
			"subagent chat should NOT have workspace tool %q", tool)
	}
	for _, tool := range subagentTools {
		require.NotContains(t, childCalls[0], tool,
			"subagent chat should NOT have subagent tool %q", tool)
	}
}

func TestInterruptChatClearsWorkerInDatabase(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "db-transition",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	updated := replica.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)
	require.False(t, updated.WorkerID.Valid)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status)
	require.False(t, fromDB.WorkerID.Valid)
}

func TestUpdateChatHeartbeatRequiresOwnership(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "heartbeat-ownership",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	workerID := uuid.New()
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	rows, err := db.UpdateChatHeartbeat(ctx, database.UpdateChatHeartbeatParams{
		ID:       chat.ID,
		WorkerID: uuid.New(),
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), rows)

	rows, err = db.UpdateChatHeartbeat(ctx, database.UpdateChatHeartbeatParams{
		ID:       chat.ID,
		WorkerID: workerID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
}

func TestSendMessageQueueBehaviorQueuesWhenBusy(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "queue-when-busy",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	workerID := uuid.New()
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	result, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, result.Queued)
	require.NotNil(t, result.QueuedMessage)
	require.Equal(t, database.ChatStatusRunning, result.Chat.Status)
	require.Equal(t, workerID, result.Chat.WorkerID.UUID)
	require.True(t, result.Chat.WorkerID.Valid)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 1)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
}

func TestSendMessageQueuesWhenWaitingWithQueuedBacklog(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "queue-when-waiting-with-backlog",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("older queued"),
	})
	require.NoError(t, err)
	_, err = db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:  chat.ID,
		Content: queuedContent,
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusWaiting,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)

	result, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("newer queued")},
	})
	require.NoError(t, err)
	require.True(t, result.Queued)
	require.NotNil(t, result.QueuedMessage)
	require.Equal(t, database.ChatStatusWaiting, result.Chat.Status)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 2)

	olderSDK := db2sdk.ChatQueuedMessage(queued[0])
	require.Len(t, olderSDK.Content, 1)
	require.Equal(t, "older queued", olderSDK.Content[0].Text)

	newerSDK := db2sdk.ChatQueuedMessage(queued[1])
	require.Len(t, newerSDK.Content, 1)
	require.Equal(t, "newer queued", newerSDK.Content[0].Text)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
}

func TestSendMessageInterruptBehaviorQueuesAndInterruptsWhenBusy(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "interrupt-when-busy",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	result, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("interrupt")},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)

	// The message should be queued, not inserted directly.
	require.True(t, result.Queued)
	require.NotNil(t, result.QueuedMessage)

	// The chat should transition to waiting (interrupt signal),
	// not pending.
	require.Equal(t, database.ChatStatusWaiting, result.Chat.Status)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status)

	// The message should be in the queue, not in chat_messages.
	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 1)

	// Only the initial user message should be in chat_messages.
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
}

func TestEditMessageUpdatesAndTruncatesAndClearsQueue(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "edit-message",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("original")},
	})
	require.NoError(t, err)

	initialMessages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, initialMessages, 1)
	editedMessageID := initialMessages[0].ID

	_, err = replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("follow-up")},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)
	_, err = replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("another")},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)

	queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("queued"),
	})
	require.NoError(t, err)
	_, err = db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:  chat.ID,
		Content: queuedContent,
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	editResult, err := replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: editedMessageID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.NoError(t, err)
	// The edited message is soft-deleted and a new message is inserted,
	// so the returned message ID will differ from the original.
	require.NotEqual(t, editedMessageID, editResult.Message.ID)
	require.Equal(t, database.ChatStatusPending, editResult.Chat.Status)
	require.False(t, editResult.Chat.WorkerID.Valid)

	editedSDK := db2sdk.ChatMessage(editResult.Message)
	require.Len(t, editedSDK.Content, 1)
	require.Equal(t, "edited", editedSDK.Content[0].Text)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, editResult.Message.ID, messages[0].ID)
	onlyMessage := db2sdk.ChatMessage(messages[0])
	require.Len(t, onlyMessage.Content, 1)
	require.Equal(t, "edited", onlyMessage.Content[0].Text)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 0)

	chatFromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusPending, chatFromDB.Status)
	require.False(t, chatFromDB.WorkerID.Valid)
}

func TestCreateChatInsertsWorkspaceAwarenessMessage(t *testing.T) {
	t.Parallel()

	t.Run("WithWorkspace", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())

		ctx := testutil.Context(t, testutil.WaitLong)
		user, model := seedChatDependencies(ctx, t, db)

		org := dbgen.Organization(t, db, database.Organization{})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		tpl := dbgen.Template(t, db, database.Template{
			CreatedBy:       user.ID,
			OrganizationID:  org.ID,
			ActiveVersionID: tv.ID,
		})
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OwnerID:            user.ID,
			WorkspaceID:        uuid.NullUUID{UUID: workspace.ID, Valid: true},
			Title:              "test-with-workspace",
			ModelConfigID:      model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
		})
		require.NoError(t, err)

		messages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)

		var workspaceMsg *database.ChatMessage
		for _, msg := range messages {
			if msg.Role == database.ChatMessageRoleSystem {
				content := string(msg.Content.RawMessage)
				if strings.Contains(content, "attached to a workspace") {
					workspaceMsg = &msg
					break
				}
			}
		}
		require.NotNil(t, workspaceMsg, "workspace awareness system message should exist")
		require.Equal(t, database.ChatMessageRoleSystem, workspaceMsg.Role)
		require.Equal(t, database.ChatMessageVisibilityModel, workspaceMsg.Visibility)
	})

	t.Run("WithoutWorkspace", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())

		ctx := testutil.Context(t, testutil.WaitLong)
		user, model := seedChatDependencies(ctx, t, db)

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OwnerID:            user.ID,
			Title:              "test-without-workspace",
			ModelConfigID:      model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
		})
		require.NoError(t, err)

		messages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)

		var workspaceMsg *database.ChatMessage
		for _, msg := range messages {
			if msg.Role == database.ChatMessageRoleSystem {
				content := string(msg.Content.RawMessage)
				if strings.Contains(content, "no workspace associated") {
					workspaceMsg = &msg
					break
				}
			}
		}
		require.NotNil(t, workspaceMsg, "workspace awareness system message should exist")
		require.Equal(t, database.ChatMessageRoleSystem, workspaceMsg.Role)
		require.Equal(t, database.ChatMessageVisibilityModel, workspaceMsg.Visibility)
	})
}

func TestCreateChatRejectsWhenUsageLimitReached(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	existingChat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "existing-limit-chat",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              existingChat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{100},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	beforeChats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnerID:   user.ID,
		AfterID:   uuid.Nil,
		OffsetOpt: 0,
		LimitOpt:  100,
	})
	require.NoError(t, err)
	require.Len(t, beforeChats, 1)

	_, err = replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "over-limit",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.Error(t, err)

	var limitErr *chatd.UsageLimitExceededError
	require.ErrorAs(t, err, &limitErr)
	require.Equal(t, int64(100), limitErr.LimitMicros)
	require.Equal(t, int64(100), limitErr.ConsumedMicros)

	afterChats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnerID:   user.ID,
		AfterID:   uuid.Nil,
		OffsetOpt: 0,
		LimitOpt:  100,
	})
	require.NoError(t, err)
	require.Len(t, afterChats, len(beforeChats))
}

func TestPromoteQueuedAllowsAlreadyQueuedMessageWhenUsageLimitReached(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "queued-limit-reached",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	queuedResult, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, queuedResult.Queued)
	require.NotNil(t, queuedResult.QueuedMessage)

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{100},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusWaiting,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)

	result, err := replica.PromoteQueued(ctx, chatd.PromoteQueuedOptions{
		ChatID:          chat.ID,
		QueuedMessageID: queuedResult.QueuedMessage.ID,
		CreatedBy:       user.ID,
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatMessageRoleUser, result.PromotedMessage.Role)

	chat, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusPending, chat.Status)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, queued)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 3)
	require.Equal(t, database.ChatMessageRoleUser, messages[2].Role)
}

func TestInterruptAutoPromotionIgnoresLaterUsageLimitIncrease(t *testing.T) {
	t.Parallel()

	const acquireInterval = 10 * time.Millisecond

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	acquireTrap := clock.Trap().NewTicker("chatd", "acquire")
	defer acquireTrap.Close()

	assertPendingWithoutQueuedMessages := func(chatID uuid.UUID) {
		t.Helper()

		queued, dbErr := db.GetChatQueuedMessages(ctx, chatID)
		require.NoError(t, dbErr)
		require.Empty(t, queued)

		fromDB, dbErr := db.GetChatByID(ctx, chatID)
		require.NoError(t, dbErr)
		require.Equal(t, database.ChatStatusPending, fromDB.Status)
		require.False(t, fromDB.WorkerID.Valid)
	}

	streamStarted := make(chan struct{})
	interrupted := make(chan struct{})
	secondRequestStarted := make(chan struct{})
	thirdRequestStarted := make(chan struct{})
	allowFinish := make(chan struct{})
	var requestCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		switch requestCount.Add(1) {
		case 1:
			chunks := make(chan chattest.OpenAIChunk, 1)
			go func() {
				defer close(chunks)
				chunks <- chattest.OpenAITextChunks("partial")[0]
				select {
				case <-streamStarted:
				default:
					close(streamStarted)
				}
				<-req.Context().Done()
				select {
				case <-interrupted:
				default:
					close(interrupted)
				}
				<-allowFinish
			}()
			return chattest.OpenAIResponse{StreamingChunks: chunks}
		case 2:
			close(secondRequestStarted)
		case 3:
			close(thirdRequestStarted)
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.Clock = clock
		cfg.PendingChatAcquireInterval = acquireInterval
		cfg.InFlightChatStaleAfter = testutil.WaitSuperLong
	})
	acquireTrap.MustWait(ctx).MustRelease(ctx)

	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "interrupt-autopromote-limit",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	clock.Advance(acquireInterval).MustWait(ctx)
	testutil.TryReceive(ctx, t, streamStarted)

	queuedResult, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)
	require.True(t, queuedResult.Queued)
	require.NotNil(t, queuedResult.QueuedMessage)

	testutil.TryReceive(ctx, t, interrupted)

	close(allowFinish)
	chatd.WaitUntilIdleForTest(server)
	assertPendingWithoutQueuedMessages(chat.ID)

	// Keep the acquire loop frozen here so "queued" stays pending.
	// That makes the later send queue because the chat is still busy,
	// rather than because the scheduler happened to be slow.
	laterQueuedResult, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("later queued")},
	})
	require.NoError(t, err)
	require.True(t, laterQueuedResult.Queued)
	require.NotNil(t, laterQueuedResult.QueuedMessage)

	spendChat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{},
		ParentChatID:      uuid.NullUUID{},
		RootChatID:        uuid.NullUUID{},
		LastModelConfigID: model.ID,
		Title:             "other-spend",
		Mode:              database.NullChatMode{},
	})
	require.NoError(t, err)

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("spent elsewhere"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              spendChat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{100},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	clock.Advance(acquireInterval).MustWait(ctx)
	testutil.TryReceive(ctx, t, secondRequestStarted)
	chatd.WaitUntilIdleForTest(server)
	assertPendingWithoutQueuedMessages(chat.ID)

	clock.Advance(acquireInterval).MustWait(ctx)
	testutil.TryReceive(ctx, t, thirdRequestStarted)
	chatd.WaitUntilIdleForTest(server)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, queued)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status)
	require.False(t, fromDB.WorkerID.Valid)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)

	userTexts := make([]string, 0, 3)
	for _, message := range messages {
		if message.Role != database.ChatMessageRoleUser {
			continue
		}
		sdkMessage := db2sdk.ChatMessage(message)
		if len(sdkMessage.Content) != 1 {
			continue
		}
		userTexts = append(userTexts, sdkMessage.Content[0].Text)
	}
	require.Equal(t, []string{"hello", "queued", "later queued"}, userTexts)
}

func TestEditMessageRejectsWhenUsageLimitReached(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "edit-limit-reached",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("original")},
	})
	require.NoError(t, err)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	editedMessageID := messages[0].ID

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{100},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: editedMessageID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.Error(t, err)

	var limitErr *chatd.UsageLimitExceededError
	require.ErrorAs(t, err, &limitErr)
	require.Equal(t, int64(100), limitErr.LimitMicros)
	require.Equal(t, int64(100), limitErr.ConsumedMicros)

	messages, err = db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	originalMessage := db2sdk.ChatMessage(messages[0])
	require.Len(t, originalMessage.Content, 1)
	require.Equal(t, "original", originalMessage.Content[0].Text)
}

func TestEditMessageRejectsMissingMessage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "missing-edited-message",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: 999999,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, chatd.ErrEditedMessageNotFound))
}

func TestEditMessageRejectsNonUserMessage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "non-user-edited-message",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	assistantMessages, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)
	assistantMessage := assistantMessages[0]

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: assistantMessage.ID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, chatd.ErrEditedMessageNotUser))
}

func TestRecoverStaleChatsPeriodically(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Use a very short stale threshold so the periodic recovery
	// kicks in quickly during the test.
	staleAfter := 500 * time.Millisecond

	// Create a chat and simulate a dead worker by setting the chat
	// to running with a heartbeat in the past.
	deadWorkerID := uuid.New()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "stale-recovery-periodic",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: deadWorkerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	require.NoError(t, err)

	// Start a new replica. Its startup recovery will reset the
	// chat (since the heartbeat is old), but the key point is that
	// the periodic loop also recovers newly-stale chats.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitLong,
		InFlightChatStaleAfter:     staleAfter,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// The startup recovery should have already reset our stale
	// chat.
	require.Eventually(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending
	}, testutil.WaitMedium, testutil.IntervalFast)

	// Now simulate a second stale chat appearing AFTER startup.
	// This tests the periodic recovery, not just the startup one.
	deadWorkerID2 := uuid.New()
	chat2, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "stale-recovery-periodic-2",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat2.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: deadWorkerID2, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	require.NoError(t, err)

	// The periodic stale recovery loop (running at staleAfter/5 =
	// 100ms intervals) should pick this up without a restart.
	require.Eventually(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat2.ID)
		if err != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestNewReplicaRecoversStaleChatFromDeadReplica(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Simulate a chat left running by a dead replica with a stale
	// heartbeat (well beyond the stale threshold).
	deadReplicaID := uuid.New()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "orphaned-chat",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	// Set the heartbeat far in the past so it's definitely stale.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: deadReplicaID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	require.NoError(t, err)

	// Start a new replica — it should recover the stale chat on
	// startup.
	newReplica := newTestServer(t, db, ps, uuid.New())
	_ = newReplica

	require.Eventually(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending &&
			!fromDB.WorkerID.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestWaitingChatsAreNotRecoveredAsStale(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Create a chat in waiting status — this should NOT be touched
	// by stale recovery.
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "waiting-chat",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	// Start a replica with a short stale threshold.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitLong,
		InFlightChatStaleAfter:     500 * time.Millisecond,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// Wait long enough for multiple periodic recovery cycles to
	// run (staleAfter/5 = 100ms intervals).
	require.Never(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return fromDB.Status != database.ChatStatusWaiting
	}, time.Second, testutil.IntervalFast,
		"waiting chat should not be modified by stale recovery")
}

func TestUpdateChatStatusPersistsLastError(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	_ = newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "error-persisted",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	// Simulate a chat that failed with an error.
	errorMessage := "stream response: status 500: internal server error"
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusError,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{String: errorMessage, Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, chat.Status)
	require.Equal(t, sql.NullString{String: errorMessage, Valid: true}, chat.LastError)

	// Verify the error is persisted when re-read from the database.
	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, fromDB.Status)
	require.Equal(t, sql.NullString{String: errorMessage, Valid: true}, fromDB.LastError)

	// Verify the error is cleared when the chat transitions to a
	// non-error status (e.g. pending after a retry).
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusPending,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusPending, chat.Status)
	require.False(t, chat.LastError.Valid)

	fromDB, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, fromDB.LastError.Valid)
}

func TestSubscribeSnapshotIncludesStatusEvent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "status-snapshot",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	snapshot, _, cancel, ok := replica.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// The first event in the snapshot must be a status event.
	require.NotEmpty(t, snapshot)
	require.Equal(t, codersdk.ChatStreamEventTypeStatus, snapshot[0].Type)
	require.NotNil(t, snapshot[0].Status)
	require.Equal(t, codersdk.ChatStatusPending, snapshot[0].Status.Status)
}

func TestPersistToolResultWithBinaryData(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const binaryOutputBase64 = "SEVBREVSAAAAc29tZSBkYXRhAABtb3JlIGRhdGEARU5E"
	binaryOutput, err := io.ReadAll(base64.NewDecoder(
		base64.StdEncoding,
		strings.NewReader(binaryOutputBase64),
	))
	require.NoError(t, err)

	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([][]chattest.OpenAIMessage, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Binary tool result test")
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, append([]chattest.OpenAIMessage(nil), req.Messages...))
		streamedCallsMu.Unlock()

		if streamedCallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"execute",
					`{"command":"cat /home/coder/binary_file.bin"}`,
				),
			)
		}
		// Include literal \u0000 in the response text, which is
		// what a real LLM writes when explaining binary output.
		// json.Marshal encodes the backslash as \\, producing
		// \\u0000 in the JSON bytes. The sanitizer must not
		// corrupt this into invalid JSON.
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("The file contains \\u0000 null bytes.")...,
		)
	})

	// Use "openai-compat" provider so the chatd framework uses the
	// /chat/completions endpoint, where the mock server supports
	// streaming tool calls. The default "openai" provider routes to
	// /responses which only handles text deltas in the mock.
	user, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().
		SetExtraHeaders(gomock.Any()).
		AnyTimes()
	mockConn.EXPECT().
		LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, nil).
		AnyTimes()
	mockConn.EXPECT().
		ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).
		AnyTimes()
	mockConn.EXPECT().
		StartProcess(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
			require.Equal(t, "cat /home/coder/binary_file.bin", req.Command)
			return workspacesdk.StartProcessResponse{ID: "proc-binary", Started: true}, nil
		}).
		Times(1)
	mockConn.EXPECT().
		ProcessOutput(gomock.Any(), "proc-binary", gomock.Any()).
		Return(workspacesdk.ProcessOutputResponse{
			Output:   string(binaryOutput),
			Running:  false,
			ExitCode: ptrRef(0),
		}, nil).
		AnyTimes()

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:       user.ID,
		Title:         "binary-tool-result",
		ModelConfigID: model.ID,
		WorkspaceID:   uuid.NullUUID{UUID: ws.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Read /home/coder/binary_file.bin."),
		},
	})
	require.NoError(t, err)

	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat run failed", "last_error=%q", chatResult.LastError.String)
	}

	var toolMessage *database.ChatMessage
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for i := range messages {
			if messages[i].Role == database.ChatMessageRoleTool {
				toolMessage = &messages[i]
				return true
			}
		}
		return false
	}, testutil.IntervalFast)
	require.NotNil(t, toolMessage)

	parts, err := chatprompt.ParseContent(*toolMessage)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[0].Type)
	require.Equal(t, "execute", parts[0].ToolName)

	var result chattool.ExecuteResult
	require.NoError(t, json.Unmarshal(parts[0].Result, &result))
	require.True(t, result.Success)
	require.Equal(t, string(binaryOutput), result.Output)
	require.Equal(t, 0, result.ExitCode)

	require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))
	streamedCallsMu.Lock()
	recordedStreamCalls := append([][]chattest.OpenAIMessage(nil), streamedCalls...)
	streamedCallsMu.Unlock()
	require.GreaterOrEqual(t, len(recordedStreamCalls), 2)

	var foundToolResultInSecondCall bool
	for _, message := range recordedStreamCalls[1] {
		if message.Role != "tool" {
			continue
		}
		if !json.Valid([]byte(message.Content)) {
			continue
		}
		var result chattool.ExecuteResult
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		if result.Output == string(binaryOutput) {
			foundToolResultInSecondCall = true
			break
		}
	}
	require.True(t, foundToolResultInSecondCall, "expected second streamed model call to include execute tool output")
}

func ptrRef[T any](v T) *T {
	return &v
}

func TestSubscribeNoPubsubNoDuplicateMessageParts(t *testing.T) {
	t.Parallel()

	// Use nil pubsub to force the no-pubsub path.
	db, _ := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, nil, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "no-dup-parts",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	snapshot, events, cancel, ok := replica.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Snapshot should have events (at minimum: status + message).
	require.NotEmpty(t, snapshot)

	// The events channel should NOT immediately produce any
	// events — the snapshot already contained everything. Before
	// the fix, localSnapshot was replayed into the channel,
	// causing duplicates.
	require.Never(t, func() bool {
		select {
		case <-events:
			return true
		default:
			return false
		}
	}, 200*time.Millisecond, testutil.IntervalFast,
		"expected no duplicate events after snapshot")
}

func TestSubscribeAfterMessageID(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Create a chat — this inserts one initial "user" message.
	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "after-id-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("first")},
	})
	require.NoError(t, err)

	// Insert two more messages so we have three total visible
	// messages (the initial user message plus these two).
	secondContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("second"),
	})
	require.NoError(t, err)

	msg2Results, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(secondContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)
	msg2 := msg2Results[0]

	thirdContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("third"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleUser},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(thirdContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	// Control: Subscribe with afterMessageID=0 returns ALL messages.
	allSnapshot, _, cancelAll, ok := replica.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	cancelAll()

	allMessages := filterMessageEvents(allSnapshot)
	require.Len(t, allMessages, 3, "afterMessageID=0 should return all three messages")

	// Subscribe with afterMessageID set to the second message's ID.
	// Only the third message (inserted after msg2) should appear.
	partialSnapshot, _, cancelPartial, ok := replica.Subscribe(ctx, chat.ID, nil, msg2.ID)
	require.True(t, ok)
	cancelPartial()

	partialMessages := filterMessageEvents(partialSnapshot)
	require.Len(t, partialMessages, 1, "afterMessageID=msg2.ID should return only messages after msg2")
	require.Equal(t, codersdk.ChatMessageRoleUser, partialMessages[0].Message.Role)
}

// filterMessageEvents returns only the Message-type events from a
// snapshot slice, which is useful for ignoring status / queue events.
func filterMessageEvents(events []codersdk.ChatStreamEvent) []codersdk.ChatStreamEvent {
	return slice.Filter(events, func(e codersdk.ChatStreamEvent) bool {
		return e.Type == codersdk.ChatStreamEventTypeMessage
	})
}

func TestCreateWorkspaceTool_EndToEnd(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	agentToken := uuid.NewString()
	// Add a startup script so the agent spends time in the
	// "starting" lifecycle state. This lets us verify that
	// create_workspace waits for scripts to finish.
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken, func(g *proto.GraphComplete) {
			g.Resources[0].Agents[0].Scripts = []*proto.Script{{
				DisplayName: "setup",
				Script:      "sleep 5",
				RunOnStart:  true,
			}}
		}),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	// Start the test workspace agent so create_workspace can wait for
	// the agent to become reachable before returning.
	_ = agenttest.New(t, client.URL, agentToken)

	workspaceName := "chat-ws-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	createWorkspaceArgs := fmt.Sprintf(
		`{"template_id":%q,"name":%q}`,
		template.ID.String(),
		workspaceName,
	)

	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([][]chattest.OpenAIMessage, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Create workspace test")
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, append([]chattest.OpenAIMessage(nil), req.Messages...))
		streamedCallsMu.Unlock()

		if streamedCallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("create_workspace", createWorkspaceArgs),
			)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Workspace created and ready.")...,
		)
	})

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Create a workspace from the template and continue.",
			},
		},
	})
	require.NoError(t, err)

	var chatResult codersdk.Chat
	require.Eventually(t, func() bool {
		got, getErr := expClient.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == codersdk.ChatStatusWaiting || got.Status == codersdk.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == codersdk.ChatStatusError {
		lastError := ""
		if chatResult.LastError != nil {
			lastError = *chatResult.LastError
		}
		require.FailNowf(t, "chat run failed", "last_error=%q", lastError)
	}

	require.NotNil(t, chatResult.WorkspaceID)
	workspaceID := *chatResult.WorkspaceID
	workspace, err := client.Workspace(ctx, workspaceID)
	require.NoError(t, err)
	require.Equal(t, workspaceName, workspace.Name)

	chatMsgs, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)

	var foundCreateWorkspaceResult bool
	for _, message := range chatMsgs.Messages {
		if message.Role != codersdk.ChatMessageRoleTool {
			continue
		}
		for _, part := range message.Content {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.ToolName != "create_workspace" {
				continue
			}
			var result map[string]any
			require.NoError(t, json.Unmarshal(part.Result, &result))
			created, ok := result["created"].(bool)
			require.True(t, ok)
			require.True(t, created)
			foundCreateWorkspaceResult = true
		}
	}
	require.True(t, foundCreateWorkspaceResult, "expected create_workspace tool result message")

	// Verify that the tool waited for startup scripts to
	// complete. The agent should be in "ready" state by the
	// time create_workspace returns its result.
	workspace, err = client.Workspace(ctx, workspaceID)
	require.NoError(t, err)
	var agentLifecycle codersdk.WorkspaceAgentLifecycle
	for _, res := range workspace.LatestBuild.Resources {
		for _, agt := range res.Agents {
			agentLifecycle = agt.LifecycleState
		}
	}
	require.Equal(t, codersdk.WorkspaceAgentLifecycleReady, agentLifecycle,
		"agent should be ready after create_workspace returns; startup scripts were not awaited")

	require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))
	streamedCallsMu.Lock()
	recordedStreamCalls := append([][]chattest.OpenAIMessage(nil), streamedCalls...)
	streamedCallsMu.Unlock()
	require.GreaterOrEqual(t, len(recordedStreamCalls), 2)

	var foundToolResultInSecondCall bool
	for _, message := range recordedStreamCalls[1] {
		if message.Role != "tool" {
			continue
		}
		if !json.Valid([]byte(message.Content)) {
			continue
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		created, ok := result["created"].(bool)
		if ok && created {
			foundToolResultInSecondCall = true
			break
		}
	}
	require.True(t, foundToolResultInSecondCall, "expected second streamed model call to include create_workspace tool output")
}

func TestStartWorkspaceTool_EndToEnd(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	// Create a workspace, then stop it so start_workspace has
	// something to start. We intentionally skip starting a test
	// agent — the echo provisioner creates new agent rows for each
	// build, so an agent started for build 1 cannot serve build 3.
	// The tool handles the no-agent case gracefully.
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	workspace = coderdtest.MustTransitionWorkspace(
		t, client, workspace.ID,
		codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop,
	)

	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([][]chattest.OpenAIMessage, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Start workspace test")
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, append([]chattest.OpenAIMessage(nil), req.Messages...))
		streamedCallsMu.Unlock()

		if streamedCallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("start_workspace", "{}"),
			)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Workspace started and ready.")...,
		)
	})

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

	// Create a chat with the stopped workspace pre-associated.
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Start the workspace.",
			},
		},
		WorkspaceID: &workspace.ID,
	})
	require.NoError(t, err)

	var chatResult codersdk.Chat
	require.Eventually(t, func() bool {
		got, getErr := expClient.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == codersdk.ChatStatusWaiting || got.Status == codersdk.ChatStatusError
	}, testutil.WaitSuperLong, testutil.IntervalFast)

	if chatResult.Status == codersdk.ChatStatusError {
		lastError := ""
		if chatResult.LastError != nil {
			lastError = *chatResult.LastError
		}
		require.FailNowf(t, "chat run failed", "last_error=%q", lastError)
	}

	// Verify the workspace was started.
	require.NotNil(t, chatResult.WorkspaceID)
	updatedWorkspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkspaceTransitionStart, updatedWorkspace.LatestBuild.Transition)

	chatMsgs, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)

	// Verify start_workspace tool result exists in the chat messages.
	var foundStartWorkspaceResult bool
	for _, message := range chatMsgs.Messages {
		if message.Role != codersdk.ChatMessageRoleTool {
			continue
		}
		for _, part := range message.Content {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.ToolName != "start_workspace" {
				continue
			}
			var result map[string]any
			require.NoError(t, json.Unmarshal(part.Result, &result))
			started, ok := result["started"].(bool)
			require.True(t, ok)
			require.True(t, started)
			foundStartWorkspaceResult = true
		}
	}
	require.True(t, foundStartWorkspaceResult, "expected start_workspace tool result message")

	// Verify the LLM received the tool result in its second call.
	require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))
	streamedCallsMu.Lock()
	recordedStreamCalls := append([][]chattest.OpenAIMessage(nil), streamedCalls...)
	streamedCallsMu.Unlock()
	require.GreaterOrEqual(t, len(recordedStreamCalls), 2)

	var foundToolResultInSecondCall bool
	for _, message := range recordedStreamCalls[1] {
		if message.Role != "tool" {
			continue
		}
		if !json.Valid([]byte(message.Content)) {
			continue
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		started, ok := result["started"].(bool)
		if ok && started {
			foundToolResultInSecondCall = true
			break
		}
	}
	require.True(t, foundToolResultInSecondCall, "expected second streamed model call to include start_workspace tool output")
}

func TestHeartbeatBumpsWorkspaceUsage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}
		// Block until the request context is canceled so the chat
		// stays in a processing state long enough for heartbeats
		// to fire.
		chunks := make(chan chattest.OpenAIChunk)
		go func() {
			defer close(chunks)
			<-req.Context().Done()
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	}))

	// Create a workspace with a full build chain so we can verify
	// both last_used_at (dormancy) and deadline (autostop) bumps.
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
		CreatedBy:       user.ID,
	})
	require.NoError(t, db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
		ID:                tmpl.ID,
		UpdatedAt:         dbtime.Now(),
		AllowUserAutostop: true,
		ActivityBump:      int64(time.Hour),
	}))
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     tmpl.ID,
		Ttl:            sql.NullInt64{Valid: true, Int64: int64(8 * time.Hour)},
	})
	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		CompletedAt: sql.NullTime{
			Valid: true,
			Time:  dbtime.Now().Add(-30 * time.Minute),
		},
	})
	// Build deadline is 30 minutes in the past — close enough to
	// be bumped by the default 1-hour activity bump.
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             pj.ID,
		Transition:        database.WorkspaceTransitionStart,
		Deadline:          dbtime.Now().Add(-30 * time.Minute),
	})
	originalDeadline := build.Deadline

	// Set up a short heartbeat interval and a UsageTracker that
	// flushes frequently so last_used_at gets updated in the DB.
	flushTick := make(chan time.Time)
	flushDone := make(chan int, 1)
	tracker := workspacestats.NewTracker(db,
		workspacestats.TrackerWithTickFlush(flushTick, flushDone),
		workspacestats.TrackerWithLogger(slogtest.Make(t, nil)),
	)
	t.Cleanup(func() { tracker.Close() })

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	// Wrap the database with dbauthz so the chatd server's
	// AsChatd context is enforced on every query, matching
	// production behavior.
	authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), coderdtest.AccessControlStorePointer())
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   authzDB,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
		ChatHeartbeatInterval:      100 * time.Millisecond,
		UsageTracker:               tracker,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// Create a chat WITHOUT a workspace, the normal starting state.
	// In production, CreateChat is called from the HTTP handler with
	// the authenticated user's context. Here we use AsChatd since
	// the chatd server processes everything under that role.
	chatCtx := dbauthz.AsChatd(ctx)
	chat, err := server.CreateChat(chatCtx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "usage-tracking-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Wait for the chat to start processing and at least one
	// heartbeat to fire.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, listErr := db.GetChatByID(ctx, chat.ID)
		if listErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusRunning &&
			fromDB.HeartbeatAt.Valid &&
			fromDB.HeartbeatAt.Time.After(fromDB.CreatedAt)
	}, testutil.IntervalFast,
		"chat should be running with at least one heartbeat")

	// Flush the tracker and verify nothing was tracked yet
	// (no workspace linked).
	testutil.RequireSend(ctx, t, flushTick, time.Now())
	count := testutil.RequireReceive(ctx, t, flushDone)
	require.Equal(t, 0, count,
		"expected no workspaces to be flushed before association")

	// Link the workspace to the chat in the DB, simulating what
	// the create_workspace tool does mid-conversation.
	_, err = db.UpdateChatWorkspace(ctx, database.UpdateChatWorkspaceParams{
		WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
		ID:          chat.ID,
	})
	require.NoError(t, err)

	// The heartbeat re-reads the workspace association from the DB
	// on each tick. Wait for the tracker to pick it up.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		select {
		case flushTick <- time.Now():
		case <-ctx.Done():
			return false
		}
		select {
		case c := <-flushDone:
			return c > 0
		case <-ctx.Done():
			return false
		}
	}, testutil.IntervalMedium,
		"expected usage tracker to flush the late-associated workspace")

	// Verify the workspace's last_used_at was actually updated.
	updatedWs, err := db.GetWorkspaceByID(ctx, ws.ID)
	require.NoError(t, err)
	require.True(t, updatedWs.LastUsedAt.After(ws.LastUsedAt),
		"workspace last_used_at should have been bumped")

	// Verify the workspace build deadline was also extended.
	// The SQL only writes when 5% of the deadline has elapsed —
	// most calls perform a read-only CTE lookup. Wider ±2
	// minute tolerance than activitybump_test.go because the bump
	// happens asynchronously via the heartbeat goroutine.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		updatedBuild, buildErr := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, ws.ID)
		if buildErr != nil || !updatedBuild.Deadline.After(originalDeadline) {
			return false
		}
		now := dbtime.Now()
		return updatedBuild.Deadline.After(now.Add(time.Hour-2*time.Minute)) &&
			updatedBuild.Deadline.Before(now.Add(time.Hour+2*time.Minute))
	}, testutil.IntervalFast,
		"workspace build deadline should have been bumped to ~now+1h")
}

func TestHeartbeatNoWorkspaceNoBump(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}
		chunks := make(chan chattest.OpenAIChunk)
		go func() {
			defer close(chunks)
			<-req.Context().Done()
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	}))

	// Set up UsageTracker with manual tick/flush.
	usageTickCh := make(chan time.Time)
	flushCh := make(chan int, 1)
	tracker := workspacestats.NewTracker(db,
		workspacestats.TrackerWithTickFlush(usageTickCh, flushCh),
		workspacestats.TrackerWithLogger(slogtest.Make(t, nil)),
	)
	t.Cleanup(func() { tracker.Close() })

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
		ChatHeartbeatInterval:      100 * time.Millisecond,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// Create a chat WITHOUT linking a workspace.
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "no-workspace-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Wait for the chat to be acquired and at least one heartbeat
	// to fire.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, listErr := db.GetChatByID(ctx, chat.ID)
		if listErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusRunning &&
			fromDB.HeartbeatAt.Valid &&
			fromDB.HeartbeatAt.Time.After(fromDB.CreatedAt)
	}, testutil.IntervalFast,
		"chat should be running with at least one heartbeat")

	// Flush the tracker. Since no workspace was linked, count
	// should be 0.
	testutil.RequireSend(ctx, t, usageTickCh, time.Now())
	count := testutil.RequireReceive(ctx, t, flushCh)
	require.Equal(t, 0, count, "expected no workspaces to be flushed when chat has no workspace")
}

func newTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	replicaID uuid.UUID,
) *chatd.Server {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  replicaID,
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitLong,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

// newActiveTestServer creates a chatd server that actively polls for
// and processes pending chats. Use this instead of newTestServer when
// the test needs the chat loop to actually run. Optional config
// overrides are applied after the defaults.
func newActiveTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	overrides ...func(*chatd.Config),
) *chatd.Server {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	cfg := chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
	}
	for _, o := range overrides {
		o(&cfg)
	}
	server := chatd.New(cfg)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

func seedChatDependencies(
	ctx context.Context,
	t *testing.T,
	db database.Store,
) (database.User, database.ChatModelConfig) {
	t.Helper()
	return seedChatDependenciesWithProvider(ctx, t, db, "openai", "")
}

// seedChatDependenciesWithProvider creates a user, chat provider, and
// model config for the given provider type and base URL.
func seedChatDependenciesWithProvider(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	provider string,
	baseURL string,
) (database.User, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    provider,
		DisplayName: provider,
		APIKey:      "test-key",
		BaseUrl:     baseURL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	})
	require.NoError(t, err)
	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             provider,
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	return user, model
}

// seedWorkspaceWithAgent creates a full workspace chain with a connected
// agent. This is the common setup needed by tests that exercise tool
// execution against a workspace.
func seedWorkspaceWithAgent(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
) (database.WorkspaceTable, database.WorkspaceAgent) {
	t.Helper()

	org := dbgen.Organization(t, db, database.Organization{})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      userID,
	})
	tpl := dbgen.Template(t, db, database.Template{
		CreatedBy:       userID,
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     tpl.ID,
		OwnerID:        userID,
		OrganizationID: org.ID,
	})
	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		InitiatorID:    userID,
		OrganizationID: org.ID,
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		TemplateVersionID: tv.ID,
		WorkspaceID:       ws.ID,
		JobID:             pj.ID,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: res.ID,
	})
	return ws, agent
}

func setOpenAIProviderBaseURL(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	baseURL string,
) {
	t.Helper()

	provider, err := db.GetChatProviderByProvider(ctx, "openai")
	require.NoError(t, err)

	_, err = db.UpdateChatProvider(ctx, database.UpdateChatProviderParams{
		ID:          provider.ID,
		DisplayName: provider.DisplayName,
		APIKey:      provider.APIKey,
		BaseUrl:     baseURL,
		ApiKeyKeyID: provider.ApiKeyKeyID,
		Enabled:     provider.Enabled,
	})
	require.NoError(t, err)
}

func TestInterruptChatDoesNotSendWebPushNotification(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Set up a mock OpenAI that blocks until the request context is
	// canceled (i.e. until the chat is interrupted).
	streamStarted := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunks := make(chan chattest.OpenAIChunk, 1)
		go func() {
			defer close(chunks)
			chunks <- chattest.OpenAITextChunks("partial")[0]
			select {
			case <-streamStarted:
			default:
				close(streamStarted)
			}
			// Block until the chat context is canceled by the interrupt.
			<-req.Context().Done()
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	})

	// Mock webpush dispatcher that records calls.
	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "interrupt-no-push",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Wait for the chat to be picked up and start streaming.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusRunning && fromDB.WorkerID.Valid
	}, testutil.IntervalFast)

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		select {
		case <-streamStarted:
			return true
		default:
			return false
		}
	}, testutil.IntervalFast)

	// Interrupt the chat.
	updated := server.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)

	// Wait for the chat to finish processing and return to waiting.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusWaiting && !fromDB.WorkerID.Valid
	}, testutil.IntervalFast)

	// Verify no web push notification was dispatched.
	require.Equal(t, int32(0), mockPush.dispatchCount.Load(),
		"expected no web push dispatch for an interrupted chat")
}

// mockWebpushDispatcher implements webpush.Dispatcher and records Dispatch calls.
type mockWebpushDispatcher struct {
	dispatchCount atomic.Int32
	mu            sync.Mutex
	lastMessage   codersdk.WebpushMessage
	lastUserID    uuid.UUID
}

func (m *mockWebpushDispatcher) Dispatch(_ context.Context, userID uuid.UUID, msg codersdk.WebpushMessage) error {
	m.dispatchCount.Add(1)
	m.mu.Lock()
	m.lastMessage = msg
	m.lastUserID = userID
	m.mu.Unlock()
	return nil
}

func (m *mockWebpushDispatcher) getLastMessage() codersdk.WebpushMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastMessage
}

func (*mockWebpushDispatcher) Test(_ context.Context, _ codersdk.WebpushSubscription) error {
	return nil
}

func (*mockWebpushDispatcher) PublicKey() string {
	return "test-vapid-public-key"
}

func TestSuccessfulChatSendsWebPushWithNavigationData(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Set up a mock OpenAI that returns a simple successful response.
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	// Mock webpush dispatcher that captures the dispatched message.
	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "push-nav-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Wait for the chat to complete and return to waiting status.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusWaiting && !fromDB.WorkerID.Valid && mockPush.dispatchCount.Load() == 1
	}, testutil.IntervalFast)

	// Verify a web push notification was dispatched exactly once.
	require.Equal(t, int32(1), mockPush.dispatchCount.Load(),
		"expected exactly one web push dispatch for a completed chat")

	// Verify the notification was sent to the correct user.
	mockPush.mu.Lock()
	capturedMsg := mockPush.lastMessage
	capturedUserID := mockPush.lastUserID
	mockPush.mu.Unlock()

	require.Equal(t, user.ID, capturedUserID,
		"web push should be dispatched to the chat owner")

	// Verify the Data field contains the correct navigation URL.
	expectedURL := fmt.Sprintf("/agents/%s", chat.ID)
	require.Equal(t, expectedURL, capturedMsg.Data["url"],
		"web push Data should contain the chat navigation URL")
}

func TestCloseDuringShutdownContextCanceledShouldRetryOnNewReplica(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var requestCount atomic.Int32
	streamStarted := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		// Ignore non-streaming requests (e.g. title generation) so
		// they don't interfere with the request counter used to
		// coordinate the streaming chat flow.
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("shutdown-retry")
		}
		if requestCount.Add(1) == 1 {
			chunks := make(chan chattest.OpenAIChunk, 1)
			go func() {
				defer close(chunks)
				chunks <- chattest.OpenAITextChunks("partial")[0]
				select {
				case <-streamStarted:
				default:
					close(streamStarted)
				}
				<-req.Context().Done()
			}()
			return chattest.OpenAIResponse{StreamingChunks: chunks}
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("retry", " complete")...)
	})

	loggerA := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	serverA := chatd.New(chatd.Config{
		Logger:                     loggerA,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
	})
	t.Cleanup(func() {
		require.NoError(t, serverA.Close())
	})

	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := serverA.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "shutdown-retry",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusRunning && fromDB.WorkerID.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.Eventually(t, func() bool {
		select {
		case <-streamStarted:
			return true
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.NoError(t, serverA.Close())

	require.Eventually(t, func() bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending &&
			!fromDB.WorkerID.Valid &&
			!fromDB.LastError.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)

	loggerB := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	serverB := chatd.New(chatd.Config{
		Logger:                     loggerB,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
	})
	t.Cleanup(func() {
		require.NoError(t, serverB.Close())
	})

	require.Eventually(t, func() bool {
		return requestCount.Load() >= 2
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.Eventually(t, func() bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusWaiting &&
			!fromDB.WorkerID.Valid &&
			!fromDB.LastError.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestSuccessfulChatSendsWebPushWithSummary(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const assistantText = "I have completed the task successfully and all tests are passing now."
	const summaryText = "Completed task and verified all tests pass."

	var nonStreamingRequests atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			nonStreamingRequests.Add(1)
			return chattest.OpenAINonStreamingResponse(summaryText)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks(assistantText)...,
		)
	})

	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	_, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "summary-push-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do the thing")},
	})
	require.NoError(t, err)

	// The push notification is dispatched asynchronously after the
	// chat finishes, so we poll for it rather than checking
	// immediately after the status transitions to waiting.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return mockPush.dispatchCount.Load() >= 1
	}, testutil.IntervalFast)

	msg := mockPush.getLastMessage()
	require.Equal(t, summaryText, msg.Body,
		"push body should be the LLM-generated summary")
	require.NotEqual(t, "Agent has finished running.", msg.Body,
		"push body should not use the default fallback text")
	require.Equal(t, int32(1), nonStreamingRequests.Load(),
		"expected exactly one non-streaming request for push summary generation")
}

func TestSuccessfulChatSendsWebPushFallbackWithoutSummaryForEmptyAssistantText(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var nonStreamingRequests atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			nonStreamingRequests.Add(1)
			return chattest.OpenAINonStreamingResponse("unexpected summary request")
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("   ")...,
		)
	})

	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	_, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "empty-summary-push-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do the thing")},
	})
	require.NoError(t, err)

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return mockPush.dispatchCount.Load() >= 1
	}, testutil.IntervalFast)

	msg := mockPush.getLastMessage()
	require.Equal(t, "Agent has finished running.", msg.Body,
		"push body should fall back when the final assistant text is empty")
	require.Equal(t, int32(0), nonStreamingRequests.Load(),
		"push summary should not be requested when final assistant text has no usable text")
}

func TestComputerUseSubagentToolsAndModel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Track tools and model from the Anthropic LLM calls (the
	// computer use child chat). We use a raw HTTP handler because
	// the chattest AnthropicRequest struct does not capture tools.
	type anthropicCall struct {
		Model string
		Tools []string
	}
	var anthropicMu sync.Mutex
	var anthropicCalls []anthropicCall

	anthropicSrv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var req struct {
				Model  string `json:"model"`
				Stream bool   `json:"stream"`
				Tools  []struct {
					Name string `json:"name"`
				} `json:"tools"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			names := make([]string, len(req.Tools))
			for i, tool := range req.Tools {
				names[i] = tool.Name
			}
			anthropicMu.Lock()
			anthropicCalls = append(anthropicCalls, anthropicCall{
				Model: req.Model,
				Tools: names,
			})
			anthropicMu.Unlock()

			if !req.Stream {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "msg-test",
					"type":        "message",
					"role":        "assistant",
					"model":       chattool.ComputerUseModelName,
					"content":     []map[string]any{{"type": "text", "text": "Done."}},
					"stop_reason": "end_turn",
					"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
				})
				return
			}

			// Stream a minimal Anthropic SSE response.
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher, _ := w.(http.Flusher)

			chunks := []map[string]any{
				{
					"type": "message_start",
					"message": map[string]any{
						"id":    "msg-test",
						"type":  "message",
						"role":  "assistant",
						"model": chattool.ComputerUseModelName,
					},
				},
				{
					"type":  "content_block_start",
					"index": 0,
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				},
				{
					"type":  "content_block_delta",
					"index": 0,
					"delta": map[string]any{
						"type": "text_delta",
						"text": "Done.",
					},
				},
				{"type": "content_block_stop", "index": 0},
				{
					"type":  "message_delta",
					"delta": map[string]any{"stop_reason": "end_turn"},
					"usage": map[string]any{"output_tokens": 5},
				},
				{"type": "message_stop"},
			}

			for _, chunk := range chunks {
				chunkBytes, _ := json.Marshal(chunk)
				eventType, _ := chunk["type"].(string)
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n",
					eventType, chunkBytes)
				flusher.Flush()
			}
		},
	))
	t.Cleanup(anthropicSrv.Close)

	// OpenAI mock for the root chat. The first streaming call
	// triggers spawn_computer_use_agent; subsequent calls reply
	// with text.
	var openAICallCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if openAICallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"spawn_computer_use_agent",
					`{"prompt":"do the desktop thing","title":"cu-sub"}`,
				),
			)
		}
		// Include literal \u0000 in the response text, which is
		// what a real LLM writes when explaining binary output.
		// json.Marshal encodes the backslash as \\, producing
		// \\u0000 in the JSON bytes. The sanitizer must not
		// corrupt this into invalid JSON.
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("The file contains \\u0000 null bytes.")...,
		)
	})

	// Seed the DB: user, openai-compat provider, model config.
	user, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	// Add an Anthropic provider pointing to our mock server.
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    "anthropic",
		DisplayName: "Anthropic",
		APIKey:      "test-anthropic-key",
		BaseUrl:     anthropicSrv.URL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	})
	require.NoError(t, err)

	err = db.UpsertChatDesktopEnabled(ctx, true)
	require.NoError(t, err)

	// Build workspace + agent records so getWorkspaceConn can
	// resolve the agent for the computer use child.
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	// Mock agent connection that returns valid display dimensions
	// for the initial screenshot check in the computer use path.
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().
		ExecuteDesktopAction(gomock.Any(), gomock.Any()).
		Return(workspacesdk.DesktopActionResponse{
			ScreenshotWidth:  1920,
			ScreenshotHeight: 1080,
			ScreenshotData:   "iVBOR",
		}, nil).
		AnyTimes()
	mockConn.EXPECT().
		SetExtraHeaders(gomock.Any()).
		AnyTimes()
	mockConn.EXPECT().
		LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, xerrors.New("not found")).
		AnyTimes()

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	// Create a root chat with a workspace so the child inherits it.
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:       user.ID,
		Title:         "computer-use-detection",
		ModelConfigID: model.ID,
		WorkspaceID:   uuid.NullUUID{UUID: ws.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Use the desktop to check the UI"),
		},
	})
	require.NoError(t, err)

	// Wait for the root chat AND the computer use child to finish.
	// The root chat spawns the child, then the chatd server picks
	// up and runs the child (which hits the Anthropic mock).
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		if got.Status != database.ChatStatusWaiting &&
			got.Status != database.ChatStatusError {
			return false
		}
		// Ensure the Anthropic mock received at least one call.
		anthropicMu.Lock()
		n := len(anthropicCalls)
		anthropicMu.Unlock()
		return n >= 1
	}, testutil.WaitLong, testutil.IntervalFast)

	anthropicMu.Lock()
	calls := append([]anthropicCall(nil), anthropicCalls...)
	anthropicMu.Unlock()

	require.NotEmpty(t, calls,
		"expected at least one Anthropic LLM call")

	childModel := calls[0].Model
	childTools := calls[0].Tools

	// 1. Verify the model is the computer use model.
	require.Equal(t, chattool.ComputerUseModelName, childModel,
		"computer use subagent should use %s",
		chattool.ComputerUseModelName)

	// 2. Verify the computer tool is present.
	require.Contains(t, childTools, "computer",
		"computer use subagent should have the computer tool")

	// 3. Verify standard workspace tools are present (the same
	//    set a regular subagent gets).
	standardTools := []string{
		"read_file", "write_file", "edit_files", "execute",
		"process_output", "process_list", "process_signal",
	}
	for _, tool := range standardTools {
		require.Contains(t, childTools, tool,
			"computer use subagent should have standard tool %q",
			tool)
	}

	// 4. Verify workspace provisioning tools are NOT present.
	workspaceProvisioningTools := []string{
		"list_templates", "read_template",
		"create_workspace", "start_workspace",
	}
	for _, tool := range workspaceProvisioningTools {
		require.NotContains(t, childTools, tool,
			"computer use subagent should NOT have workspace "+
				"provisioning tool %q", tool)
	}

	// 5. Verify subagent tools are NOT present.
	subagentTools := []string{
		"spawn_agent", "spawn_computer_use_agent",
		"wait_agent", "message_agent", "close_agent",
	}
	for _, tool := range subagentTools {
		require.NotContains(t, childTools, tool,
			"computer use subagent should NOT have subagent "+
				"tool %q", tool)
	}

	// 6. Verify the child chat has Mode = computer_use in
	//    the DB.
	allChats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnerID: user.ID,
	})
	require.NoError(t, err)
	var children []database.Chat
	for _, c := range allChats {
		if c.ParentChatID.Valid && c.ParentChatID.UUID == chat.ID {
			children = append(children, c)
		}
	}
	require.Len(t, children, 1)
	require.True(t, children[0].Mode.Valid)
	require.Equal(t, database.ChatModeComputerUse,
		children[0].Mode.ChatMode)
}

func TestInterruptChatPersistsPartialResponse(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Set up a mock OpenAI that streams a partial response and then
	// blocks until the request context is canceled (simulating an
	// interrupt mid-stream).
	chunksDelivered := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunks := make(chan chattest.OpenAIChunk, 1)
		go func() {
			defer close(chunks)
			// Send two partial text chunks so there is meaningful
			// content to persist.
			for _, c := range chattest.OpenAITextChunks("hello world") {
				chunks <- c
			}
			// Signal that chunks have been written to the HTTP response.
			select {
			case <-chunksDelivered:
			default:
				close(chunksDelivered)
			}
			// Block until interrupt cancels the context.
			<-req.Context().Done()
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	})

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "interrupt-persist-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Subscribe to the chat's event stream so we can observe
	// message_part events — proof the chatloop has actually
	// processed the streamed chunks.
	_, events, subCancel, ok := server.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	defer subCancel()

	// Wait for the mock to finish sending chunks.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		select {
		case <-chunksDelivered:
			return true
		default:
			return false
		}
	}, testutil.IntervalFast)

	// Drain the event channel until we see a message_part event,
	// which means the chatloop has consumed and published the chunk.
	gotMessagePart := false
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		for {
			select {
			case ev := <-events:
				if ev.Type == codersdk.ChatStreamEventTypeMessagePart {
					gotMessagePart = true
					return true
				}
			default:
				return gotMessagePart
			}
		}
	}, testutil.IntervalFast)
	require.True(t, gotMessagePart, "should have received at least one message_part event")

	// Now interrupt the chat — the chatloop has processed content.
	updated := server.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)

	// Wait for the partial assistant message to be persisted.
	// After the interrupt, the chatloop runs persistInterruptedStep
	// which inserts the message and publishes a "message" event.
	// We poll the DB directly for the assistant message rather than
	// relying on the chat status (which transitions to "waiting"
	// before the persist completes).
	var assistantMsg *database.ChatMessage
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		msgs, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for i := range msgs {
			if msgs[i].Role == database.ChatMessageRoleAssistant {
				assistantMsg = &msgs[i]
				return true
			}
		}
		return false
	}, testutil.IntervalFast)
	require.NotNilf(t, assistantMsg, "expected a persisted assistant message after interrupt")

	// Parse the content and verify it contains the partial text.
	parts, err := chatprompt.ParseContent(*assistantMsg)
	require.NoError(t, err)

	var foundText string
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText {
			foundText += part.Text
		}
	}
	require.Contains(t, foundText, "hello world",
		"partial assistant response should contain the streamed text")
}

func TestProcessChatPanicRecovery(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	// Wrap the database so we can trigger a panic on the main
	// goroutine of processChat. The chatloop's executeTools has
	// its own recover, so panicking inside a tool goroutine won't
	// reach the processChat-level recovery. Instead, we panic
	// during PersistStep's InTx call, which runs synchronously on
	// the processChat goroutine.
	panicWrapper := &panicOnInTxDB{Store: db}

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Panic recovery test")
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("hello")...,
		)
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	// Pass the panic wrapper to the server, but use the real
	// database for seeding so those operations don't panic.
	server := newActiveTestServer(t, panicWrapper, ps)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:       user.ID,
		Title:         "panic-recovery",
		ModelConfigID: model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
		},
	})
	require.NoError(t, err)

	// Enable the panic now that CreateChat's InTx has completed.
	// The next InTx call is PersistStep inside the chatloop,
	// running synchronously on the processChat goroutine.
	panicWrapper.enablePanic()

	// Wait for the panic to be recovered and the chat to
	// transition to error status.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	require.True(t, chatResult.LastError.Valid, "LastError should be set")
	require.Contains(t, chatResult.LastError.String, "chat processing panicked")
	require.Contains(t, chatResult.LastError.String, "intentional test panic")
}

// panicOnInTxDB wraps a database.Store and panics on the first InTx
// call after enablePanic is called. Subsequent calls pass through
// so the processChat cleanup defer can update the chat status.
type panicOnInTxDB struct {
	database.Store
	active   atomic.Bool
	panicked atomic.Bool
}

func (d *panicOnInTxDB) enablePanic() { d.active.Store(true) }

func (d *panicOnInTxDB) InTx(f func(database.Store) error, opts *database.TxOptions) error {
	if d.active.Load() && !d.panicked.Load() {
		d.panicked.Store(true)
		panic("intentional test panic")
	}
	return d.Store.InTx(f, opts)
}

// TestMCPServerToolInvocation verifies that when a chat has
// mcp_server_ids set, the chat loop connects to those MCP servers,
// discovers their tools, and the LLM can invoke them.
//
// NOTE: This test uses a raw database.Store (no dbauthz wrapper).
// The chatd RBAC authorization of GetMCPServerConfigsByIDs (which
// requires ActionRead on ResourceDeploymentConfig) is covered by
// the chatd role definition tests, not here.
func TestMCPServerToolInvocation(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Start a real MCP server that exposes an "echo" tool.
	mcpSrv := mcpserver.NewMCPServer("test-mcp", "1.0.0")
	mcpSrv.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	mcpHTTP := mcpserver.NewStreamableHTTPServer(mcpSrv)
	mcpTS := httptest.NewServer(mcpHTTP)
	t.Cleanup(mcpTS.Close)

	// Track which tool names are sent to the LLM and capture
	// whether the MCP tool result appears in the second call.
	var (
		callCount      atomic.Int32
		llmToolNames   []string
		llmToolsMu     sync.Mutex
		foundMCPResult atomic.Bool
	)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		// Record tool names from the first streamed call.
		if callCount.Add(1) == 1 {
			names := make([]string, 0, len(req.Tools))
			for _, tool := range req.Tools {
				names = append(names, tool.Function.Name)
			}
			llmToolsMu.Lock()
			llmToolNames = names
			llmToolsMu.Unlock()

			// Ask the LLM to call the MCP echo tool.
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"test-mcp__echo",
					`{"input":"hello from LLM"}`,
				),
			)
		}

		// Second call: verify the tool result was fed back.
		for _, msg := range req.Messages {
			if msg.Role == "tool" && strings.Contains(msg.Content, "echo: hello from LLM") {
				foundMCPResult.Store(true)
			}
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Got it!")...,
		)
	})

	user, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	// Seed the MCP server config in the database. This must
	// happen after seedChatDependencies so user.ID exists for
	// the foreign key.
	mcpConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "Test MCP",
		Slug:          "test-mcp",
		Url:           mcpTS.URL,
		Transport:     "streamable_http",
		AuthType:      "none",
		Availability:  "default_off",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
		CreatedBy:     user.ID,
		UpdatedBy:     user.ID,
	})
	require.NoError(t, err)

	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:       user.ID,
		Title:         "mcp-tool-test",
		ModelConfigID: model.ID,
		WorkspaceID:   uuid.NullUUID{UUID: ws.ID, Valid: true},
		MCPServerIDs:  []uuid.UUID{mcpConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Echo something via MCP."),
		},
	})
	require.NoError(t, err)

	// Verify MCPServerIDs were persisted on the chat record.
	dbChat, getErr := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, getErr)
	require.Equal(t, []uuid.UUID{mcpConfig.ID}, dbChat.MCPServerIDs)

	// Wait for the chat to finish processing.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chatResult.LastError.String)
	}

	// The MCP tool (test-mcp__echo) should appear in the tool
	// list sent to the LLM.
	llmToolsMu.Lock()
	recordedNames := append([]string(nil), llmToolNames...)
	llmToolsMu.Unlock()
	require.Contains(t, recordedNames, "test-mcp__echo",
		"MCP tool should be in the tool list sent to the LLM")

	// The tool result from the MCP server ("echo: hello from
	// LLM") should have been fed back to the LLM as a tool
	// message in the second call.
	require.True(t, foundMCPResult.Load(),
		"MCP tool result should appear in the second LLM call")

	// Verify the tool result was persisted in the database.
	var foundToolMessage bool
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleTool {
				continue
			}
			parts, parseErr := chatprompt.ParseContent(msg)
			if parseErr != nil || len(parts) == 0 {
				continue
			}
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolResult &&
					part.ToolName == "test-mcp__echo" &&
					strings.Contains(string(part.Result), "echo: hello from LLM") {
					foundToolMessage = true
					return true
				}
			}
		}
		return false
	}, testutil.IntervalFast)
	require.True(t, foundToolMessage,
		"MCP tool result should be persisted as a tool message in the database")
}

func TestChatTemplateAllowlistEnforcement(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)

	// Set up a mock OpenAI server. The first streaming call triggers
	// list_templates; subsequent calls respond with text.
	var callCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if callCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("list_templates", `{}`),
			)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Here are the templates.")...,
		)
	})

	user, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	// Create two templates the user can see.
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	tplAllowed := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "allowed-template",
	})
	tplBlocked := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "blocked-template",
	})

	// Set the allowlist to only tplAllowed.
	allowlistJSON, err := json.Marshal([]string{tplAllowed.ID.String()})
	require.NoError(t, err)
	err = db.UpsertChatTemplateAllowlist(dbauthz.AsSystemRestricted(ctx), string(allowlistJSON))
	require.NoError(t, err)

	server := newActiveTestServer(t, db, ps)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:       user.ID,
		Title:         "allowlist-test",
		ModelConfigID: model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("List templates"),
		},
	})
	require.NoError(t, err)

	// Wait for the chat to finish processing.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat run failed", "last_error=%q", chatResult.LastError.String)
	}

	// Find the list_templates tool result in the persisted messages.
	var toolResult string
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleTool {
				continue
			}
			parts, parseErr := chatprompt.ParseContent(msg)
			if parseErr != nil {
				continue
			}
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolResult &&
					part.ToolName == "list_templates" {
					toolResult = string(part.Result)
					return true
				}
			}
		}
		return false
	}, testutil.IntervalFast)

	require.NotEmpty(t, toolResult, "list_templates tool result should be persisted")

	// The result should contain only the allowed template.
	require.Contains(t, toolResult, tplAllowed.ID.String(),
		"allowed template should appear in list_templates result")
	require.NotContains(t, toolResult, tplBlocked.ID.String(),
		"blocked template should NOT appear in list_templates result")
}
