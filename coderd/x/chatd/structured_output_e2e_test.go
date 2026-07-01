package chatd_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/structuredoutput"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const structuredE2ESchema = `{
	"type": "object",
	"properties": {"answer": {"type": "string"}},
	"required": ["answer"],
	"additionalProperties": false
}`

func structuredE2EContent(text string) []codersdk.ChatMessagePart {
	return []codersdk.ChatMessagePart{
		codersdk.ChatMessageText(text),
		codersdk.ChatMessageResponseFormat(codersdk.ChatResponseFormat{
			Type: codersdk.ChatResponseFormatTypeJSONSchema,
			JSONSchema: &codersdk.ChatResponseFormatJSONSchema{
				Name:   "answer_report",
				Schema: json.RawMessage(structuredE2ESchema),
			},
		}),
	}
}

func TestActiveServer_StructuredOutput(t *testing.T) {
	t.Parallel()

	t.Run("finalizer success finishes turn", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		var rawBodiesMu sync.Mutex
		var rawBodies []string
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			rawBodiesMu.Lock()
			rawBodies = append(rawBodies, string(req.RawBody))
			rawBodiesMu.Unlock()
			streamedCallCount.Add(1)
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"42"}}`),
			)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		server := newActiveTestServer(t, db, ps)

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           testAPIKeyID(t, db, user.ID),
			Title:              "structured-output-success",
			ModelConfigID:      model.ID,
			InitialUserContent: structuredE2EContent("what is the answer?"),
		})
		require.NoError(t, err)

		chatResult := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chatResult.WorkerID.Valid)
		require.Equal(t, int32(1), streamedCallCount.Load(),
			"successful finalizer result should stop the turn after one model call")

		parts := chatToolParts(ctx, t, db, chat.ID)
		call := requireToolCallPart(t, parts, structuredoutput.ToolName)
		require.JSONEq(t, `{"output":{"answer":"42"}}`, string(call.Args))
		result := requireToolResultPart(t, parts, structuredoutput.ToolName)
		require.False(t, result.IsError)
		require.JSONEq(t, `{"answer":"42"}`, string(result.Result))

		rawBodiesMu.Lock()
		bodies := append([]string(nil), rawBodies...)
		rawBodiesMu.Unlock()
		require.Len(t, bodies, 1)
		// Required tool choice is set on structured output steps.
		require.Contains(t, bodies[0], `"tool_choice":"required"`)
		// The finalizer tool definition carries the caller schema.
		require.Contains(t, bodies[0], structuredoutput.ToolName)
		// The response-format part must never reach the provider.
		require.NotContains(t, bodies[0], "response-format")
		require.NotContains(t, bodies[0], "response_format")
	})

	t.Run("invalid args retry then success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamedCallCount.Add(1) {
			case 1:
				// Missing required "answer" property.
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{}}`),
				)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"fixed"}}`),
				)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		server := newActiveTestServer(t, db, ps)

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           testAPIKeyID(t, db, user.ID),
			Title:              "structured-output-retry",
			ModelConfigID:      model.ID,
			InitialUserContent: structuredE2EContent("answer with structure"),
		})
		require.NoError(t, err)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), streamedCallCount.Load(),
			"validation failure should retry within the same turn")

		parts := chatToolParts(ctx, t, db, chat.ID)
		var sawError, sawSuccess bool
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.ToolName != structuredoutput.ToolName {
				continue
			}
			if part.IsError {
				sawError = true
				require.Contains(t, string(part.Result), "does not satisfy the required schema")
			} else {
				sawSuccess = true
				require.JSONEq(t, `{"answer":"fixed"}`, string(part.Result))
			}
		}
		require.True(t, sawError, "first invalid finalizer call should produce an error tool result")
		require.True(t, sawSuccess, "second finalizer call should produce the validated result")
	})

	t.Run("text only response does not finish turn", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamedCallCount.Add(1) {
			case 1:
				// A plain-text answer must not complete the turn.
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("the answer is 42")...)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"42"}}`),
				)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		server := newActiveTestServer(t, db, ps)

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           testAPIKeyID(t, db, user.ID),
			Title:              "structured-output-text-retry",
			ModelConfigID:      model.ID,
			InitialUserContent: structuredE2EContent("answer with structure"),
		})
		require.NoError(t, err)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), streamedCallCount.Load(),
			"a text-only step must regenerate instead of finishing the turn")

		result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), structuredoutput.ToolName)
		require.False(t, result.IsError)
	})

	t.Run("dynamic tool pauses then finalizes", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamedCallCount.Add(1) {
			case 1:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("my_dynamic_tool", `{"query":"data"}`),
				)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"from dynamic"}}`),
				)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		dynamicToolsJSON := dynamicToolJSON(t, "my_dynamic_tool")
		server := newActiveTestServer(t, db, ps)

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           testAPIKeyID(t, db, user.ID),
			Title:              "structured-output-dynamic",
			ModelConfigID:      model.ID,
			InitialUserContent: structuredE2EContent("use the dynamic tool then answer"),
			DynamicTools:       dynamicToolsJSON,
		})
		require.NoError(t, err)

		// 1. The dynamic tool call pauses the turn as usual.
		var chatResult database.Chat
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			got, getErr := db.GetChatByID(ctx, chat.ID)
			if getErr != nil {
				return false
			}
			chatResult = got
			return got.Status == database.ChatStatusRequiresAction || got.Status == database.ChatStatusError
		}, testutil.IntervalFast)
		require.Equal(t, database.ChatStatusRequiresAction, chatResult.Status,
			"expected requires_action, got %s (last_error=%q)",
			chatResult.Status, chatLastErrorMessage(chatResult.LastError))

		call := requireToolCallPart(t, chatToolParts(ctx, t, db, chat.ID), "my_dynamic_tool")

		// 2. Submitting results resumes the turn, which then
		// finalizes with structured output.
		err = server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
			ChatID:        chat.ID,
			UserID:        user.ID,
			ModelConfigID: chatResult.LastModelConfigID,
			Results: []codersdk.ToolResult{{
				ToolCallID: call.ToolCallID,
				Output:     json.RawMessage(`{"result":"dynamic data"}`),
			}},
			DynamicTools: dynamicToolsJSON,
		})
		require.NoError(t, err)

		chatResult = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), streamedCallCount.Load())
		result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), structuredoutput.ToolName)
		require.False(t, result.IsError)
		require.JSONEq(t, `{"answer":"from dynamic"}`, string(result.Result))
	})

	t.Run("finalizer batched with another tool is rejected then retried", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamedCallCount.Add(1) {
			case 1:
				// The finalizer is exclusive: batching it with another
				// tool call must fail both with policy errors. Both
				// calls share one chunk with distinct indexes so the
				// stream parser keeps them separate.
				batched := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/x"}`)
				finalizerCall := chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"early"}}`).Choices[0].ToolCalls[0]
				finalizerCall.Index = 1
				batched.Choices[0].ToolCalls = append(batched.Choices[0].ToolCalls, finalizerCall)
				return chattest.OpenAIStreamingResponse(batched)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"alone"}}`),
				)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		server := newActiveTestServer(t, db, ps)

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           testAPIKeyID(t, db, user.ID),
			Title:              "structured-output-exclusive",
			ModelConfigID:      model.ID,
			InitialUserContent: structuredE2EContent("answer with structure"),
		})
		require.NoError(t, err)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), streamedCallCount.Load())

		parts := chatToolParts(ctx, t, db, chat.ID)
		var policyError, success bool
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.ToolName != structuredoutput.ToolName {
				continue
			}
			if part.IsError && strings.Contains(string(part.Result), "must be called alone") {
				policyError = true
			}
			if !part.IsError {
				success = true
				require.JSONEq(t, `{"answer":"alone"}`, string(part.Result))
			}
		}
		require.True(t, policyError, "batched finalizer call should produce an exclusivity policy error")
		require.True(t, success, "retried lone finalizer call should succeed")
	})
}
