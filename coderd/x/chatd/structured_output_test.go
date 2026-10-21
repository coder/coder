package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const soSchema = `{"type":"object","properties":{"v":{"type":"string"}},"required":["v"]}`

// soRun is how a chat with a structured output request settled.
type soRun struct {
	chat   database.Chat
	result database.ChatStructuredOutput
	calls  int32
}

// soChat configures runStructuredOutputChat.
type soChat struct {
	// seed sets up the model and returns the AI gateway transport.
	seed      func(db database.Store) (database.User, database.Organization, database.ChatModelConfig, aibridge.TransportFactory)
	configure func(*chatd.Config)
	dynamic   string
	// settle reports whether the chat reached the state the test waits
	// for; nil waits for waiting or error.
	settle func(database.Chat) bool
	// during runs once the chat is created, before the test waits.
	during func(ctx context.Context, server *chatd.Server, chat database.Chat, messageID int64)
}

// runStructuredOutputChat creates a chat whose initial message asks for a
// structured output and returns its result once the chat settles.
func runStructuredOutputChat(t *testing.T, calls *atomic.Int32, opts soChat) soRun {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model, factory := opts.seed(db)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		if opts.configure != nil {
			opts.configure(cfg)
		}
	})
	request, rejection := chatd.ParseResponseFormat(json.RawMessage(`{"type":"json_schema","json_schema":{"name":"report","schema":`+soSchema+`}}`), true)
	require.Nil(t, rejection)
	var tools json.RawMessage
	if opts.dynamic != "" {
		tools = dynamicToolJSON(t, opts.dynamic)
	}
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID, OwnerID: user.ID, Title: "structured", ModelConfigID: model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("report please")}, DynamicTools: tools,
		StructuredOutputRequest: request,
	})
	require.NoError(t, err)
	pending, err := db.GetPendingChatStructuredOutput(ctx, chat.ID)
	require.NoError(t, err)
	if opts.during != nil {
		opts.during(ctx, server, chat, pending.MessageID)
	}
	settle := opts.settle
	if settle == nil {
		settle = func(chat database.Chat) bool {
			return chat.Status == database.ChatStatusWaiting || chat.Status == database.ChatStatusError
		}
	}
	var run soRun
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		run.chat, err = db.GetChatByID(ctx, chat.ID)
		return err == nil && settle(run.chat)
	}, testutil.IntervalFast)
	run.result, err = db.GetChatStructuredOutput(ctx, database.GetChatStructuredOutputParams{ChatID: chat.ID, MessageID: pending.MessageID})
	require.NoError(t, err)
	run.calls = calls.Load()
	return run
}

// openAISeed seeds an OpenAI-compatible model whose streaming call number
// step answers with respond.
func openAISeed(t *testing.T, calls *atomic.Int32, provider string, options json.RawMessage, respond func(step int) chattest.OpenAIResponse) func(database.Store) (database.User, database.Organization, database.ChatModelConfig, aibridge.TransportFactory) {
	return func(db database.Store) (database.User, database.Organization, database.ChatModelConfig, aibridge.TransportFactory) {
		url := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			return respond(int(calls.Add(1)) - 1)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, provider, url)
		if options != nil {
			var err error
			model.Options = options
			model, err = db.UpdateChatModelConfig(context.Background(), database.UpdateChatModelConfigParams{
				ID: model.ID, DisplayName: model.DisplayName, Model: model.Model, Enabled: model.Enabled,
				ContextLimit: model.ContextLimit, CompressionThreshold: model.CompressionThreshold, Options: model.Options, AIProviderID: model.AIProviderID,
			})
			require.NoError(t, err)
		}
		return user, org, model, chattest.NewMockAIBridgeTransport(t, url)
	}
}

// anthropicSeed seeds an Anthropic model. A non-zero threshold sets a 100
// token context limit with that compaction threshold, and compaction calls
// get a summary.
func anthropicSeed(t *testing.T, calls *atomic.Int32, threshold int32, respond func(step int, req *chattest.AnthropicRequest) chattest.AnthropicResponse) func(database.Store) (database.User, database.Organization, database.ChatModelConfig, aibridge.TransportFactory) {
	return func(db database.Store) (database.User, database.Organization, database.ChatModelConfig, aibridge.TransportFactory) {
		url := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}
			if strings.Contains(anthropicRequestBody(t, *req), "You are performing a context compaction") {
				return anthropicCompactionResponse(t, req, "structured compaction summary")
			}
			return respond(int(calls.Add(1))-1, req)
		})
		user, org, model := seedAnthropicChatDependencies(t, db, url)
		if threshold != 0 {
			model = updateChatModelCompressionThreshold(t, db, model, 100, threshold)
		}
		return user, org, model, chattest.NewMockAIBridgeTransport(t, url, chattest.WithPreservePath())
	}
}

func soFinalizer(args string) chattest.OpenAIResponse {
	return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk(chatstructured.FinalizerToolName, args))
}

func soText() chattest.OpenAIResponse {
	return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
}

func soFinish(reason string) chattest.OpenAIResponse {
	return chattest.OpenAIStreamingResponse(chattest.OpenAIChunk{Object: "chat.completion.chunk", Choices: []chattest.OpenAIChunkChoice{{FinishReason: reason}}})
}

func anthropicWithInputTokens(chunks []chattest.AnthropicChunk, tokens int) chattest.AnthropicResponse {
	for i := range chunks {
		if chunks[i].Type == "message_start" {
			chunks[i].Message.Usage = map[string]int{"input_tokens": tokens}
		}
	}
	return chattest.AnthropicStreamingResponse(chunks...)
}

func requireResult(t *testing.T, result database.ChatStructuredOutput, status database.ChatStructuredOutputStatus, code codersdk.ChatStructuredOutputErrorCode) {
	t.Helper()
	require.Equal(t, status, result.Status, "error code %q", result.ErrorCode.String)
	require.Equal(t, string(code), result.ErrorCode.String)
}

// TestStructuredOutputCloseRule is acceptance 10: across the exit paths of
// the F2 inventory, no result stays pending once the chat reaches waiting
// or error, and each closes with the code its exit implies. Only the paths
// that keep a turn running keep the result pending.
func TestStructuredOutputCloseRule(t *testing.T) {
	t.Parallel()
	succeeded, failed := database.ChatStructuredOutputStatusSucceeded, database.ChatStructuredOutputStatusFailed
	// Responses are built per call: a streaming response is read once.
	type response = func() chattest.OpenAIResponse
	valid := func() chattest.OpenAIResponse { return soFinalizer(`{"output":{"v":"OK"}}`) }
	invalid := func() chattest.OpenAIResponse { return soFinalizer(`{"output":{"v":1}}`) }
	finish := func(reason string) response { return func() chattest.OpenAIResponse { return soFinish(reason) } }
	providerError := func() chattest.OpenAIResponse {
		return chattest.OpenAIErrorResponse(400, "invalid_request_error", "boom")
	}
	steps := func(responses ...response) func(int) chattest.OpenAIResponse {
		return func(step int) chattest.OpenAIResponse {
			if step < len(responses) {
				return responses[step]()
			}
			return soText()
		}
	}
	for _, tt := range []struct {
		name      string
		respond   func(int) chattest.OpenAIResponse
		configure func(*chatd.Config)
		dynamic   string
		status    database.ChatStructuredOutputStatus
		code      codersdk.ChatStructuredOutputErrorCode
		chat      database.ChatStatus
		rejected  int32
	}{
		{name: "Succeeded", respond: steps(valid), status: succeeded, chat: database.ChatStatusWaiting},
		{name: "TextAnswer", respond: steps(), status: failed, code: codersdk.ChatStructuredOutputErrorCodeNotProduced, chat: database.ChatStatusWaiting},
		{name: "EmptyStep", respond: steps(finish("stop")), status: failed, code: codersdk.ChatStructuredOutputErrorCodeNotProduced, chat: database.ChatStatusWaiting},
		{name: "ContentFilter", respond: steps(finish("content_filter")), status: failed, code: codersdk.ChatStructuredOutputErrorCodeGenerationFailed, chat: database.ChatStatusError},
		{name: "NoOutputRetried", respond: steps(finish("tool_calls"), valid), status: succeeded, chat: database.ChatStatusWaiting},
		{name: "ValidationExhausted", respond: steps(invalid, invalid, invalid), status: failed, code: codersdk.ChatStructuredOutputErrorCodeValidationExhausted, chat: database.ChatStatusWaiting, rejected: 3},
		{name: "RepairedAfterRejections", respond: steps(invalid, invalid, valid), status: succeeded, chat: database.ChatStatusWaiting, rejected: 2},
		{
			name: "MaxSteps", respond: steps(invalid, invalid), status: failed, code: codersdk.ChatStructuredOutputErrorCodeNotProduced, chat: database.ChatStatusWaiting, rejected: 2,
			configure: func(cfg *chatd.Config) { cfg.Limits.MaxStepsPerTurn = 2 },
		},
		{
			name: "MaxStepsKeepsValue", respond: steps(valid, valid), status: succeeded, chat: database.ChatStatusWaiting,
			configure: func(cfg *chatd.Config) { cfg.Limits.MaxStepsPerTurn = 1 },
		},
		{
			name: "CallerToolsDisabled", respond: steps(soDynamicCall, valid), status: succeeded, chat: database.ChatStatusWaiting, dynamic: "dyn",
			configure: func(cfg *chatd.Config) { cfg.DisableCallerSuppliedTools = true },
		},
		{name: "ProviderError", respond: steps(providerError), status: failed, code: codersdk.ChatStructuredOutputErrorCodeGenerationFailed, chat: database.ChatStatusError},
		{name: "ProviderErrorAfterValue", respond: steps(valid, providerError), status: succeeded, chat: database.ChatStatusError},
		{name: "MixedBatchRejected", respond: steps(soMixedBatch, valid), status: succeeded, chat: database.ChatStatusWaiting, dynamic: "dyn", rejected: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			run := runStructuredOutputChat(t, &calls, soChat{
				seed: openAISeed(t, &calls, "openai-compat", nil, tt.respond), configure: tt.configure, dynamic: tt.dynamic,
			})
			require.Equal(t, tt.chat, run.chat.Status, chatLastErrorMessage(run.chat.LastError))
			requireResult(t, run.result, tt.status, tt.code)
			require.Equal(t, tt.rejected, run.result.RejectedCalls)
		})
	}

	// A client tool call keeps the turn open: the chat waits in
	// requires_action and the result stays pending on purpose.
	t.Run("RequiresActionKeepsPending", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		run := runStructuredOutputChat(t, &calls, soChat{
			seed: openAISeed(t, &calls, "openai-compat", nil, steps(soDynamicCall)), dynamic: "dyn",
			settle: func(chat database.Chat) bool { return chat.Status == database.ChatStatusRequiresAction },
		})
		requireResult(t, run.result, database.ChatStructuredOutputStatusPending, "")
	})

	// An edit of the request message starts a newer turn, which closes
	// the running request as interrupted.
	t.Run("EditSupersedes", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		blocked, block := make(chan struct{}), make(chan struct{})
		var once sync.Once
		run := runStructuredOutputChat(t, &calls, soChat{
			seed: openAISeed(t, &calls, "openai-compat", nil, func(step int) chattest.OpenAIResponse {
				if step == 0 {
					once.Do(func() { close(blocked) })
					<-block
				}
				return soText()
			}),
			during: func(ctx context.Context, server *chatd.Server, chat database.Chat, messageID int64) {
				t.Cleanup(func() { close(block) })
				testutil.TryReceive(ctx, t, blocked)
				_, err := server.EditMessage(ctx, chatd.EditMessageOptions{
					ChatID: chat.ID, CreatedBy: chat.OwnerID, EditedMessageID: messageID,
					Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
				})
				require.NoError(t, err)
			},
		})
		requireResult(t, run.result, database.ChatStructuredOutputStatusFailed, codersdk.ChatStructuredOutputErrorCodeInterrupted)
	})

	// Compaction that still leaves the history over the limit ends the
	// turn with an error. A validated value from before survives it (D13).
	for _, withValue := range []bool{false, true} {
		t.Run(map[bool]string{false: "CompactionStillOverLimit", true: "CompactionStillOverLimitKeepsValue"}[withValue], func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			run := runStructuredOutputChat(t, &calls, soChat{seed: anthropicSeed(t, &calls, 70, func(step int, _ *chattest.AnthropicRequest) chattest.AnthropicResponse {
				if withValue && step == 1 {
					return anthropicWithInputTokens(chattest.AnthropicToolCallChunks(chatstructured.FinalizerToolName, `{"output":{"v":"OK"}}`), 80)
				}
				return anthropicWithInputTokens(chattest.AnthropicToolCallChunks("dyn", `{}`), 80)
			}), configure: func(cfg *chatd.Config) { cfg.DisableCallerSuppliedTools = true }})
			require.Equal(t, database.ChatStatusError, run.chat.Status)
			if withValue {
				requireResult(t, run.result, database.ChatStructuredOutputStatusSucceeded, "")
				return
			}
			requireResult(t, run.result, database.ChatStructuredOutputStatusFailed, codersdk.ChatStructuredOutputErrorCodeGenerationFailed)
		})
	}
}

func soDynamicCall() chattest.OpenAIResponse {
	return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("dyn", `{}`))
}

// soMixedBatch emits a finalizer call next to a client tool call in one
// step; the exclusive policy rejects the batch.
func soMixedBatch() chattest.OpenAIResponse {
	chunk := chattest.OpenAIToolCallChunk(chatstructured.FinalizerToolName, `{"output":{"v":"MIXED"}}`)
	other := chattest.OpenAIToolCallChunk("dyn", `{}`).Choices[0].ToolCalls[0]
	other.Index = 1
	chunk.Choices[0].ToolCalls = append(chunk.Choices[0].ToolCalls, other)
	return chattest.OpenAIStreamingResponse(chunk)
}

// TestStructuredOutputConfigurationError is acceptance 8: a model with
// strict tool schemas, or a tool named like the finalizer, fails the
// request as configuration_error without a model call.
func TestStructuredOutputConfigurationError(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		provider string
		options  json.RawMessage
		dynamic  string
		reason   string
	}{
		{name: "StrictToolSchemas", provider: "openai", options: json.RawMessage(`{"provider_options":{"openai":{"strict_json_schema":true}}}`), reason: "The model's strict tool schema setting cannot be used with structured output."},
		{name: "ToolNameCollision", provider: "openai-compat", dynamic: chatstructured.FinalizerToolName, reason: "Another tool is named coder_structured_output, which structured output reserves."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			run := runStructuredOutputChat(t, &calls, soChat{
				seed: openAISeed(t, &calls, tt.provider, tt.options, func(int) chattest.OpenAIResponse { return soText() }), dynamic: tt.dynamic,
			})
			require.Equal(t, database.ChatStatusError, run.chat.Status)
			require.Equal(t, tt.reason, chatLastErrorMessage(run.chat.LastError))
			requireResult(t, run.result, database.ChatStructuredOutputStatusFailed, codersdk.ChatStructuredOutputErrorCodeConfigurationError)
			require.Zero(t, run.calls, "no model call")
		})
	}
}

// TestStructuredOutputAnthropicOutputLimit is acceptance 11: a finalizer
// call that the output token limit cuts gets exactly one tool result, so
// the next request to Anthropic carries no duplicate tool_use_id.
func TestStructuredOutputAnthropicOutputLimit(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var cutID string
	var mu sync.Mutex
	var bodies []string
	run := runStructuredOutputChat(t, &calls, soChat{seed: anthropicSeed(t, &calls, 0, func(step int, req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		mu.Lock()
		bodies = append(bodies, anthropicRequestBody(t, *req))
		mu.Unlock()
		switch step {
		case 0:
			chunks := chattest.AnthropicToolCallChunks(chatstructured.FinalizerToolName, `{"output":{"v":"tr`)
			for i := range chunks {
				if chunks[i].Type == "content_block_start" {
					mu.Lock()
					cutID = chunks[i].ContentBlock.ID
					mu.Unlock()
				}
				if chunks[i].Type == "message_delta" {
					chunks[i].StopReason = "max_tokens"
				}
			}
			return chattest.AnthropicStreamingResponse(chunks...)
		case 1:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks(chatstructured.FinalizerToolName, `{"output":{"v":"OK"}}`)...)
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
	})})
	require.Equal(t, database.ChatStatusWaiting, run.chat.Status)
	requireResult(t, run.result, database.ChatStructuredOutputStatusSucceeded, "")
	require.EqualValues(t, 1, run.result.RejectedCalls)
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, cutID)
	require.GreaterOrEqual(t, len(bodies), 2)
	require.Equal(t, 1, strings.Count(bodies[1], `"tool_use_id":"`+cutID+`"`), "the cut call has one tool result")
}

// TestStructuredOutputDeletedWithChat is the second half of acceptance 9:
// the result row goes away with its chat.
func TestStructuredOutputDeletedWithChat(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", "http://127.0.0.1:1")
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: model.ID})
	message := dbgen.ChatMessage(t, db, database.ChatMessage{ChatID: chat.ID, Role: database.ChatMessageRoleUser})
	_, err := db.InsertChatStructuredOutput(ctx, database.InsertChatStructuredOutputParams{MessageID: message.ID, ChatID: chat.ID, Request: json.RawMessage(`{}`)})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "DELETE FROM chats WHERE id = $1", chat.ID)
	require.NoError(t, err)
	_, err = db.GetChatStructuredOutput(ctx, database.GetChatStructuredOutputParams{ChatID: chat.ID, MessageID: message.ID})
	require.ErrorIs(t, err, sql.ErrNoRows)
}
