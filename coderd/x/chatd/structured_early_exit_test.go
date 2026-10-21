package chatd_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestStructuredEarlyExits drives the turn endings that chatloop and the
// step budget add on top of the finalizer loop, each with an open
// structured output request, and checks that the request ends in exactly
// one receipt with the right status.
func TestStructuredEarlyExits(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type":"object","properties":{"v":{"type":"string"}},"required":["v"]}`)
	valid := toolCallsChunk([]chattest.OpenAIToolCall{finalizerToolCall(chatstructured.FinalizerToolName, `{"output":{"v":"OK"}}`)})
	text := func() chattest.OpenAIResponse {
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	}

	// OpenAI chat completions reports a step with tool calls as a tool_calls
	// finish even when the output token limit cut it, so the cut finalizer
	// arguments fail screening as malformed JSON: one error result, one
	// rejection, and the repaired call succeeds.
	t.Run("OpenAILengthCutsFinalizerCall", func(t *testing.T) {
		t.Parallel()
		cut := toolCallsChunk([]chattest.OpenAIToolCall{finalizerToolCall(chatstructured.FinalizerToolName, `{"output":{"v":"tr`)})
		cut.Choices[0].FinishReason = "length"
		run := runStructuredChat(t, schema, nil, func(step int) chattest.OpenAIResponse {
			switch step {
			case 0:
				return chattest.OpenAIStreamingResponse(cut)
			case 1:
				return chattest.OpenAIStreamingResponse(valid)
			}
			return text()
		})
		require.Equal(t, database.ChatStatusWaiting, run.status)
		resultsPerCall := map[string]int{}
		for _, result := range run.results {
			resultsPerCall[result.ToolCallID]++
		}
		for id, n := range resultsPerCall {
			require.Equal(t, 1, n, "tool call %s has one result", id)
		}
		require.Equal(t, 1, run.state.Rejections)
		require.Len(t, run.receipts, 1)
		require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, run.receipts[0].Status)
		require.JSONEq(t, `{"v":"OK"}`, string(run.receipts[0].Value))
	})

	// A content filter finish without visible output fails the turn, which
	// closes the request as generation_failed.
	t.Run("ContentFilterFailsTurn", func(t *testing.T) {
		t.Parallel()
		registry := prometheus.NewRegistry()
		run := runStructuredChat(t, schema, func(cfg *chatd.Config, _ database.Store) {
			cfg.PrometheusRegistry = registry
		}, func(int) chattest.OpenAIResponse {
			chunk := chattest.OpenAIChunk{Object: "chat.completion.chunk", Choices: []chattest.OpenAIChunkChoice{{FinishReason: "content_filter"}}}
			return chattest.OpenAIStreamingResponse(chunk)
		})
		require.Equal(t, database.ChatStatusError, run.status)
		require.Len(t, run.requests, 1, "a content filter refusal is not retried")
		require.Len(t, run.receipts, 1)
		require.Equal(t, codersdk.ChatStructuredOutputErrorCodeGenerationFailed, run.receipts[0].Error.Code)
		require.Equal(t, map[string]float64{"error": 1}, turnOutcomes(t, registry))
	})

	// The step budget ends a turn that keeps answering with text before the
	// repair budget does, as not_produced. The decision that closes the
	// request carries no finish reason; the turn still counts once, as
	// completed.
	t.Run("MaxStepsEndsTurn", func(t *testing.T) {
		t.Parallel()
		registry := prometheus.NewRegistry()
		run := runStructuredChat(t, schema, func(cfg *chatd.Config, _ database.Store) {
			cfg.Limits.MaxStepsPerTurn = 2
			cfg.PrometheusRegistry = registry
		}, func(int) chattest.OpenAIResponse { return text() })
		require.Equal(t, database.ChatStatusWaiting, run.status)
		require.Len(t, run.requests, 2)
		require.Equal(t, 2, run.state.Rejections)
		require.Len(t, run.receipts, 1)
		require.Equal(t, codersdk.ChatStructuredOutputErrorCodeNotProduced, run.receipts[0].Error.Code)
		require.Equal(t, map[string]float64{"completed": 1}, turnOutcomes(t, registry))
	})

	// A candidate that is current when the step budget runs out succeeds.
	t.Run("MaxStepsKeepsCandidate", func(t *testing.T) {
		t.Parallel()
		registry := prometheus.NewRegistry()
		run := runStructuredChat(t, schema, func(cfg *chatd.Config, _ database.Store) {
			cfg.Limits.MaxStepsPerTurn = 1
			cfg.PrometheusRegistry = registry
		}, func(step int) chattest.OpenAIResponse {
			if step == 0 {
				return chattest.OpenAIStreamingResponse(valid)
			}
			return text()
		})
		require.Equal(t, database.ChatStatusWaiting, run.status)
		require.Len(t, run.requests, 1)
		require.Len(t, run.receipts, 1)
		require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, run.receipts[0].Status)
		require.Equal(t, map[string]float64{"completed": 1}, turnOutcomes(t, registry))
	})
}

// TestStructuredAnthropicOutputLimit cuts a finalizer call with the output
// token limit on a provider that reports it (Anthropic max_tokens). chatloop
// answers every cut call with an error result and the finalizer screen
// rejects the incomplete call; the call must still have exactly one result.
func TestStructuredAnthropicOutputLimit(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type":"object","properties":{"v":{"type":"string"}},"required":["v"]}`)
	run := runStructuredAnthropicChat(t, schema, false, 0, nil, func(step int) chattest.AnthropicResponse {
		switch step {
		case 0:
			chunks := chattest.AnthropicToolCallChunks(chatstructured.FinalizerToolName, `{"output":{"v":"tr`)
			for i := range chunks {
				if chunks[i].Type == "message_delta" {
					chunks[i].StopReason = "max_tokens"
				}
			}
			return chattest.AnthropicStreamingResponse(chunks...)
		case 1:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks(chatstructured.FinalizerToolName, `{"output":{"v":"OK"}}`)...)
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
	})
	require.Equal(t, database.ChatStatusWaiting, run.status)
	resultsPerCall := map[string]int{}
	for _, result := range run.results {
		resultsPerCall[result.ToolCallID]++
	}
	require.Len(t, resultsPerCall, 2, "the cut call and the repaired call")
	for id, n := range resultsPerCall {
		require.Equal(t, 1, n, "tool call %s has exactly one result", id)
	}
	require.Equal(t, 1, run.state.Rejections)
	require.Len(t, run.receipts, 1)
	require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, run.receipts[0].Status)
	require.JSONEq(t, `{"v":"OK"}`, string(run.receipts[0].Value))
}

// runStructuredAnthropicChat is runStructuredChatOn on the Anthropic fake.
// respond answers streaming generation call number step. A non-zero
// compactionThreshold sets a 100 token context limit with that compaction
// threshold, and every compaction call is answered with a summary and
// counted in compactions.
func runStructuredAnthropicChat(t *testing.T, schema json.RawMessage, afterOrdinaryTurn bool, compactionThreshold int32, compactions *atomic.Int32, respond func(step int) chattest.AnthropicResponse) finalizerRun {
	t.Helper()
	var streamed atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		if strings.Contains(anthropicRequestBody(t, *req), "You are performing a context compaction") {
			require.NotNil(t, compactions, "unexpected compaction")
			compactions.Add(1)
			return anthropicCompactionResponse(t, req, "structured compaction summary")
		}
		return respond(int(streamed.Add(1)) - 1)
	})
	return runStructuredChatOn(t, schema, afterOrdinaryTurn, nil, func(db database.Store) (database.User, database.Organization, database.ChatModelConfig, aibridge.TransportFactory) {
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		if compactionThreshold != 0 {
			model = updateChatModelCompressionThreshold(t, db, model, 100, compactionThreshold)
		}
		return user, org, model, chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath())
	})
}

// withInputTokens sets the prompt usage that an Anthropic stream reports.
func withInputTokens(chunks []chattest.AnthropicChunk, tokens int) chattest.AnthropicResponse {
	for i := range chunks {
		if chunks[i].Type == "message_start" {
			chunks[i].Message.Usage = map[string]int{"input_tokens": tokens}
		}
	}
	return chattest.AnthropicStreamingResponse(chunks...)
}

// TestStructuredCompaction runs automatic compaction inside a turn with an
// open structured output request. Compaction compresses the request row
// and copies the pending user rows as model-only rows; the request must
// survive it and end in exactly one receipt.
func TestStructuredCompaction(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type":"object","properties":{"v":{"type":"string"}},"required":["v"]}`)
	text := func(tokens int) chattest.AnthropicResponse {
		return withInputTokens(chattest.AnthropicTextChunks("draft"), tokens)
	}
	finalize := func(tokens int) chattest.AnthropicResponse {
		return withInputTokens(chattest.AnthropicToolCallChunks(chatstructured.FinalizerToolName, `{"output":{"v":"OK"}}`), tokens)
	}

	// A rejected text step over the threshold compacts; the step after
	// compaction finalizes and the request succeeds.
	t.Run("RequestSurvivesCompaction", func(t *testing.T) {
		t.Parallel()
		var compactions atomic.Int32
		run := runStructuredAnthropicChat(t, schema, false, 70, &compactions, func(step int) chattest.AnthropicResponse {
			switch step {
			case 0:
				return text(80)
			case 1:
				return finalize(10)
			}
			return text(10)
		})
		require.Equal(t, database.ChatStatusWaiting, run.status)
		require.Equal(t, int32(1), compactions.Load())
		require.Equal(t, 1, run.state.Rejections)
		require.Len(t, run.receipts, 1)
		require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, run.receipts[0].Status)
		require.JSONEq(t, `{"v":"OK"}`, string(run.receipts[0].Value))
	})

	// A step after compaction still over the threshold ends the turn with
	// the still-over-limit error, which closes the request as
	// generation_failed.
	t.Run("StillOverLimitFailsRequest", func(t *testing.T) {
		t.Parallel()
		var compactions atomic.Int32
		run := runStructuredAnthropicChat(t, schema, false, 70, &compactions, func(int) chattest.AnthropicResponse {
			return text(80)
		})
		require.Equal(t, database.ChatStatusError, run.status)
		require.Equal(t, int32(1), compactions.Load())
		require.Len(t, run.receipts, 1)
		require.Equal(t, codersdk.ChatStructuredOutputErrorCodeGenerationFailed, run.receipts[0].Error.Code)
	})

	// The still-over-limit error also fails a request that holds a valid
	// candidate: an error ends the turn as generation_failed, as for any
	// other turn error after a candidate.
	t.Run("StillOverLimitDropsCandidate", func(t *testing.T) {
		t.Parallel()
		var compactions atomic.Int32
		run := runStructuredAnthropicChat(t, schema, false, 70, &compactions, func(step int) chattest.AnthropicResponse {
			if step == 1 {
				return finalize(80)
			}
			return text(80)
		})
		require.Equal(t, database.ChatStatusError, run.status)
		require.Equal(t, int32(1), compactions.Load())
		require.NotNil(t, run.state.Candidate, "the finalizer call recorded a candidate")
		require.Len(t, run.receipts, 1)
		require.Equal(t, codersdk.ChatStructuredOutputErrorCodeGenerationFailed, run.receipts[0].Error.Code)
	})

	// The previous turn's usage compacts the history before the first step
	// of a structured turn. Compaction copies the request row as a
	// model-only pending row; the copy must not open a second request and
	// the original stays open until the finalizer closes it.
	t.Run("CompactionBeforeFirstStep", func(t *testing.T) {
		t.Parallel()
		var compactions atomic.Int32
		run := runStructuredAnthropicChat(t, schema, true, 70, &compactions, func(step int) chattest.AnthropicResponse {
			switch step {
			case 0:
				return text(80)
			case 1:
				return finalize(10)
			}
			return text(10)
		})
		require.Equal(t, database.ChatStatusWaiting, run.status)
		require.Equal(t, int32(1), compactions.Load())
		require.Equal(t, 1, run.hiddenRequests, "compaction copied the request row")
		require.Zero(t, run.state.Rejections)
		require.Len(t, run.receipts, 1)
		require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, run.receipts[0].Status)
		require.Equal(t, run.state.Request.RequestID, run.receipts[0].RequestID)
	})

	// A candidate survives compaction, and a step that ends the turn closes
	// the request with it before the decision checks the threshold, even
	// when that step is over it.
	t.Run("CandidateSurvivesCompaction", func(t *testing.T) {
		t.Parallel()
		var compactions atomic.Int32
		run := runStructuredAnthropicChat(t, schema, false, 70, &compactions, func(step int) chattest.AnthropicResponse {
			if step == 0 {
				return finalize(80)
			}
			return text(80)
		})
		require.Equal(t, database.ChatStatusWaiting, run.status)
		require.Equal(t, int32(1), compactions.Load())
		require.Len(t, run.receipts, 1)
		require.JSONEq(t, `{"v":"OK"}`, string(run.receipts[0].Value))
		require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, run.receipts[0].Status)
	})
}

// turnOutcomes returns the non-zero turn outcome counts by outcome label
// once any turn has closed. The runner records the outcome after the commit
// that a test observes, so it waits for the first count.
func turnOutcomes(t *testing.T, registry *prometheus.Registry) map[string]float64 {
	t.Helper()
	var out map[string]float64
	testutil.Eventually(testutil.Context(t, testutil.WaitShort), t, func(context.Context) bool {
		out = gatherTurnOutcomes(t, registry)
		return len(out) > 0
	}, testutil.IntervalFast)
	return out
}

func gatherTurnOutcomes(t *testing.T, registry *prometheus.Registry) map[string]float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	out := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "coderd_chatd_turn_outcomes_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "outcome" && metric.GetCounter().GetValue() > 0 {
					out[label.GetValue()] += metric.GetCounter().GetValue()
				}
			}
		}
	}
	return out
}
