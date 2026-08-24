package chatd_test

// Shared helpers for chatd active-server tests.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type anthropicRequestRecorder struct {
	mu       sync.Mutex
	requests []chattest.AnthropicRequest
}

func newAnthropicRequestRecorder() *anthropicRequestRecorder {
	return &anthropicRequestRecorder{}
}

func (r *anthropicRequestRecorder) record(req *chattest.AnthropicRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, *req)
}

func (r *anthropicRequestRecorder) all() []chattest.AnthropicRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]chattest.AnthropicRequest(nil), r.requests...)
}

func filterAnthropicStreamingRequests(requests []chattest.AnthropicRequest) []chattest.AnthropicRequest {
	out := make([]chattest.AnthropicRequest, 0, len(requests))
	for _, req := range requests {
		if req.Stream {
			out = append(out, req)
		}
	}
	return out
}

func seedAnthropicChatDependencies(t *testing.T, db database.Store, baseURL string) (database.User, database.Organization, database.ChatModelConfig) {
	t.Helper()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic}, func(params *database.InsertAIProviderParams) {
		params.BaseUrl = baseURL
	})
	dbgen.AIProviderKey(t, db, database.AIProviderKey{ProviderID: provider.ID})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "claude-sonnet-4-20250514",
		IsDefault:    true,
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	})
	return user, org, model
}

func anthropicMessageHasEphemeralCacheControl(t *testing.T, message chattest.AnthropicRequestMessage) bool {
	t.Helper()
	return strings.Contains(string(message.Content), `"cache_control":{"type":"ephemeral"}`)
}

func anthropicRequestBody(t *testing.T, req chattest.AnthropicRequest) string {
	t.Helper()
	data, err := json.Marshal(req.Messages)
	require.NoError(t, err)
	return string(data)
}

func insertSystemTextMessage(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	text string,
	modelID uuid.UUID,
) {
	t.Helper()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)
	params := chatd.BuildSingleChatMessageInsertParams(
		chatID,
		database.ChatMessageRoleSystem,
		content,
		database.ChatMessageVisibilityBoth,
		modelID,
		chatprompt.CurrentContentVersion,
		uuid.Nil,
	)
	_, err = db.InsertChatMessages(ctx, params)
	require.NoError(t, err)
}

func insertOrphanProviderToolCall(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID, modelID uuid.UUID) {
	t.Helper()
	reasoningMetadata, err := json.Marshal(fantasy.ProviderMetadata{
		fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{RedactedData: "redacted-payload"},
	})
	require.NoError(t, err)
	parts := []codersdk.ChatMessagePart{
		{
			Type:             codersdk.ChatMessagePartTypeReasoning,
			ProviderMetadata: reasoningMetadata,
		},
		{
			Type:             codersdk.ChatMessagePartTypeToolCall,
			ToolCallID:       "ws-orphan",
			ToolName:         "web_search",
			Args:             json.RawMessage(`{"query":"coder"}`),
			ProviderExecuted: true,
		},
		codersdk.ChatMessageText("partial"),
	}
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	params := chatd.BuildSingleChatMessageInsertParams(
		chatID,
		database.ChatMessageRoleAssistant,
		content,
		database.ChatMessageVisibilityBoth,
		modelID,
		chatprompt.CurrentContentVersion,
		uuid.Nil,
	)
	_, err = db.InsertChatMessages(ctx, params)
	require.NoError(t, err)
}

func createChatThroughServer(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	server *chatd.Server,
	orgID uuid.UUID,
	userID uuid.UUID,
	modelID uuid.UUID,
	text string,
) database.Chat {
	t.Helper()
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     orgID,
		OwnerID:            userID,
		Title:              "test chat",
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)},
		ModelConfigID:      modelID,
	})
	require.NoError(t, err)
	return chat
}

func waitForChatStatus(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID, status database.ChatStatus) database.Chat {
	t.Helper()
	var chat database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		latest, err := db.GetChatByID(ctx, chatID)
		if err != nil {
			return false
		}
		chat = latest
		return latest.Status == status && !latest.WorkerID.Valid && !latest.RunnerID.Valid
	}, testutil.IntervalFast)
	return chat
}

func chatMessages(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) []database.ChatMessage {
	t.Helper()
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	require.NoError(t, err)
	return messages
}

func requireTextPart(t *testing.T, msg database.ChatMessage, text string) {
	t.Helper()
	parts, err := chatprompt.ParseContent(msg)
	require.NoError(t, err)
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText && part.Text == text {
			return
		}
	}
	t.Fatalf("missing text part %q in message %d", text, msg.ID)
}
