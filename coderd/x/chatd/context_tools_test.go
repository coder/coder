package chatd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

const (
	agentClearSentinelPrefix = "You cleared your own context by calling clear_context."
	agentCompactSummaryStart = "You compacted your own context by calling compact_context."
	compactionSummaryPrompt  = "You are performing a context compaction"
)

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

// anthropicContentBlocks decodes a request message's content, which
// the SDK sends either as a string or as a block array.
func anthropicContentBlocks(t *testing.T, msg chattest.AnthropicRequestMessage) []chattest.AnthropicContentBlock {
	t.Helper()
	var text string
	if err := json.Unmarshal(msg.Content, &text); err == nil {
		return []chattest.AnthropicContentBlock{{Type: "text", Text: text}}
	}
	var blocks []chattest.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(msg.Content, &blocks))
	return blocks
}

// anthropicHasStandaloneText reports whether any message carries a text
// block equal to text.
func anthropicHasStandaloneText(t *testing.T, messages []chattest.AnthropicRequestMessage, text string) bool {
	t.Helper()
	for _, msg := range messages {
		for _, block := range anthropicContentBlocks(t, msg) {
			if block.Type == "text" && block.Text == text {
				return true
			}
		}
	}
	return false
}

// compactContextTestServer wires an Anthropic fake whose first stream
// emits compact_context(followUp), whose summary call is answered by
// summarize, and whose later streams answer with plain text while
// recording their requests. Usage stays far below the threshold so
// only the tool can trigger a compaction.
type compactContextTestServer struct {
	streamCount     *atomic.Int32
	compactionCalls *atomic.Int32
	summaryRequests *[]chattest.AnthropicRequest
	streamRequests  *[]chattest.AnthropicRequest
	mu              *sync.Mutex
	url             string
}

func newCompactContextTestServer(
	t *testing.T,
	followUp string,
	summarize func(req *chattest.AnthropicRequest) chattest.AnthropicResponse,
	laterStreams func(n int32, req *chattest.AnthropicRequest) chattest.AnthropicResponse,
) compactContextTestServer {
	t.Helper()
	s := compactContextTestServer{
		streamCount:     &atomic.Int32{},
		compactionCalls: &atomic.Int32{},
		summaryRequests: &[]chattest.AnthropicRequest{},
		streamRequests:  &[]chattest.AnthropicRequest{},
		mu:              &sync.Mutex{},
	}
	args, err := json.Marshal(map[string]string{"follow_up": followUp})
	require.NoError(t, err)
	s.url = chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		body := anthropicRequestBody(t, *req)
		if !req.Stream {
			if strings.Contains(body, compactionSummaryPrompt) {
				s.compactionCalls.Add(1)
				s.mu.Lock()
				*s.summaryRequests = append(*s.summaryRequests, *req)
				s.mu.Unlock()
				return summarize(req)
			}
			return chattest.AnthropicNonStreamingResponse("title")
		}
		n := s.streamCount.Add(1)
		s.mu.Lock()
		*s.streamRequests = append(*s.streamRequests, *req)
		s.mu.Unlock()
		if n == 1 {
			return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks("compact_context", string(args))...)
		}
		if laterStreams != nil {
			if response := laterStreams(n, req); response.StreamingChunks != nil || response.Response != nil || response.Error != nil {
				return response
			}
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
			InputTokens:  10,
			OutputTokens: 5,
		}, "continuing after compaction")...)
	})
	return s
}

func (s compactContextTestServer) streamRequest(t *testing.T, n int) chattest.AnthropicRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Greater(t, len(*s.streamRequests), n-1)
	return (*s.streamRequests)[n-1]
}

func (s compactContextTestServer) summaryRequest(t *testing.T) chattest.AnthropicRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Len(t, *s.summaryRequests, 1)
	return (*s.summaryRequests)[0]
}

func TestActiveServer_CompactContextTool(t *testing.T) {
	t.Parallel()

	const (
		followUp          = "Resume from /home/coder/plans/PLAN.md step 5; the diff is committed on branch feature/x."
		compactionSummary = "agent compaction summary"
	)

	t.Run("compacts and continues from the summary and follow-up", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		fake := newCompactContextTestServer(t, followUp, func(*chattest.AnthropicRequest) chattest.AnthropicResponse {
			return anthropicCompactionResponse(compactionSummary)
		}, nil)
		user, org, model := seedAnthropicChatDependencies(t, db, fake.url)
		model = updateChatModelCompressionThreshold(t, db, model, 100, 70)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, fake.url, chattest.WithPreservePath()))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)
		require.Equal(t, int32(1), fake.compactionCalls.Load(), "exactly one summary call, so the marker was not re-forced")
		require.False(t, chat.CompactionRequestedAt.Valid)
		require.Equal(t, int32(2), fake.streamCount.Load(), "tool step, then one continuation")

		parts := chatToolParts(ctx, t, db, chat.ID)
		compactResult := requireToolResultPart(t, parts, "compact_context")
		require.False(t, compactResult.IsError)
		require.Contains(t, string(compactResult.Result), "Compaction scheduled. Follow-up: "+followUp)

		messages := chatMessages(ctx, t, db, chat.ID)
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		compressed := compressedChatSummarizedMessages(t, append(promptMessages, messages...))
		require.Len(t, compressed.results, 1)
		var result map[string]any
		require.NoError(t, json.Unmarshal(singlePartOfType(t, compressed.results[0], codersdk.ChatMessagePartTypeToolResult).Result, &result))
		require.Equal(t, "agent", result["source"])
		require.Equal(t, compactionSummary, result["summary"])
		require.Len(t, compressed.summaries, 1)
		require.True(t, strings.HasPrefix(messageText(t, compressed.summaries[0]), agentCompactSummaryStart))

		// The summarizer sees the call and its result last, never the
		// follow-up as a standalone message, plus the fixed agent hint.
		summary := fake.summaryRequest(t)
		summaryBody := anthropicRequestBody(t, summary)
		require.Contains(t, summaryBody, chatloop.AgentCompactionSummaryHint)
		require.Contains(t, summaryBody, "hello from the user")
		require.False(t, anthropicHasStandaloneText(t, summary.Messages, followUp), "follow-up is excluded from the summarizer input")
		// The summary prompt is the final user message; the message before
		// it carries the compact_context tool result.
		require.GreaterOrEqual(t, len(summary.Messages), 2)
		finalBlocks := anthropicContentBlocks(t, summary.Messages[len(summary.Messages)-1])
		var promptSeen, resultSeen bool
		for _, block := range finalBlocks {
			if block.Type == "text" && strings.Contains(block.Text, compactionSummaryPrompt) {
				promptSeen = true
			}
			if block.Type == "tool_result" {
				resultSeen = true
			}
		}
		require.True(t, promptSeen, "summary prompt is the final message")
		if !resultSeen {
			for _, block := range anthropicContentBlocks(t, summary.Messages[len(summary.Messages)-2]) {
				if block.Type == "tool_result" {
					resultSeen = true
				}
			}
		}
		require.True(t, resultSeen, "compact_context result immediately precedes the summary prompt")

		// The continuation sees the prefixed summary and the follow-up
		// as the final user text, and none of the pre-compaction prompt.
		second := fake.streamRequest(t, 2)
		secondBody := anthropicRequestBody(t, second)
		require.Contains(t, secondBody, agentCompactSummaryStart)
		require.Contains(t, secondBody, compactionSummary)
		require.NotContains(t, secondBody, "hello from the user")
		require.NotContains(t, secondBody, "Compaction scheduled. Follow-up")
		lastMessage := second.Messages[len(second.Messages)-1]
		require.Equal(t, "user", lastMessage.Role)
		blocks := anthropicContentBlocks(t, lastMessage)
		require.Equal(t, followUp, blocks[len(blocks)-1].Text, "follow-up is the final user text")
	})

	failureCases := []struct {
		name      string
		summarize func(*chattest.AnthropicRequest) chattest.AnthropicResponse
	}{
		{
			name: "non-retryable summary error",
			summarize: func(*chattest.AnthropicRequest) chattest.AnthropicResponse {
				return chattest.AnthropicResponse{Error: &chattest.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Type:       "invalid_request_error",
					Message:    "summary rejected",
				}}
			},
		},
		{
			name: "empty summary",
			summarize: func(*chattest.AnthropicRequest) chattest.AnthropicResponse {
				return anthropicCompactionResponse("")
			},
		},
	}
	for _, tc := range failureCases {
		t.Run(tc.name+" continues the turn", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			fake := newCompactContextTestServer(t, followUp, tc.summarize, nil)
			user, org, model := seedAnthropicChatDependencies(t, db, fake.url)
			model = updateChatModelCompressionThreshold(t, db, model, 100, 70)
			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, fake.url, chattest.WithPreservePath()))
			})
			chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
			chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
			require.False(t, chat.LastError.Valid, "a failed agent compaction must not park the chat in error")
			require.Equal(t, int32(1), fake.compactionCalls.Load(), "one summary attempt, so the marker was not re-forced")
			require.False(t, chat.CompactionRequestedAt.Valid)
			require.Equal(t, int32(2), fake.streamCount.Load(), "the turn continues after the failure")

			messages := chatMessages(ctx, t, db, chat.ID)
			promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
			require.NoError(t, err)
			require.Empty(t, compressedChatSummarizedMessages(t, append(promptMessages, messages...)).summaries, "no boundary")
			var noticeFound bool
			for _, msg := range messages {
				if msg.Role == database.ChatMessageRoleSystem && msg.Visibility == database.ChatMessageVisibilityUser &&
					strings.HasPrefix(messageText(t, msg), "Assistant-requested compaction failed:") {
					noticeFound = true
				}
			}
			require.True(t, noticeFound, "user-visible failure notice is persisted")
			var noteFound bool
			for _, msg := range promptMessages {
				if msg.Visibility == database.ChatMessageVisibilityModel && strings.HasPrefix(messageText(t, msg), "compact_context failed (") {
					noteFound = true
				}
			}
			require.True(t, noteFound, "model-only failure note is persisted")

			secondBody := anthropicRequestBody(t, fake.streamRequest(t, 2))
			require.Contains(t, secondBody, "hello from the user", "context was not compacted")
			require.Contains(t, secondBody, followUp)
			require.Contains(t, secondBody, "Do not call compact_context again in this context segment")
		})
	}

	t.Run("second compact after a failure is rejected", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		args, err := json.Marshal(map[string]string{"follow_up": "try again"})
		require.NoError(t, err)
		fake := newCompactContextTestServer(t, followUp, func(*chattest.AnthropicRequest) chattest.AnthropicResponse {
			return anthropicCompactionResponse("")
		}, func(n int32, _ *chattest.AnthropicRequest) chattest.AnthropicResponse {
			if n == 2 {
				return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks("compact_context", string(args))...)
			}
			return chattest.AnthropicResponse{}
		})
		user, org, model := seedAnthropicChatDependencies(t, db, fake.url)
		model = updateChatModelCompressionThreshold(t, db, model, 100, 70)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, fake.url, chattest.WithPreservePath()))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)
		require.Equal(t, int32(1), fake.compactionCalls.Load(), "the rejected call never reaches the summarizer")
		require.Equal(t, int32(3), fake.streamCount.Load())

		results := toolResultParts(chatToolParts(ctx, t, db, chat.ID), "compact_context")
		require.Len(t, results, 2)
		require.False(t, results[0].IsError)
		require.True(t, results[1].IsError)
		require.Contains(t, string(results[1].Result), "did not produce a boundary")
	})

	preCompactCases := []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "pre_compact dispatch failure continues the turn",
			status: http.StatusInternalServerError,
		},
		{
			// The hook protocol has no permission decision for pre_compact;
			// a deny response is a malformed response and a dispatch failure.
			name:   "pre_compact deny response continues the turn",
			status: http.StatusOK,
			body:   `{"permission":{"decision":"deny","reason":"compaction is not allowed here"}}`,
		},
	}
	for _, tc := range preCompactCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			fake := newCompactContextTestServer(t, followUp, func(*chattest.AnthropicRequest) chattest.AnthropicResponse {
				t.Errorf("summary requested after pre_compact failure")
				return anthropicCompactionResponse("unexpected summary")
			}, nil)
			var preCompactCalls atomic.Int32
			consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request agenthooks.Request
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				body := `{}`
				if request.Type == agenthooks.EventPreCompact {
					preCompactCalls.Add(1)
					w.WriteHeader(tc.status)
					body = tc.body
				}
				_, err := w.Write([]byte(body))
				require.NoError(t, err)
			}))
			t.Cleanup(consumer.Close)
			user, org, model := seedAnthropicChatDependencies(t, db, fake.url)
			model = updateChatModelCompressionThreshold(t, db, model, 100, 70)
			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, fake.url, chattest.WithPreservePath()))
				cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
			})
			chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
			chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
			require.False(t, chat.LastError.Valid)
			require.False(t, chat.CompactionRequestedAt.Valid)
			require.Equal(t, int32(1), preCompactCalls.Load())
			require.Equal(t, int32(0), fake.compactionCalls.Load())
			require.Equal(t, int32(2), fake.streamCount.Load())

			messages := chatMessages(ctx, t, db, chat.ID)
			var noticeFound bool
			for _, msg := range messages {
				if msg.Role == database.ChatMessageRoleSystem && strings.Contains(messageText(t, msg), "Assistant-requested compaction failed:") {
					noticeFound = true
				}
			}
			require.True(t, noticeFound, "failure notice is persisted")
			require.False(t, hasCompactionRows(t, db, chat.ID))
			secondBody := anthropicRequestBody(t, fake.streamRequest(t, 2))
			require.Contains(t, secondBody, "compact_context failed (")
			require.Contains(t, secondBody, followUp)
		})
	}

	t.Run("pre_compact and post_compact hooks fire", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		fake := newCompactContextTestServer(t, followUp, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			require.Contains(t, anthropicRequestBody(t, *req), "preserve deployment constraints")
			return anthropicCompactionResponse(compactionSummary)
		}, nil)
		var postCompactCalls atomic.Int32
		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request agenthooks.Request
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			body := `{}`
			switch request.Type {
			case agenthooks.EventPreCompact:
				body = `{"model_context":"preserve deployment constraints","user_message":"compaction starting"}`
			case agenthooks.EventPostCompact:
				postCompactCalls.Add(1)
				body = `{"model_context":"post compact context","user_message":"compaction complete"}`
			}
			_, err := w.Write([]byte(body))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)
		user, org, model := seedAnthropicChatDependencies(t, db, fake.url)
		model = updateChatModelCompressionThreshold(t, db, model, 100, 70)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, fake.url, chattest.WithPreservePath()))
			cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)
		require.Equal(t, int32(1), fake.compactionCalls.Load())
		require.Equal(t, int32(1), postCompactCalls.Load())
		require.True(t, hasCompactionRows(t, db, chat.ID))

		messages := chatMessages(ctx, t, db, chat.ID)
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.True(t, hasMessageText(t, messages, "compaction starting", database.ChatMessageVisibilityUser))
		require.True(t, hasMessageText(t, messages, "compaction complete", database.ChatMessageVisibilityUser))
		require.True(t, hasMessageText(t, promptMessages, "post compact context", database.ChatMessageVisibilityModel))
		secondBody := anthropicRequestBody(t, fake.streamRequest(t, 2))
		require.Contains(t, secondBody, followUp)
		require.Contains(t, secondBody, "post compact context")
	})
}
