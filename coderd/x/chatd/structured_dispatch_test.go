package chatd_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// finalizerRun is what one turn with an open structured request committed.
type finalizerRun struct {
	tools    []chattest.OpenAITool
	results  []codersdk.ChatMessagePart
	controls []chatstructured.Control
	state    chatstructured.ActiveRequestState
	client   []codersdk.ChatMessage
	receipts []codersdk.ChatStructuredOutput
	requests [][]chattest.OpenAIMessage
	status   database.ChatStatus
}

// runFinalizerTurn runs one turn whose first model step emits calls, then
// answers with text. A nil schema starts an ordinary chat.
func runFinalizerTurn(t *testing.T, schema json.RawMessage, calls ...chattest.OpenAIToolCall) finalizerRun {
	t.Helper()
	if len(calls) == 0 {
		return runStructuredTurn(t, schema, 0)
	}
	return runStructuredTurn(t, schema, 0, calls)
}

// runStructuredTurn runs one turn whose model steps emit steps in order and
// then answer with text; a non-zero failStatus fails the first model call
// before them.
func runStructuredTurn(t *testing.T, schema json.RawMessage, failStatus int, steps ...[]chattest.OpenAIToolCall) finalizerRun {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	var run finalizerRun
	var mu sync.Mutex
	var streamed atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		step := int(streamed.Add(1)) - 1
		mu.Lock()
		if step == 0 {
			run.tools = append([]chattest.OpenAITool(nil), req.Tools...)
		}
		run.requests = append(run.requests, append([]chattest.OpenAIMessage(nil), req.Messages...))
		mu.Unlock()
		if failStatus != 0 {
			if step == 0 {
				return chattest.OpenAIResponse{Error: &chattest.ErrorResponse{StatusCode: failStatus, Type: "invalid_request_error", Message: "PROVIDER_BODY_MARKER"}}
			}
			step--
		}
		if step >= len(steps) {
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		}
		calls := steps[step]
		chunk := chattest.OpenAIToolCallChunk(calls[0].Function.Name, calls[0].Function.Arguments)
		for i, call := range calls[1:] {
			call.Index, call.ID, call.Type = i+1, "call_"+uuid.NewString()[:8], "function"
			chunk.Choices[0].ToolCalls = append(chunk.Choices[0].ToolCalls, call)
		}
		return chattest.OpenAIStreamingResponse(chunk)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	tools, err := json.Marshal([]mcp.Tool{{Name: "dyn", InputSchema: map[string]any{"type": "object"}}})
	require.NoError(t, err)
	content := []codersdk.ChatMessagePart{codersdk.ChatMessageText("answer")}
	if schema != nil {
		request, err := chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: uuid.New(), Name: "report", Schema: schema})
		require.NoError(t, err)
		content = append(content, request)
	}
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID, OwnerID: user.ID, Title: "finalizer", ModelConfigID: model.ID, InitialUserContent: content, DynamicTools: tools,
	})
	require.NoError(t, err)
	var done database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		done, err = db.GetChatByID(ctx, chat.ID)
		return err == nil && (done.Status == database.ChatStatusWaiting || done.Status == database.ChatStatusError)
	}, testutil.IntervalFast)
	run.status = done.Status

	history, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	rows := make([]chatstructured.Row, 0, len(history))
	for _, msg := range history {
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		rows = append(rows, chatstructured.Row{ID: msg.ID, Role: codersdk.ChatMessageRole(msg.Role), Visibility: chatstructured.Visibility(msg.Visibility), Parts: parts})
		run.client = append(run.client, db2sdk.ChatMessage(msg))
		if out, err := chatstate.ReceiptRowOutcome(msg); err == nil {
			run.receipts = append(run.receipts, out)
		}
		for _, part := range parts {
			switch part.Type {
			case codersdk.ChatMessagePartTypeToolResult:
				run.results = append(run.results, part)
			case codersdk.ChatMessagePartTypeStructuredOutputControl:
				control, err := chatstructured.DecodeControlPart(part)
				require.NoError(t, err)
				run.controls = append(run.controls, control)
			}
		}
	}
	if schema != nil {
		run.state, err = chatstructured.ActiveRequest(rows)
		require.NoError(t, err)
	}
	mu.Lock()
	defer mu.Unlock()
	return run
}

func finalizerToolCall(name, args string) chattest.OpenAIToolCall {
	return chattest.OpenAIToolCall{Function: chattest.OpenAIToolCallFunction{Name: name, Arguments: args}}
}

func TestFinalizerDispatch(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}},"required":["a"]}`)
	valid := finalizerToolCall(chatstructured.FinalizerToolName, `{"output":{"a":"CANDIDATE_MARKER"}}`)

	t.Run("ToolList", func(t *testing.T) {
		t.Parallel()
		open, ordinary := runFinalizerTurn(t, schema), runFinalizerTurn(t, nil)
		compiled, err := chatstructured.CompileSchema(schema)
		require.NoError(t, err)
		definition, err := json.Marshal(compiled.FinalizerDefinition().InputSchema)
		require.NoError(t, err)
		var offered []chattest.OpenAITool
		for _, tool := range open.tools {
			if tool.Function.Name == chatstructured.FinalizerToolName {
				require.JSONEq(t, string(definition), string(tool.Function.Parameters))
				continue
			}
			offered = append(offered, tool)
		}
		require.Len(t, offered, len(open.tools)-1)
		require.Equal(t, ordinary.tools, offered)
	})

	t.Run("Candidate", func(t *testing.T) {
		t.Parallel()
		run := runFinalizerTurn(t, schema, valid)
		require.Len(t, run.results, 1)
		require.False(t, run.results[0].IsError)
		require.NotContains(t, string(run.results[0].Result), "CANDIDATE_MARKER")
		require.Len(t, run.controls, 1)
		require.Equal(t, chatstructured.ControlCandidate, run.controls[0].Kind)
		require.JSONEq(t, `{"a":"CANDIDATE_MARKER"}`, string(run.controls[0].Value))
		require.Equal(t, run.controls[0].Value, run.state.Candidate)
		require.Zero(t, run.state.Rejections)
		clientJSON, err := json.Marshal(run.client)
		require.NoError(t, err)
		require.Equal(t, 2, strings.Count(string(clientJSON), "CANDIDATE_MARKER"), "only the call arguments and the receipt carry the value")
	})

	for name, calls := range map[string][]chattest.OpenAIToolCall{
		"SchemaMismatch": {finalizerToolCall(chatstructured.FinalizerToolName, `{"output":{"a":1}}`)},
		// Stored arguments render 1e200 as a 201 digit decimal, which the
		// executor rejects as too long: feedback and a rejection, not an error.
		"UnstorableNumber": {finalizerToolCall(chatstructured.FinalizerToolName, `{"output":{"a":"x","b":1e200}}`)},
		"TwoFinalizers":    {valid, valid},
		"LocalSibling":     {valid, finalizerToolCall("read_file", `{"path":"/tmp/a"}`)},
		"DynamicSibling":   {valid, finalizerToolCall("dyn", `{}`)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			run := runFinalizerTurn(t, schema, calls...)
			require.Len(t, run.results, len(calls))
			for _, result := range run.results {
				require.True(t, result.IsError)
			}
			// Two text answers are rejected as repairs, which spends the budget.
			require.Len(t, run.controls, 3)
			require.Equal(t, chatstructured.ControlRejection, run.controls[0].Kind)
			require.Nil(t, run.state.Candidate)
			require.Equal(t, 3, run.state.Rejections)
			require.Equal(t, codersdk.ChatStructuredOutputErrorCodeValidationExhausted, run.state.Outcome.Error.Code)
		})
	}
}
