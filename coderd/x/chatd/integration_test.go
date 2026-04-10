package chatd_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
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
// The test requires ANTHROPIC_TEST_API_KEY to be set.
func TestAnthropicWebSearchRoundTrip(t *testing.T) {
	t.Parallel()

	apiKey := os.Getenv("ANTHROPIC_TEST_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_TEST_API_KEY not set; skipping Anthropic integration test")
	}
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Stand up a full coderd with the agents experiment.
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: deploymentValues,
	})
	user := coderdtest.CreateFirstUser(t, client)
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
		OrganizationID: user.OrganizationID,
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
// The test requires OPENAI_TEST_API_KEY to be set.
func TestOpenAIReasoningRoundTrip(t *testing.T) {
	t.Parallel()

	apiKey := os.Getenv("OPENAI_TEST_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_TEST_API_KEY not set; skipping OpenAI integration test")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Stand up a full coderd with the agents experiment.
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: deploymentValues,
	})
	user := coderdtest.CreateFirstUser(t, client)
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
		OrganizationID: user.OrganizationID,
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
// The test requires OPENAI_TEST_API_KEY to be set.
func TestOpenAIReasoningRoundTripStoreFalse(t *testing.T) {
	t.Parallel()

	apiKey := os.Getenv("OPENAI_TEST_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_TEST_API_KEY not set; skipping OpenAI integration test")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Stand up a full coderd with the agents experiment.
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: deploymentValues,
	})
	user := coderdtest.CreateFirstUser(t, client)
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
		OrganizationID: user.OrganizationID,
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
