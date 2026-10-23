package chatd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

// The token-budget tests use a 100-token window at threshold 70, so the
// handoff, final and forced points are 70, 80 and 90.
const (
	budgetContextLimit = int64(100)
	budgetThreshold    = int32(70)
	budgetUserPrompt   = "read the file and continue"
)

type budgetToolCall struct {
	name string
	args string
}

// budgetToolCallsResponse streams one assistant message with the tool calls
// and prompt usage inputTokens.
func budgetToolCallsResponse(inputTokens int, calls ...budgetToolCall) chattest.AnthropicResponse {
	chunks := []chattest.AnthropicChunk{{
		Type: "message_start",
		Message: chattest.AnthropicChunkMessage{
			ID:    "msg-" + uuid.NewString()[:8],
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-opus-20240229",
			Usage: map[string]int{"input_tokens": inputTokens},
		},
	}}
	for i, call := range calls {
		chunks = append(chunks,
			chattest.AnthropicChunk{
				Type:  "content_block_start",
				Index: i,
				ContentBlock: chattest.AnthropicContentBlock{
					Type:  "tool_use",
					ID:    "toolu_" + uuid.NewString()[:8],
					Name:  call.name,
					Input: json.RawMessage("{}"),
				},
			},
			chattest.AnthropicChunk{
				Type:  "content_block_delta",
				Index: i,
				Delta: chattest.AnthropicDeltaBlock{Type: "input_json_delta", PartialJSON: call.args},
			},
			chattest.AnthropicChunk{Type: "content_block_stop", Index: i},
		)
	}
	chunks = append(chunks,
		chattest.AnthropicChunk{Type: "message_delta", StopReason: "tool_use", UsageMap: map[string]int{"output_tokens": 5}},
		chattest.AnthropicChunk{Type: "message_stop"},
	)
	return chattest.AnthropicStreamingResponse(chunks...)
}

func budgetReadFile(path string) budgetToolCall {
	return budgetToolCall{name: "read_file", args: fmt.Sprintf(`{"path":%q}`, path)}
}

func budgetNotesWrite(content string) budgetToolCall {
	return budgetToolCall{name: "notes", args: fmt.Sprintf(`{"action":"write","path":"checkpoint.md","content":%q}`, content)}
}

var budgetNewContext = budgetToolCall{name: "new_context", args: "{}"}

func budgetTextResponse(inputTokens int, text string) chattest.AnthropicResponse {
	return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
		InputTokens:  inputTokens,
		OutputTokens: 5,
	}, text)...)
}

// budgetRequestText returns every string in the request messages, decoded
// and joined by newlines, so assertions can match nudge and lead-in text.
func budgetRequestText(t *testing.T, req *chattest.AnthropicRequest) string {
	t.Helper()
	raw, err := json.Marshal(req.Messages)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	var b strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case string:
			_, _ = b.WriteString(v)
			_ = b.WriteByte('\n')
		case []any:
			for _, item := range v {
				walk(item)
			}
		case map[string]any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(decoded)
	return b.String()
}

type budgetFixture struct {
	db     database.Store
	server *chatd.Server
	chat   database.Chat
	user   database.User
}

// newBudgetFixture starts a chatd server with the chat-token-budget
// experiment and a chat with a workspace agent that serves read_file.
// stream answers the streaming generation calls in order (1-based).
func newBudgetFixture(
	ctx context.Context,
	t *testing.T,
	readPaths []string,
	compaction func(req *chattest.AnthropicRequest) chattest.AnthropicResponse,
	stream func(n int32, req *chattest.AnthropicRequest) chattest.AnthropicResponse,
	beforeCreate func(db database.Store),
) budgetFixture {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	var streamCount atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if strings.Contains(anthropicRequestBody(t, *req), "You are performing a context compaction") {
			require.NotNil(t, compaction, "unexpected compaction model call")
			return compaction(req)
		}
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		return stream(streamCount.Add(1), req)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateChatModelCompressionThreshold(t, db, model, budgetContextLimit, budgetThreshold)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	for _, path := range readPaths {
		mockConn.EXPECT().ReadFileLines(gomock.Any(), path, int64(1), int64(0), gomock.Any()).
			Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1\tpackage main"}, nil).
			Times(1)
	}
	if beforeCreate != nil {
		beforeCreate(db)
	}
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.Experiments = codersdk.ExperimentsKnown
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		Title:          "token-budget",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText(budgetUserPrompt),
		},
	})
	require.NoError(t, err)
	return budgetFixture{db: db, server: server, chat: chat, user: user}
}

// budgetBoundaryResult returns the only chat_summarized result of the
// chat and decodes it.
func budgetBoundaryResult(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) (compressedCompactionMessages, map[string]any) {
	t.Helper()
	messages := chatMessages(ctx, t, db, chatID)
	promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
	require.NoError(t, err)
	compressed := compressedChatSummarizedMessages(t, append(promptMessages, messages...))
	require.Len(t, compressed.summaries, 1)
	require.Len(t, compressed.calls, 1)
	require.Len(t, compressed.results, 1)
	part := singlePartOfType(t, compressed.results[0], codersdk.ChatMessagePartTypeToolResult)
	var result map[string]any
	require.NoError(t, json.Unmarshal(part.Result, &result))
	return compressed, result
}

func TestActiveServer_TokenBudget(t *testing.T) {
	t.Parallel()

	t.Run("NewContextRollsOverInTheSameTurn", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		var mu sync.Mutex
		bodies := map[int32]string{}
		f := newBudgetFixture(ctx, t, []string{"/tmp/a.txt"}, nil, func(n int32, req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			body := budgetRequestText(t, req)
			mu.Lock()
			bodies[n] = body
			mu.Unlock()
			switch n {
			case 1:
				return budgetToolCallsResponse(75, budgetReadFile("/tmp/a.txt"))
			case 2:
				return budgetToolCallsResponse(76, budgetNotesWrite("CKPT-1"))
			case 3:
				return budgetToolCallsResponse(77, budgetNewContext)
			case 4:
				return budgetTextResponse(20, "done")
			default:
				require.FailNow(t, "unexpected model call", "call %d", n)
				return chattest.AnthropicResponse{}
			}
		}, nil)
		waitForChatStatus(ctx, t, f.db, f.chat.ID, database.ChatStatusWaiting)

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, bodies, 4)
		// The visible user row carries its item ID.
		require.Contains(t, bodies[1], budgetUserPrompt+"\n[id: ")
		require.NotContains(t, bodies[1], "Context usage is")
		// The request after the read_file step has the handoff nudge.
		require.Contains(t, bodies[2], `<context_budget stage="handoff">`)
		require.Contains(t, bodies[2], "Context usage is 75% (75 of 100 tokens). This window ends at 90 tokens.")
		require.NotContains(t, bodies[3], "almost full")
		// The new window has the lead-in and the copied user message only.
		require.Contains(t, bodies[4], `<context_window id="w:1" previous="w:0">`)
		require.Contains(t, bodies[4], "You called new_context.")
		require.Contains(t, bodies[4], "checkpoint.md  6 bytes  written in w:0")
		require.Contains(t, bodies[4], budgetUserPrompt)
		for _, old := range []string{"/tmp/a.txt", "CKPT-1", "context_budget", "new_context must", "[id: "} {
			require.NotContains(t, bodies[4], old)
		}

		compressed, result := budgetBoundaryResult(ctx, t, f.db, f.chat.ID)
		require.Equal(t, "new_context", result["source"])
		require.Equal(t, "CKPT-1", result["summary"])
		require.EqualValues(t, 90, result["threshold_percent"])
		require.EqualValues(t, 100, result["trigger_context_limit_tokens"])
		require.EqualValues(t, 77, result["context_tokens"])
		require.Positive(t, result["estimated_context_tokens"])
		require.Equal(t, database.ChatMessageVisibilityModel, compressed.summaries[0].Visibility)
		messages := chatMessages(ctx, t, f.db, f.chat.ID)
		requireTextPart(t, messages[len(messages)-1], "done")
	})

	fallbackStreams := func(t *testing.T, bodies map[int32]string, mu *sync.Mutex) func(n int32, req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		return func(n int32, req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			mu.Lock()
			bodies[n] = budgetRequestText(t, req)
			mu.Unlock()
			switch n {
			case 1:
				return budgetToolCallsResponse(75, budgetReadFile("/tmp/a.txt"))
			case 2:
				return budgetToolCallsResponse(85, budgetReadFile("/tmp/b.txt"))
			case 3:
				return budgetToolCallsResponse(92, budgetReadFile("/tmp/c.txt"))
			case 4:
				return budgetTextResponse(20, "done")
			default:
				require.FailNow(t, "unexpected model call", "call %d", n)
				return chattest.AnthropicResponse{}
			}
		}
	}
	readPaths := []string{"/tmp/a.txt", "/tmp/b.txt", "/tmp/c.txt"}

	t.Run("SummaryFallbackAtForcedPoint", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		var mu sync.Mutex
		bodies := map[int32]string{}
		var compactions atomic.Int32
		f := newBudgetFixture(ctx, t, readPaths, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			compactions.Add(1)
			return anthropicCompactionResponse(t, req, "budget summary")
		}, fallbackStreams(t, bodies, &mu), nil)
		waitForChatStatus(ctx, t, f.db, f.chat.ID, database.ChatStatusWaiting)

		mu.Lock()
		defer mu.Unlock()
		require.Contains(t, bodies[2], "Context usage is 75%")
		require.Contains(t, bodies[3], "This window is almost full (85 of 100).")
		require.Contains(t, bodies[4], "budget summary")
		require.Equal(t, int32(1), compactions.Load())
		_, result := budgetBoundaryResult(ctx, t, f.db, f.chat.ID)
		require.Equal(t, "automatic", result["source"])
		require.EqualValues(t, 90, result["threshold_percent"])
	})

	t.Run("NewWindowFallbackAtForcedPoint", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		var mu sync.Mutex
		bodies := map[int32]string{}
		f := newBudgetFixture(ctx, t, readPaths, nil, fallbackStreams(t, bodies, &mu), func(db database.Store) {
			require.NoError(t, db.UpsertChatContextWindowConfig(ctx, `{"fallback":"new_window"}`))
		})
		waitForChatStatus(ctx, t, f.db, f.chat.ID, database.ChatStatusWaiting)

		mu.Lock()
		defer mu.Unlock()
		require.Contains(t, bodies[4], `<context_window id="w:1" previous="w:0">`)
		require.Contains(t, bodies[4], "The window reached its limit before you called new_context. Your notes can be out of date.")
		require.Contains(t, bodies[4], "You have no notes.")
		require.NotContains(t, bodies[4], "/tmp/c.txt")
		_, result := budgetBoundaryResult(ctx, t, f.db, f.chat.ID)
		require.Equal(t, "context_limit", result["source"])
		require.Equal(t, "No notes were saved.", result["summary"])
		require.EqualValues(t, 90, result["threshold_percent"])
	})
}

// TestNewContext drives new_context through a chat: it refuses until a
// note changed in this window, a mixed batch is rejected, and only the
// successful call ends the window.
func TestNewContext(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	var mu sync.Mutex
	bodies := map[int32]string{}
	f := newBudgetFixture(ctx, t, nil, nil, func(n int32, req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		mu.Lock()
		bodies[n] = budgetRequestText(t, req)
		mu.Unlock()
		switch n {
		case 1:
			return budgetToolCallsResponse(75, budgetNewContext)
		case 2:
			return budgetToolCallsResponse(76, budgetNotesWrite("MIXED"), budgetNewContext)
		case 3:
			return budgetToolCallsResponse(77, budgetNotesWrite("CKPT-2"))
		case 4:
			return budgetToolCallsResponse(78, budgetNewContext)
		case 5:
			return budgetTextResponse(20, "done")
		default:
			require.FailNow(t, "unexpected model call", "call %d", n)
			return chattest.AnthropicResponse{}
		}
	}, nil)
	waitForChatStatus(ctx, t, f.db, f.chat.ID, database.ChatStatusWaiting)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, bodies, 5)
	require.Contains(t, bodies[2], "Save your checkpoint with notes first.")
	require.NotContains(t, bodies[2], "context_window")
	require.Contains(t, bodies[3], "new_context must be called alone")
	require.NotContains(t, bodies[3], "context_window")
	require.NotContains(t, bodies[4], "context_window")
	require.Contains(t, bodies[5], "Read your notes first:")

	_, result := budgetBoundaryResult(ctx, t, f.db, f.chat.ID)
	require.Equal(t, "new_context", result["source"])
	// The mixed batch did not run its notes write.
	note, err := f.db.GetChatNote(ctx, database.GetChatNoteParams{ChatID: f.chat.ID, Path: "checkpoint.md"})
	require.NoError(t, err)
	require.Equal(t, "CKPT-2", note.Content)
}

// TestRunGenerationStep_NoContextNudgeOnStopHook checks that the stop-hook
// continuation is kept and no nudge is committed on that path, although
// usage is past the handoff point.
func TestRunGenerationStep_NoContextNudgeOnStopHook(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	var secondBody atomic.Value
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1:
			return budgetTextResponse(75, "first answer")
		case 2:
			secondBody.Store(budgetRequestText(t, req))
			return budgetTextResponse(75, "second answer")
		default:
			require.FailNow(t, "stop nudge exceeded continuation cap")
			return chattest.AnthropicResponse{}
		}
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateChatModelCompressionThreshold(t, db, model, budgetContextLimit, budgetThreshold)
	var stopCalls atomic.Int32
	consumer := stopConsumer(t, func() (int, string) {
		stopCalls.Add(1)
		return http.StatusOK, `{"model_context":"continue please"}`
	})
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.Experiments = codersdk.ExperimentsKnown
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "continue once")
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return stopCalls.Load() == 2
	}, testutil.IntervalFast)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Equal(t, int32(2), modelCalls.Load())
	body, ok := secondBody.Load().(string)
	require.True(t, ok)
	require.Contains(t, body, "continue please")
	require.NotContains(t, body, "context_budget")

	promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	for _, message := range promptMessages {
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		for _, part := range parts {
			require.NotContains(t, part.Text, "context_budget")
		}
	}
}
