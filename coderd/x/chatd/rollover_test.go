package chatd_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const rolloverLeadInPrefix = "A context window rollover started a fresh context window."

func TestActiveServer_ContextRollover(t *testing.T) {
	t.Parallel()

	withoutTokenBudget := slices.DeleteFunc(slices.Clone(codersdk.ExperimentsKnown), func(e codersdk.Experiment) bool {
		return e == codersdk.ExperimentChatTokenBudget
	})

	// W = 100 with the default 32k output cap gives a usable limit of 75;
	// the 70% summary threshold sits below it.
	tests := []struct {
		name             string
		experiments      codersdk.Experiments
		strategy         codersdk.ChatCompactionStrategy
		firstInputTokens int
		wantSummary      bool
		wantRollover     bool
	}{
		{
			name:             "TokenBudgetRollsOverAtUsableLimit",
			experiments:      codersdk.ExperimentsKnown,
			strategy:         codersdk.ChatCompactionStrategyTokenBudget,
			firstInputTokens: 80,
			wantRollover:     true,
		},
		{
			name:             "TokenBudgetDoesNotSummarizeAtThreshold",
			experiments:      codersdk.ExperimentsKnown,
			strategy:         codersdk.ChatCompactionStrategyTokenBudget,
			firstInputTokens: 72,
		},
		{
			name:             "ExperimentOffSummarizes",
			experiments:      withoutTokenBudget,
			strategy:         codersdk.ChatCompactionStrategyTokenBudget,
			firstInputTokens: 80,
			wantSummary:      true,
		},
		{
			name:             "SummarizePreferenceSummarizes",
			experiments:      codersdk.ExperimentsKnown,
			strategy:         codersdk.ChatCompactionStrategySummarize,
			firstInputTokens: 80,
			wantSummary:      true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			requests := newAnthropicRequestRecorder()
			var streamCount, compactionRequests atomic.Int32
			anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
				if strings.Contains(anthropicRequestBody(t, *req), "You are performing a context compaction") {
					compactionRequests.Add(1)
					return anthropicCompactionResponse(t, req, "summary of the first turn")
				}
				if !req.Stream {
					return chattest.AnthropicNonStreamingResponse("title")
				}
				requests.record(req)
				if streamCount.Add(1) == 1 {
					return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
						InputTokens:  tc.firstInputTokens,
						OutputTokens: 5,
					}, "first answer")...)
				}
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
					InputTokens:  20,
					OutputTokens: 5,
				}, "second answer")...)
			})
			user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
			model = updateChatModelCompressionThreshold(t, db, model, 100, 70)
			require.NoError(t, db.UpsertUserChatCompactionStrategy(ctx, database.UpsertUserChatCompactionStrategyParams{
				UserID:             user.ID,
				CompactionStrategy: string(tc.strategy),
			}))
			reg := prometheus.NewRegistry()
			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
				cfg.Experiments = tc.experiments
				cfg.PrometheusRegistry = reg
			})

			chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "first request")
			waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
			_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
				ChatID:        chat.ID,
				CreatedBy:     user.ID,
				ModelConfigID: model.ID,
				Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("second request")},
			})
			require.NoError(t, err)
			chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
			require.False(t, chat.LastError.Valid)
			require.Equal(t, int32(2), streamCount.Load())

			first := filterAnthropicStreamingRequests(requests.all())[0]
			tokenBudget := !tc.wantSummary
			require.Equal(t, tokenBudget, slices.Contains(anthropicToolNames(first), "new_context"))
			require.Equal(t, tokenBudget, slices.Contains(anthropicToolNames(first), "save_checkpoint"))
			require.Equal(t, tokenBudget, strings.Contains(anthropicSystemText(t, first), "<context_window>"))

			messages := chatMessages(ctx, t, db, chat.ID)
			promptRows, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
			require.NoError(t, err)
			calls, results := rolloverBoundaryParts(t, messages)
			if tc.wantSummary {
				require.Equal(t, int32(1), compactionRequests.Load())
			} else {
				require.Equal(t, int32(0), compactionRequests.Load(), "token budget never summarizes automatically")
			}
			if !tc.wantRollover {
				require.Empty(t, calls)
				require.Empty(t, results)
				return
			}

			require.Len(t, calls, 1, "exactly one rollover: the new window must not roll over again")
			require.Len(t, results, 1)
			require.JSONEq(t, `{"reason":"limit","requested_by":"system","previous_window_id":"w:0","context_tokens":80,"budget_tokens":75}`, string(calls[0].Args))
			require.JSONEq(t, `{"reason":"limit","requested_by":"system","previous_window_id":"w:0","context_tokens":80,"budget_tokens":75}`, string(results[0].Result))

			var anchor database.ChatMessage
			for _, row := range promptRows {
				if row.Compressed {
					anchor = row
				}
			}
			require.Equal(t, database.ChatMessageRoleUser, anchor.Role)
			require.Equal(t, database.ChatMessageVisibilityModel, anchor.Visibility)
			require.Contains(t, messageText(t, anchor), rolloverLeadInPrefix)
			require.Contains(t, messageText(t, anchor), "No checkpoint was saved")

			second := anthropicRequestBody(t, filterAnthropicStreamingRequests(requests.all())[1])
			require.Contains(t, second, rolloverLeadInPrefix)
			require.Contains(t, second, "second request", "the pending user message is replayed after the anchor")
			require.NotContains(t, second, "first request")
			require.NotContains(t, second, "first answer")
			requireTextPart(t, messages[len(messages)-1], "second answer")
			requireChatdMetricCounter(t, reg, "coderd_chatd_context_rollover_total", 1, map[string]string{
				"provider": "anthropic",
				"model":    "claude-sonnet-4-20250514",
				"reason":   "limit",
			})
		})
	}
}

func TestActiveServer_ManualCompactionUnderTokenBudget(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var compactionRequests atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if strings.Contains(anthropicRequestBody(t, *req), "You are performing a context compaction") {
			compactionRequests.Add(1)
			return anthropicCompactionResponse(t, req, "manual summary")
		}
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
			InputTokens:  10,
			OutputTokens: 5,
		}, "assistant answer")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateChatModelCompressionThreshold(t, db, model, 100, 70)
	require.NoError(t, db.UpsertUserChatCompactionStrategy(ctx, database.UpsertUserChatCompactionStrategyParams{
		UserID:             user.ID,
		CompactionStrategy: string(codersdk.ChatCompactionStrategyTokenBudget),
	}))
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
	chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	_, err := server.CompactChat(ctx, chat)
	require.NoError(t, err)
	chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.False(t, chat.LastError.Valid)
	require.Equal(t, int32(1), compactionRequests.Load())

	messages := chatMessages(ctx, t, db, chat.ID)
	compressed := compressedChatSummarizedMessages(t, messages)
	require.Len(t, compressed.results, 1)
	resultPart := singlePartOfType(t, compressed.results[0], codersdk.ChatMessagePartTypeToolResult)
	var result map[string]any
	require.NoError(t, json.Unmarshal(resultPart.Result, &result))
	require.Equal(t, "manual", result["source"])
	require.Equal(t, "manual summary", result["summary"])
}

func TestActiveServer_NewContextRollover(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newAnthropicRequestRecorder()
	var streamCount atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		requests.record(req)
		if streamCount.Add(1) == 1 {
			chunks := []chattest.AnthropicChunk{anthropicMessageStartChunk("msg-checkpoint")}
			chunks = append(chunks, anthropicToolUseChunksWithoutMessageEnvelope(0, "toolu_save", "save_checkpoint", `{"checkpoint":"goal: ship it"}`)...)
			chunks = append(chunks, anthropicToolUseChunksWithoutMessageEnvelope(1, "toolu_new", "new_context", `{}`)...)
			chunks = append(chunks,
				chattest.AnthropicChunk{Type: "message_delta", StopReason: "tool_use", Usage: chattest.AnthropicUsage{InputTokens: 10, OutputTokens: 5}},
				chattest.AnthropicChunk{Type: "message_stop"},
			)
			return chattest.AnthropicStreamingResponse(chunks...)
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
			InputTokens:  20,
			OutputTokens: 5,
		}, "continued")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateChatModelCompressionThreshold(t, db, model, 200_000, 70)
	require.NoError(t, db.UpsertUserChatCompactionStrategy(ctx, database.UpsertUserChatCompactionStrategyParams{
		UserID:             user.ID,
		CompactionStrategy: string(codersdk.ChatCompactionStrategyTokenBudget),
	}))
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})

	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "first request")
	chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.False(t, chat.LastError.Valid)
	require.Equal(t, int32(2), streamCount.Load(), "the receipt rolls over once, then the fresh window answers")

	messages := chatMessages(ctx, t, db, chat.ID)
	calls, results := rolloverBoundaryParts(t, messages)
	require.Len(t, calls, 1)
	require.Len(t, results, 1)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(results[0].Result, &payload))
	require.Equal(t, "model", payload["reason"])
	require.Equal(t, "model", payload["requested_by"])
	require.Equal(t, "w:0", payload["previous_window_id"])
	require.Equal(t, "goal: ship it", payload["checkpoint"])
	requireTextPart(t, messages[len(messages)-1], "continued")

	promptRows, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var anchor database.ChatMessage
	for _, row := range promptRows {
		if row.Compressed {
			anchor = row
		}
	}
	require.Contains(t, messageText(t, anchor), "You requested this fresh window with new_context")
	require.Contains(t, messageText(t, anchor), "<checkpoint>\ngoal: ship it\n</checkpoint>")

	var userRequest database.ChatMessage
	for _, msg := range messages {
		if msg.Role == database.ChatMessageRoleUser && msg.Visibility == database.ChatMessageVisibilityBoth {
			userRequest = msg
		}
	}
	streams := filterAnthropicStreamingRequests(requests.all())
	require.Len(t, streams, 2)
	firstSystem := anthropicSystemText(t, streams[0])
	require.Contains(t, firstSystem, "<context-window-guidance>")
	require.Contains(t, firstSystem, "<context_window>\nCurrent context window id: w:0\n</context_window>")
	require.Contains(t, anthropicRequestBody(t, streams[0]), fmt.Sprintf(`first request\n[id: %d]`, userRequest.ID))
	require.Contains(t, anthropicSystemText(t, streams[1]),
		fmt.Sprintf("<context_window>\nCurrent context window id: w:%d\nPrevious context window id: w:0\n</context_window>", anchor.ID))
	second := anthropicRequestBody(t, streams[1])
	require.Contains(t, second, "goal: ship it")
	require.NotContains(t, second, "first request")
	require.Equal(t, anthropicToolNames(streams[0]), anthropicToolNames(streams[1]))
}

func anthropicToolNames(req chattest.AnthropicRequest) []string {
	names := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func anthropicSystemText(t *testing.T, req chattest.AnthropicRequest) string {
	t.Helper()
	var blocks []struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(req.System, &blocks))
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		texts = append(texts, block.Text)
	}
	return strings.Join(texts, "\n")
}

func rolloverBoundaryParts(t *testing.T, messages []database.ChatMessage) (calls, results []codersdk.ChatMessagePart) {
	t.Helper()
	for _, msg := range messages {
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, part := range parts {
			if part.ToolName != "chat_context_rollover" {
				continue
			}
			require.True(t, msg.Compressed, "rollover boundary rows are compressed")
			switch part.Type {
			case codersdk.ChatMessagePartTypeToolCall:
				require.Equal(t, database.ChatMessageVisibilityUser, msg.Visibility)
				calls = append(calls, part)
			case codersdk.ChatMessagePartTypeToolResult:
				require.Equal(t, database.ChatMessageVisibilityBoth, msg.Visibility)
				results = append(results, part)
			}
		}
	}
	return calls, results
}
