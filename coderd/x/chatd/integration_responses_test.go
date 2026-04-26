package chatd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOpenAIResponsesNoStaleWebSearchReplay(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const (
		reasoningID = "rs_no_stale_reasoning"
		webSearchID = "ws_no_stale_search"
	)
	var recorder responsesRequestRecorder
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		requestNumber := recorder.record(req)
		switch requestNumber {
		case 1:
			resp := chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("search result summary")...,
			)
			resp.ResponseID = "resp_no_stale_first"
			resp.Reasoning = &chattest.OpenAIReasoningItem{
				ID:               reasoningID,
				Summary:          "checked provider-side search state",
				EncryptedContent: "encrypted-no-stale",
			}
			resp.WebSearch = &chattest.OpenAIWebSearchCall{
				ID:    webSearchID,
				Query: "coder changelog",
			}
			return resp
		default:
			resp := chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("follow-up answer")...,
			)
			resp.ResponseID = "resp_no_stale_second"
			return resp
		}
	})

	user, org, _ := seedChatDependenciesWithProvider(ctx, t, db, "openai", openAIURL)
	model := insertOpenAIResponsesModelConfig(ctx, t, db, user.ID, false, true)
	server := newActiveTestServer(t, db, ps)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          uniqueResponsesTitle(t, "no-stale"),
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("search for the latest Coder docs"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)
	require.Len(t, recorder.all(), 1)

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("summarize the result without searching again"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	requests := recorder.all()
	require.Len(t, requests, 2)
	followup := requests[1]
	require.NotNil(t, followup.Store)
	require.False(t, *followup.Store)
	require.Nil(t, followup.PreviousResponseID)
	require.NotEmpty(t, followup.Prompt)
	requireNoResponsesProviderItemReplay(t, followup.Prompt, reasoningID, webSearchID)
	require.NotContains(t, promptItemTypes(followup.Prompt), "web_search_call")
}

func TestOpenAIResponsesFullReplayPairsReasoningAndWebSearch(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const (
		reasoningID = "rs_full_replay_reasoning"
		webSearchID = "ws_full_replay_search"
	)
	var recorder responsesRequestRecorder
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		requestNumber := recorder.record(req)
		switch requestNumber {
		case 1:
			resp := chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("search result summary")...,
			)
			resp.ResponseID = "resp_full_replay_first"
			resp.Reasoning = &chattest.OpenAIReasoningItem{
				ID:               reasoningID,
				Summary:          "checked provider-side search state",
				EncryptedContent: "encrypted-full-replay",
			}
			resp.WebSearch = &chattest.OpenAIWebSearchCall{
				ID:    webSearchID,
				Query: "coder changelog",
			}
			return resp
		default:
			resp := chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("follow-up answer")...,
			)
			resp.ResponseID = "resp_full_replay_second"
			return resp
		}
	})

	user, org, _ := seedChatDependenciesWithProvider(ctx, t, db, "openai", openAIURL)
	firstModel := insertOpenAIResponsesModelConfig(ctx, t, db, user.ID, true, true)
	secondModel := insertOpenAIResponsesModelConfig(ctx, t, db, user.ID, true, true)
	server := newActiveTestServer(t, db, ps)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          uniqueResponsesTitle(t, "full-replay"),
		ModelConfigID:  firstModel.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("search for the latest Coder docs"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)
	require.Len(t, recorder.all(), 1)

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: secondModel.ID,
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("summarize the result without searching again"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	requests := recorder.all()
	require.Len(t, requests, 2)
	followup := requests[1]
	require.NotNil(t, followup.Store)
	require.True(t, *followup.Store)
	require.Nil(t, followup.PreviousResponseID)
	require.NotEmpty(t, followup.Prompt)
	requirePromptItemReferenceOrder(t, followup.Prompt, reasoningID, webSearchID)
}

func TestOpenAIResponsesChainModeSkipsWhenLocalCallPending(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var recorder responsesRequestRecorder
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		recorder.record(req)
		resp := chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("resolved after local call")...,
		)
		resp.ResponseID = "resp_local_pending_next"
		return resp
	})

	user, org, _ := seedChatDependenciesWithProvider(ctx, t, db, "openai", openAIURL)
	model := insertOpenAIResponsesModelConfig(ctx, t, db, user.ID, true, false)
	chat := insertOpenAIResponsesChat(ctx, t, db, org.ID, user.ID, model.ID, "local-pending")

	callID := fmt.Sprintf("call_local_%d", time.Now().UnixNano())
	localCall := codersdk.ChatMessageToolCall(
		callID,
		"read_file",
		json.RawMessage(`{"path":"README.md"}`),
	)
	insertOpenAIResponsesMessages(ctx, t, db, chat.ID, user.ID, model.ID,
		persistedResponsesMessage{
			role: database.ChatMessageRoleUser,
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("please inspect the README"),
			},
		},
		persistedResponsesMessage{
			role:               database.ChatMessageRoleAssistant,
			parts:              []codersdk.ChatMessagePart{localCall},
			providerResponseID: "resp_local_pending_prior",
		},
	)

	server := newActiveTestServer(t, db, ps)
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("continue after that tool call"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	requests := recorder.all()
	require.Len(t, requests, 1)
	request := requests[0]
	require.NotNil(t, request.Store)
	require.True(t, *request.Store)
	require.Nil(t, request.PreviousResponseID)
	require.NotEmpty(t, request.Prompt)
	requirePromptItemWithTypeAndCallID(t, request.Prompt, "function_call", callID)
	requirePromptItemWithTypeAndCallID(t, request.Prompt, "function_call_output", callID)
}

func TestOpenAIResponsesChainModeStillFiresForProviderExecutedOnly(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var recorder responsesRequestRecorder
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		recorder.record(req)
		resp := chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("chained answer")...,
		)
		resp.ResponseID = "resp_provider_only_next"
		return resp
	})

	user, org, _ := seedChatDependenciesWithProvider(ctx, t, db, "openai", openAIURL)
	model := insertOpenAIResponsesModelConfig(ctx, t, db, user.ID, true, true)
	chat := insertOpenAIResponsesChat(ctx, t, db, org.ID, user.ID, model.ID, "provider-only")

	const (
		previousResponseID = "resp_provider_only_prior"
		webSearchID        = "ws_provider_only_search"
	)
	webSearchCall := codersdk.ChatMessageToolCall(
		webSearchID,
		"web_search",
		json.RawMessage(`{"query":"coder docs"}`),
	)
	webSearchCall.ProviderExecuted = true
	webSearchResult := codersdk.ChatMessageToolResult(
		webSearchID,
		"web_search",
		json.RawMessage(`{"status":"completed"}`),
		false,
		false,
	)
	webSearchResult.ProviderExecuted = true
	insertOpenAIResponsesMessages(ctx, t, db, chat.ID, user.ID, model.ID,
		persistedResponsesMessage{
			role: database.ChatMessageRoleUser,
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("look up the docs"),
			},
		},
		persistedResponsesMessage{
			role: database.ChatMessageRoleAssistant,
			parts: []codersdk.ChatMessagePart{
				webSearchCall,
				webSearchResult,
			},
			providerResponseID: previousResponseID,
		},
	)

	server := newActiveTestServer(t, db, ps)
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("what did it find"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	requests := recorder.all()
	require.Len(t, requests, 1)
	request := requests[0]
	require.NotNil(t, request.Store)
	require.True(t, *request.Store)
	require.NotNil(t, request.PreviousResponseID)
	require.Equal(t, previousResponseID, *request.PreviousResponseID)
	require.NotEmpty(t, request.Prompt)
	requireNoResponsesProviderItemReplay(t, request.Prompt, webSearchID)
	require.NotContains(t, promptItemTypes(request.Prompt), "web_search_call")
	require.NotContains(t, promptItemRoles(request.Prompt), "assistant")
}

type recordedResponsesRequest struct {
	Prompt             []interface{}
	Store              *bool
	PreviousResponseID *string
}

type responsesRequestRecorder struct {
	mu       sync.Mutex
	requests []recordedResponsesRequest
}

func (r *responsesRequestRecorder) record(req *chattest.OpenAIRequest) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	var store *bool
	if req.Store != nil {
		value := *req.Store
		store = &value
	}
	var previousResponseID *string
	if req.PreviousResponseID != nil {
		value := *req.PreviousResponseID
		previousResponseID = &value
	}
	r.requests = append(r.requests, recordedResponsesRequest{
		Prompt:             append([]interface{}(nil), req.Prompt...),
		Store:              store,
		PreviousResponseID: previousResponseID,
	})
	return len(r.requests)
}

func (r *responsesRequestRecorder) all() []recordedResponsesRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedResponsesRequest(nil), r.requests...)
}

type persistedResponsesMessage struct {
	role               database.ChatMessageRole
	parts              []codersdk.ChatMessagePart
	providerResponseID string
}

func insertOpenAIResponsesModelConfig(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	store bool,
	webSearchEnabled bool,
) database.ChatModelConfig {
	t.Helper()
	return insertChatModelConfigWithCallConfig(
		ctx,
		t,
		db,
		userID,
		"openai",
		"gpt-4o",
		codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
					Store:            &store,
					WebSearchEnabled: &webSearchEnabled,
				},
			},
		},
	)
}

func insertOpenAIResponsesChat(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	organizationID uuid.UUID,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
	titlePrefix string,
) database.Chat {
	t.Helper()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    organizationID,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Title:             uniqueResponsesTitle(t, titlePrefix),
		Status:            database.ChatStatusWaiting,
		MCPServerIDs:      []uuid.UUID{},
		ClientType:        database.ChatClientTypeApi,
	})
	require.NoError(t, err)
	return chat
}

func insertOpenAIResponsesMessages(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	createdBy uuid.UUID,
	modelConfigID uuid.UUID,
	messages ...persistedResponsesMessage,
) {
	t.Helper()
	params := database.InsertChatMessagesParams{ChatID: chatID}
	for _, message := range messages {
		content, err := chatprompt.MarshalParts(message.parts)
		require.NoError(t, err)
		params.CreatedBy = append(params.CreatedBy, createdBy)
		params.ModelConfigID = append(params.ModelConfigID, modelConfigID)
		params.Role = append(params.Role, message.role)
		params.Content = append(params.Content, string(content.RawMessage))
		params.ContentVersion = append(params.ContentVersion, chatprompt.CurrentContentVersion)
		params.Visibility = append(params.Visibility, database.ChatMessageVisibilityBoth)
		params.InputTokens = append(params.InputTokens, 0)
		params.OutputTokens = append(params.OutputTokens, 0)
		params.TotalTokens = append(params.TotalTokens, 0)
		params.ReasoningTokens = append(params.ReasoningTokens, 0)
		params.CacheCreationTokens = append(params.CacheCreationTokens, 0)
		params.CacheReadTokens = append(params.CacheReadTokens, 0)
		params.ContextLimit = append(params.ContextLimit, 0)
		params.Compressed = append(params.Compressed, false)
		params.TotalCostMicros = append(params.TotalCostMicros, 0)
		params.RuntimeMs = append(params.RuntimeMs, 0)
		params.ProviderResponseID = append(params.ProviderResponseID, message.providerResponseID)
	}
	_, err := db.InsertChatMessages(ctx, params)
	require.NoError(t, err)
}

func requireResponsesChatWaiting(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
) {
	t.Helper()
	chat, err := db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	if chat.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chat.LastError.String)
	}
	require.Equal(t, database.ChatStatusWaiting, chat.Status)
}

func uniqueResponsesTitle(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s-%s-%d", prefix, t.Name(), time.Now().UnixNano())
}

func promptItemTypes(prompt []interface{}) []string {
	types := make([]string, 0, len(prompt))
	for _, item := range prompt {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemType := chattest.StringResponseField(itemMap, "type"); itemType != "" {
			types = append(types, itemType)
		}
	}
	return types
}

func promptItemRoles(prompt []interface{}) []string {
	roles := make([]string, 0, len(prompt))
	for _, item := range prompt {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if role := chattest.StringResponseField(itemMap, "role"); role != "" {
			roles = append(roles, role)
		}
	}
	return roles
}

func requirePromptItemWithTypeAndCallID(
	t *testing.T,
	prompt []interface{},
	itemType string,
	callID string,
) map[string]interface{} {
	t.Helper()
	for _, item := range prompt {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if chattest.StringResponseField(itemMap, "type") == itemType &&
			chattest.StringResponseField(itemMap, "call_id") == callID {
			return itemMap
		}
	}
	promptJSON, err := json.Marshal(prompt)
	require.NoError(t, err)
	require.FailNowf(t, "prompt item missing",
		"missing type=%q call_id=%q in prompt %s", itemType, callID, promptJSON)
	return nil
}

// requireNoResponsesProviderItemReplay rejects the explicit stale IDs and all
// provider-managed Responses item IDs. Chain mode should rely on
// previous_response_id, not replay rs_ or ws_ identifiers in prompt input.
func requireNoResponsesProviderItemReplay(
	t *testing.T,
	prompt []interface{},
	staleIDs ...string,
) {
	t.Helper()
	stale := make(map[string]struct{}, len(staleIDs))
	for _, id := range staleIDs {
		stale[id] = struct{}{}
	}
	for _, item := range prompt {
		assertNoResponsesProviderItemReplay(t, item, stale)
	}
}

func assertNoResponsesProviderItemReplay(
	t *testing.T,
	value interface{},
	staleIDs map[string]struct{},
) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, raw := range typed {
			if text, ok := raw.(string); ok {
				if key == "type" && text == "web_search_call" {
					require.FailNow(t, "prompt replayed web_search_call provider item")
				}
				if key == "id" || key == "call_id" || key == "item_id" {
					if _, isStale := staleIDs[text]; isStale {
						require.FailNowf(t, "prompt replayed stale provider item ID",
							"field %q contained stale provider ID %q", key, text)
					}
					if strings.HasPrefix(text, "ws_") || strings.HasPrefix(text, "rs_") {
						require.FailNowf(t, "prompt replayed provider item ID",
							"field %q contained provider-managed ID %q", key, text)
					}
				}
			}
			assertNoResponsesProviderItemReplay(t, raw, staleIDs)
		}
	case []interface{}:
		for _, item := range typed {
			assertNoResponsesProviderItemReplay(t, item, staleIDs)
		}
	}
}

func requirePromptItemReferenceOrder(
	t *testing.T,
	prompt []interface{},
	firstID string,
	secondID string,
) {
	t.Helper()
	firstIndex := -1
	secondIndex := -1
	for index, item := range prompt {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		itemID := chattest.StringResponseField(itemMap, "id")
		if itemID == "" {
			itemID = chattest.StringResponseField(itemMap, "item_id")
		}
		switch itemID {
		case firstID:
			firstIndex = index
		case secondID:
			secondIndex = index
		}
	}
	require.NotEqual(t, -1, firstIndex, "missing first item reference")
	require.NotEqual(t, -1, secondIndex, "missing second item reference")
	require.Less(t, firstIndex, secondIndex)
}
