package chatd_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

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

// These tests cover local tool-call chain safety only. A provider-executed
// web-search replay fixture would require substantially more persisted
// Responses setup, so it is intentionally left out of this focused coverage.
func TestOpenAIResponsesUnsafeHistoryDisablesChainMode(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	recorder := newOpenAIResponsesRequestRecorder()
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		recorder.record(req)
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title":"OpenAI Responses unsafe chain test"}`)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Unsafe chain fallback complete.")...,
		)
	})

	user, org, _ := seedChatDependenciesWithProvider(ctx, t, db, "openai", openAIURL)
	model := insertOpenAIResponsesStoreModel(ctx, t, db, user.ID)
	chat := insertOpenAIResponsesChainChat(ctx, t, db, org.ID, user.ID, model.ID, "unsafe history")
	insertOpenAIResponsesMessages(ctx, t, db, chat.ID, user.ID, model.ID, []openAIResponsesMessageSeed{
		{
			role: database.ChatMessageRoleUser,
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("initial user marker for unsafe chain"),
			},
		},
		{
			role:               database.ChatMessageRoleAssistant,
			providerResponseID: "resp_unsafe",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("previous assistant marker for unsafe chain"),
				codersdk.ChatMessageToolCall("call_unsafe", "read_file", json.RawMessage(`{"path":"main.go"}`)),
			},
		},
		{
			role: database.ChatMessageRoleUser,
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("trailing user marker for unsafe chain"),
			},
		},
	})

	server := newActiveTestServer(t, db, ps)
	chatResult := waitForTerminalChat(ctx, t, db, chat.ID)
	chatd.WaitUntilIdleForTest(server)
	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chatResult.LastError.String)
	}

	streamed := recorder.streamingRequests()
	require.NotEmpty(t, streamed)
	first := streamed[0]
	require.Nil(t, first.PreviousResponseID)
	validateOpenAIResponsesPrompt(t, first.Prompt)
	require.True(t, openAIResponsesPromptContainsString(first.Prompt, "initial user marker for unsafe chain"))
	require.True(t, openAIResponsesPromptContainsString(first.Prompt, "previous assistant marker for unsafe chain"))
}

func TestOpenAIResponsesCleanHistoryPreservesChainMode(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	recorder := newOpenAIResponsesRequestRecorder()
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		recorder.record(req)
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title":"OpenAI Responses clean chain test"}`)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Clean chain complete.")...,
		)
	})

	user, org, _ := seedChatDependenciesWithProvider(ctx, t, db, "openai", openAIURL)
	model := insertOpenAIResponsesStoreModel(ctx, t, db, user.ID)
	chat := insertOpenAIResponsesChainChat(ctx, t, db, org.ID, user.ID, model.ID, "clean history")
	insertOpenAIResponsesMessages(ctx, t, db, chat.ID, user.ID, model.ID, []openAIResponsesMessageSeed{
		{
			role: database.ChatMessageRoleUser,
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("initial user marker for clean chain"),
			},
		},
		{
			role:               database.ChatMessageRoleAssistant,
			providerResponseID: "resp_clean",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("previous assistant marker for clean chain"),
			},
		},
		{
			role: database.ChatMessageRoleUser,
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("trailing user marker for clean chain"),
			},
		},
	})

	server := newActiveTestServer(t, db, ps)
	chatResult := waitForTerminalChat(ctx, t, db, chat.ID)
	chatd.WaitUntilIdleForTest(server)
	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chatResult.LastError.String)
	}

	streamed := recorder.streamingRequests()
	require.NotEmpty(t, streamed)
	first := streamed[0]
	require.NotNil(t, first.PreviousResponseID)
	require.Equal(t, "resp_clean", *first.PreviousResponseID)
	require.True(t, openAIResponsesPromptContainsString(first.Prompt, "trailing user marker for clean chain"))
	require.False(t, openAIResponsesPromptContainsString(first.Prompt, "initial user marker for clean chain"))
	require.False(t, openAIResponsesPromptContainsString(first.Prompt, "previous assistant marker for clean chain"))
}

type openAIResponsesRequestRecorder struct {
	mu       sync.Mutex
	requests []*chattest.OpenAIRequest
}

func newOpenAIResponsesRequestRecorder() *openAIResponsesRequestRecorder {
	return &openAIResponsesRequestRecorder{}
}

func (r *openAIResponsesRequestRecorder) record(req *chattest.OpenAIRequest) {
	var previousResponseID *string
	if req.PreviousResponseID != nil {
		value := *req.PreviousResponseID
		previousResponseID = &value
	}
	var store *bool
	if req.Store != nil {
		value := *req.Store
		store = &value
	}
	clone := &chattest.OpenAIRequest{
		Model:              req.Model,
		Messages:           append([]chattest.OpenAIMessage(nil), req.Messages...),
		Stream:             req.Stream,
		Tools:              append([]chattest.OpenAITool(nil), req.Tools...),
		Prompt:             append([]interface{}(nil), req.Prompt...),
		Store:              store,
		PreviousResponseID: previousResponseID,
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, clone)
}

func (r *openAIResponsesRequestRecorder) streamingRequests() []*chattest.OpenAIRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*chattest.OpenAIRequest, 0, len(r.requests))
	for _, req := range r.requests {
		if req.Stream {
			out = append(out, req)
		}
	}
	return out
}

type openAIResponsesMessageSeed struct {
	role               database.ChatMessageRole
	parts              []codersdk.ChatMessagePart
	providerResponseID string
}

func insertOpenAIResponsesStoreModel(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
) database.ChatModelConfig {
	t.Helper()
	storeEnabled := true
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
					Store: &storeEnabled,
				},
			},
		},
	)
}

func insertOpenAIResponsesChainChat(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	organizationID uuid.UUID,
	userID uuid.UUID,
	modelConfigID uuid.UUID,
	title string,
) database.Chat {
	t.Helper()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    organizationID,
		OwnerID:           userID,
		LastModelConfigID: modelConfigID,
		Title:             title,
		Status:            database.ChatStatusPending,
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
	userID uuid.UUID,
	modelConfigID uuid.UUID,
	seeds []openAIResponsesMessageSeed,
) {
	t.Helper()
	params := database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           make([]uuid.UUID, 0, len(seeds)),
		ModelConfigID:       make([]uuid.UUID, 0, len(seeds)),
		Role:                make([]database.ChatMessageRole, 0, len(seeds)),
		Content:             make([]string, 0, len(seeds)),
		ContentVersion:      make([]int16, 0, len(seeds)),
		Visibility:          make([]database.ChatMessageVisibility, 0, len(seeds)),
		InputTokens:         make([]int64, 0, len(seeds)),
		OutputTokens:        make([]int64, 0, len(seeds)),
		TotalTokens:         make([]int64, 0, len(seeds)),
		ReasoningTokens:     make([]int64, 0, len(seeds)),
		CacheCreationTokens: make([]int64, 0, len(seeds)),
		CacheReadTokens:     make([]int64, 0, len(seeds)),
		ContextLimit:        make([]int64, 0, len(seeds)),
		Compressed:          make([]bool, 0, len(seeds)),
		TotalCostMicros:     make([]int64, 0, len(seeds)),
		RuntimeMs:           make([]int64, 0, len(seeds)),
		ProviderResponseID:  make([]string, 0, len(seeds)),
	}
	for _, seed := range seeds {
		content, err := chatprompt.MarshalParts(seed.parts)
		require.NoError(t, err)
		params.CreatedBy = append(params.CreatedBy, userID)
		params.ModelConfigID = append(params.ModelConfigID, modelConfigID)
		params.Role = append(params.Role, seed.role)
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
		params.ProviderResponseID = append(params.ProviderResponseID, seed.providerResponseID)
	}
	_, err := db.InsertChatMessages(ctx, params)
	require.NoError(t, err)
}

func validateOpenAIResponsesPrompt(t *testing.T, prompt []interface{}) {
	t.Helper()
	calls := map[string]bool{}
	outputs := map[string]bool{}

	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]interface{}:
			if itemType, _ := typed["type"].(string); itemType != "" {
				switch itemType {
				case "web_search_call":
					require.FailNow(t, "prompt contains web_search_call history")
				case "function_call":
					callID := openAIResponsesPromptCallID(typed)
					require.NotEmpty(t, callID, "function_call item is missing a call ID")
					if _, ok := calls[callID]; ok {
						require.FailNowf(t, "duplicate function_call ID", "call_id=%q", callID)
					}
					if outputs[callID] {
						require.FailNowf(t, "function_call_output appeared before call", "call_id=%q", callID)
					}
					calls[callID] = false
				case "function_call_output":
					callID := openAIResponsesPromptCallID(typed)
					require.NotEmpty(t, callID, "function_call_output item is missing a call ID")
					if outputs[callID] {
						require.FailNowf(t, "duplicate function_call_output ID", "call_id=%q", callID)
					}
					if _, ok := calls[callID]; !ok {
						require.FailNowf(t, "function_call_output without prior call", "call_id=%q", callID)
					}
					calls[callID] = true
					outputs[callID] = true
				}
			}
			for _, nested := range typed {
				walk(nested)
			}
		case []interface{}:
			for _, nested := range typed {
				walk(nested)
			}
		}
	}
	walk(prompt)

	for callID, matched := range calls {
		if !matched {
			require.FailNowf(t, "function_call without later output", "call_id=%q", callID)
		}
	}
}

func openAIResponsesPromptCallID(item map[string]interface{}) string {
	if callID, _ := item["call_id"].(string); callID != "" {
		return callID
	}
	id, _ := item["id"].(string)
	return id
}

func openAIResponsesPromptContainsString(prompt []interface{}, want string) bool {
	return openAIResponsesValueContainsString(prompt, want)
}

func openAIResponsesValueContainsString(value any, want string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, want)
	case map[string]interface{}:
		for _, nested := range typed {
			if openAIResponsesValueContainsString(nested, want) {
				return true
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if openAIResponsesValueContainsString(nested, want) {
				return true
			}
		}
	}
	return false
}
