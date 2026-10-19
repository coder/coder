package chatd_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

// A finalizer call with unsafe argument bytes for an open structured request
// commits with placeholder arguments, a rejection and an error result, and
// the turn continues. The same call in an ordinary chat is unchanged.
func TestFinalizerArgumentsScreenedBeforeCommit(t *testing.T) {
	t.Parallel()
	const marker = "RAW_FINALIZER_MARKER"
	unsafeArgs := `{"output":1,"output":"` + marker + `"}`
	for _, governing := range []bool{true, false} {
		t.Run(map[bool]string{true: "OpenRequest", false: "Ordinary"}[governing], func(t *testing.T) {
			t.Parallel()
			db, ps := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			var streamed atomic.Int32
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("title")
				}
				if streamed.Add(1) == 1 {
					return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk(chatstructured.FinalizerToolName, unsafeArgs))
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
			})
			user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
			factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
			})
			content := []codersdk.ChatMessagePart{codersdk.ChatMessageText("answer")}
			requestID := uuid.New()
			if governing {
				request, err := chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: requestID, Name: "report", Schema: json.RawMessage(`{}`)})
				require.NoError(t, err)
				content = append(content, request)
			}
			chat, err := server.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID: org.ID, OwnerID: user.ID, Title: "finalizer", ModelConfigID: model.ID, InitialUserContent: content,
			})
			require.NoError(t, err)
			var done database.Chat
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				done, err = db.GetChatByID(ctx, chat.ID)
				return err == nil && (done.Status == database.ChatStatusWaiting || done.Status == database.ChatStatusError)
			}, testutil.IntervalFast)
			require.Equal(t, database.ChatStatusWaiting, done.Status, chatLastErrorMessage(done.LastError))
			// An open request repairs the text answer until its budget ends.
			require.EqualValues(t, map[bool]int{true: 3, false: 2}[governing], streamed.Load())

			visible, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
			require.NoError(t, err)
			prompt, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
			require.NoError(t, err)
			rows := make([]chatstructured.Row, 0, len(visible))
			var args []string
			var results []codersdk.ChatMessagePart
			for _, msg := range visible {
				parts, err := chatprompt.ParseContent(msg)
				require.NoError(t, err)
				rows = append(rows, chatstructured.Row{ID: msg.ID, Role: codersdk.ChatMessageRole(msg.Role), Visibility: chatstructured.Visibility(msg.Visibility), Parts: parts})
				for _, part := range parts {
					switch part.Type {
					case codersdk.ChatMessagePartTypeToolCall:
						args = append(args, string(part.Args))
					case codersdk.ChatMessagePartTypeToolResult:
						results = append(results, part)
					}
				}
			}
			state, err := chatstructured.ActiveRequest(rows)
			require.NoError(t, err)
			// The call is resolved exactly once and never dispatched again.
			require.Len(t, results, 1)
			require.True(t, results[0].IsError)
			if !governing {
				// JSONB normalizes the stored bytes; the raw value still lands.
				require.Len(t, args, 1)
				require.Contains(t, args[0], marker)
				require.False(t, state.Active)
				return
			}
			require.Equal(t, []string{"{}"}, args)
			require.Contains(t, string(results[0].Result), chatstructured.ErrDuplicateKey.Error())
			require.Equal(t, 3, state.Rejections)
			require.Nil(t, state.Candidate)
			require.True(t, state.Closed)
			for _, msg := range append(visible, prompt...) {
				require.NotContains(t, string(msg.Content.RawMessage), marker)
			}
		})
	}
}

// A rejected finalizer call leaves hook admission but keeps its ID in the
// step, so an ordinary call sharing that ID still fails the batch before any
// pre_tool_use dispatch instead of committing unadmitted.
func TestRejectedFinalizerCallKeepsDuplicateToolUseIDCheck(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunk := chattest.OpenAIToolCallChunk(chatstructured.FinalizerToolName, `{"output":1,"output":2}`)
		ordinary := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/x"}`).Choices[0].ToolCalls[0]
		chunk.Choices[0].ToolCalls[0].ID, ordinary.ID, ordinary.Index = "call_duplicate", "call_duplicate", 1
		chunk.Choices[0].ToolCalls = append(chunk.Choices[0].ToolCalls, ordinary)
		return chattest.OpenAIStreamingResponse(chunk)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		return `{}`
	})
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	request, err := chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: uuid.New(), Name: "report", Schema: json.RawMessage(`{}`)})
	require.NoError(t, err)
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID, OwnerID: user.ID, Title: "finalizer", ModelConfigID: model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("answer"), request},
	})
	require.NoError(t, err)
	var done database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		done, err = db.GetChatByID(ctx, chat.ID)
		return err == nil && (done.Status == database.ChatStatusWaiting || done.Status == database.ChatStatusError)
	}, testutil.IntervalFast)
	require.Equal(t, database.ChatStatusError, done.Status)
	require.Contains(t, chatLastErrorMessage(done.LastError), "duplicate tool use ID")
	require.Zero(t, hookCalls.Load(), "a duplicated ID must be rejected before any dispatch")
}
