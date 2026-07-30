package chatd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

func TestCompactionHooksHintAndPostCommitResponses(t *testing.T) {
	t.Parallel()

	var postSawCommitted atomic.Bool
	fixture := startCompactionHookChat(t,
		func(t *testing.T, db database.Store, request agenthooks.Request) (int, string) {
			switch request.Type {
			case agenthooks.EventPreCompact:
				return http.StatusOK, `{"model_context":"preserve deployment constraints","user_message":"compaction starting"}`
			case agenthooks.EventPostCompact:
				postSawCommitted.Store(hasCompactionRows(t, db, request.Meta.ChatID))
				return http.StatusOK, `{"model_context":"post compact context","user_message":"compaction complete"}`
			default:
				return http.StatusOK, `{}`
			}
		},
		func(t *testing.T, body string) {
			require.Contains(t, body, "preserve deployment constraints")
		},
	)

	waitCtx := testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(waitCtx, t, func(context.Context) bool {
		updated, err := fixture.db.GetChatByID(waitCtx, fixture.chat.ID)
		return err == nil && updated.Status == database.ChatStatusWaiting && !updated.Archived
	}, testutil.IntervalFast)
	// post_compact runs before its effects commit with the compaction step.
	require.False(t, postSawCommitted.Load())
	require.Equal(t, int32(1), fixture.compactionCalls.Load())
	require.Equal(t, int32(2), fixture.streamCalls.Load(),
		"automatic compaction continues the turn, and hook effects must not suppress that")

	userMessages := chatMessages(fixture.ctx, t, fixture.db, fixture.chat.ID)
	promptMessages, err := fixture.db.GetChatMessagesForPromptByChatID(fixture.ctx, fixture.chat.ID)
	require.NoError(t, err)
	require.True(t, hasMessageText(t, userMessages, "compaction starting", database.ChatMessageVisibilityUser))
	require.True(t, hasMessageText(t, userMessages, "compaction complete", database.ChatMessageVisibilityUser))
	require.True(t, hasMessageText(t, promptMessages, "post compact context", database.ChatMessageVisibilityModel))
	require.False(t, hasMessageText(t, promptMessages, "preserve deployment constraints", database.ChatMessageVisibilityModel))
}

func TestPreCompactHookFailureAbortsCompaction(t *testing.T) {
	t.Parallel()

	fixture := startCompactionHookChat(t,
		func(_ *testing.T, _ database.Store, request agenthooks.Request) (int, string) {
			if request.Type == agenthooks.EventPreCompact {
				return http.StatusInternalServerError, ""
			}
			return http.StatusOK, `{}`
		},
		func(t *testing.T, _ string) {
			require.FailNow(t, "compaction model called after pre_compact failure")
		},
	)
	waitCtx := testutil.Context(t, testutil.WaitLong)
	failed := waitForChatStatus(waitCtx, t, fixture.db, fixture.chat.ID, database.ChatStatusError)
	require.Equal(t, int32(0), fixture.compactionCalls.Load())
	require.False(t, hasCompactionRows(t, fixture.db, fixture.chat.ID))
	require.Equal(t, int32(1), fixture.preCompactCalls.Load())
	require.Zero(t, fixture.postCompactCalls.Load())
	lastError := chatLastErrorMessage(failed.LastError)
	require.Contains(t, lastError, "hook dispatch failed: pre_compact: http_error")
}

func TestPostCompactHookFailureKeepsCompaction(t *testing.T) {
	t.Parallel()

	var postSawCommitted atomic.Bool
	fixture := startCompactionHookChat(t,
		func(t *testing.T, db database.Store, request agenthooks.Request) (int, string) {
			if request.Type == agenthooks.EventPostCompact {
				postSawCommitted.Store(hasCompactionRows(t, db, request.Meta.ChatID))
				return http.StatusInternalServerError, ""
			}
			return http.StatusOK, `{}`
		},
		func(*testing.T, string) {},
	)
	waitCtx := testutil.Context(t, testutil.WaitLong)
	failed := waitForChatStatus(waitCtx, t, fixture.db, fixture.chat.ID, database.ChatStatusError)
	require.False(t, postSawCommitted.Load())
	require.Equal(t, int32(1), fixture.compactionCalls.Load())
	require.True(t, hasCompactionRows(t, fixture.db, fixture.chat.ID))
	require.Equal(t, int32(1), fixture.preCompactCalls.Load())
	require.Equal(t, int32(1), fixture.postCompactCalls.Load())
	lastError := chatLastErrorMessage(failed.LastError)
	require.Contains(t, lastError, "hook dispatch failed: post_compact: http_error")
}

func TestManualCompactionPostCompactEffects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		postCompact    string
		wantFollowUp   bool
		wantVisibleMsg string
	}{
		{
			name:           "user message resumes generation",
			postCompact:    `{"user_message":"compaction complete"}`,
			wantFollowUp:   true,
			wantVisibleMsg: "compaction complete",
		},
		{
			name:         "model context alone finishes the turn",
			postCompact:  `{"model_context":"post compact context"}`,
			wantFollowUp: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			var streamCalls atomic.Int32
			var compactionCalls atomic.Int32
			anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
				body := anthropicRequestBody(t, *req)
				if !req.Stream {
					if strings.Contains(body, "You are performing a context compaction") {
						compactionCalls.Add(1)
						return anthropicCompactionResponse("manual hook compaction summary")
					}
					return chattest.AnthropicNonStreamingResponse("title")
				}
				// Low usage keeps automatic compaction out of the way, so
				// only the manual request can trigger one.
				streamCalls.Add(1)
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
					InputTokens:  10,
					OutputTokens: 5,
				}, "assistant answer")...)
			})
			user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
			model = updateChatModelCompressionThreshold(t, db, model, 100, 70)

			consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request agenthooks.Request
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				body := `{}`
				if request.Type == agenthooks.EventPostCompact {
					body = test.postCompact
				}
				_, err := w.Write([]byte(body))
				require.NoError(t, err)
			}))
			t.Cleanup(consumer.Close)

			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
				cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
			})
			chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello from the user")
			chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
			require.Equal(t, int32(1), streamCalls.Load())

			_, err := server.CompactChat(ctx, chat)
			require.NoError(t, err)
			chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
			require.False(t, chat.LastError.Valid)
			require.Equal(t, int32(1), compactionCalls.Load())

			wantStreams := int32(1)
			if test.wantFollowUp {
				wantStreams = 2
			}
			require.Equal(t, wantStreams, streamCalls.Load())
			if test.wantVisibleMsg != "" {
				messages := chatMessages(ctx, t, db, chat.ID)
				require.True(t, hasMessageText(t, messages, test.wantVisibleMsg, database.ChatMessageVisibilityUser))
			}
		})
	}
}

type compactionHookFixture struct {
	ctx              context.Context
	db               database.Store
	chat             database.Chat
	compactionCalls  *atomic.Int32
	streamCalls      *atomic.Int32
	preCompactCalls  *atomic.Int32
	postCompactCalls *atomic.Int32
}

func startCompactionHookChat(
	t *testing.T,
	hookResponse func(*testing.T, database.Store, agenthooks.Request) (int, string),
	inspectCompaction func(*testing.T, string),
) compactionHookFixture {
	t.Helper()

	const (
		contextLimit     = int64(100)
		thresholdPercent = int32(70)
	)
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var compactionCalls atomic.Int32
	var preCompactCalls atomic.Int32
	var postCompactCalls atomic.Int32
	var streamCalls atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		body := anthropicRequestBody(t, *req)
		if !req.Stream {
			if strings.Contains(body, "You are performing a context compaction") {
				compactionCalls.Add(1)
				inspectCompaction(t, body)
				return anthropicCompactionResponse("hook compaction summary")
			}
			return chattest.AnthropicNonStreamingResponse("title")
		}
		if streamCalls.Add(1) == 1 {
			return highUsageReadFileResponse("/tmp/hook.txt")
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
			InputTokens:  20,
			OutputTokens: 5,
		}, "continued after compaction")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateChatModelCompressionThreshold(t, db, model, contextLimit, thresholdPercent)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		switch request.Type {
		case agenthooks.EventPreCompact:
			preCompactCalls.Add(1)
		case agenthooks.EventPostCompact:
			postCompactCalls.Add(1)
		}
		status, body := hookResponse(t, db, request)
		w.WriteHeader(status)
		if body != "" {
			_, err := w.Write([]byte(body))
			require.NoError(t, err)
		}
	}))
	t.Cleanup(consumer.Close)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/hook.txt", int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{
			Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1\tpackage main",
		}, nil).
		Times(1)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
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
		Title:          "compaction-hooks",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("trigger compaction hooks"),
		},
	})
	require.NoError(t, err)
	return compactionHookFixture{
		ctx:              ctx,
		db:               db,
		chat:             chat,
		compactionCalls:  &compactionCalls,
		streamCalls:      &streamCalls,
		preCompactCalls:  &preCompactCalls,
		postCompactCalls: &postCompactCalls,
	}
}

func hasCompactionRows(t *testing.T, db database.Store, chatID uuid.UUID) bool {
	t.Helper()
	userMessages := chatMessages(t.Context(), t, db, chatID)
	promptMessages, err := db.GetChatMessagesForPromptByChatID(t.Context(), chatID)
	require.NoError(t, err)
	compressed := compressedChatSummarizedMessages(t, append(promptMessages, userMessages...))
	return len(compressed.summaries) > 0 && len(compressed.calls) > 0 && len(compressed.results) > 0
}

func hasMessageText(t *testing.T, messages []database.ChatMessage, text string, visibility database.ChatMessageVisibility) bool {
	t.Helper()
	for _, message := range messages {
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		if message.Visibility == visibility && len(parts) == 1 && parts[0].Text == text {
			return true
		}
	}
	return false
}
