package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOpenAIResponsesNoStaleWebSearchReplay checks that a follow-up after
// an unstored turn sends no web search ID or item_reference, which
// OpenAI cannot resolve, and sends the search inline instead.
func TestOpenAIResponsesNoStaleWebSearchReplay(t *testing.T) {
	t.Parallel()

	// Both cases, including the stored destination, send the search
	// inline right after the reasoning item.
	const wantSearchReplay = `{"type":"web_search_call","status":"completed","action":{"type":"search","query":"coder changelog"}}`
	for _, tc := range []struct {
		name             string
		destinationStore bool
		summary          string
	}{
		{name: "stateless", summary: "checked provider-side search state"},
		{name: "switch to stored", destinationStore: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
						Summary:          tc.summary,
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
			followupModel := model
			if tc.destinationStore {
				followupModel = insertOpenAIResponsesModelConfig(t, db, user.ID, true, false)
			}
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
				ModelConfigID: followupModel.ID,
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
			require.Equal(t, tc.destinationStore, *followup.Store)
			require.NotEmpty(t, followup.Prompt)
			requireNoResponsesProviderItemReplay(t, followup.Prompt, webSearchID)
			reasoningIndex := requireInlineReasoningItem(t, followup.Prompt, reasoningID, "encrypted-no-stale",
				tc.summary)
			require.Greater(t, len(followup.Prompt), reasoningIndex+1, "no item after the reasoning item")
			replayed, err := json.Marshal(followup.Prompt[reasoningIndex+1])
			require.NoError(t, err)
			require.JSONEq(t, wantSearchReplay, string(replayed))
		})
	}
}

func TestOpenAIResponsesStatelessReasoningReplay(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const (
		reasoningID = "rs_stateless_reasoning"
		blob        = "encrypted-stateless"
	)
	var recorder responsesRequestRecorder
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch recorder.record(req) {
		case 1:
			resp := chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("list_templates", `{}`),
			)
			resp.Reasoning = &chattest.OpenAIReasoningItem{
				ID:               reasoningID,
				EncryptedContent: blob,
			}
			return resp
		default:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("done")...,
			)
		}
	})

	user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	// An empty call config is the default deployment shape: no stored
	// responses and no requested reasoning summary.
	model := insertChatModelConfigWithCallConfig(t, db, user.ID, "openai", "gpt-4o",
		codersdk.ChatModelCallConfig{})
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newOpenAIResponsesTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          uniqueResponsesTitle(t, "stateless-reasoning"),
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("list the templates"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)
	require.Len(t, recorder.all(), 2)

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("thanks"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	requests := recorder.all()
	require.Len(t, requests, 3)
	for _, request := range requests {
		require.NotNil(t, request.Store)
		require.False(t, *request.Store)
		require.Contains(t, request.Include, "reasoning.encrypted_content")
	}
	// requests[1] continues the tool step from in-memory history;
	// requests[2] rebuilds the prompt from persisted messages.
	for _, request := range requests[1:] {
		reasoningIndex := requireInlineReasoningItem(t, request.Prompt, reasoningID, blob, "")
		require.NotContains(t, promptItemTypes(request.Prompt), "item_reference")
		functionCallIndex := slices.IndexFunc(request.Prompt, func(item interface{}) bool {
			itemMap, ok := item.(map[string]interface{})
			return ok && chattest.StringResponseField(itemMap, "type") == "function_call"
		})
		require.NotEqual(t, -1, functionCallIndex, "missing function_call item")
		require.Less(t, reasoningIndex, functionCallIndex, "types=%v", promptItemTypes(request.Prompt))
	}
}

func TestOpenAIResponsesReasoningNotReplayedAfterModelConfigChange(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		change func(cfg *database.UpdateChatModelConfigParams, otherProvider uuid.UUID)
	}{
		{
			name: "ProviderMoved",
			change: func(cfg *database.UpdateChatModelConfigParams, otherProvider uuid.UUID) {
				cfg.AIProviderID = uuid.NullUUID{UUID: otherProvider, Valid: true}
			},
		},
		{
			name: "ModelChanged",
			change: func(cfg *database.UpdateChatModelConfigParams, _ uuid.UUID) {
				cfg.Model = "gpt-4.1"
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			const (
				reasoningID = "rs_changed_config"
				blob        = "encrypted-changed-config"
			)
			var recorder responsesRequestRecorder
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("title")
				}
				resp := chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("answer")...)
				if recorder.record(req) == 1 {
					resp.Reasoning = &chattest.OpenAIReasoningItem{
						ID:               reasoningID,
						EncryptedContent: blob,
					}
				}
				return resp
			})

			user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
			model := insertChatModelConfigWithCallConfig(t, db, user.ID, "openai", "gpt-4o",
				codersdk.ChatModelCallConfig{})
			otherProvider := dbgen.ChatProvider(t, db, database.ChatProvider{
				Provider: "openai",
				BaseUrl:  openAIURL,
			})
			factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
			server := newOpenAIResponsesTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
			})

			chat, err := server.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				Title:          uniqueResponsesTitle(t, "changed-config"),
				ModelConfigID:  model.ID,
				InitialUserContent: []codersdk.ChatMessagePart{
					codersdk.ChatMessageText("hello"),
				},
			})
			require.NoError(t, err)
			waitForChatProcessed(ctx, t, db, chat.ID, server)
			requireResponsesChatWaiting(ctx, t, db, chat.ID)
			require.Len(t, recorder.all(), 1)

			update := database.UpdateChatModelConfigParams{
				ID:                   model.ID,
				Model:                model.Model,
				DisplayName:          model.DisplayName,
				Enabled:              model.Enabled,
				IsDefault:            model.IsDefault,
				ContextLimit:         model.ContextLimit,
				CompressionThreshold: model.CompressionThreshold,
				Options:              model.Options,
				AIProviderID:         model.AIProviderID,
			}
			tc.change(&update, otherProvider.ID)
			_, err = db.UpdateChatModelConfig(ctx, update)
			require.NoError(t, err)

			_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
				ChatID:        chat.ID,
				CreatedBy:     user.ID,
				ModelConfigID: model.ID,
				Content: []codersdk.ChatMessagePart{
					codersdk.ChatMessageText("thanks"),
				},
			})
			require.NoError(t, err)
			waitForChatProcessed(ctx, t, db, chat.ID, server)
			requireResponsesChatWaiting(ctx, t, db, chat.ID)

			requests := recorder.all()
			require.Len(t, requests, 2)
			followup := requests[1]
			require.NotEmpty(t, followup.Prompt)
			require.NotContains(t, promptItemTypes(followup.Prompt), "reasoning")
			requireNoResponsesProviderItemReplay(t, followup.Prompt, reasoningID)
			encoded, err := json.Marshal(followup.Prompt)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), blob)
		})
	}
}

func TestOpenAIResponsesRetriesWithoutRejectedEncryptedReasoning(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// freshReasoningAt is the request whose response carries the
		// first reasoning produced after the rejection.
		freshReasoningAt int
	}{
		{name: "RetryReturnsReasoning", freshReasoningAt: 3},
		{name: "RetryReturnsNoReasoning", freshReasoningAt: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			const (
				staleID   = "rs_rejected_reasoning"
				staleBlob = "encrypted-by-another-organization"
				freshID   = "rs_fresh_reasoning"
				freshBlob = "encrypted-by-current-organization"
			)
			var recorder responsesRequestRecorder
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("title")
				}
				n := recorder.record(req)
				// Later turns reach an organization that cannot decrypt the
				// reasoning from the first turn.
				if n > 1 && slices.ContainsFunc(req.Prompt, func(item interface{}) bool {
					itemMap, ok := item.(map[string]interface{})
					return ok && chattest.StringResponseField(itemMap, "id") == staleID
				}) {
					resp := chattest.OpenAIErrorResponse(http.StatusBadRequest, "invalid_request_error",
						"The encrypted content could not be verified.")
					resp.Error.Code = "invalid_encrypted_content"
					return resp
				}
				resp := chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
				if n == 3 {
					resp = chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("list_templates", `{}`))
				}
				switch n {
				case 1:
					resp.Reasoning = &chattest.OpenAIReasoningItem{ID: staleID, EncryptedContent: staleBlob}
				case tc.freshReasoningAt:
					resp.Reasoning = &chattest.OpenAIReasoningItem{ID: freshID, EncryptedContent: freshBlob}
				}
				return resp
			})

			user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
			model := insertChatModelConfigWithCallConfig(t, db, user.ID, "openai", "gpt-4o",
				codersdk.ChatModelCallConfig{})
			factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
			newServer := func() *chatd.Server {
				return newOpenAIResponsesTestServer(t, db, ps, func(cfg *chatd.Config) {
					cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
				})
			}
			server := newServer()
			sendMessage := func(server *chatd.Server, chatID uuid.UUID, text string) {
				t.Helper()
				_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
					ChatID:        chatID,
					CreatedBy:     user.ID,
					ModelConfigID: model.ID,
					Content: []codersdk.ChatMessagePart{
						codersdk.ChatMessageText(text),
					},
				})
				require.NoError(t, err)
				waitForChatProcessed(ctx, t, db, chatID, server)
				requireResponsesChatWaiting(ctx, t, db, chatID)
			}

			chat, err := server.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				Title:          uniqueResponsesTitle(t, "rejected-reasoning"),
				ModelConfigID:  model.ID,
				InitialUserContent: []codersdk.ChatMessagePart{
					codersdk.ChatMessageText("hello"),
				},
			})
			require.NoError(t, err)
			waitForChatProcessed(ctx, t, db, chat.ID, server)
			requireResponsesChatWaiting(ctx, t, db, chat.ID)
			sendMessage(server, chat.ID, "list the templates")

			// The next turn runs on a server that never saw the rejection,
			// like a restarted or different replica.
			require.NoError(t, server.Close())
			sendMessage(newServer(), chat.ID, "thanks")

			// Only the first request after the change is rejected.
			requests := recorder.all()
			require.Len(t, requests, 5)
			requireInlineReasoningItem(t, requests[1].Prompt, staleID, staleBlob, "")
			retry := requests[2]
			require.NotEmpty(t, retry.Prompt)
			require.NotContains(t, promptItemTypes(retry.Prompt), "reasoning")
			// requests[3] is the step after the tool call; requests[4] is
			// the next turn on the fresh server.
			for i, request := range requests[3:] {
				requireNoResponsesProviderItemReplay(t, request.Prompt, staleID)
				if i+3 < tc.freshReasoningAt {
					require.NotContains(t, promptItemTypes(request.Prompt), "reasoning")
					continue
				}
				requireInlineReasoningItem(t, request.Prompt, freshID, freshBlob, "")
			}
		})
	}
}

func TestOpenAIResponsesReasoningNotReplayedAfterCredentialChange(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const (
		userAPIKey      = "sk-user-reasoning"
		deploymentID    = "rs_deployment_key"
		deploymentBlob  = "encrypted-by-deployment-organization"
		personalKeyID   = "rs_personal_key"
		personalKeyBlob = "encrypted-by-personal-organization"
	)
	var (
		recorder   responsesRequestRecorder
		authMu     sync.Mutex
		authByCall = map[int]string{}
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		n := recorder.record(req)
		authMu.Lock()
		authByCall[n] = req.Header.Get("Authorization")
		authMu.Unlock()
		resp := chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("answer")...)
		switch n {
		case 1:
			resp.Reasoning = &chattest.OpenAIReasoningItem{ID: deploymentID, EncryptedContent: deploymentBlob}
		case 2:
			resp.Reasoning = &chattest.OpenAIReasoningItem{ID: personalKeyID, EncryptedContent: personalKeyBlob}
		}
		return resp
	})

	user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	model := insertChatModelConfigWithCallConfig(t, db, user.ID, "openai", "gpt-4o",
		codersdk.ChatModelCallConfig{})
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newOpenAIResponsesTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          uniqueResponsesTitle(t, "credential-change"),
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	_, err = db.UpsertUserAIProviderKey(ctx, database.UpsertUserAIProviderKeyParams{
		ID:           uuid.New(),
		UserID:       user.ID,
		AIProviderID: model.AIProviderID.UUID,
		APIKey:       userAPIKey,
	})
	require.NoError(t, err)

	for _, text := range []string{"switched keys", "thanks"} {
		_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText(text),
			},
		})
		require.NoError(t, err)
		waitForChatProcessed(ctx, t, db, chat.ID, server)
		requireResponsesChatWaiting(ctx, t, db, chat.ID)
	}

	requests := recorder.all()
	require.Len(t, requests, 3)
	authMu.Lock()
	require.NotEqual(t, "Bearer "+userAPIKey, authByCall[1])
	require.Equal(t, "Bearer "+userAPIKey, authByCall[2])
	authMu.Unlock()
	// Reasoning from the deployment's key is left out before the
	// personal key's organization can reject it.
	require.NotContains(t, promptItemTypes(requests[1].Prompt), "reasoning")
	requireNoResponsesProviderItemReplay(t, requests[1].Prompt, deploymentID)
	requireInlineReasoningItem(t, requests[2].Prompt, personalKeyID, personalKeyBlob, "")
	requireNoResponsesProviderItemReplay(t, requests[2].Prompt, deploymentID)
}

func TestOpenAIResponsesAdvisorPromptOmitsReasoning(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const (
		reasoningID = "rs_parent_reasoning"
		blob        = "encrypted-parent-reasoning"
	)
	var recorder responsesRequestRecorder
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch recorder.record(req) {
		case 1:
			resp := chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("answer")...)
			resp.Reasoning = &chattest.OpenAIReasoningItem{
				ID:               reasoningID,
				EncryptedContent: blob,
			}
			return resp
		case 2:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("advisor", `{"question":"what next?"}`),
			)
		case 3:
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("advice")...)
		default:
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		}
	})

	user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	model := insertChatModelConfigWithCallConfig(t, db, user.ID, "openai", "gpt-4o",
		codersdk.ChatModelCallConfig{})
	seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   1,
		MaxOutputTokens: 1024,
	})
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newOpenAIResponsesTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          uniqueResponsesTitle(t, "advisor-reasoning"),
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("ask the advisor"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	requests := recorder.all()
	require.Len(t, requests, 4)
	requireInlineReasoningItem(t, requests[1].Prompt, reasoningID, blob, "")
	advisorRequest := requests[2]
	require.NotEmpty(t, advisorRequest.Prompt)
	require.NotContains(t, promptItemTypes(advisorRequest.Prompt), "reasoning")
	encoded, err := json.Marshal(advisorRequest.Prompt)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "what next?")
	require.NotContains(t, string(encoded), blob)
	requireInlineReasoningItem(t, requests[3].Prompt, reasoningID, blob, "")
}

func TestOpenAIResponsesPersistsProviderResponseID(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		resp := chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("answer")...)
		resp.ResponseID = "resp_persisted"
		return resp
	})

	user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	model := insertOpenAIResponsesModelConfig(t, db, user.ID, false, false)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newOpenAIResponsesTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          uniqueResponsesTitle(t, "response-id"),
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	responseIDs := make(map[database.ChatMessageRole]sql.NullString)
	for _, message := range messages {
		responseIDs[message.Role] = message.ProviderResponseID
	}
	require.Equal(t, sql.NullString{String: "resp_persisted", Valid: true}, responseIDs[database.ChatMessageRoleAssistant])
	require.False(t, responseIDs[database.ChatMessageRoleUser].Valid)
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
	// The referenced search must not also be sent inline.
	require.NotContains(t, promptItemTypes(followup.Prompt), "web_search_call")
}

func TestOpenAIResponsesNarrationKeepsPhase(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var recorder responsesRequestRecorder
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if recorder.record(req) > 1 {
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("follow-up answer")...)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAIChunk{Choices: []chattest.OpenAIChunkChoice{{Index: 0, Delta: "Checking the docs.", Phase: "commentary"}}},
			chattest.OpenAIChunk{Choices: []chattest.OpenAIChunkChoice{{Index: 1, Delta: "The docs cover it.", Phase: "final_answer"}}},
		)
	})

	user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	model := insertOpenAIResponsesModelConfig(t, db, user.ID, true, false)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newOpenAIResponsesTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          uniqueResponsesTitle(t, "narration"),
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("do the docs cover this?"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	var assistant []codersdk.ChatMessagePart
	for _, message := range messages {
		if message.Role == database.ChatMessageRoleAssistant {
			assistant = append(assistant, db2sdk.ChatMessage(message).Content...)
		}
	}
	narration := codersdk.ChatMessageText("Checking the docs.")
	narration.Narration = true
	require.Equal(t, []codersdk.ChatMessagePart{
		narration,
		codersdk.ChatMessageText("The docs cover it."),
	}, assistant)

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("thanks"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)
	requireResponsesChatWaiting(ctx, t, db, chat.ID)

	requests := recorder.all()
	require.Len(t, requests, 2)
	var replayed [][2]any
	for _, item := range requests[1].Prompt {
		itemMap, ok := item.(map[string]interface{})
		if !ok || chattest.StringResponseField(itemMap, "role") != "assistant" {
			continue
		}
		replayed = append(replayed, [2]any{itemMap["content"], itemMap["phase"]})
	}
	require.Equal(t, [][2]any{
		{"Checking the docs.", "commentary"},
		{"The docs cover it.", "final_answer"},
	}, replayed)
}

type recordedResponsesRequest struct {
	Prompt  []interface{}
	Store   *bool
	Include []string
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
	var body struct {
		Include []string `json:"include"`
	}
	_ = json.Unmarshal(req.RawBody, &body)
	r.requests = append(r.requests, recordedResponsesRequest{
		Prompt:  append([]interface{}(nil), req.Prompt...),
		Store:   store,
		Include: body.Include,
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
		} else if chattest.ResponsesItemReferenceID(itemMap) != "" {
			types = append(types, "item_reference")
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
		if chattest.ResponsesItemReferenceID(typed) != "" {
			require.FailNow(t, "prompt replayed provider item reference")
		}
		for key, raw := range typed {
			if text, ok := raw.(string); ok {
				if key == "id" || key == "call_id" || key == "item_id" {
					if _, isStale := staleIDs[text]; isStale {
						require.FailNowf(t, "prompt replayed stale provider item ID",
							"field %q contained stale provider ID %q", key, text)
					}
					// Finalized reasoning is replayed inline, so only hosted
					// tool items are provider-managed state here.
					if strings.HasPrefix(text, "ws_") {
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

// requireInlineReasoningItem asserts that the prompt replays the reasoning
// item in full and returns its index. An empty summary must still be sent
// as an empty array.
func requireInlineReasoningItem(
	t *testing.T,
	prompt []interface{},
	id string,
	encryptedContent string,
	summary string,
) int {
	t.Helper()
	for index, item := range prompt {
		itemMap, ok := item.(map[string]interface{})
		if !ok || chattest.StringResponseField(itemMap, "type") != "reasoning" ||
			chattest.StringResponseField(itemMap, "id") != id {
			continue
		}
		require.Equal(t, encryptedContent, chattest.StringResponseField(itemMap, "encrypted_content"))
		summaryItems, ok := itemMap["summary"].([]interface{})
		require.True(t, ok, "reasoning summary must be an array, got %T", itemMap["summary"])
		var texts []string
		for _, summaryItem := range summaryItems {
			summaryMap, ok := summaryItem.(map[string]interface{})
			require.True(t, ok)
			texts = append(texts, chattest.StringResponseField(summaryMap, "text"))
		}
		require.Equal(t, summary, strings.Join(texts, ""))
		return index
	}
	require.FailNowf(t, "missing inline reasoning item", "id=%q types=%v", id, promptItemTypes(prompt))
	return -1
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
