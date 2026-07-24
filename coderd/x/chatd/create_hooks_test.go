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
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
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
		data := decodeHookData[agenthooks.UserPromptSubmitData](t, request)
		require.Equal(t, "passthrough", data.Prompt)
		var hookParts []codersdk.ChatMessagePart
		require.NoError(t, json.Unmarshal(data.Parts, &hookParts))
		require.Equal(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("passthrough")}, hookParts)

		messages := chatMessages(ctx, t, db, chat.ID)
		initialUser := messages[len(messages)-1]
		require.Equal(t, database.ChatMessageRoleUser, initialUser.Role)
		require.Equal(t, database.ChatMessageVisibilityBoth, initialUser.Visibility)
		require.Equal(t, "passthrough", hookMessageText(t, initialUser))
	})

	t.Run("override", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, requests := newCreateHookTestServer(t, db, ps, http.StatusOK, `{"permission":{"decision":"allow","input_override":{"prompt":"redacted"}}}`)

		opts := createHookOptions(t, db, user.ID, org.ID, model.ID, "secret")
		opts.Title = chatprompt.FallbackTitle(chatprompt.TitleText(opts.InitialUserContent, nil))
		opts.TitleDerivedFromContent = true
		chat, err := server.CreateChat(ctx, opts)
		require.NoError(t, err)
		request := testutil.RequireReceive(ctx, t, requests)
		require.NotNil(t, request.Meta.TurnID)
		messages := chatMessages(ctx, t, db, chat.ID)
		initialUser := messages[len(messages)-1]
		require.Equal(t, "redacted", hookMessageText(t, initialUser))
		require.Equal(t, "redacted", chat.Title, "prompt-derived title must be recomputed from the override")
	})

	t.Run("override keeps explicit title", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, _ := newCreateHookTestServer(t, db, ps, http.StatusOK, `{"permission":{"decision":"allow","input_override":{"prompt":"redacted"}}}`)

		chat, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "secret"))
		require.NoError(t, err)
		require.Equal(t, "create hook test", chat.Title)
	})

	t.Run("invalid model config rejected before dispatch", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, requests := newCreateHookTestServer(t, db, ps, http.StatusOK, `{}`)

		opts := createHookOptions(t, db, user.ID, org.ID, model.ID, "prompt")
		opts.ModelConfigID = uuid.New()
		_, err := server.CreateChat(ctx, opts)
		require.ErrorIs(t, err, chatd.ErrInvalidModelConfigID)
		select {
		case request := <-requests:
			t.Fatalf("unexpected hook dispatch %s for rejected create", request.Type)
		default:
		}
	})

	t.Run("override recomputes paste-derived title", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, _ := newCreateHookTestServer(t, db, ps, http.StatusOK, `{"permission":{"decision":"allow","input_override":{"prompt":"redacted"}}}`)

		opts := createHookOptions(t, db, user.ID, org.ID, model.ID, "   ")
		opts.Title = chatprompt.FallbackTitle("secret paste content")
		opts.TitleDerivedFromContent = true
		chat, err := server.CreateChat(ctx, opts)
		require.NoError(t, err)
		require.Equal(t, "redacted", chat.Title,
			"paste-derived title must be recomputed from the override")
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
		require.NotEmpty(t, promptMessages)
		initialUser := promptMessages[len(promptMessages)-1]
		require.Equal(t, database.ChatMessageRoleUser, initialUser.Role)
		require.Equal(t, database.ChatMessageVisibilityBoth, initialUser.Visibility)
		parts, err := chatprompt.ParseContent(initialUser)
		require.NoError(t, err)
		require.Equal(t, []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("prompt"),
			{Type: codersdk.ChatMessagePartTypeHookContext, Text: "model only"},
			{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "user only"},
		}, parts)
	})

	t.Run("deny", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server, requests := newCreateHookTestServer(t, db, ps, http.StatusOK, `{"permission":{"decision":"deny"},"user_message":"blocked"}`)

		_, err := server.CreateChat(ctx, createHookOptions(t, db, user.ID, org.ID, model.ID, "prompt"))
		var denied *chathooks.UserPromptDeniedError
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
		var dispatchErr *dispatch.Error
		require.ErrorAs(t, err, &dispatchErr)
		require.Equal(t, dispatch.ResultHTTPError, dispatchErr.Class)
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
	messages := chatMessages(ctx, t, db, chat.ID)
	initialUser := messages[len(messages)-1]
	require.Equal(t, "unchanged", hookMessageText(t, initialUser))
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
	}
}

func requireCreateHookChatMissing(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) {
	t.Helper()
	_, err := db.GetChatByID(ctx, chatID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
