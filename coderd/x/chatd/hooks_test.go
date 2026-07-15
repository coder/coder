package chatd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

func TestSendMessageUserPromptSubmitHook(t *testing.T) {
	t.Parallel()

	t.Run("override", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
		})

		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request agenthooks.Request
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			data, err := request.Decode()
			require.NoError(t, err)
			require.Equal(t, &agenthooks.UserPromptSubmitData{Prompt: "before"}, data)
			require.NotNil(t, request.Meta.TurnID)
			_, err = w.Write([]byte(`{"permission":{"decision":"allow","input_override":{"prompt":"after"}}}`))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)

		server := newHookTestServer(t, db, ps, consumer)
		result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:    chat.ID,
			CreatedBy: user.ID,
			Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("before")},
			APIKeyID:  testAPIKeyID(t, db, user.ID),
		})
		require.NoError(t, err)
		require.True(t, result.Message.TurnID.Valid)
		parts, err := chatprompt.ParseContent(result.Message)
		require.NoError(t, err)
		require.Equal(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("after")}, parts)
	})

	t.Run("deny", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
		})

		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(`{"permission":{"decision":"deny"},"user_message":"blocked"}`))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)

		server := newHookTestServer(t, db, ps, consumer)
		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:    chat.ID,
			CreatedBy: user.ID,
			Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("blocked prompt")},
			APIKeyID:  testAPIKeyID(t, db, user.ID),
		})
		var denied *chatd.UserPromptDeniedError
		require.ErrorAs(t, err, &denied)
		require.Equal(t, "blocked", denied.UserMessage)

		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
		require.NoError(t, err)
		require.Empty(t, messages)
	})
}

func newHookTestServer(t *testing.T, db database.Store, ps dbpubsub.Pubsub, consumer *httptest.Server) *chatd.Server {
	t.Helper()
	dispatcher := chathooks.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		db,
		consumer.Client(),
		consumer.URL,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
	return newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.HookDispatcher = dispatcher
	})
}
