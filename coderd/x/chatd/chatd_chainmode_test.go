package chatd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestActiveServer_ChainBrokenRecovery(t *testing.T) {
	t.Parallel()

	const (
		previousResponseID = "resp_poisoned"
		recoveredAnswer    = "recovered answer"
	)
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newOpenAIRequestRecorder()
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		requests.record(req)
		if req.PreviousResponseID != nil {
			return chattest.OpenAIErrorResponse(http.StatusNotFound, "invalid_request_error", chainBrokenProviderErrorMessage)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks(recoveredAnswer)...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	model = updateModelForChainMode(t, db, model)

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "first user")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	insertProviderResponseID(ctx, t, db, chat.ID, "first assistant", model.ID, previousResponseID)
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		APIKeyID:      testAPIKeyID(t, db, user.ID),
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("follow up")},
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)

	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	got := requests.all()
	require.GreaterOrEqual(t, len(got), 3)
	generationRequests := filterStreamingRequests(got)
	require.Len(t, generationRequests, 3)
	require.Nil(t, generationRequests[0].PreviousResponseID)
	require.Equal(t, previousResponseID, requirePreviousResponseID(t, generationRequests[1]))
	require.Nil(t, generationRequests[2].PreviousResponseID)
	requireRawPromptContains(t, generationRequests[2], "first user")
	requireRawPromptContains(t, generationRequests[2], "first assistant")
	requireRawPromptContains(t, generationRequests[2], "follow up")

	messages := chatMessages(ctx, t, db, chat.ID)
	requireTextPart(t, messages[len(messages)-1], recoveredAnswer)
}

func TestActiveServer_ChainBrokenRecoveryAppliesProviderPromptPrep(t *testing.T) {
	t.Parallel()

	const previousResponseID = "resp_anthropic_chain"
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newAnthropicRequestRecorder()
	var streamCalls atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requests.record(req)
		if streamCalls.Add(1) == 2 {
			return chattest.AnthropicErrorResponse(http.StatusInternalServerError, "server_error", chainBrokenProviderErrorMessage)
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("anthropic answer")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateModelForChainMode(t, db, model)

	factory := chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath())
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	insertSystemTextMessage(ctx, t, db, chat.ID, "sys-1", model.ID)
	insertProviderResponseID(ctx, t, db, chat.ID, "hi", model.ID, previousResponseID)
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		APIKeyID:      testAPIKeyID(t, db, user.ID),
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("follow up")},
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)

	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	generationRequests := filterAnthropicStreamingRequests(requests.all())
	require.Len(t, generationRequests, 2)
	recovered := generationRequests[1]
	require.Len(t, recovered.Messages, 4)
	require.True(t, anthropicSystemHasEphemeralCacheControl(t, recovered))
	require.False(t, anthropicMessageHasEphemeralCacheControl(t, recovered.Messages[0]))
	require.False(t, anthropicMessageHasEphemeralCacheControl(t, recovered.Messages[1]))
	require.True(t, anthropicMessageHasEphemeralCacheControl(t, recovered.Messages[2]))
	require.True(t, anthropicMessageHasEphemeralCacheControl(t, recovered.Messages[3]))
}

func TestActiveServer_NonChainBrokenRetryPreservesChainMode(t *testing.T) {
	t.Parallel()

	const previousResponseID = "resp_still_valid"
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newOpenAIRequestRecorder()
	var streamCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		requests.record(req)
		if req.Stream && streamCalls.Add(1) == 2 {
			return chattest.OpenAIServerErrorResponse()
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("answer")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	model = updateModelForChainMode(t, db, model)

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "first user")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	insertProviderResponseID(ctx, t, db, chat.ID, "first assistant", model.ID, previousResponseID)
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		APIKeyID:      testAPIKeyID(t, db, user.ID),
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("follow up")},
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)

	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	generationRequests := filterStreamingRequests(requests.all())
	require.Len(t, generationRequests, 3)
	require.Equal(t, previousResponseID, requirePreviousResponseID(t, generationRequests[1]))
	require.Equal(t, previousResponseID, requirePreviousResponseID(t, generationRequests[2]))
	requireRawPromptNotContains(t, generationRequests[2], "first user")
	requireRawPromptContains(t, generationRequests[2], "follow up")
}

func TestActiveServer_ChainBrokenRecoveryPersistsAcrossGenerationActions(t *testing.T) {
	t.Parallel()

	const previousResponseID = "resp_tool_poisoned"
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newOpenAIRequestRecorder()
	var streamCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		requests.record(req)
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title":"test"}`)
		}
		switch streamCalls.Add(1) {
		case 1:
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("first answer")...)
		case 2:
			return chattest.OpenAIErrorResponse(http.StatusNotFound, "invalid_request_error", chainBrokenProviderErrorMessage)
		case 3:
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_skill", `{"name":"x"}`))
		default:
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("final answer")...)
		}
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	model = updateModelForChainMode(t, db, model)

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "first user")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	insertProviderResponseID(ctx, t, db, chat.ID, "first assistant", model.ID, previousResponseID)
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		APIKeyID:      testAPIKeyID(t, db, user.ID),
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("follow up")},
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)

	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	generationRequests := filterStreamingRequests(requests.all())
	require.Len(t, generationRequests, 4)
	require.Equal(t, previousResponseID, requirePreviousResponseID(t, generationRequests[1]))
	require.Nil(t, generationRequests[2].PreviousResponseID)
	require.Nil(t, generationRequests[3].PreviousResponseID)
}

func TestActiveServer_ChainBrokenWithoutChainModeIsSafe(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newOpenAIRequestRecorder()
	var streamCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		requests.record(req)
		if req.Stream && streamCalls.Add(1) == 1 {
			return chattest.OpenAIErrorResponse(http.StatusNotFound, "invalid_request_error", chainBrokenProviderErrorMessage)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("recovered")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	model = updateModelForChainMode(t, db, model)

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "only user")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	generationRequests := filterStreamingRequests(requests.all())
	require.Len(t, generationRequests, 2)
	require.Nil(t, generationRequests[0].PreviousResponseID)
	require.Nil(t, generationRequests[1].PreviousResponseID)
}

func TestActiveServer_ChainBrokenRecoveryDropsOrphanProviderToolCall(t *testing.T) {
	t.Parallel()

	const previousResponseID = "resp_orphan_provider_tool"
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newAnthropicRequestRecorder()
	var streamCalls atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requests.record(req)
		if streamCalls.Add(1) == 2 {
			return chattest.AnthropicErrorResponse(http.StatusInternalServerError, "server_error", chainBrokenProviderErrorMessage)
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("cleaned")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateModelForChainMode(t, db, model)

	factory := chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath())
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "first user")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	insertProviderResponseID(ctx, t, db, chat.ID, "first assistant", model.ID, previousResponseID)
	insertOrphanProviderToolCall(ctx, t, db, chat.ID, model.ID)
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		APIKeyID:      testAPIKeyID(t, db, user.ID),
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue")},
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)

	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	generationRequests := filterAnthropicStreamingRequests(requests.all())
	require.Len(t, generationRequests, 2)
	recoveredBody := anthropicRequestBody(t, generationRequests[1])
	require.NotContains(t, recoveredBody, "web_search")
	require.Contains(t, recoveredBody, "partial")
	require.Contains(t, recoveredBody, "continue")
	requireAnthropicRequestRedactedReasoning(t, generationRequests[1], "redacted-payload")
}

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
	_ = testAPIKeyID(t, db, user.ID)
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

func anthropicSystemHasEphemeralCacheControl(t *testing.T, req chattest.AnthropicRequest) bool {
	t.Helper()
	return strings.Contains(string(req.System), `"cache_control":{"type":"ephemeral"}`)
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

func requireAnthropicRequestRedactedReasoning(t *testing.T, req chattest.AnthropicRequest, redactedData string) {
	t.Helper()
	body := anthropicRequestBody(t, req)
	require.Contains(t, body, "redacted-payload")
	require.Contains(t, body, redactedData)
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

const chainBrokenProviderErrorMessage = "Previous response with id 'resp_abc' not found."

type openAIRequestRecorder struct {
	mu       sync.Mutex
	requests []chattest.OpenAIRequest
}

func newOpenAIRequestRecorder() *openAIRequestRecorder {
	return &openAIRequestRecorder{}
}

func (r *openAIRequestRecorder) record(req *chattest.OpenAIRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, *req)
}

func (r *openAIRequestRecorder) all() []chattest.OpenAIRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]chattest.OpenAIRequest(nil), r.requests...)
}

func updateModelForChainMode(t *testing.T, db database.Store, model database.ChatModelConfig) database.ChatModelConfig {
	t.Helper()
	store := true
	options, err := json.Marshal(codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{Store: &store},
		},
	})
	require.NoError(t, err)
	updated, err := db.UpdateChatModelConfig(context.Background(), database.UpdateChatModelConfigParams{
		ID:                   model.ID,
		DisplayName:          model.DisplayName,
		Model:                model.Model,
		Enabled:              model.Enabled,
		ContextLimit:         model.ContextLimit,
		CompressionThreshold: model.CompressionThreshold,
		Options:              options,
		AIProviderID:         model.AIProviderID,
	})
	require.NoError(t, err)
	return updated
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
		APIKeyID:           testAPIKeyID(t, db, userID),
		Title:              "chain mode test",
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

func insertProviderResponseID(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	text string,
	modelID uuid.UUID,
	providerResponseID string,
) {
	t.Helper()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
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
	params.ProviderResponseID[0] = providerResponseID
	_, err = db.InsertChatMessages(ctx, params)
	require.NoError(t, err)
}

func chatMessages(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) []database.ChatMessage {
	t.Helper()
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	require.NoError(t, err)
	return messages
}

func filterStreamingRequests(requests []chattest.OpenAIRequest) []chattest.OpenAIRequest {
	out := make([]chattest.OpenAIRequest, 0, len(requests))
	for _, req := range requests {
		if req.Stream {
			out = append(out, req)
		}
	}
	return out
}

func requirePreviousResponseID(t *testing.T, req chattest.OpenAIRequest) string {
	t.Helper()
	require.NotNil(t, req.PreviousResponseID)
	return *req.PreviousResponseID
}

func requireRawPromptContains(t *testing.T, req chattest.OpenAIRequest, text string) {
	t.Helper()
	require.Contains(t, string(req.RawBody), text)
}

func requireRawPromptNotContains(t *testing.T, req chattest.OpenAIRequest, text string) {
	t.Helper()
	require.NotContains(t, string(req.RawBody), text)
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
