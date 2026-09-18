package chatd_test

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

const agentClearSentinelPrefix = "You cleared your own context by calling clear_context."

// toolResultParts returns every tool-result part for toolName in order.
func toolResultParts(parts []codersdk.ChatMessagePart, toolName string) []codersdk.ChatMessagePart {
	var out []codersdk.ChatMessagePart
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == toolName {
			out = append(out, part)
		}
	}
	return out
}

// openAIMessageIndex returns the index of the first message whose
// content contains text, or -1.
func openAIMessageIndex(messages []chattest.OpenAIMessage, text string) int {
	for i, msg := range messages {
		if strings.Contains(msg.Content, text) {
			return i
		}
	}
	return -1
}

// clearContextChunk builds a streamed clear_context call with the given id.
func clearContextChunk(callID, followUp string) chattest.OpenAIChunk {
	args, _ := json.Marshal(map[string]string{"follow_up": followUp})
	chunk := chattest.OpenAIToolCallChunk("clear_context", string(args))
	chunk.Choices[0].ToolCalls[0].ID = callID
	return chunk
}

func TestActiveServer_ClearContextTool(t *testing.T) {
	t.Parallel()

	const followUp = "Resume from /home/coder/plans/PLAN.md step 4; child agent ids are in agents.txt."

	t.Run("clears and continues from the follow-up", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamCount atomic.Int32
		var secondMessages []chattest.OpenAIMessage
		var messagesMu sync.Mutex
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamCount.Add(1) {
			case 1:
				return chattest.OpenAIStreamingResponse(clearContextChunk("call_clear", followUp))
			default:
				messagesMu.Lock()
				secondMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
				messagesMu.Unlock()
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("continuing after clear")...)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		reg := prometheus.NewRegistry()
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.PrometheusRegistry = reg
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)
		require.False(t, chat.CompactionRequestedAt.Valid)
		require.Equal(t, int32(2), streamCount.Load(), "one tool step, one continuation")
		requireChatdMetricCounter(t, reg, "coderd_chatd_context_tool_calls_total", 1, map[string]string{"tool": "clear_context", "outcome": "success"})

		parts := chatToolParts(ctx, t, db, chat.ID)
		clearResult := requireToolResultPart(t, parts, "clear_context")
		require.False(t, clearResult.IsError)
		require.Contains(t, string(clearResult.Result), "Context cleared. Follow-up: "+followUp)

		messages := chatMessages(ctx, t, db, chat.ID)
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		compressed := compressedChatClearedMessages(t, append(promptMessages, messages...))
		require.Len(t, compressed.summaries, 1)
		require.Contains(t, messageText(t, compressed.summaries[0]), agentClearSentinelPrefix)
		require.Len(t, compressed.results, 1)
		var result map[string]any
		require.NoError(t, json.Unmarshal(singlePartOfType(t, compressed.results[0], codersdk.ChatMessagePartTypeToolResult).Result, &result))
		require.Equal(t, "agent", result["source"])

		messagesMu.Lock()
		modelMessages := append([]chattest.OpenAIMessage(nil), secondMessages...)
		messagesMu.Unlock()
		sentinelIndex := openAIMessageIndex(modelMessages, agentClearSentinelPrefix)
		followUpIndex := openAIMessageIndex(modelMessages, followUp)
		require.NotEqual(t, -1, sentinelIndex, "sentinel reaches the model")
		require.NotEqual(t, -1, followUpIndex, "follow-up reaches the model")
		require.Less(t, sentinelIndex, followUpIndex)
		require.Equal(t, "user", modelMessages[followUpIndex].Role)
		require.Equal(t, len(modelMessages)-1, followUpIndex, "follow-up is the last message")
		require.False(t, openAIMessagesContain(modelMessages, "hello from the user"), "pre-clear prompt must not reach the model")
		require.False(t, openAIMessagesContain(modelMessages, "Context cleared. Follow-up"), "the tool result is behind the boundary")
	})

	t.Run("blank follow-up is rejected without a boundary", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamCount.Add(1) == 1 {
				return chattest.OpenAIStreamingResponse(clearContextChunk("call_blank", "  "))
			}
			require.True(t, openAIMessagesContain(req.Messages, "hello from the user"), "history is intact after a rejected call")
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		reg := prometheus.NewRegistry()
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.PrometheusRegistry = reg
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)

		result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "clear_context")
		require.True(t, result.IsError)
		require.Contains(t, string(result.Result), "follow_up is required")
		requireChatdMetricCounter(t, reg, "coderd_chatd_context_tool_calls_total", 1, map[string]string{"tool": "clear_context", "outcome": "rejected"})
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, compressedChatClearedMessages(t, promptMessages).summaries)
	})

	t.Run("mixed batch is rejected as a whole", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamCount.Add(1) == 1 {
				listChunk := chattest.OpenAIToolCallChunk("list_templates", `{}`)
				clearChunk := clearContextChunk("call_clear_batch", followUp)
				clearCall := clearChunk.Choices[0].ToolCalls[0]
				clearCall.Index = 1
				listChunk.Choices[0].ToolCalls = append(listChunk.Choices[0].ToolCalls, clearCall)
				return chattest.OpenAIStreamingResponse(listChunk)
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		reg := prometheus.NewRegistry()
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.PrometheusRegistry = reg
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)

		parts := chatToolParts(ctx, t, db, chat.ID)
		clearResult := requireToolResultPart(t, parts, "clear_context")
		listResult := requireToolResultPart(t, parts, "list_templates")
		require.True(t, clearResult.IsError)
		require.True(t, listResult.IsError)
		require.Contains(t, string(clearResult.Result), "clear_context must be called alone")
		require.Contains(t, string(listResult.Result), "Retry your tool calls without clear_context, then call clear_context alone in a later step.")
		require.NotContains(t, string(listResult.Result), "separately first")
		requireChatdMetricCounter(t, reg, "coderd_chatd_context_tool_calls_total", 1, map[string]string{"tool": "clear_context", "outcome": "denied"})
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, compressedChatClearedMessages(t, promptMessages).summaries)
	})

	t.Run("immediate second clear is rejected", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamCount.Add(1) {
			case 1:
				return chattest.OpenAIStreamingResponse(clearContextChunk("call_clear_1", followUp))
			case 2:
				return chattest.OpenAIStreamingResponse(clearContextChunk("call_clear_2", "again"))
			default:
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)
		require.Equal(t, int32(3), streamCount.Load())

		results := toolResultParts(chatToolParts(ctx, t, db, chat.ID), "clear_context")
		require.Len(t, results, 2)
		require.False(t, results[0].IsError)
		require.True(t, results[1].IsError)
		require.Contains(t, string(results[1].Result), "nothing has happened since the last context boundary")
		messages := chatMessages(ctx, t, db, chat.ID)
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, compressedChatClearedMessages(t, append(promptMessages, messages...)).calls, 1, "exactly one boundary")
	})

	t.Run("pre_tool_use override rewrites the follow-up", func(t *testing.T) {
		t.Parallel()

		const overridden = "Overridden follow-up from the hook."
		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamCount atomic.Int32
		var secondMessages []chattest.OpenAIMessage
		var messagesMu sync.Mutex
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamCount.Add(1) == 1 {
				return chattest.OpenAIStreamingResponse(clearContextChunk("call_override", followUp))
			}
			messagesMu.Lock()
			secondMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
			messagesMu.Unlock()
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
			require.Equal(t, "clear_context", data.ToolName)
			override, err := json.Marshal(map[string]any{"permission": map[string]any{
				"decision":       "allow",
				"input_override": map[string]string{"follow_up": overridden},
			}})
			require.NoError(t, err)
			return string(override)
		})
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)

		result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "clear_context")
		require.False(t, result.IsError)
		require.Contains(t, string(result.Result), overridden)
		require.NotContains(t, string(result.Result), followUp)

		messagesMu.Lock()
		modelMessages := append([]chattest.OpenAIMessage(nil), secondMessages...)
		messagesMu.Unlock()
		require.True(t, openAIMessagesContain(modelMessages, overridden))
		require.False(t, openAIMessagesContain(modelMessages, followUp), "the original follow-up never reaches the model")
	})

	t.Run("pre_tool_use denial leaves the context intact", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamCount.Add(1) == 1 {
				return chattest.OpenAIStreamingResponse(clearContextChunk("call_denied", followUp))
			}
			require.True(t, openAIMessagesContain(req.Messages, "hello from the user"))
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
			return `{"permission":{"decision":"deny","reason":"context management is disabled here"}}`
		})
		reg := prometheus.NewRegistry()
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
			cfg.PrometheusRegistry = reg
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)

		result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "clear_context")
		require.True(t, result.IsError)
		require.Contains(t, string(result.Result), "blocked by an external policy")
		requireChatdMetricCounter(t, reg, "coderd_chatd_context_tool_calls_total", 1, map[string]string{"tool": "clear_context", "outcome": "denied"})
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, compressedChatClearedMessages(t, promptMessages).summaries)
	})
}
