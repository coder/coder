package chatd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

// promptHooksTurn is a deployment with a fake model and a hook consumer. The
// consumer answers every user_prompt_submit with reply, which a test can
// change while the chat is idle.
type promptHooksTurn struct {
	db     database.Store
	server *chatd.Server
	user   database.User
	org    database.Organization
	model  database.ChatModelConfig

	mu       sync.Mutex
	reply    string
	prompts  []string
	modelIn  [][]chattest.OpenAIMessage
	streamed atomic.Int32
}

func newPromptHooksTurn(t *testing.T, reply string, overrides ...func(*chatd.Config)) *promptHooksTurn {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	h := &promptHooksTurn{db: db, reply: reply}
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		h.mu.Lock()
		h.modelIn = append(h.modelIn, append([]chattest.OpenAIMessage(nil), req.Messages...))
		h.mu.Unlock()
		h.streamed.Add(1)
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	h.user, h.org, h.model = seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.mu.Lock()
		reply := h.reply
		if request.Type == agenthooks.EventUserPromptSubmit {
			h.prompts = append(h.prompts, decodeHookData[agenthooks.UserPromptSubmitData](t, request).Prompt)
		}
		h.mu.Unlock()
		if request.Type != agenthooks.EventUserPromptSubmit {
			reply = `{}`
		}
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(consumer.Close)
	h.server = newActiveTestServer(t, db, ps, append([]func(*chatd.Config){func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	}}, overrides...)...)
	return h
}

func (h *promptHooksTurn) setReply(reply string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reply = reply
}

func (h *promptHooksTurn) promptsSeen() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.prompts...)
}

// modelSaw reports whether any model request carried text in a user message.
func (h *promptHooksTurn) modelSaw(text string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, messages := range h.modelIn {
		for _, message := range messages {
			if message.Role == "user" && strings.Contains(message.Content, text) {
				return true
			}
		}
	}
	return false
}

func (h *promptHooksTurn) newChat(t *testing.T, status database.ChatStatus) database.Chat {
	t.Helper()
	return dbgen.Chat(t, h.db, database.Chat{
		OrganizationID:    h.org.ID,
		OwnerID:           h.user.ID,
		LastModelConfigID: h.model.ID,
		Status:            status,
		TitleSource:       database.ChatTitleSourceUser,
	})
}

func (h *promptHooksTurn) addMessage(t *testing.T, chatID uuid.UUID, text string, pending bool) database.ChatMessage {
	t.Helper()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)
	return dbgen.ChatMessage(t, h.db, database.ChatMessage{
		ChatID:             chatID,
		CreatedBy:          uuid.NullUUID{UUID: h.user.ID, Valid: true},
		ModelConfigID:      uuid.NullUUID{UUID: h.model.ID, Valid: true},
		Content:            content,
		PromptHooksPending: pending,
	})
}

func (h *promptHooksTurn) messageText(ctx context.Context, t *testing.T, id int64) (string, database.ChatMessage) {
	t.Helper()
	message, err := h.db.GetChatMessageByIDForStream(ctx, id)
	require.NoError(t, err)
	parts, err := chatprompt.ParseContent(message)
	require.NoError(t, err)
	var text strings.Builder
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText {
			text.WriteString(part.Text)
		}
	}
	return text.String(), message
}

// TestPromptHooksPendingTurn starts a turn from a message with
// prompt_hooks_pending set, with no writer of the flag but the test.
func TestPromptHooksPendingTurn(t *testing.T) {
	t.Parallel()

	const (
		rewrite = `{"permission":{"decision":"allow","input_override":{"prompt":"rewritten"}}}`
		deny    = `{"permission":{"decision":"deny"},"user_message":"blocked by policy"}`
	)
	tests := []struct {
		name       string
		reply      string
		wantText   string
		wantDenied bool
	}{
		{name: "Allow", reply: `{}`, wantText: "flagged"},
		{name: "Rewrite", reply: rewrite, wantText: "rewritten"},
		{name: "Deny", reply: deny, wantText: "flagged", wantDenied: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitSuperLong)
			h := newPromptHooksTurn(t, tt.reply)
			// The message is seeded before any worker can acquire the chat, as
			// a running chat that nobody owns yet.
			chat := h.newChat(t, database.ChatStatusWaiting)
			flagged := h.addMessage(t, chat.ID, "flagged", true)
			_, err := h.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{ID: chat.ID, Status: database.ChatStatusRunning})
			require.NoError(t, err)

			if tt.wantDenied {
				failed := waitForChatStatus(ctx, t, h.db, chat.ID, database.ChatStatusError)
				var chatErr codersdk.ChatError
				require.NoError(t, json.Unmarshal(failed.LastError.RawMessage, &chatErr))
				require.Equal(t, codersdk.ChatErrorKindHookDenied, chatErr.Kind)
				require.Equal(t, "blocked by policy", chatErr.Message)
				require.Zero(t, h.streamed.Load(), "the model must not see a denied message")
				require.Empty(t, chatMessages(ctx, t, h.db, chat.ID))

				// The chat takes the next message.
				h.setReply(`{}`)
				_, err := h.server.SendMessage(ctx, chatd.SendMessageOptions{
					ChatID:  chat.ID,
					Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("next")},
				})
				require.NoError(t, err)
				waitForChatStatus(ctx, t, h.db, chat.ID, database.ChatStatusWaiting)
				require.False(t, h.modelSaw("flagged"))
				require.True(t, h.modelSaw("next"))
				return
			}

			waitForChatStatus(ctx, t, h.db, chat.ID, database.ChatStatusWaiting)
			require.Equal(t, []string{"flagged"}, h.promptsSeen())
			require.True(t, h.modelSaw(tt.wantText))
			text, message := h.messageText(ctx, t, flagged.ID)
			require.Equal(t, tt.wantText, text)
			require.False(t, message.PromptHooksPending)
			require.False(t, message.Deleted)
		})
	}
}

// TestPromptHooksPendingQueuedMessage promotes a queued message that carries
// the flag, and runs the hooks when its turn starts.
func TestPromptHooksPendingQueuedMessage(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	h := newPromptHooksTurn(t, `{"permission":{"decision":"allow","input_override":{"prompt":"replaced"}}}`)
	chat := h.newChat(t, database.ChatStatusWaiting)
	h.addMessage(t, chat.ID, "first", false)
	queued, err := h.db.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:             chat.ID,
		Content:            json.RawMessage(`[{"type":"text","text":"queued"}]`),
		CreatedBy:          h.user.ID,
		BusyBehavior:       database.ChatBusyBehaviorQueue,
		PromptHooksPending: true,
	})
	require.NoError(t, err)
	_, err = h.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{ID: chat.ID, Status: database.ChatStatusRunning})
	require.NoError(t, err)

	waitForChatStatus(ctx, t, h.db, chat.ID, database.ChatStatusWaiting)
	// The seeded first message ran no hook, and the promoted one ran it once.
	require.Equal(t, []string{"queued"}, h.promptsSeen())
	require.True(t, h.modelSaw("replaced"))
	require.False(t, h.modelSaw("queued"))
	promoted := chatMessages(ctx, t, h.db, chat.ID)
	var found bool
	for _, message := range promoted {
		if message.QueuedMessageID.Valid && message.QueuedMessageID.Int64 == queued.ID {
			found = true
			require.False(t, message.PromptHooksPending)
		}
	}
	require.True(t, found, "the queued message must reach history")
}

// TestUserPromptSubmitRunsOncePerHumanMessage checks that a message without
// the flag runs its hooks when it is sent, and not again when its turn starts.
func TestUserPromptSubmitRunsOncePerHumanMessage(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	h := newPromptHooksTurn(t, `{}`)
	chat := h.newChat(t, database.ChatStatusWaiting)

	_, err := h.server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("human")},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, h.db, chat.ID, database.ChatStatusWaiting)

	require.True(t, h.modelSaw("human"))
	require.Equal(t, []string{"human"}, h.promptsSeen())
}
