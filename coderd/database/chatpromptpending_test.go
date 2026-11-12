package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

type promptHooksRow struct {
	Deleted bool
	Pending bool
	Content string
	Version int16
	HasTsv  bool
}

func readPromptHooksRow(ctx context.Context, t *testing.T, sqlDB *sql.DB, id int64) promptHooksRow {
	t.Helper()
	var row promptHooksRow
	err := sqlDB.QueryRowContext(ctx,
		`SELECT deleted, prompt_hooks_pending, content::text, content_version, search_tsv IS NOT NULL
		FROM chat_messages WHERE id = $1`, id,
	).Scan(&row.Deleted, &row.Pending, &row.Content, &row.Version, &row.HasTsv)
	require.NoError(t, err)
	return row
}

func TestChatPromptHooksPendingQueries(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID})
	newChat := func(t *testing.T) database.Chat {
		t.Helper()
		return dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: model.ID})
	}
	newMessage := func(t *testing.T, chatID uuid.UUID, pending bool) database.ChatMessage {
		t.Helper()
		return dbgen.ChatMessage(t, db, database.ChatMessage{
			ChatID:             chatID,
			CreatedBy:          uuid.NullUUID{UUID: user.ID, Valid: true},
			Content:            pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"original"}]`), Valid: true},
			PromptHooksPending: pending,
		})
	}

	t.Run("InsertCarriesTheFlagPerRow", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := newChat(t)
		params := database.InsertChatMessagesParams{
			ChatID:         chat.ID,
			CreatedBy:      []uuid.UUID{user.ID, user.ID},
			ModelConfigID:  []uuid.UUID{model.ID, model.ID},
			Role:           []database.ChatMessageRole{database.ChatMessageRoleUser, database.ChatMessageRoleUser},
			Content:        []string{`[]`, `[]`},
			ContentVersion: []int16{1, 1},
			Visibility:     []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth, database.ChatMessageVisibilityBoth},
		}
		fillBatchColumns(&params)

		withoutArray, err := db.InsertChatMessages(ctx, params)
		require.NoError(t, err)
		require.Len(t, withoutArray, 2)
		require.False(t, withoutArray[0].PromptHooksPending)
		require.False(t, withoutArray[1].PromptHooksPending)

		params.PromptHooksPending = []bool{true, false}
		withArray, err := db.InsertChatMessages(ctx, params)
		require.NoError(t, err)
		require.Len(t, withArray, 2)
		require.True(t, withArray[0].PromptHooksPending)
		require.False(t, withArray[1].PromptHooksPending)
	})

	t.Run("Resolve", func(t *testing.T) {
		t.Parallel()
		newContent := pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"rewritten"}]`), Valid: true}
		tests := []struct {
			name string
			arg  database.ResolveChatMessagePromptHooksParams
			want promptHooksRow
		}{
			{
				name: "Allow",
				want: promptHooksRow{Content: `[{"text": "original", "type": "text"}]`, Version: 1, HasTsv: true},
			},
			{
				name: "Rewrite",
				arg:  database.ResolveChatMessagePromptHooksParams{Content: newContent, ContentVersion: sql.NullInt16{Int16: 2, Valid: true}},
				want: promptHooksRow{Content: `[{"text": "rewritten", "type": "text"}]`, Version: 2},
			},
			{
				name: "Deny",
				arg:  database.ResolveChatMessagePromptHooksParams{Deleted: true},
				want: promptHooksRow{Deleted: true, Content: `[{"text": "original", "type": "text"}]`, Version: 1, HasTsv: true},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				chat := newChat(t)
				message := newMessage(t, chat.ID, true)
				_, err := sqlDB.ExecContext(ctx,
					`UPDATE chat_messages SET search_tsv = to_tsvector('english', 'original'), search_tsv_config = 'english' WHERE id = $1`,
					message.ID)
				require.NoError(t, err)

				arg := tt.arg
				arg.ID, arg.ChatID = message.ID, chat.ID
				resolved, err := db.ResolveChatMessagePromptHooks(ctx, arg)
				require.NoError(t, err)
				require.False(t, resolved.PromptHooksPending)
				require.Equal(t, tt.want.Deleted, resolved.Deleted)

				got := readPromptHooksRow(ctx, t, sqlDB, message.ID)
				require.JSONEq(t, tt.want.Content, got.Content)
				got.Content, tt.want.Content = "", ""
				require.Equal(t, tt.want, got)
			})
		}

		// The row matches only while it is flagged, undeleted and in the chat.
		t.Run("MatchesNothing", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			chat, other := newChat(t), newChat(t)
			resolved := newMessage(t, chat.ID, true)
			plain := newMessage(t, chat.ID, false)
			deleted := newMessage(t, chat.ID, true)
			_, err := sqlDB.ExecContext(ctx, `UPDATE chat_messages SET deleted = true WHERE id = $1`, deleted.ID)
			require.NoError(t, err)
			_, err = db.ResolveChatMessagePromptHooks(ctx, database.ResolveChatMessagePromptHooksParams{ID: resolved.ID, ChatID: chat.ID})
			require.NoError(t, err)

			for name, arg := range map[string]database.ResolveChatMessagePromptHooksParams{
				"AlreadyResolved": {ID: resolved.ID, ChatID: chat.ID},
				"NeverFlagged":    {ID: plain.ID, ChatID: chat.ID},
				"Deleted":         {ID: deleted.ID, ChatID: chat.ID},
				"OtherChat":       {ID: newMessage(t, chat.ID, true).ID, ChatID: other.ID},
			} {
				_, err := db.ResolveChatMessagePromptHooks(ctx, arg)
				require.ErrorIs(t, err, sql.ErrNoRows, name)
			}
		})
	})

	t.Run("SoftDeleteClearsTheFlag", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := newChat(t)
		first, second, third := newMessage(t, chat.ID, true), newMessage(t, chat.ID, true), newMessage(t, chat.ID, true)

		require.NoError(t, db.SoftDeleteChatMessageByID(ctx, first.ID))
		require.NoError(t, db.SoftDeleteChatMessagesAfterID(ctx, database.SoftDeleteChatMessagesAfterIDParams{ChatID: chat.ID, AfterID: second.ID - 1}))
		for _, id := range []int64{first.ID, second.ID, third.ID} {
			row := readPromptHooksRow(ctx, t, sqlDB, id)
			require.True(t, row.Deleted)
			require.False(t, row.Pending)
		}
	})

	t.Run("ClearChatMessages", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, other := newChat(t), newChat(t)
		mine := []database.ChatMessage{newMessage(t, chat.ID, true), newMessage(t, chat.ID, true)}
		theirs := newMessage(t, other.ID, true)

		require.NoError(t, db.ClearChatMessagesPromptHooks(ctx, chat.ID))
		for _, message := range mine {
			require.False(t, readPromptHooksRow(ctx, t, sqlDB, message.ID).Pending)
		}
		require.True(t, readPromptHooksRow(ctx, t, sqlDB, theirs.ID).Pending)
	})

	t.Run("QueuedMessage", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := newChat(t)
		queued, err := db.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
			ChatID:             chat.ID,
			Content:            json.RawMessage(`[{"type":"text","text":"queued"}]`),
			CreatedBy:          user.ID,
			BusyBehavior:       database.ChatBusyBehaviorQueue,
			PromptHooksPending: true,
		})
		require.NoError(t, err)
		require.True(t, queued.PromptHooksPending)

		// An edit runs the hooks on the new content, so it clears the flag.
		edited, err := db.UpdateChatQueuedMessageContent(ctx, database.UpdateChatQueuedMessageContentParams{
			ID:      queued.ID,
			ChatID:  chat.ID,
			Content: json.RawMessage(`[{"type":"text","text":"edited"}]`),
		})
		require.NoError(t, err)
		require.False(t, edited.PromptHooksPending)
	})
}

// fillBatchColumns sets the parallel arrays of an insert that these tests
// leave empty to one zero value per row.
func fillBatchColumns(params *database.InsertChatMessagesParams) {
	n := len(params.Role)
	params.ReasoningEffort = make([]string, n)
	params.InputTokens = make([]int64, n)
	params.OutputTokens = make([]int64, n)
	params.TotalTokens = make([]int64, n)
	params.ReasoningTokens = make([]int64, n)
	params.CacheCreationTokens = make([]int64, n)
	params.CacheReadTokens = make([]int64, n)
	params.ContextLimit = make([]int64, n)
	params.Compressed = make([]bool, n)
	params.RuntimeMs = make([]int64, n)
	params.ProviderResponseID = make([]string, n)
	params.QueuedMessageID = make([]int64, n)
}
