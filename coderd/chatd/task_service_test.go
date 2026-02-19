package chatd

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestTaskService_AwaitTaskReportRejectsNonDescendant(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	childAID := uuid.New()
	childBID := uuid.New()

	store := newTaskServiceTestStore(
		testTaskChat(rootID, uuid.Nil, database.ChatTaskStatusReported),
		testTaskChat(childAID, rootID, database.ChatTaskStatusQueued),
		testTaskChat(childBID, rootID, database.ChatTaskStatusQueued),
	)
	service := newTaskService(store, nil)

	_, err := service.AwaitTaskReport(context.Background(), childAID, childBID, time.Second)
	require.ErrorIs(t, err, ErrTaskNotDescendant)
}

func TestTaskService_AwaitTaskReportResolvesWaiter(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	childID := uuid.New()

	store := newTaskServiceTestStore(
		testTaskChat(rootID, uuid.Nil, database.ChatTaskStatusReported),
		testTaskChat(childID, rootID, database.ChatTaskStatusRunning),
	)
	service := newTaskService(store, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type awaitResult struct {
		report string
		err    error
	}
	awaitCh := make(chan awaitResult, 1)
	go func() {
		report, err := service.AwaitTaskReport(ctx, rootID, childID, time.Second)
		awaitCh <- awaitResult{report: report, err: err}
	}()

	require.NoError(t, service.MarkTaskReported(context.Background(), childID, "completed"))

	select {
	case res := <-awaitCh:
		require.NoError(t, res.err)
		require.Equal(t, "completed", res.report)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for awaited task report: %v", ctx.Err())
	}
}

func TestTaskService_HasActiveDescendants(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	childID := uuid.New()

	store := newTaskServiceTestStore(
		testTaskChat(rootID, uuid.Nil, database.ChatTaskStatusReported),
		testTaskChat(childID, rootID, database.ChatTaskStatusQueued),
	)
	service := newTaskService(store, nil)

	active, err := service.HasActiveDescendants(context.Background(), rootID)
	require.NoError(t, err)
	require.True(t, active)

	require.NoError(t, service.MarkTaskReported(context.Background(), childID, "done"))

	active, err = service.HasActiveDescendants(context.Background(), rootID)
	require.NoError(t, err)
	require.False(t, active)
}

func TestTaskService_AwaitTaskReportFallsBackToPersistedReport(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	childID := uuid.New()

	child := testTaskChat(childID, rootID, database.ChatTaskStatusReported)
	child.TaskReport = sql.NullString{
		String: "persisted report",
		Valid:  true,
	}

	store := newTaskServiceTestStore(
		testTaskChat(rootID, uuid.Nil, database.ChatTaskStatusReported),
		child,
	)
	service := newTaskService(store, nil)

	report, err := service.AwaitTaskReport(context.Background(), rootID, childID, time.Second)
	require.NoError(t, err)
	require.Equal(t, "persisted report", report)
}

func TestTaskService_MarkTaskReportedInsertsParentToolMessage(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()
	childID := uuid.New()

	store := newTaskServiceTestStore(
		testTaskChat(parentID, uuid.Nil, database.ChatTaskStatusReported),
		testTaskChat(childID, parentID, database.ChatTaskStatusRunning),
	)
	service := newTaskService(store, nil)

	require.NoError(t, service.MarkTaskReported(context.Background(), childID, "child complete"))

	messages := store.chatMessagesByChatID(parentID)
	require.Len(t, messages, 1)

	msg := messages[0]
	require.Equal(t, string(fantasy.MessageRoleTool), msg.Role)
	require.True(t, msg.ToolCallID.Valid)
	expectedToolCallID := taskReportToolCallID(childID)
	require.Equal(t, expectedToolCallID, msg.ToolCallID.String)
	require.Regexp(t, `^[a-zA-Z0-9_-]+$`, msg.ToolCallID.String)

	blocks, err := parseToolResults(msg.Content)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, expectedToolCallID, blocks[0].ToolCallID)
	require.Regexp(t, `^[a-zA-Z0-9_-]+$`, blocks[0].ToolCallID)
	require.Equal(t, toolAgentReport, blocks[0].ToolName)

	payload, ok := blocks[0].Result.(map[string]any)
	require.True(t, ok)
	require.Equal(t, childID.String(), payload["chat_id"])
	require.Equal(t, "child complete", payload["report"])
}

func TestTaskService_MarkTaskReportedWakesParentToPending(t *testing.T) {
	t.Parallel()

	for _, parentStatus := range []database.ChatStatus{
		database.ChatStatusWaiting,
		database.ChatStatusCompleted,
	} {
		parentStatus := parentStatus
		t.Run(string(parentStatus), func(t *testing.T) {
			t.Parallel()

			parentID := uuid.New()
			childID := uuid.New()

			parent := testTaskChat(parentID, uuid.Nil, database.ChatTaskStatusReported)
			parent.Status = parentStatus

			store := newTaskServiceTestStore(
				parent,
				testTaskChat(childID, parentID, database.ChatTaskStatusRunning),
			)
			service := newTaskService(store, nil)

			require.NoError(t, service.MarkTaskReported(context.Background(), childID, "done"))

			updatedParent, err := store.GetChatByID(context.Background(), parentID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusPending, updatedParent.Status)
		})
	}
}

func TestTaskService_MarkTaskReportedDoesNotWakeRunningParent(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()
	childID := uuid.New()

	parent := testTaskChat(parentID, uuid.Nil, database.ChatTaskStatusReported)
	parent.Status = database.ChatStatusRunning

	store := newTaskServiceTestStore(
		parent,
		testTaskChat(childID, parentID, database.ChatTaskStatusRunning),
	)
	service := newTaskService(store, nil)

	require.NoError(t, service.MarkTaskReported(context.Background(), childID, "done"))

	updatedParent, err := store.GetChatByID(context.Background(), parentID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, updatedParent.Status)
}

func TestTaskService_SynthesizeFallbackTaskReport(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	store := newTaskServiceTestStore(
		testTaskChat(chatID, uuid.Nil, database.ChatTaskStatusAwaitingReport),
	)
	service := newTaskService(store, nil)

	userContent, err := marshalContentBlocks([]fantasy.Content{
		fantasy.TextContent{Text: "user prompt"},
	})
	require.NoError(t, err)
	_, err = store.InsertChatMessage(context.Background(), database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    string(fantasy.MessageRoleUser),
		Content: userContent,
	})
	require.NoError(t, err)

	firstAssistant, err := marshalContentBlocks([]fantasy.Content{
		fantasy.TextContent{Text: "first summary"},
	})
	require.NoError(t, err)
	_, err = store.InsertChatMessage(context.Background(), database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    string(fantasy.MessageRoleAssistant),
		Content: firstAssistant,
	})
	require.NoError(t, err)

	secondAssistant, err := marshalContentBlocks([]fantasy.Content{
		fantasy.TextContent{Text: "latest summary"},
	})
	require.NoError(t, err)
	_, err = store.InsertChatMessage(context.Background(), database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    string(fantasy.MessageRoleAssistant),
		Content: secondAssistant,
	})
	require.NoError(t, err)

	report := service.SynthesizeFallbackTaskReport(context.Background(), chatID)
	require.Equal(t, "latest summary", report)
}

func TestTaskService_SynthesizeFallbackTaskReportDefault(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	store := newTaskServiceTestStore(
		testTaskChat(chatID, uuid.Nil, database.ChatTaskStatusAwaitingReport),
	)
	service := newTaskService(store, nil)

	report := service.SynthesizeFallbackTaskReport(context.Background(), chatID)
	require.Equal(t, defaultFallbackTaskReport, report)
}

type taskServiceTestStore struct {
	mu            sync.Mutex
	chats         map[uuid.UUID]database.Chat
	messages      []database.ChatMessage
	nextMessageID int64
}

func newTaskServiceTestStore(chats ...database.Chat) *taskServiceTestStore {
	byID := make(map[uuid.UUID]database.Chat, len(chats))
	for _, chat := range chats {
		byID[chat.ID] = chat
	}
	return &taskServiceTestStore{
		chats:         byID,
		nextMessageID: 1,
	}
}

func (s *taskServiceTestStore) GetChatByID(_ context.Context, id uuid.UUID) (database.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, ok := s.chats[id]
	if !ok {
		return database.Chat{}, sql.ErrNoRows
	}
	return chat, nil
}

func (s *taskServiceTestStore) GetChatMessagesByChatID(
	_ context.Context,
	chatID uuid.UUID,
) ([]database.ChatMessage, error) {
	return s.chatMessagesByChatID(chatID), nil
}

func (s *taskServiceTestStore) InsertChat(_ context.Context, arg database.InsertChatParams) (database.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskStatus := database.ChatTaskStatusReported
	if arg.TaskStatus.Valid {
		taskStatus = arg.TaskStatus.ChatTaskStatus
	}

	chat := database.Chat{
		ID:               uuid.New(),
		OwnerID:          arg.OwnerID,
		WorkspaceID:      arg.WorkspaceID,
		WorkspaceAgentID: arg.WorkspaceAgentID,
		Title:            arg.Title,
		Status:           database.ChatStatusWaiting,
		ModelConfig:      arg.ModelConfig,
		ParentChatID:     arg.ParentChatID,
		RootChatID:       arg.RootChatID,
		TaskStatus:       taskStatus,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	s.chats[chat.ID] = chat
	return chat, nil
}

func (s *taskServiceTestStore) InsertChatMessage(
	_ context.Context,
	arg database.InsertChatMessageParams,
) (database.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	message := database.ChatMessage{
		ID:         s.nextMessageID,
		ChatID:     arg.ChatID,
		CreatedAt:  time.Now(),
		Role:       arg.Role,
		Content:    arg.Content,
		ToolCallID: arg.ToolCallID,
		Thinking:   arg.Thinking,
		Hidden:     arg.Hidden,
	}
	s.nextMessageID++
	s.messages = append(s.messages, message)
	return message, nil
}

func (s *taskServiceTestStore) ListChildChatsByParentID(
	_ context.Context,
	parentChatID uuid.UUID,
) ([]database.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]database.Chat, 0)
	for _, chat := range s.chats {
		if chat.ParentChatID.Valid && chat.ParentChatID.UUID == parentChatID {
			out = append(out, chat)
		}
	}
	return out, nil
}

func (s *taskServiceTestStore) UpdateChatStatus(
	_ context.Context,
	arg database.UpdateChatStatusParams,
) (database.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, ok := s.chats[arg.ID]
	if !ok {
		return database.Chat{}, sql.ErrNoRows
	}

	chat.Status = arg.Status
	chat.WorkerID = arg.WorkerID
	chat.StartedAt = arg.StartedAt
	chat.UpdatedAt = time.Now()
	s.chats[arg.ID] = chat
	return chat, nil
}

func (s *taskServiceTestStore) UpdateChatTaskStatus(
	_ context.Context,
	arg database.UpdateChatTaskStatusParams,
) (database.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, ok := s.chats[arg.ID]
	if !ok {
		return database.Chat{}, sql.ErrNoRows
	}

	chat.TaskStatus = arg.TaskStatus
	chat.UpdatedAt = time.Now()
	s.chats[arg.ID] = chat
	return chat, nil
}

func (s *taskServiceTestStore) UpdateChatTaskReport(
	_ context.Context,
	arg database.UpdateChatTaskReportParams,
) (database.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, ok := s.chats[arg.ID]
	if !ok {
		return database.Chat{}, sql.ErrNoRows
	}

	chat.TaskReport = arg.TaskReport
	chat.UpdatedAt = time.Now()
	s.chats[arg.ID] = chat
	return chat, nil
}

func (s *taskServiceTestStore) chatMessagesByChatID(chatID uuid.UUID) []database.ChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]database.ChatMessage, 0)
	for _, message := range s.messages {
		if message.ChatID == chatID {
			out = append(out, message)
		}
	}
	return out
}

func testTaskChat(id uuid.UUID, parentID uuid.UUID, taskStatus database.ChatTaskStatus) database.Chat {
	parentChatID := uuid.NullUUID{}
	if parentID != uuid.Nil {
		parentChatID = uuid.NullUUID{UUID: parentID, Valid: true}
	}

	rootChatID := uuid.NullUUID{UUID: id, Valid: true}
	if parentID != uuid.Nil {
		rootChatID = uuid.NullUUID{UUID: parentID, Valid: true}
	}

	return database.Chat{
		ID:           id,
		OwnerID:      uuid.New(),
		Status:       database.ChatStatusWaiting,
		ParentChatID: parentChatID,
		RootChatID:   rootChatID,
		TaskStatus:   taskStatus,
		TaskReport:   sql.NullString{},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}
