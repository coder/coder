package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateChatUserPromptSubmitHook(t *testing.T) {
	t.Parallel()

	t.Run("passthrough", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, requests := newCreateHookTestServer(t, db, ps, http.StatusOK, `{}`)

		chat, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "passthrough"))
		require.NoError(t, err)
		request := testutil.RequireReceive(ctx, t, requests)
		require.Equal(t, agenthooks.EventUserPromptSubmit, request.Type)
		require.Equal(t, chat.ID, request.Meta.ChatID)
		require.Equal(t, user.ID, request.Meta.OwnerID)
		require.NotNil(t, request.Meta.TurnID)
		data, err := request.Decode()
		require.NoError(t, err)
		require.Equal(t, &agenthooks.UserPromptSubmitData{Prompt: "passthrough"}, data)

		messages := createHookMessages(ctx, t, db, chat.ID)
		initialUser := messages[len(messages)-1]
		require.Equal(t, database.ChatMessageRoleUser, initialUser.Role)
		require.Equal(t, database.ChatMessageVisibilityBoth, initialUser.Visibility)
		require.Equal(t, "passthrough", hookMessageText(t, initialUser))
		require.Equal(t, uuid.NullUUID{UUID: *request.Meta.TurnID, Valid: true}, initialUser.TurnID)
	})

	t.Run("override", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, requests := newCreateHookTestServer(t, db, ps, http.StatusOK, `{"permission":{"decision":"allow","input_override":{"prompt":"redacted"}}}`)

		chat, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "secret"))
		require.NoError(t, err)
		request := testutil.RequireReceive(ctx, t, requests)
		require.NotNil(t, request.Meta.TurnID)
		messages := createHookMessages(ctx, t, db, chat.ID)
		initialUser := messages[len(messages)-1]
		require.Equal(t, "redacted", hookMessageText(t, initialUser))
		require.Equal(t, uuid.NullUUID{UUID: *request.Meta.TurnID, Valid: true}, initialUser.TurnID)
	})

	t.Run("response messages", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, _ := newCreateHookTestServer(t, db, ps, http.StatusOK, `{"model_context":"model only","user_message":"user only"}`)

		chat, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "prompt"))
		require.NoError(t, err)
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(promptMessages), 2)
		modelContext := promptMessages[len(promptMessages)-2]
		initialUser := promptMessages[len(promptMessages)-1]
		require.Equal(t, database.ChatMessageRoleUser, modelContext.Role)
		require.Equal(t, database.ChatMessageVisibilityModel, modelContext.Visibility)
		require.Equal(t, "model only", hookMessageText(t, modelContext))
		require.Equal(t, database.ChatMessageRoleUser, initialUser.Role)
		require.Equal(t, database.ChatMessageVisibilityBoth, initialUser.Visibility)
		require.Equal(t, "prompt", hookMessageText(t, initialUser))
		require.Equal(t, initialUser.TurnID, modelContext.TurnID)

		userMessages := createHookMessages(ctx, t, db, chat.ID)
		require.GreaterOrEqual(t, len(userMessages), 2)
		userNotice := userMessages[len(userMessages)-2]
		require.Equal(t, database.ChatMessageRoleSystem, userNotice.Role)
		require.Equal(t, database.ChatMessageVisibilityUser, userNotice.Visibility)
		require.Equal(t, "user only", hookMessageText(t, userNotice))
		require.Equal(t, initialUser.TurnID, userNotice.TurnID)
	})

	t.Run("allowed tools", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, _ := newCreateHookTestServer(t, db, ps, http.StatusOK, `{"allowed_tools":["read_file","write_file"]}`)

		chat, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "prompt"))
		require.NoError(t, err)
		require.True(t, chat.HookAllowedTools.Valid)
		var allowedTools []string
		require.NoError(t, json.Unmarshal(chat.HookAllowedTools.RawMessage, &allowedTools))
		require.Equal(t, []string{"read_file", "write_file"}, allowedTools)
	})

	t.Run("deny", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, requests := newCreateHookTestServer(t, db, ps, http.StatusOK, `{"permission":{"decision":"deny"},"user_message":"blocked"}`)

		_, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "prompt"))
		var denied *chatd.UserPromptDeniedError
		require.ErrorAs(t, err, &denied)
		require.Equal(t, "blocked", denied.UserMessage)
		request := testutil.RequireReceive(ctx, t, requests)
		requireCreateHookChatMissing(ctx, t, db, request.Meta.ChatID)
	})

	t.Run("dispatch failure", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, requests := newCreateHookTestServer(t, db, ps, http.StatusInternalServerError, "")

		_, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "prompt"))
		var dispatchErr *chathooks.DispatchError
		require.ErrorAs(t, err, &dispatchErr)
		require.Equal(t, "http_error", dispatchErr.Class)
		request := testutil.RequireReceive(ctx, t, requests)
		requireCreateHookChatMissing(ctx, t, db, request.Meta.ChatID)
	})

	t.Run("end chat", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, requests := newCreateHookTestServer(t, db, ps, http.StatusOK, `{"user_message":"ended","end_chat":true}`)

		_, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "prompt"))
		var denied *chatd.UserPromptDeniedError
		require.ErrorAs(t, err, &denied)
		require.Equal(t, "ended", denied.UserMessage)
		request := testutil.RequireReceive(ctx, t, requests)
		requireCreateHookChatMissing(ctx, t, db, request.Meta.ChatID)
	})
}

func TestCreateChatHooksDisabledUnchanged(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	server := newTestServer(t, db, ps, uuid.New())

	chat, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "unchanged"))
	require.NoError(t, err)
	messages := createHookMessages(ctx, t, db, chat.ID)
	initialUser := messages[len(messages)-1]
	require.False(t, initialUser.TurnID.Valid)
	require.Equal(t, "unchanged", hookMessageText(t, initialUser))
	require.False(t, chat.HookAllowedTools.Valid)
}

func newCreateHookTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	statusCode int,
	response string,
) (*chatd.Server, <-chan agenthooks.Request) {
	t.Helper()
	requests := make(chan agenthooks.Request, 2)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests <- request
		w.WriteHeader(statusCode)
		if response != "" {
			_, err := w.Write([]byte(response))
			require.NoError(t, err)
		}
	}))
	t.Cleanup(consumer.Close)
	return newHookTestServer(t, db, ps, consumer), requests
}

func createHookOptions(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	organizationID uuid.UUID,
	modelConfigID uuid.UUID,
	prompt string,
) chatd.CreateOptions {
	t.Helper()
	return chatd.CreateOptions{
		OrganizationID:     organizationID,
		OwnerID:            userID,
		Title:              "create hook test",
		ModelConfigID:      modelConfigID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)},
		APIKeyID:           testAPIKeyID(t, db, userID),
	}
}

func createHookMessages(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) []database.ChatMessage {
	t.Helper()
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	require.NoError(t, err)
	return messages
}

func requireCreateHookChatMissing(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) {
	t.Helper()
	_, err := db.GetChatByID(ctx, chatID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
