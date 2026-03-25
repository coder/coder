package chatd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
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

// TestAnthropicWebSearchRoundTrip is an integration test that verifies
// provider-executed tool results (web_search) survive the full
// persist → reconstruct → re-send cycle. It sends a query that
// triggers Anthropic's web_search server tool, waits for completion,
// then sends a follow-up message. If the PE tool result was lost or
// corrupted during persistence, Anthropic rejects the second request:
//
//	web_search tool use with id srvtoolu_... was found without a
//	corresponding web_search_tool_result block
//
// The test requires ANTHROPIC_API_KEY to be set.
func TestAnthropicWebSearchRoundTrip(t *testing.T) {
	t.Parallel()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping Anthropic integration test")
	}
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Stand up a full coderd with the agents experiment.
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: deploymentValues,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	// Configure an Anthropic provider with the real API key.
	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "anthropic",
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})
	require.NoError(t, err)

	// Create a model config that enables web_search.
	contextLimit := int64(200000)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
		ModelConfig: &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
					WebSearchEnabled: ptr.Ref(true),
				},
			},
		},
	})
	require.NoError(t, err)

	// --- Step 1: Send a message that triggers web_search ---
	t.Log("Creating chat with web search query...")
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "What is the current weather in San Francisco right now? Use web search to find out.",
			},
		},
	})
	require.NoError(t, err)
	t.Logf("Chat created: %s (status=%s)", chat.ID, chat.Status)

	// Stream events until the chat reaches a terminal status.
	events, closer, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer.Close()

	waitForChatDone(ctx, t, events, "step 1")

	// Verify the chat completed and messages were persisted.
	chatData, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	chatMsgs, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	t.Logf("Chat status after step 1: %s, messages: %d",
		chatData.Status, len(chatMsgs.Messages))
	logMessages(t, chatMsgs.Messages)

	require.Equal(t, codersdk.ChatStatusWaiting, chatData.Status,
		"chat should be in waiting status after step 1")

	// Find the first assistant message and verify it has the
	// content parts the UI needs to render web search results:
	// tool-call(PE), source, tool-result(PE), and text.
	assistantMsg := findAssistantWithText(t, chatMsgs.Messages)
	require.NotNil(t, assistantMsg,
		"expected an assistant message with text content after step 1")

	partTypes := partTypeSet(assistantMsg.Content)
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeToolCall,
		"assistant message should contain a PE tool-call part")
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeSource,
		"assistant message should contain source parts for UI citations")
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeToolResult,
		"assistant message should contain a PE tool-result part")
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeText,
		"assistant message should contain a text part")

	// Verify the PE tool-call is marked as provider-executed.
	for _, part := range assistantMsg.Content {
		if part.Type == codersdk.ChatMessagePartTypeToolCall {
			require.True(t, part.ProviderExecuted,
				"web_search tool-call should be provider-executed")
			break
		}
	}

	// --- Step 2: Send a follow-up message ---
	// This is the critical test: if PE tool results were lost during
	// persistence, the reconstructed conversation will be rejected
	// by Anthropic because server_tool_use has no matching
	// web_search_tool_result.
	t.Log("Sending follow-up message...")
	_, err = expClient.CreateChatMessage(ctx, chat.ID,
		codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Thanks! What about New York?",
				},
			},
		})
	require.NoError(t, err)

	// Stream the follow-up response.
	events2, closer2, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer2.Close()

	waitForChatDone(ctx, t, events2, "step 2")

	// Verify the follow-up completed and produced content.
	chatData2, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	chatMsgs2, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	t.Logf("Chat status after step 2: %s, messages: %d",
		chatData2.Status, len(chatMsgs2.Messages))
	logMessages(t, chatMsgs2.Messages)

	require.Equal(t, codersdk.ChatStatusWaiting, chatData2.Status,
		"chat should be in waiting status after step 2")
	require.Greater(t, len(chatMsgs2.Messages), len(chatMsgs.Messages),
		"follow-up should have added more messages")

	// The last assistant message should have text.
	lastAssistant := findLastAssistantWithText(t, chatMsgs2.Messages)
	require.NotNil(t, lastAssistant,
		"expected an assistant message with text in the follow-up")

	t.Log("Anthropic web_search round-trip test passed.")
}

// waitForChatDone drains the event stream until the chat reaches
// a terminal status (waiting, completed, or error).
func waitForChatDone(
	ctx context.Context,
	t *testing.T,
	events <-chan codersdk.ChatStreamEvent,
	label string,
) {
	t.Helper()
	for {
		select {
		case <-ctx.Done():
			require.FailNow(t, "timed out waiting for "+label+" completion")
		case event, ok := <-events:
			if !ok {
				return
			}
			switch event.Type {
			case codersdk.ChatStreamEventTypeError:
				if event.Error != nil {
					t.Logf("[%s] stream error: %s", label, event.Error.Message)
				}
			case codersdk.ChatStreamEventTypeStatus:
				if event.Status != nil {
					t.Logf("[%s] status → %s", label, event.Status.Status)
					switch event.Status.Status {
					case codersdk.ChatStatusWaiting,
						codersdk.ChatStatusCompleted:
						return
					case codersdk.ChatStatusError:
						require.FailNow(t, label+" ended with error status")
					}
				}
			case codersdk.ChatStreamEventTypeMessage:
				if event.Message != nil {
					t.Logf("[%s] persisted message: role=%s parts=%d",
						label, event.Message.Role, len(event.Message.Content))
				}
			case codersdk.ChatStreamEventTypeMessagePart:
				// Streaming delta — just note it.
				if event.MessagePart != nil {
					t.Logf("[%s] part: type=%s",
						label, event.MessagePart.Part.Type)
				}
			}
		}
	}
}

// findAssistantWithText returns the first assistant message that
// contains a non-empty text part.
func findAssistantWithText(t *testing.T, msgs []codersdk.ChatMessage) *codersdk.ChatMessage {
	t.Helper()
	for i := range msgs {
		if msgs[i].Role != "assistant" {
			continue
		}
		for _, part := range msgs[i].Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text != "" {
				return &msgs[i]
			}
		}
	}
	return nil
}

// findLastAssistantWithText returns the last assistant message that
// contains a non-empty text part.
func findLastAssistantWithText(t *testing.T, msgs []codersdk.ChatMessage) *codersdk.ChatMessage {
	t.Helper()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "assistant" {
			continue
		}
		for _, part := range msgs[i].Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text != "" {
				return &msgs[i]
			}
		}
	}
	return nil
}

// logMessages prints a summary of all messages for debugging.
func logMessages(t *testing.T, msgs []codersdk.ChatMessage) {
	t.Helper()
	for i, msg := range msgs {
		types := make([]string, 0, len(msg.Content))
		for _, part := range msg.Content {
			s := string(part.Type)
			if part.ProviderExecuted {
				s += "(PE)"
			}
			types = append(types, s)
		}
		t.Logf("  msg[%d] role=%s parts=%v", i, msg.Role, types)
	}
}

// TestOpenAIReasoningRoundTrip is an integration test that verifies
// reasoning items from OpenAI's Responses API survive the full
// persist → reconstruct → re-send cycle when Store: true. It sends
// a query to a reasoning model, waits for completion, then sends a
// follow-up message. If reasoning items are sent back without their
// required following output item, the API rejects the second request:
//
//	Item 'rs_xxx' of type 'reasoning' was provided without its
//	required following item.
//
// The test requires OPENAI_API_KEY to be set.
func TestOpenAIReasoningRoundTrip(t *testing.T) {
	t.Parallel()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set; skipping OpenAI integration test")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Stand up a full coderd with the agents experiment.
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: deploymentValues,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	// Configure an OpenAI provider with the real API key.
	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai",
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})
	require.NoError(t, err)

	// Create a model config for a reasoning model with Store: true
	// (the default). Using o4-mini because it always produces
	// reasoning items.
	contextLimit := int64(200000)
	isDefault := true
	reasoningSummary := "auto"
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "o4-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
		ModelConfig: &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
					Store:            ptr.Ref(true),
					ReasoningSummary: &reasoningSummary,
				},
			},
		},
	})
	require.NoError(t, err)

	// --- Step 1: Send a message that triggers reasoning ---
	t.Log("Creating chat with reasoning query...")
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "What is 2+2? Be brief.",
			},
		},
	})
	require.NoError(t, err)
	t.Logf("Chat created: %s (status=%s)", chat.ID, chat.Status)

	// Stream events until the chat reaches a terminal status.
	events, closer, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer.Close()

	waitForChatDone(ctx, t, events, "step 1")

	// Verify the chat completed and messages were persisted.
	chatData, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	chatMsgs, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	t.Logf("Chat status after step 1: %s, messages: %d",
		chatData.Status, len(chatMsgs.Messages))
	logMessages(t, chatMsgs.Messages)

	require.Equal(t, codersdk.ChatStatusWaiting, chatData.Status,
		"chat should be in waiting status after step 1")

	// Verify the assistant message has reasoning content.
	assistantMsg := findAssistantWithText(t, chatMsgs.Messages)
	require.NotNil(t, assistantMsg,
		"expected an assistant message with text content after step 1")

	partTypes := partTypeSet(assistantMsg.Content)
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeReasoning,
		"assistant message should contain reasoning parts from o4-mini")
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeText,
		"assistant message should contain a text part")

	// --- Step 2: Send a follow-up message ---
	// This is the critical test: if reasoning items are sent back
	// without their required following item, the API will reject
	// the request with:
	//   Item 'rs_xxx' of type 'reasoning' was provided without its
	//   required following item.
	t.Log("Sending follow-up message...")
	_, err = expClient.CreateChatMessage(ctx, chat.ID,
		codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "And what is 3+3? Be brief.",
				},
			},
		})
	require.NoError(t, err)

	// Stream the follow-up response.
	events2, closer2, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer2.Close()

	waitForChatDone(ctx, t, events2, "step 2")

	// Verify the follow-up completed and produced content.
	chatData2, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	chatMsgs2, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	t.Logf("Chat status after step 2: %s, messages: %d",
		chatData2.Status, len(chatMsgs2.Messages))
	logMessages(t, chatMsgs2.Messages)

	require.Equal(t, codersdk.ChatStatusWaiting, chatData2.Status,
		"chat should be in waiting status after step 2")
	require.Greater(t, len(chatMsgs2.Messages), len(chatMsgs.Messages),
		"follow-up should have added more messages")

	// The last assistant message should have text.
	lastAssistant := findLastAssistantWithText(t, chatMsgs2.Messages)
	require.NotNil(t, lastAssistant,
		"expected an assistant message with text in the follow-up")

	t.Log("OpenAI reasoning round-trip test passed.")
}

// TestOpenAIReasoningRoundTripStoreFalse is an integration test that verifies
// follow-up messages succeed when reasoning items were created with
// store: false, where OpenAI response item IDs are ephemeral and are not
// persisted on OpenAI's servers. It sends a query to a reasoning model,
// waits for completion, then sends a follow-up message to ensure chatd can
// reconstruct the conversation without relying on persisted provider item IDs.
//
// The test guards against the prior failure mode where the follow-up request
// was rejected with an error like:
//
//	Item with id 'msg_xxx' not found. Items are not persisted when
//	store is set to false.
//
// The test requires OPENAI_API_KEY to be set.
func TestOpenAIReasoningRoundTripStoreFalse(t *testing.T) {
	t.Parallel()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set; skipping OpenAI integration test")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Stand up a full coderd with the agents experiment.
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: deploymentValues,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	// Configure an OpenAI provider with the real API key.
	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai",
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})
	require.NoError(t, err)

	// Create a model config for a reasoning model with Store: false.
	// Using o4-mini because it always produces reasoning items.
	contextLimit := int64(200000)
	isDefault := true
	reasoningSummary := "auto"
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "o4-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
		ModelConfig: &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
					Store:            ptr.Ref(false),
					ReasoningSummary: &reasoningSummary,
				},
			},
		},
	})
	require.NoError(t, err)

	// --- Step 1: Send a message that triggers reasoning ---
	t.Log("Creating chat with reasoning query...")
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "What is 2+2? Be brief.",
			},
		},
	})
	require.NoError(t, err)
	t.Logf("Chat created: %s (status=%s)", chat.ID, chat.Status)

	// Stream events until the chat reaches a terminal status.
	events, closer, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer.Close()

	waitForChatDone(ctx, t, events, "step 1")

	// Verify the chat completed and messages were persisted.
	chatData, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	chatMsgs, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	t.Logf("Chat status after step 1: %s, messages: %d",
		chatData.Status, len(chatMsgs.Messages))
	logMessages(t, chatMsgs.Messages)

	require.Equal(t, codersdk.ChatStatusWaiting, chatData.Status,
		"chat should be in waiting status after step 1")

	// Verify the assistant message has reasoning content.
	assistantMsg := findAssistantWithText(t, chatMsgs.Messages)
	require.NotNil(t, assistantMsg,
		"expected an assistant message with text content after step 1")

	partTypes := partTypeSet(assistantMsg.Content)
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeReasoning,
		"assistant message should contain reasoning parts from o4-mini")
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeText,
		"assistant message should contain a text part")

	// --- Step 2: Send a follow-up message ---
	// This is the critical test: when Store is false, item IDs are
	// ephemeral and cannot be looked up from OpenAI later.
	t.Log("Sending follow-up message...")
	_, err = expClient.CreateChatMessage(ctx, chat.ID,
		codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "And what is 3+3? Be brief.",
				},
			},
		})
	if err != nil {
		require.NotContains(t, err.Error(),
			"Items are not persisted when store is set to false.",
			"follow-up should reconstruct ephemeral reasoning items instead of sending stale provider item IDs")
	}
	require.NoError(t, err)

	// Stream the follow-up response.
	events2, closer2, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer2.Close()

	waitForChatDone(ctx, t, events2, "step 2")

	// Verify the follow-up completed and produced content.
	chatData2, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	chatMsgs2, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	t.Logf("Chat status after step 2: %s, messages: %d",
		chatData2.Status, len(chatMsgs2.Messages))
	logMessages(t, chatMsgs2.Messages)

	require.Equal(t, codersdk.ChatStatusWaiting, chatData2.Status,
		"chat should be in waiting status after step 2")
	require.Greater(t, len(chatMsgs2.Messages), len(chatMsgs.Messages),
		"follow-up should have added more messages")

	// The last assistant message should have text.
	lastAssistant := findLastAssistantWithText(t, chatMsgs2.Messages)
	require.NotNil(t, lastAssistant,
		"expected an assistant message with text in the follow-up")

	t.Log("OpenAI reasoning round-trip store=false test passed.")
}

// partTypeSet returns the set of part types present in a message.
func partTypeSet(parts []codersdk.ChatMessagePart) map[codersdk.ChatMessagePartType]struct{} {
	set := make(map[codersdk.ChatMessagePartType]struct{}, len(parts))
	for _, p := range parts {
		set[p.Type] = struct{}{}
	}
	return set
}

type openAIStoreMode string

const (
	openAIStoreModeTrue  openAIStoreMode = "store_true"
	openAIStoreModeFalse openAIStoreMode = "store_false"
)

func TestOpenAIReasoningWithWebSearchRoundTrip(t *testing.T) {
	t.Parallel()
	runOpenAIReasoningWithWebSearchRoundTripTest(t, openAIStoreModeTrue)
}

func TestOpenAIReasoningWithWebSearchRoundTripStoreFalse(t *testing.T) {
	t.Parallel()
	runOpenAIReasoningWithWebSearchRoundTripTest(t, openAIStoreModeFalse)
}

func runOpenAIReasoningWithWebSearchRoundTripTest(t *testing.T, storeMode openAIStoreMode) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	store := storeMode == openAIStoreModeTrue

	type capturedOpenAIRequest struct {
		Stream             bool          `json:"stream,omitempty"`
		Store              *bool         `json:"store,omitempty"`
		PreviousResponseID *string       `json:"previous_response_id,omitempty"`
		Prompt             []interface{} `json:"input,omitempty"`
	}

	var (
		streamRequestCount atomic.Int32
		firstReq           *capturedOpenAIRequest
		secondReq          *capturedOpenAIRequest
		mu                 sync.Mutex
	)
	upstreamOpenAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("reasoning + web search title")
		}

		switch req.Header.Get("X-Request-Ordinal") {
		case "1":
			return chattest.OpenAIResponse{
				ResponseID: "resp_first_test",
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
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("Follow-up answer.")...,
			)
		}
	})
	captureServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read OpenAI request body: %v", err)
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = r.Body.Close()

		if r.URL.Path == "/responses" {
			var captured capturedOpenAIRequest
			if err := json.Unmarshal(body, &captured); err != nil {
				t.Errorf("decode OpenAI request body: %v", err)
				http.Error(rw, err.Error(), http.StatusBadRequest)
				return
			}
			if captured.Stream {
				requestCount := streamRequestCount.Add(1)
				r.Header.Set("X-Request-Ordinal", strconv.Itoa(int(requestCount)))

				mu.Lock()
				switch requestCount {
				case 1:
					firstReq = &captured
				default:
					secondReq = &captured
				}
				mu.Unlock()
			}
		}

		upstreamReq, err := http.NewRequestWithContext(
			r.Context(),
			r.Method,
			upstreamOpenAIURL+r.URL.RequestURI(),
			bytes.NewReader(body),
		)
		if err != nil {
			t.Errorf("create upstream OpenAI request: %v", err)
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		upstreamReq.Header = r.Header.Clone()

		resp, err := http.DefaultClient.Do(upstreamReq)
		if err != nil {
			t.Errorf("forward OpenAI request: %v", err)
			http.Error(rw, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for key, values := range resp.Header {
			for _, value := range values {
				rw.Header().Add(key, value)
			}
		}
		rw.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(rw, resp.Body); err != nil {
			t.Errorf("copy OpenAI response body: %v", err)
		}
	}))
	t.Cleanup(captureServer.Close)
	openAIURL := captureServer.URL

	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: deploymentValues,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(200000)
	isDefault := true
	reasoningEffort := "medium"
	reasoningSummary := "auto"
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "o4-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
		ModelConfig: &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
					Store:            ptr.Ref(store),
					ReasoningEffort:  &reasoningEffort,
					ReasoningSummary: &reasoningSummary,
					WebSearchEnabled: ptr.Ref(true),
				},
			},
		},
	})
	require.NoError(t, err)

	t.Logf("Creating chat with reasoning + web search query (store=%t)...", store)
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "Search for the latest AI news and summarize it briefly.",
		}},
	})
	require.NoError(t, err)

	events, closer, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer.Close()

	waitForChatDone(ctx, t, events, "step 1")

	chatData, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	chatMsgs, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusWaiting, chatData.Status,
		"chat should be in waiting status after step 1")

	assistantMsg := findAssistantWithText(t, chatMsgs.Messages)
	require.NotNil(t, assistantMsg,
		"expected an assistant message with text content after step 1")

	partTypes := partTypeSet(assistantMsg.Content)
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeReasoning,
		"assistant message should contain reasoning parts")
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeToolCall,
		"assistant message should contain a provider-executed web search tool call")
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeToolResult,
		"assistant message should contain a provider-executed web search tool result")
	require.Contains(t, partTypes, codersdk.ChatMessagePartTypeText,
		"assistant message should contain a text part")

	var foundReasoning, foundWebSearchCall, foundText bool
	for _, part := range assistantMsg.Content {
		switch part.Type {
		case codersdk.ChatMessagePartTypeReasoning:
			// fantasy emits a leading newline when the reasoning summary part is
			// added, so match the persisted summary text after trimming whitespace.
			if strings.TrimSpace(part.Text) == "thinking about the question" {
				foundReasoning = true
			}
		case codersdk.ChatMessagePartTypeToolCall:
			if part.ToolName == "web_search" {
				require.True(t, part.ProviderExecuted,
					"web search tool-call should be marked provider-executed")
				foundWebSearchCall = true
			}
		case codersdk.ChatMessagePartTypeText:
			if part.Text == "Here is what I found." {
				foundText = true
			}
		}
	}
	require.True(t, foundReasoning, "expected reasoning summary text to be persisted")
	require.True(t, foundWebSearchCall, "expected persisted web_search tool call")
	require.True(t, foundText, "expected streamed assistant text to be persisted")

	t.Log("Sending follow-up message...")
	_, err = expClient.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "What is the follow-up takeaway?",
		}},
	})
	if !store && err != nil {
		require.NotContains(t, err.Error(),
			"Items are not persisted when store is set to false.",
			"follow-up should reconstruct store=false responses without stale provider item IDs")
	}
	require.NoError(t, err)

	events2, closer2, err := expClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer2.Close()

	waitForChatDone(ctx, t, events2, "step 2")

	chatData2, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	chatMsgs2, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusWaiting, chatData2.Status,
		"chat should be in waiting status after step 2")
	require.Greater(t, len(chatMsgs2.Messages), len(chatMsgs.Messages),
		"follow-up should have added more messages")
	require.NotNil(t, findLastAssistantWithText(t, chatMsgs2.Messages),
		"expected an assistant message with text after the follow-up")
	require.Equal(t, int32(2), streamRequestCount.Load(),
		"expected exactly two streamed OpenAI responses")

	mu.Lock()
	defer mu.Unlock()

	require.NotNil(t, firstReq, "expected first streaming request to be captured")
	if store {
		require.NotNil(t, firstReq.Store, "first request should have store field")
		require.True(t, *firstReq.Store, "store should be true")
	} else if firstReq.Store != nil {
		require.False(t, *firstReq.Store, "store should be false")
	}

	require.NotNil(t, secondReq, "expected second streaming request to be captured")
	foundAssistantReplay := false
	for _, item := range secondReq.Prompt {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		if role == "assistant" {
			foundAssistantReplay = true
		}
		if store {
			require.NotEqual(t, "assistant", role,
				"store=true chain-mode prompt should not replay assistant messages")
			require.NotEqual(t, "tool", role,
				"store=true chain-mode prompt should not replay tool messages")
		}
	}

	if store {
		require.NotNil(t, secondReq.PreviousResponseID,
			"store=true follow-up should set previous_response_id")
		require.Equal(t, "resp_first_test", *secondReq.PreviousResponseID,
			"previous_response_id should match the first response's ID")
	} else {
		if secondReq.PreviousResponseID != nil {
			require.Empty(t, *secondReq.PreviousResponseID,
				"store=false follow-up should not set previous_response_id")
		}
		require.True(t, foundAssistantReplay,
			"store=false follow-up should replay prior assistant history")
	}
}
