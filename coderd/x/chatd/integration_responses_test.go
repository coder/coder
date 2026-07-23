package chatd_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
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

	user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	model := insertOpenAIResponsesModelConfig(t, db, user.ID, false, true)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newOpenAIResponsesTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

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

	user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	firstModel := insertOpenAIResponsesModelConfig(t, db, user.ID, true, true)
	secondModel := insertOpenAIResponsesModelConfig(t, db, user.ID, true, true)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newOpenAIResponsesTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

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
	require.NotEmpty(t, followup.Prompt)
	requirePromptItemReferenceOrder(t, followup.Prompt, reasoningID, webSearchID)
}

type recordedResponsesRequest struct {
	Prompt []interface{}
	Store  *bool
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
	r.requests = append(r.requests, recordedResponsesRequest{
		Prompt: append([]interface{}(nil), req.Prompt...),
		Store:  store,
	})
	return len(r.requests)
}

func (r *responsesRequestRecorder) all() []recordedResponsesRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedResponsesRequest(nil), r.requests...)
}

func newOpenAIResponsesTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	overrides ...func(*chatd.Config),
) *chatd.Server {
	t.Helper()
	allOverrides := append([]func(*chatd.Config){func(cfg *chatd.Config) {
		// Let CreateChat and SendMessage publish their pending status
		// before wake-driven processing starts. The responses tests are
		// not exercising periodic polling, and PostgreSQL can otherwise
		// deliver that stale pending notification after processChat
		// subscribes to control events.
		cfg.PendingChatAcquireInterval = testutil.WaitLong
	}}, overrides...)
	return newActiveTestServer(t, db, ps, allOverrides...)
}

func insertOpenAIResponsesModelConfig(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	store bool,
	webSearchEnabled bool,
) database.ChatModelConfig {
	t.Helper()
	return insertChatModelConfigWithCallConfig(
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
		require.FailNowf(t, "chat failed", "last_error=%q", chatLastErrorMessage(chat.LastError))
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
