package database_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
)

// searchChatMessageContentSQL is the candidate baseline query for chat message
// search. This spike validates its behaviour directly against PostgreSQL; when
// promoted beyond a spike it becomes a sqlc query (e.g. SearchChatMessageContent)
// with dbmem/dbauthz wiring. It returns the distinct chat IDs of user-visible,
// non-deleted messages whose extracted text matches the full-text query.
const searchChatMessageContentSQL = `
SELECT DISTINCT cm.chat_id
FROM chat_messages cm
WHERE cm.deleted = false
  AND cm.visibility IN ('user', 'both')
  AND cm.content_text IS NOT NULL
  AND to_tsvector('simple', cm.content_text) @@ websearch_to_tsquery('simple', $1)
`

func TestChatMessageSearchSpike(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB), "migrations")
	db := database.New(sqlDB)
	ctx := context.Background()

	// Shared fixtures. We intentionally do not disable triggers, since the
	// content_text trigger under test must run on insert.
	u := dbgen.User(t, db, database.User{})
	o := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID, OrganizationID: o.ID})
	p := dbgen.ChatProvider(t, db, database.ChatProvider{})
	m := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Provider: p.Provider})

	newChat := func(t *testing.T) database.Chat {
		t.Helper()
		return dbgen.Chat(t, db, database.Chat{
			OwnerID:           u.ID,
			OrganizationID:    o.ID,
			LastModelConfigID: m.ID,
		})
	}

	addMessage := func(t *testing.T, chatID uuid.UUID, role database.ChatMessageRole, vis database.ChatMessageVisibility, contentJSON string) database.ChatMessage {
		t.Helper()
		return dbgen.ChatMessage(t, db, database.ChatMessage{
			ChatID:     chatID,
			Role:       role,
			Visibility: vis,
			Content:    pqtype.NullRawMessage{RawMessage: json.RawMessage(contentJSON), Valid: true},
		})
	}

	// textPart builds a single-text-part content array.
	textPart := func(text string) string {
		return `[{"type":"text","text":"` + text + `"}]`
	}

	search := func(t *testing.T, query string) []uuid.UUID {
		t.Helper()
		rows, err := sqlDB.QueryContext(ctx, searchChatMessageContentSQL, query)
		require.NoError(t, err)
		defer rows.Close()
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		require.NoError(t, rows.Err())
		return ids
	}

	contains := func(ids []uuid.UUID, want uuid.UUID) bool {
		for _, id := range ids {
			if id == want {
				return true
			}
		}
		return false
	}

	t.Run("BasicMatchAndNoMatch", func(t *testing.T) {
		chat := newChat(t)
		addMessage(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
			textPart("we discussed oauthcase token refresh"))
		require.True(t, contains(search(t, "oauthcase"), chat.ID))
		require.NotContains(t, search(t, "kubernetescase"), chat.ID)
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		chat := newChat(t)
		addMessage(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
			textPart("lowercase oauthmixed token"))
		require.True(t, contains(search(t, "OAUTHMIXED"), chat.ID))
	})

	t.Run("MultiWordAnded", func(t *testing.T) {
		chat := newChat(t)
		addMessage(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
			textPart("tokenword refreshword flow diagram"))
		require.True(t, contains(search(t, "tokenword refreshword"), chat.ID))
		require.False(t, contains(search(t, "tokenword missingword"), chat.ID))
	})

	t.Run("QuotedPhrase", func(t *testing.T) {
		chat := newChat(t)
		addMessage(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
			textPart("please refreshword the tokenword now"))
		require.True(t, contains(search(t, `"refreshword the tokenword"`), chat.ID))
		require.False(t, contains(search(t, `"tokenword refreshword"`), chat.ID))
	})

	t.Run("TextPartsOnly", func(t *testing.T) {
		chat := newChat(t)
		addMessage(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
			`[{"type":"text","text":"alphaword"},{"type":"reasoning","text":"betamaxword"}]`)
		require.True(t, contains(search(t, "alphaword"), chat.ID), "sanity: text part is indexed")
		require.False(t, contains(search(t, "betamaxword"), chat.ID), "non-text part must be ignored")
	})

	t.Run("VisibilityModelExcluded", func(t *testing.T) {
		modelChat := newChat(t)
		addMessage(t, modelChat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel,
			textPart("secretword hidden"))
		require.False(t, contains(search(t, "secretword"), modelChat.ID))

		userChat := newChat(t)
		addMessage(t, userChat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityUser,
			textPart("useronlyword visible"))
		require.True(t, contains(search(t, "useronlyword"), userChat.ID))
	})

	t.Run("DeletedExcluded", func(t *testing.T) {
		chat := newChat(t)
		msg := addMessage(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
			textPart("gonesoonword"))
		// Mark deleted directly so the content trigger (which only fires on
		// INSERT or UPDATE OF content) does not refire.
		_, err := sqlDB.ExecContext(ctx, "UPDATE chat_messages SET deleted = true WHERE id = $1", msg.ID)
		require.NoError(t, err)
		require.False(t, contains(search(t, "gonesoonword"), chat.ID))
	})

	t.Run("AgentMessageMatched", func(t *testing.T) {
		chat := newChat(t)
		addMessage(t, chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth,
			textPart("assistantonlyword reply"))
		require.True(t, contains(search(t, "assistantonlyword"), chat.ID))
	})

	t.Run("LegacyNonArrayContent", func(t *testing.T) {
		chat := newChat(t)
		// Pre-000434 rows stored content as a scalar JSON string. The trigger
		// must not error on these; content_text stays NULL and never matches.
		_, err := sqlDB.ExecContext(ctx,
			`INSERT INTO chat_messages (chat_id, role, content, visibility, content_version)
			 VALUES ($1, 'user', '"legacyword"'::jsonb, 'both', 0)`, chat.ID)
		require.NoError(t, err, "legacy scalar content must insert without error")
		require.False(t, contains(search(t, "legacyword"), chat.ID))
	})
}
