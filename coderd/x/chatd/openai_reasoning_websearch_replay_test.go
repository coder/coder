package chatd_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOpenAIReasoningWithWebSearchRoundTrip verifies that an OpenAI
// Responses API turn that produces both a reasoning item and a
// provider-executed web_search_call survives the persist, reload, and
// replay cycle when Store is true and chain mode is not active.
//
// Live OpenAI rejects a follow-up turn that replays a web_search_call
// (via item_reference) without its preceding reasoning item:
//
//	Item 'ws_xxx' of type 'web_search_call' was provided without its
//	required 'reasoning' item: 'rs_xxx'.
//
// To force the replay path (rather than chain mode, which short-circuits
// it via previous_response_id), turn 2 is dispatched against a different
// model_config_id. chatd then sends the full prior history to the
// provider and the fake server's ValidateResponsesAPIInput enforces the
// same pairing rule as live OpenAI.
func TestOpenAIReasoningWithWebSearchRoundTrip(t *testing.T) {
	t.Parallel()
	runOpenAIReasoningWithWebSearchRoundTripTest(t, true)
}

// TestOpenAIReasoningWithWebSearchRoundTripStoreFalse verifies the same
// flow with Store=false, where reasoning items must NOT be replayed
// (the IDs are ephemeral). This guards against accidentally emitting an
// item_reference for an unstored reasoning item, which would produce
// "Item not found" on real OpenAI.
func TestOpenAIReasoningWithWebSearchRoundTripStoreFalse(t *testing.T) {
	t.Parallel()
	runOpenAIReasoningWithWebSearchRoundTripTest(t, false)
}

func runOpenAIReasoningWithWebSearchRoundTripTest(t *testing.T, store bool) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)

	var (
		mu                sync.Mutex
		streamRequests    atomic.Int32
		validationFailure error
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title":"Reasoning + web search"}`)
		}

		// Validate the request input to mimic the live OpenAI 400.
		// Any orphan web_search_call or function_call surfaces here.
		if errResp := chattest.ValidateResponsesAPIInput(req.Input); errResp != nil {
			mu.Lock()
			if validationFailure == nil {
				validationFailure = &openAIValidationError{message: errResp.Message}
			}
			mu.Unlock()
			return chattest.OpenAIResponse{Error: errResp}
		}

		switch streamRequests.Add(1) {
		case 1:
			// Turn 1: emit reasoning + web_search_call + text.
			return chattest.OpenAIResponse{
				StreamingChunks: chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks("Here is what I found.")...,
				).StreamingChunks,
				Reasoning: &chattest.OpenAIReasoningItem{
					Summary:          "thinking about the question",
					EncryptedContent: "encrypted_data_here",
				},
				WebSearch: &chattest.OpenAIWebSearchCall{
					Query: "latest AI news",
				},
			}
		default:
			// Turn 2: text-only response.
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("Follow-up answer.")...,
			)
		}
	})

	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: deploymentValues,
	})
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	// gpt-5.5 is in fantasy's responsesReasoningModelIDs and routes
	// through the Responses API. Two model configs are created so
	// turn 2 can be sent against a different model_config_id, which
	// disables chain mode and forces the full-history replay path
	// where the reasoning + web_search pairing bug surfaces.
	contextLimit := int64(200000)
	isDefault := true
	notDefault := false
	reasoningSummary := "auto"
	openAIOptions := &codersdk.ChatModelOpenAIProviderOptions{
		Store:            ptr.Ref(store),
		ReasoningSummary: &reasoningSummary,
		WebSearchEnabled: ptr.Ref(true),
	}
	configA, err := expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "gpt-5.5",
		DisplayName:  "gpt-5.5 (turn 1)",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
		ModelConfig: &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: openAIOptions,
			},
		},
	})
	require.NoError(t, err)
	configB, err := expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "gpt-5.5",
		DisplayName:  "gpt-5.5 (turn 2)",
		ContextLimit: &contextLimit,
		IsDefault:    &notDefault,
		ModelConfig: &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: openAIOptions,
			},
		},
	})
	require.NoError(t, err)
	require.NotEqual(t, configA.ID, configB.ID,
		"two distinct model configs are required to disable chain mode")

	// --- Turn 1: produce reasoning + web_search_call + text ---
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		ModelConfigID:  &configA.ID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "What is the weather in San Francisco?",
			},
		},
	})
	require.NoError(t, err)

	events, closer, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closer.Close() })

	waitForChatDone(ctx, t, events, "step 1")

	chatData, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusWaiting, chatData.Status,
		"chat should be in waiting status after step 1")

	chatMsgs, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	logMessages(t, chatMsgs.Messages)

	// Turn 1's assistant message should contain reasoning,
	// tool-call, tool-result, and text parts.
	assistantMsg := findAssistantWithText(t, chatMsgs.Messages)
	require.NotNil(t, assistantMsg,
		"expected an assistant message with text after step 1")
	parts := partTypeSet(assistantMsg.Content)
	require.Contains(t, parts, codersdk.ChatMessagePartTypeReasoning,
		"assistant should contain reasoning parts")
	require.Contains(t, parts, codersdk.ChatMessagePartTypeToolCall,
		"assistant should contain a web_search tool call")
	require.Contains(t, parts, codersdk.ChatMessagePartTypeToolResult,
		"assistant should contain a web_search tool result")
	require.Contains(t, parts, codersdk.ChatMessagePartTypeText,
		"assistant should contain a text part")

	// --- Turn 2: send a follow-up against configB so chain mode is
	// disabled and chatd sends full history. Replay must satisfy
	// OpenAI's pairing rules; otherwise the fake server returns 400
	// and the chat lands in an Error state. ---
	_, err = expClient.CreateChatMessage(ctx, chat.ID,
		codersdk.CreateChatMessageRequest{
			ModelConfigID: &configB.ID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "And Tokyo?",
				},
			},
		})
	require.NoError(t, err)

	events2, closer2, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closer2.Close() })

	waitForChatDone(ctx, t, events2, "step 2")

	chatData2, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	chatMsgs2, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	logMessages(t, chatMsgs2.Messages)

	// If the replay is wrong, the fake server's validation failed
	// and chatd surfaced the error; surface a clear assertion.
	mu.Lock()
	failure := validationFailure
	mu.Unlock()
	if failure != nil {
		require.Failf(t,
			"OpenAI Responses API input validation failed during replay",
			"%s", failure.Error())
	}

	require.Equal(t, codersdk.ChatStatusWaiting, chatData2.Status,
		"chat should be in waiting status after step 2")
	require.Greater(t, len(chatMsgs2.Messages), len(chatMsgs.Messages),
		"follow-up should have added more messages")
	require.NotNil(t, findLastAssistantWithText(t, chatMsgs2.Messages),
		"expected an assistant message with text after step 2")

	// We expect exactly two streamed Responses API calls (one per
	// turn). Anything else suggests the chatd retry path masked a
	// validation failure.
	require.Equal(t, int32(2), streamRequests.Load(),
		"expected exactly two streamed OpenAI responses")
}

type openAIValidationError struct {
	message string
}

func (e *openAIValidationError) Error() string {
	return e.message
}
