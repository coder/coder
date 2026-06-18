//go:build bench_chat_search

package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

func BenchmarkChatMessageSearchIndex(b *testing.B) {
	ctx := b.Context()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(b)

	createChatMessageSearchTextFunction(ctx, b, sqlDB)

	config := chatMessageSearchBenchConfigFromEnv(b)

	seedStart := time.Now()
	seedChatMessageSearchCorpus(ctx, b, db, config, 0)
	seedBeforeIndex := time.Since(seedStart)
	b.Logf("seed before index: users=%d chats_per_user=%d messages_per_chat=%d total_chats=%d total_messages=%d duration=%s",
		config.Users, config.ChatsPerUser, config.MessagesPerChat, config.TotalChats(), config.TotalMessages(), seedBeforeIndex)

	indexStart := time.Now()
	createChatMessageSearchIndex(ctx, b, sqlDB)
	createIndexDuration := time.Since(indexStart)
	b.Logf("create index duration: %s", createIndexDuration)
	logChatMessageSearchIndexSize(ctx, b, sqlDB, "after create index")

	seedStart = time.Now()
	seedChatMessageSearchCorpus(ctx, b, db, config, config.TotalChats())
	seedAfterIndex := time.Since(seedStart)
	b.Logf("seed after index: users=%d chats_per_user=%d messages_per_chat=%d total_chats=%d total_messages=%d duration=%s",
		config.Users, config.ChatsPerUser, config.MessagesPerChat, config.TotalChats(), config.TotalMessages(), seedAfterIndex)
	logChatMessageSearchIndexSize(ctx, b, sqlDB, "after indexed seed")

	analyzeChatMessageSearchTables(ctx, b, sqlDB)
	runChatMessageSearchSampleQueries(ctx, b, sqlDB)
}

type chatMessageSearchBenchConfig struct {
	Users           int
	ChatsPerUser    int
	MessagesPerChat int
}

func (c chatMessageSearchBenchConfig) TotalChats() int {
	return c.Users * c.ChatsPerUser
}

func (c chatMessageSearchBenchConfig) TotalMessages() int {
	return c.TotalChats() * c.MessagesPerChat
}

func chatMessageSearchBenchConfigFromEnv(t testing.TB) chatMessageSearchBenchConfig {
	t.Helper()

	config := chatMessageSearchBenchConfig{
		Users:           benchEnvInt(t, "CODER_CHAT_SEARCH_BENCH_USERS", 100),
		ChatsPerUser:    benchEnvInt(t, "CODER_CHAT_SEARCH_BENCH_CHATS_PER_USER", 100),
		MessagesPerChat: benchEnvInt(t, "CODER_CHAT_SEARCH_BENCH_MESSAGES_PER_CHAT", 100),
	}
	t.Logf("chat search bench config: users=%d chats_per_user=%d messages_per_chat=%d total_chats=%d total_messages=%d",
		config.Users, config.ChatsPerUser, config.MessagesPerChat, config.TotalChats(), config.TotalMessages())
	return config
}

func benchEnvInt(t testing.TB, name string, fallback int) int {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	require.NoError(t, err, "parse %s", name)
	require.Positive(t, parsed, "%s must be positive", name)
	return parsed
}

func createChatMessageSearchTextFunction(ctx context.Context, t testing.TB, sqlDB *sql.DB) {
	t.Helper()

	_, err := sqlDB.ExecContext(ctx, `
CREATE OR REPLACE FUNCTION chat_message_search_text(content jsonb)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
	SELECT string_agg(part->>'text', ' ' ORDER BY ord)
	FROM jsonb_array_elements(
		CASE WHEN jsonb_typeof(content) = 'array' THEN content ELSE '[]'::jsonb END
	) WITH ORDINALITY AS t(part, ord)
	WHERE part->>'type' = 'text'
$$;
`)
	require.NoError(t, err)
}

func createChatMessageSearchIndex(ctx context.Context, t testing.TB, sqlDB *sql.DB) {
	t.Helper()

	_, err := sqlDB.ExecContext(ctx, `
CREATE INDEX idx_chat_messages_visible_fts
ON chat_messages
USING GIN (to_tsvector('simple', chat_message_search_text(content)))
WHERE deleted = false
  AND visibility IN ('user', 'both');
`)
	require.NoError(t, err)
}

func seedChatMessageSearchCorpus(ctx context.Context, t testing.TB, db database.Store, config chatMessageSearchBenchConfig, chatOffset int) {
	t.Helper()

	faker := gofakeit.New(uint64(1 + chatOffset))
	organization := dbgen.Organization(t, db, database.Organization{})
	provider := dbgen.ChatProvider(t, db, database.ChatProvider{})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Provider: provider.Provider})
	for userIndex := range config.Users {
		owner := dbgen.User(t, db, database.User{})
		for chatIndex := range config.ChatsPerUser {
			absoluteChatIndex := chatOffset + userIndex*config.ChatsPerUser + chatIndex
			chat := dbgen.Chat(t, db, database.Chat{
				OrganizationID:    organization.ID,
				OwnerID:           owner.ID,
				LastModelConfigID: modelConfig.ID,
				Title:             fmt.Sprintf("benchmark chat %d %s", absoluteChatIndex, chatSearchSeedText(faker, absoluteChatIndex)),
			})

			for messageIndex := range config.MessagesPerChat {
				absoluteIndex := absoluteChatIndex*config.MessagesPerChat + messageIndex
				role := database.ChatMessageRoleUser
				if messageIndex%2 == 1 {
					role = database.ChatMessageRoleAssistant
				}

				visibility := database.ChatMessageVisibilityBoth
				if messageIndex%53 == 52 {
					visibility = database.ChatMessageVisibilityModel
				}

				message := dbgen.ChatMessage(t, db, database.ChatMessage{
					ChatID:     chat.ID,
					CreatedBy:  uuid.NullUUID{UUID: chat.OwnerID, Valid: true},
					Role:       role,
					Visibility: visibility,
					Content:    chatSearchTextContent(t, chatSearchSeedText(faker, absoluteIndex)),
				})

				if messageIndex%97 == 96 {
					err := db.SoftDeleteChatMessageByID(ctx, message.ID)
					require.NoError(t, err)
				}
			}
		}
		t.Logf("user: %d chats:%d messages:%d", userIndex, config.ChatsPerUser, config.MessagesPerChat)
	}
}

func chatSearchTextContent(t testing.TB, text string) pqtype.NullRawMessage {
	t.Helper()

	raw, err := json.Marshal([]map[string]string{
		{"type": "text", "text": text},
	})
	require.NoError(t, err)

	return pqtype.NullRawMessage{RawMessage: raw, Valid: true}
}

func chatSearchSeedText(faker *gofakeit.Faker, index int) string {
	base := faker.Paragraph(1, 3, 12, " ")
	switch {
	case index%100 == 0:
		return base + " authentication permission denied oauth callback"
	case index%137 == 0:
		return base + " CODAGT-517 database migration failed"
	case index%251 == 0:
		return base + " workspace timeout provisioner agent disconnected"
	default:
		return base
	}
}

func logChatMessageSearchIndexSize(ctx context.Context, t testing.TB, sqlDB *sql.DB, label string) {
	t.Helper()

	var pretty string
	var bytes int64
	err := sqlDB.QueryRowContext(ctx, `
SELECT
	pg_size_pretty(pg_relation_size('idx_chat_messages_visible_fts')),
	pg_relation_size('idx_chat_messages_visible_fts');
`).Scan(&pretty, &bytes)
	require.NoError(t, err)

	t.Logf("%s: index size=%s bytes=%d", label, pretty, bytes)
}

func analyzeChatMessageSearchTables(ctx context.Context, t testing.TB, sqlDB *sql.DB) {
	t.Helper()

	_, err := sqlDB.ExecContext(ctx, `
ANALYZE chats;
ANALYZE chat_messages;
`)
	require.NoError(t, err)
}

func runChatMessageSearchSampleQueries(ctx context.Context, t testing.TB, sqlDB *sql.DB) {
	t.Helper()

	queries := []string{
		"authentication",
		"permission denied",
		"CODAGT-517",
		"database migration",
		"workspace timeout",
	}

	for _, query := range queries {
		queryStart := time.Now()
		rows, err := sqlDB.QueryContext(ctx, `
WITH search_query AS (
	SELECT websearch_to_tsquery('simple', $1) AS query
)
SELECT
	cm.chat_id,
	MAX(ts_rank(to_tsvector('simple', chat_message_search_text(cm.content)), search_query.query)) AS rank
FROM chat_messages cm
JOIN chats c ON c.id = cm.chat_id
CROSS JOIN search_query
WHERE c.parent_chat_id IS NULL
  AND cm.deleted = false
  AND cm.visibility IN ('user', 'both')
  AND to_tsvector('simple', chat_message_search_text(cm.content)) @@ search_query.query
GROUP BY cm.chat_id
ORDER BY rank DESC
LIMIT 50;
`, query)
		require.NoError(t, err)

		var count int
		for rows.Next() {
			var chatID uuid.UUID
			var rank float32
			require.NoError(t, rows.Scan(&chatID, &rank))
			count++
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())

		t.Logf("query=%q count=%d duration=%s", query, count, time.Since(queryStart))
	}
}
