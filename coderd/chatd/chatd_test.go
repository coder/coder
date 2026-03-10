package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/chatd/chattest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	proto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Done.")...,
		)
	})

	_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

	// Create a root chat whose first model call will spawn a subagent.
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
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
		got, getErr := client.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		if got.Chat.Status != codersdk.ChatStatusWaiting && got.Chat.Status != codersdk.ChatStatusError {
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

	workspaceTools := []string{"list_templates", "read_template", "create_workspace"}
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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
		Content:      []fantasy.Content{fantasy.TextContent{Text: "queued"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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
		Content:      []fantasy.Content{fantasy.TextContent{Text: "interrupt"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "original"}},
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
		Content:      []fantasy.Content{fantasy.TextContent{Text: "follow-up"}},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)
	_, err = replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []fantasy.Content{fantasy.TextContent{Text: "another"}},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)

	_, err = db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:  chat.ID,
		Content: json.RawMessage(`"queued"`),
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
		Content:         []fantasy.Content{fantasy.TextContent{Text: "edited"}},
	})
	require.NoError(t, err)
	require.Equal(t, editedMessageID, editResult.Message.ID)
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
	require.Equal(t, editedMessageID, messages[0].ID)
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: 999999,
		Content:         []fantasy.Content{fantasy.TextContent{Text: "edited"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	assistantMessage, err := db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          "assistant",
		Content: pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`"assistant"`),
			Valid:      true,
		},
		Visibility:          database.ChatMessageVisibilityBoth,
		InputTokens:         sql.NullInt64{},
		OutputTokens:        sql.NullInt64{},
		TotalTokens:         sql.NullInt64{},
		ReasoningTokens:     sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens:     sql.NullInt64{},
		ContextLimit:        sql.NullInt64{},
		Compressed:          sql.NullBool{},
	})
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: assistantMessage.ID,
		Content:         []fantasy.Content{fantasy.TextContent{Text: "edited"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "first"}},
	})
	require.NoError(t, err)

	// Insert two more messages so we have three total visible
	// messages (the initial user message plus these two).
	msg2, err := db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:                "assistant",
		Content:             pqtype.NullRawMessage{RawMessage: json.RawMessage(`"second"`), Valid: true},
		Visibility:          database.ChatMessageVisibilityBoth,
		InputTokens:         sql.NullInt64{},
		OutputTokens:        sql.NullInt64{},
		TotalTokens:         sql.NullInt64{},
		ReasoningTokens:     sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens:     sql.NullInt64{},
		ContextLimit:        sql.NullInt64{},
		Compressed:          sql.NullBool{},
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:                "user",
		Content:             pqtype.NullRawMessage{RawMessage: json.RawMessage(`"third"`), Valid: true},
		Visibility:          database.ChatMessageVisibilityBoth,
		InputTokens:         sql.NullInt64{},
		OutputTokens:        sql.NullInt64{},
		TotalTokens:         sql.NullInt64{},
		ReasoningTokens:     sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens:     sql.NullInt64{},
		ContextLimit:        sql.NullInt64{},
		Compressed:          sql.NullBool{},
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
	require.Equal(t, "user", partialMessages[0].Message.Role)
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

	_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Create a workspace from the template and continue.",
			},
		},
	})
	require.NoError(t, err)

	var chatWithMessages codersdk.ChatWithMessages
	require.Eventually(t, func() bool {
		got, getErr := client.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatWithMessages = got
		return got.Chat.Status == codersdk.ChatStatusWaiting || got.Chat.Status == codersdk.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatWithMessages.Chat.Status == codersdk.ChatStatusError {
		lastError := ""
		if chatWithMessages.Chat.LastError != nil {
			lastError = *chatWithMessages.Chat.LastError
		}
		require.FailNowf(t, "chat run failed", "last_error=%q", lastError)
	}

	require.NotNil(t, chatWithMessages.Chat.WorkspaceID)
	workspaceID := *chatWithMessages.Chat.WorkspaceID
	workspace, err := client.Workspace(ctx, workspaceID)
	require.NoError(t, err)
	require.Equal(t, workspaceName, workspace.Name)

	var foundCreateWorkspaceResult bool
	for _, message := range chatWithMessages.Messages {
		if message.Role != "tool" {
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

	_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

	// Create a chat with the stopped workspace pre-associated.
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Start the workspace.",
			},
		},
		WorkspaceID: &workspace.ID,
	})
	require.NoError(t, err)

	var chatWithMessages codersdk.ChatWithMessages
	require.Eventually(t, func() bool {
		got, getErr := client.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatWithMessages = got
		return got.Chat.Status == codersdk.ChatStatusWaiting || got.Chat.Status == codersdk.ChatStatusError
	}, testutil.WaitSuperLong, testutil.IntervalFast)

	if chatWithMessages.Chat.Status == codersdk.ChatStatusError {
		lastError := ""
		if chatWithMessages.Chat.LastError != nil {
			lastError = *chatWithMessages.Chat.LastError
		}
		require.FailNowf(t, "chat run failed", "last_error=%q", lastError)
	}

	// Verify the workspace was started.
	require.NotNil(t, chatWithMessages.Chat.WorkspaceID)
	updatedWorkspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkspaceTransitionStart, updatedWorkspace.LatestBuild.Transition)

	// Verify start_workspace tool result exists in the chat messages.
	var foundStartWorkspaceResult bool
	for _, message := range chatWithMessages.Messages {
		if message.Role != "tool" {
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

func seedChatDependencies(
	ctx context.Context,
	t *testing.T,
	db database.Store,
) (database.User, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseUrl:     "",
		ApiKeyKeyID: sql.NullString{},
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	})
	require.NoError(t, err)
	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
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

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			// Non-streaming calls are used for title
			// generation and push summary generation.
			// Return the summary text for both — the title
			// result is irrelevant to this test.
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
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "do the thing"}},
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
}
