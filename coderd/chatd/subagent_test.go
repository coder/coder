package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestProcessor_SubagentToolIncludesCreatedTitle(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()
	parent := testSubagentChat(parentID, uuid.Nil)
	parent.Title = "Parent"

	store := newSubagentServiceTestStore(parent)
	processor := &Processor{subagentService: newSubagentService(store, nil)}
	chatState := parent
	chatStateMu := &sync.Mutex{}
	tools := processor.agentTools(nil, &chatState, chatStateMu, nil)

	tool := testFindAgentTool(t, tools, toolSubagent)
	input, err := json.Marshal(subagentArgs{
		Prompt:     "Run delegated child work",
		Title:      "Delegated child",
		Background: true,
	})
	require.NoError(t, err)

	response, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "tool-call-subagent",
		Name:  toolSubagent,
		Input: string(input),
	})
	require.NoError(t, err)
	require.False(t, response.IsError)

	payload := parseToolPayload(t, response.Content)
	require.Equal(t, "Delegated child", payload["title"])
	require.NotEmpty(t, payload["chat_id"])
	require.NotEmpty(t, payload["request_id"])
	require.Equal(t, "pending", payload["status"])
}

func TestProcessor_SubagentAwaitToolIncludesTargetTitle(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()
	childID := uuid.New()
	requestID := uuid.New()

	parent := testSubagentChat(parentID, uuid.Nil)
	child := testSubagentChat(childID, parentID)
	child.Title = "Awaited child"

	store := newSubagentServiceTestStore(parent, child)
	require.NoError(t, store.insertSubagentRequestMessage(childID, requestID, "work"))
	require.NoError(t, store.insertSubagentResponseMessage(childID, requestID, "done"))

	processor := &Processor{subagentService: newSubagentService(store, nil)}
	chatState := parent
	chatStateMu := &sync.Mutex{}
	tools := processor.agentTools(nil, &chatState, chatStateMu, nil)

	tool := testFindAgentTool(t, tools, toolSubagentAwait)
	input, err := json.Marshal(subagentAwaitArgs{
		ChatID:    childID.String(),
		RequestID: requestID.String(),
	})
	require.NoError(t, err)

	response, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "tool-call-subagent-await",
		Name:  toolSubagentAwait,
		Input: string(input),
	})
	require.NoError(t, err)
	require.False(t, response.IsError)

	payload := parseToolPayload(t, response.Content)
	require.Equal(t, child.Title, payload["title"])
	require.Equal(t, childID.String(), payload["chat_id"])
	require.Equal(t, requestID.String(), payload["request_id"])
	require.Equal(t, "done", payload["report"])
	require.Equal(t, "completed", payload["status"])
}

func TestProcessor_SubagentMessageToolIncludesTargetTitle(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()
	childID := uuid.New()

	parent := testSubagentChat(parentID, uuid.Nil)
	child := testSubagentChat(childID, parentID)
	child.Title = "Message target"

	store := newSubagentServiceTestStore(parent, child)
	processor := &Processor{subagentService: newSubagentService(store, nil)}
	chatState := parent
	chatStateMu := &sync.Mutex{}
	tools := processor.agentTools(nil, &chatState, chatStateMu, nil)

	tool := testFindAgentTool(t, tools, toolSubagentMessage)
	input, err := json.Marshal(subagentMessageArgs{
		ChatID:  childID.String(),
		Message: "follow-up request",
		Await:   false,
	})
	require.NoError(t, err)

	response, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "tool-call-subagent-message",
		Name:  toolSubagentMessage,
		Input: string(input),
	})
	require.NoError(t, err)
	require.False(t, response.IsError)

	payload := parseToolPayload(t, response.Content)
	require.Equal(t, child.Title, payload["title"])
	require.Equal(t, childID.String(), payload["chat_id"])
	require.NotEmpty(t, payload["request_id"])
	require.Equal(t, "pending", payload["status"])
}

func testFindAgentTool(t *testing.T, tools []fantasy.AgentTool, name string) fantasy.AgentTool {
	t.Helper()

	for _, tool := range tools {
		if tool.Info().Name == name {
			return tool
		}
	}
	require.FailNow(t, "tool not found", "name=%s", name)
	return nil
}

func parseToolPayload(t *testing.T, content string) map[string]any {
	t.Helper()

	payload := make(map[string]any)
	require.NoError(t, json.Unmarshal([]byte(content), &payload))
	return payload
}

func TestSubagentService_AwaitSubagentReportRejectsNonDescendant(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	childAID := uuid.New()
	childBID := uuid.New()

	store := newSubagentServiceTestStore(
		testSubagentChat(rootID, uuid.Nil),
		testSubagentChat(childAID, rootID),
		testSubagentChat(childBID, rootID),
	)
	service := newSubagentService(store, nil)

	_, err := service.AwaitSubagentReport(context.Background(), childAID, childBID, uuid.New(), time.Second)
	require.ErrorIs(t, err, ErrSubagentNotDescendant)
}

func TestSubagentService_AwaitSubagentReportResolvesWaiter(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	childID := uuid.New()

	store := newSubagentServiceTestStore(
		testSubagentChat(rootID, uuid.Nil),
		testSubagentChat(childID, rootID),
	)
	service := newSubagentService(store, nil)

	_, requestID, err := service.SendSubagentMessage(context.Background(), rootID, childID, "do work")
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, requestID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type awaitResult struct {
		result SubagentAwaitResult
		err    error
	}
	awaitCh := make(chan awaitResult, 1)
	go func() {
		result, err := service.AwaitSubagentReport(ctx, rootID, childID, requestID, time.Second)
		awaitCh <- awaitResult{result: result, err: err}
	}()

	marked, err := service.MarkSubagentReported(context.Background(), childID, "completed", uuid.NullUUID{UUID: requestID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, requestID, marked.RequestID)
	require.Equal(t, "completed", marked.Report)

	select {
	case res := <-awaitCh:
		require.NoError(t, res.err)
		require.Equal(t, requestID, res.result.RequestID)
		require.Equal(t, "completed", res.result.Report)
		require.Positive(t, res.result.DurationMS)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for awaited subagent report: %v", ctx.Err())
	}
}

func TestSubagentService_AwaitSubagentReportReturnsPersistedResponseMarker(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	childID := uuid.New()
	requestID := uuid.New()

	store := newSubagentServiceTestStore(
		testSubagentChat(rootID, uuid.Nil),
		testSubagentChat(childID, rootID),
	)
	service := newSubagentService(store, nil)

	require.NoError(t, store.insertSubagentRequestMessage(childID, requestID, "first request"))
	require.NoError(t, store.insertSubagentResponseMessage(childID, requestID, "persisted report"))

	result, err := service.AwaitSubagentReport(context.Background(), rootID, childID, requestID, time.Second)
	require.NoError(t, err)
	require.Equal(t, requestID, result.RequestID)
	require.Equal(t, "persisted report", result.Report)
	require.Positive(t, result.DurationMS)
}

func TestSubagentService_HasActiveDescendants(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	childID := uuid.New()

	store := newSubagentServiceTestStore(
		testSubagentChat(rootID, uuid.Nil),
		testSubagentChat(childID, rootID),
	)
	service := newSubagentService(store, nil)

	_, requestID, err := service.SendSubagentMessage(context.Background(), rootID, childID, "follow up")
	require.NoError(t, err)

	active, err := service.HasActiveDescendants(context.Background(), rootID)
	require.NoError(t, err)
	require.True(t, active)

	_, err = service.MarkSubagentReported(context.Background(), childID, "done", uuid.NullUUID{UUID: requestID, Valid: true})
	require.NoError(t, err)

	active, err = service.HasActiveDescendants(context.Background(), rootID)
	require.NoError(t, err)
	require.False(t, active)
}

func TestSubagentService_MarkSubagentReportedInsertsParentToolUseAndToolMessage(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()
	childID := uuid.New()
	requestID := uuid.New()

	store := newSubagentServiceTestStore(
		testSubagentChat(parentID, uuid.Nil),
		testSubagentChat(childID, parentID),
	)
	service := newSubagentService(store, nil)

	require.NoError(t, store.insertSubagentRequestMessage(childID, requestID, "work"))
	_, err := service.MarkSubagentReported(context.Background(), childID, "child complete", uuid.NullUUID{UUID: requestID, Valid: true})
	require.NoError(t, err)

	messages := store.chatMessagesByChatID(parentID)
	require.Len(t, messages, 2)

	assistantMsg := messages[0]
	require.Equal(t, string(fantasy.MessageRoleAssistant), assistantMsg.Role)
	require.True(t, assistantMsg.Hidden)
	require.True(t, assistantMsg.ToolCallID.Valid)

	expectedToolCallID := subagentReportToolCallID(requestID)
	require.Equal(t, expectedToolCallID, assistantMsg.ToolCallID.String)
	require.Regexp(t, `^[a-zA-Z0-9_-]+$`, assistantMsg.ToolCallID.String)

	assistantBlocks, err := parseContentBlocks(assistantMsg.Role, assistantMsg.Content)
	require.NoError(t, err)
	require.Len(t, assistantBlocks, 1)
	assistantToolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](assistantBlocks[0])
	require.True(t, ok)
	require.Equal(t, expectedToolCallID, assistantToolCall.ToolCallID)
	require.Equal(t, toolSubagentReport, assistantToolCall.ToolName)
	require.Equal(t, "{}", assistantToolCall.Input)

	msg := messages[1]
	require.Equal(t, string(fantasy.MessageRoleTool), msg.Role)
	require.False(t, msg.Hidden)
	require.True(t, msg.ToolCallID.Valid)
	require.Equal(t, expectedToolCallID, msg.ToolCallID.String)
	require.Regexp(t, `^[a-zA-Z0-9_-]+$`, msg.ToolCallID.String)

	blocks, err := parseToolResults(msg.Content)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, expectedToolCallID, blocks[0].ToolCallID)
	require.Regexp(t, `^[a-zA-Z0-9_-]+$`, blocks[0].ToolCallID)
	require.Equal(t, toolSubagentReport, blocks[0].ToolName)

	payload, ok := blocks[0].Result.(map[string]any)
	require.True(t, ok)
	require.Equal(t, childID.String(), payload["chat_id"])
	require.Equal(t, requestID.String(), payload["request_id"])
	require.Equal(t, "child complete", payload["report"])
}

func TestSubagentService_MarkSubagentReportedWakesParentToPending(t *testing.T) {
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
			requestID := uuid.New()

			parent := testSubagentChat(parentID, uuid.Nil)
			parent.Status = parentStatus

			store := newSubagentServiceTestStore(
				parent,
				testSubagentChat(childID, parentID),
			)
			service := newSubagentService(store, nil)

			require.NoError(t, store.insertSubagentRequestMessage(childID, requestID, "run"))
			_, err := service.MarkSubagentReported(context.Background(), childID, "done", uuid.NullUUID{UUID: requestID, Valid: true})
			require.NoError(t, err)

			updatedParent, err := store.GetChatByID(context.Background(), parentID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusPending, updatedParent.Status)
		})
	}
}

func TestSubagentService_MarkSubagentReportedDoesNotWakeRunningParent(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()
	childID := uuid.New()
	requestID := uuid.New()

	parent := testSubagentChat(parentID, uuid.Nil)
	parent.Status = database.ChatStatusRunning

	store := newSubagentServiceTestStore(
		parent,
		testSubagentChat(childID, parentID),
	)
	service := newSubagentService(store, nil)

	require.NoError(t, store.insertSubagentRequestMessage(childID, requestID, "run"))
	_, err := service.MarkSubagentReported(context.Background(), childID, "done", uuid.NullUUID{UUID: requestID, Valid: true})
	require.NoError(t, err)

	updatedParent, err := store.GetChatByID(context.Background(), parentID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, updatedParent.Status)
}

func TestSubagentService_SendSubagentMessageRejectsNonDescendant(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	childAID := uuid.New()
	childBID := uuid.New()

	store := newSubagentServiceTestStore(
		testSubagentChat(rootID, uuid.Nil),
		testSubagentChat(childAID, rootID),
		testSubagentChat(childBID, rootID),
	)
	service := newSubagentService(store, nil)

	_, _, err := service.SendSubagentMessage(context.Background(), childAID, childBID, "follow up")
	require.ErrorIs(t, err, ErrSubagentNotDescendant)
}

func TestSubagentService_SendSubagentMessageRequeuesAndReturnsRequestID(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()
	childID := uuid.New()

	parent := testSubagentChat(parentID, uuid.Nil)
	child := testSubagentChat(childID, parentID)
	child.Status = database.ChatStatusCompleted

	store := newSubagentServiceTestStore(parent, child)
	service := newSubagentService(store, nil)

	updated, requestID, err := service.SendSubagentMessage(
		context.Background(),
		parentID,
		childID,
		"continue with more detail",
	)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, requestID)
	require.Equal(t, database.ChatStatusPending, updated.Status)

	stored, err := store.GetChatByID(context.Background(), childID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusPending, stored.Status)

	messages := store.chatMessagesByChatID(childID)
	require.Len(t, messages, 1)
	require.Equal(t, string(fantasy.MessageRoleUser), messages[0].Role)
	require.True(t, messages[0].SubagentRequestID.Valid)
	require.Equal(t, requestID, messages[0].SubagentRequestID.UUID)
	require.True(t, messages[0].SubagentEvent.Valid)
	require.Equal(t, subagentEventRequest, messages[0].SubagentEvent.String)

	blocks, err := parseContentBlocks(messages[0].Role, messages[0].Content)
	require.NoError(t, err)
	require.Equal(t, "continue with more detail", contentBlocksToText(blocks))
}

func TestSubagentService_SynthesizeFallbackSubagentReport(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	requestID := uuid.New()
	otherRequestID := uuid.New()

	store := newSubagentServiceTestStore(
		testSubagentChat(chatID, uuid.Nil),
	)
	service := newSubagentService(store, nil)

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
		SubagentRequestID: uuid.NullUUID{
			UUID:  otherRequestID,
			Valid: true,
		},
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
		SubagentRequestID: uuid.NullUUID{
			UUID:  requestID,
			Valid: true,
		},
	})
	require.NoError(t, err)

	report := service.SynthesizeFallbackSubagentReport(context.Background(), chatID, requestID)
	require.Equal(t, "latest summary", report)
}

func TestSubagentService_SynthesizeFallbackSubagentReportDefault(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	store := newSubagentServiceTestStore(
		testSubagentChat(chatID, uuid.Nil),
	)
	service := newSubagentService(store, nil)

	report := service.SynthesizeFallbackSubagentReport(context.Background(), chatID, uuid.New())
	require.Equal(t, defaultFallbackSubagentReport, report)
}

type subagentServiceTestStore struct {
	mu            sync.Mutex
	chats         map[uuid.UUID]database.Chat
	messages      []database.ChatMessage
	nextMessageID int64
}

func newSubagentServiceTestStore(chats ...database.Chat) *subagentServiceTestStore {
	byID := make(map[uuid.UUID]database.Chat, len(chats))
	for _, chat := range chats {
		byID[chat.ID] = chat
	}
	return &subagentServiceTestStore{
		chats:         byID,
		nextMessageID: 1,
	}
}

func (s *subagentServiceTestStore) GetChatByID(_ context.Context, id uuid.UUID) (database.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, ok := s.chats[id]
	if !ok {
		return database.Chat{}, sql.ErrNoRows
	}
	return chat, nil
}

func (s *subagentServiceTestStore) GetChatMessagesByChatID(_ context.Context, chatID uuid.UUID) ([]database.ChatMessage, error) {
	return s.chatMessagesByChatID(chatID), nil
}

func (s *subagentServiceTestStore) GetLatestPendingSubagentRequestIDByChatID(_ context.Context, chatID uuid.UUID) (uuid.NullUUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	requestedAt := make(map[uuid.UUID]time.Time)
	responded := make(map[uuid.UUID]struct{})
	for _, message := range s.messages {
		if message.ChatID != chatID || !message.SubagentRequestID.Valid {
			continue
		}
		requestID := message.SubagentRequestID.UUID
		if message.SubagentEvent.Valid {
			switch message.SubagentEvent.String {
			case subagentEventRequest:
				if current, ok := requestedAt[requestID]; !ok || message.CreatedAt.After(current) {
					requestedAt[requestID] = message.CreatedAt
				}
			case subagentEventResponse:
				responded[requestID] = struct{}{}
			}
		}
	}

	latestRequestID := uuid.Nil
	var latestRequestedAt time.Time
	for requestID, requestedAtTime := range requestedAt {
		if _, ok := responded[requestID]; ok {
			continue
		}
		if latestRequestID == uuid.Nil || requestedAtTime.After(latestRequestedAt) {
			latestRequestID = requestID
			latestRequestedAt = requestedAtTime
		}
	}

	if latestRequestID == uuid.Nil {
		return uuid.NullUUID{}, sql.ErrNoRows
	}
	return uuid.NullUUID{UUID: latestRequestID, Valid: true}, nil
}

func (s *subagentServiceTestStore) GetSubagentRequestDurationByChatIDAndRequestID(
	_ context.Context,
	arg database.GetSubagentRequestDurationByChatIDAndRequestIDParams,
) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		requestAt  time.Time
		responseAt time.Time
		hasRequest bool
		hasReply   bool
	)
	for _, message := range s.messages {
		if message.ChatID != arg.ChatID {
			continue
		}
		if !message.SubagentRequestID.Valid || message.SubagentRequestID.UUID != arg.SubagentRequestID {
			continue
		}
		if !message.SubagentEvent.Valid {
			continue
		}
		switch message.SubagentEvent.String {
		case subagentEventRequest:
			if !hasRequest || message.CreatedAt.Before(requestAt) {
				requestAt = message.CreatedAt
				hasRequest = true
			}
		case subagentEventResponse:
			if !hasReply || message.CreatedAt.After(responseAt) {
				responseAt = message.CreatedAt
				hasReply = true
			}
		}
	}

	if !hasRequest || !hasReply {
		return 0, nil
	}
	return responseAt.Sub(requestAt).Milliseconds(), nil
}

func (s *subagentServiceTestStore) GetSubagentResponseMessageByChatIDAndRequestID(
	_ context.Context,
	arg database.GetSubagentResponseMessageByChatIDAndRequestIDParams,
) (database.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.messages) - 1; i >= 0; i-- {
		message := s.messages[i]
		if message.ChatID != arg.ChatID {
			continue
		}
		if !message.SubagentRequestID.Valid || message.SubagentRequestID.UUID != arg.SubagentRequestID {
			continue
		}
		if !message.SubagentEvent.Valid || message.SubagentEvent.String != subagentEventResponse {
			continue
		}
		return message, nil
	}
	return database.ChatMessage{}, sql.ErrNoRows
}

func (s *subagentServiceTestStore) InsertChat(_ context.Context, arg database.InsertChatParams) (database.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	s.chats[chat.ID] = chat
	return chat, nil
}

func (s *subagentServiceTestStore) InsertChatMessage(_ context.Context, arg database.InsertChatMessageParams) (database.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	createdAt := time.Unix(0, 0).Add(time.Duration(s.nextMessageID) * time.Second)
	message := database.ChatMessage{
		ID:                s.nextMessageID,
		ChatID:            arg.ChatID,
		CreatedAt:         createdAt,
		Role:              arg.Role,
		Content:           arg.Content,
		ToolCallID:        arg.ToolCallID,
		Thinking:          arg.Thinking,
		Hidden:            arg.Hidden,
		SubagentRequestID: arg.SubagentRequestID,
		SubagentEvent:     arg.SubagentEvent,
	}
	s.nextMessageID++
	s.messages = append(s.messages, message)
	return message, nil
}

func (s *subagentServiceTestStore) ListChildChatsByParentID(_ context.Context, parentChatID uuid.UUID) ([]database.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]database.Chat, 0)
	for _, chat := range s.chats {
		if chat.ParentChatID.Valid && chat.ParentChatID.UUID == parentChatID {
			out = append(out, chat)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *subagentServiceTestStore) UpdateChatStatus(_ context.Context, arg database.UpdateChatStatusParams) (database.Chat, error) {
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

func (s *subagentServiceTestStore) chatMessagesByChatID(chatID uuid.UUID) []database.ChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]database.ChatMessage, 0)
	for _, message := range s.messages {
		if message.ChatID == chatID {
			out = append(out, message)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *subagentServiceTestStore) insertSubagentRequestMessage(chatID uuid.UUID, requestID uuid.UUID, text string) error {
	content, err := marshalContentBlocks([]fantasy.Content{fantasy.TextContent{Text: text}})
	if err != nil {
		return err
	}
	_, err = s.InsertChatMessage(context.Background(), database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    string(fantasy.MessageRoleUser),
		Content: content,
		SubagentRequestID: uuid.NullUUID{
			UUID:  requestID,
			Valid: true,
		},
		SubagentEvent: sql.NullString{String: subagentEventRequest, Valid: true},
	})
	return err
}

func (s *subagentServiceTestStore) insertSubagentResponseMessage(chatID uuid.UUID, requestID uuid.UUID, report string) error {
	content, err := marshalContentBlocks([]fantasy.Content{fantasy.TextContent{Text: report}})
	if err != nil {
		return err
	}
	_, err = s.InsertChatMessage(context.Background(), database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    subagentResponseMarkerRole,
		Content: content,
		Hidden:  true,
		SubagentRequestID: uuid.NullUUID{
			UUID:  requestID,
			Valid: true,
		},
		SubagentEvent: sql.NullString{String: subagentEventResponse, Valid: true},
	})
	return err
}

func testSubagentChat(id uuid.UUID, parentID uuid.UUID) database.Chat {
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
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}
