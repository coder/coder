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
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
)

func BenchmarkChatMessageSearchIndex(b *testing.B) {
	ctx := b.Context()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(b)

	createChatMessageSearchTextFunction(ctx, b, sqlDB)

	profiles := chatMessageSearchProfiles(b)
	totalChats, expectedMessages := chatMessageSearchTotals(profiles)
	b.Logf("chat search bench distribution: users=%d total_chats=%d expected_messages=%d",
		len(profiles), totalChats, expectedMessages)

	seedStart := time.Now()
	seededMessages := seedChatMessageSearchCorpus(ctx, b, db, profiles, 0)
	seedBeforeIndex := time.Since(seedStart)
	b.Logf("seed before index: users=%d total_chats=%d seeded_messages=%d duration=%s",
		len(profiles), totalChats, seededMessages, seedBeforeIndex)

	indexStart := time.Now()
	createChatMessageSearchIndex(ctx, b, sqlDB)
	createIndexDuration := time.Since(indexStart)
	b.Logf("create index duration: %s", createIndexDuration)
	logChatMessageSearchIndexSize(ctx, b, sqlDB, "after create index")

	seedStart = time.Now()
	seededMessages = seedChatMessageSearchCorpus(ctx, b, db, profiles, totalChats)
	seedAfterIndex := time.Since(seedStart)
	b.Logf("seed after index: users=%d total_chats=%d seeded_messages=%d duration=%s",
		len(profiles), totalChats, seededMessages, seedAfterIndex)
	logChatMessageSearchIndexSize(ctx, b, sqlDB, "after indexed seed")

	analyzeChatMessageSearchTables(ctx, b, sqlDB)
	runChatMessageSearchSampleQueries(ctx, b, sqlDB)
}

// userChatProfile describes one user's chat volume: how many root chats they
// own and the average number of messages per chat. The default distribution
// mirrors the Coder dev deployment, which is heavily skewed: a few users own
// hundreds of chats and some chats hold thousands of messages, while most users
// own a single short chat.
type userChatProfile struct {
	Chats       int
	AvgMessages int
}

// devChatDistribution is the per-user (root chat count, avg messages per chat)
// observed on the dev deployment. Averages are rounded to whole messages.
var devChatDistribution = []userChatProfile{
	{577, 47}, {465, 326}, {460, 151}, {269, 70}, {261, 31},
	{189, 304}, {183, 114}, {182, 219}, {135, 190}, {121, 191},
	{115, 155}, {113, 342}, {97, 96}, {94, 606}, {93, 100},
	{88, 422}, {74, 270}, {71, 139}, {71, 369}, {68, 85},
	{68, 140}, {64, 199}, {63, 87}, {60, 397}, {60, 202},
	{60, 382}, {57, 321}, {56, 331}, {51, 202}, {49, 249},
	{48, 140}, {47, 38}, {47, 140}, {47, 196}, {44, 414},
	{43, 423}, {43, 130}, {40, 93}, {40, 211}, {38, 244},
	{35, 334}, {34, 423}, {32, 89}, {28, 162}, {27, 86},
	{18, 296}, {18, 211}, {15, 75}, {13, 35}, {13, 67},
	{12, 77}, {11, 75}, {11, 196}, {11, 197}, {10, 4999},
	{10, 763}, {10, 235}, {10, 650}, {9, 71}, {9, 4},
	{7, 139}, {7, 34}, {6, 115}, {6, 122}, {6, 38},
	{6, 16}, {5, 88}, {5, 36}, {5, 134}, {5, 274},
	{5, 29}, {4, 58}, {4, 21}, {4, 186}, {4, 37},
	{4, 816}, {3, 13}, {3, 17}, {3, 12}, {3, 26},
	{3, 382}, {3, 63}, {2, 11}, {2, 189}, {2, 8},
	{2, 5}, {2, 53}, {2, 71}, {2, 27}, {2, 196},
	{2, 65}, {2, 26}, {2, 4}, {2, 127}, {2, 82},
	{2, 7}, {1, 227}, {1, 54}, {1, 204}, {1, 331},
	{1, 67}, {1, 19}, {1, 4}, {1, 18}, {1, 540},
	{1, 387}, {1, 217}, {1, 34}, {1, 20}, {1, 127},
	{1, 47}, {1, 59}, {1, 329}, {1, 4}, {1, 6},
	{1, 10}, {1, 86}, {1, 6}, {1, 6}, {1, 14},
}

// chatMessageSearchProfiles returns the dev distribution, optionally truncated
// to the first N users via CODER_CHAT_SEARCH_BENCH_USER_LIMIT to keep local
// runs short.
func chatMessageSearchProfiles(t testing.TB) []userChatProfile {
	t.Helper()

	profiles := devChatDistribution
	if limit := benchEnvInt(t, "CODER_CHAT_SEARCH_BENCH_USER_LIMIT", 0); limit > 0 && limit < len(profiles) {
		profiles = profiles[:limit]
	}
	return profiles
}

func chatMessageSearchTotals(profiles []userChatProfile) (totalChats, expectedMessages int) {
	for _, profile := range profiles {
		totalChats += profile.Chats
		expectedMessages += profile.Chats * profile.AvgMessages
	}
	return totalChats, expectedMessages
}

func benchEnvInt(t testing.TB, name string, fallback int) int {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	require.NoError(t, err, "parse %s", name)
	require.GreaterOrEqual(t, parsed, 0, "%s must not be negative", name)
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

// jitteredMessageCount returns a per-chat message count drawn uniformly from
// [1, 2*avg-1]. The range is symmetric around avg, so the expected value stays
// exactly avg while individual chats vary in size.
func jitteredMessageCount(faker *gofakeit.Faker, avg int) int {
	if avg <= 1 {
		return 1
	}
	return faker.Number(1, 2*avg-1)
}

// seedChatMessageSearchCorpus seeds the given user distribution and returns the
// actual number of messages inserted, which varies run to run because per-chat
// counts are jittered around each user's average.
func seedChatMessageSearchCorpus(ctx context.Context, t testing.TB, db database.Store, profiles []userChatProfile, chatOffset int) int {
	t.Helper()

	faker := gofakeit.New(uint64(1 + chatOffset))
	organization := dbgen.Organization(t, db, database.Organization{})
	provider := dbgen.ChatProvider(t, db, database.ChatProvider{})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Provider: provider.Provider})
	chatCounter := chatOffset
	seededMessages := 0
	for userIndex, profile := range profiles {
		owner := dbgen.User(t, db, database.User{})
		apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: owner.ID})
		for range profile.Chats {
			chat := dbgen.Chat(t, db, database.Chat{
				OrganizationID:    organization.ID,
				OwnerID:           owner.ID,
				LastModelConfigID: modelConfig.ID,
				Title:             fmt.Sprintf("benchmark chat %d %s", chatCounter, chatSearchSeedText(faker, chatCounter)),
			})

			messageCount := jitteredMessageCount(faker, profile.AvgMessages)
			params := chatMessageBatchParams(faker, chat, owner.ID, apiKey.ID, modelConfig.ID, chatCounter, messageCount)
			_, err := db.InsertChatMessages(ctx, params)
			require.NoError(t, err)
			seededMessages += messageCount
			chatCounter++
		}
		t.Logf("user: %d chats:%d avg_messages:%d", userIndex, profile.Chats, profile.AvgMessages)
	}
	return seededMessages
}

// chatMessageBatchParams builds a single InsertChatMessages call for an entire
// chat, alternating user and assistant roles and reserving a slice of messages
// as model-only to mirror hidden content that the partial index excludes.
func chatMessageBatchParams(faker *gofakeit.Faker, chat database.Chat, ownerID uuid.UUID, apiKeyID string, modelConfigID uuid.UUID, chatIndex, messagesPerChat int) database.InsertChatMessagesParams {
	params := database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           make([]uuid.UUID, messagesPerChat),
		APIKeyID:            make([]string, messagesPerChat),
		ModelConfigID:       make([]uuid.UUID, messagesPerChat),
		Role:                make([]database.ChatMessageRole, messagesPerChat),
		Content:             make([]string, messagesPerChat),
		ContentVersion:      make([]int16, messagesPerChat),
		Visibility:          make([]database.ChatMessageVisibility, messagesPerChat),
		InputTokens:         make([]int64, messagesPerChat),
		OutputTokens:        make([]int64, messagesPerChat),
		TotalTokens:         make([]int64, messagesPerChat),
		ReasoningTokens:     make([]int64, messagesPerChat),
		CacheCreationTokens: make([]int64, messagesPerChat),
		CacheReadTokens:     make([]int64, messagesPerChat),
		ContextLimit:        make([]int64, messagesPerChat),
		Compressed:          make([]bool, messagesPerChat),
		TotalCostMicros:     make([]int64, messagesPerChat),
		RuntimeMs:           make([]int64, messagesPerChat),
		ProviderResponseID:  make([]string, messagesPerChat),
	}

	for messageIndex := range messagesPerChat {
		absoluteIndex := chatIndex*messagesPerChat + messageIndex
		role := database.ChatMessageRoleUser
		createdBy := ownerID
		keyID := apiKeyID
		if messageIndex%2 == 1 {
			role = database.ChatMessageRoleAssistant
			// Assistant turns have no creator or API key.
			createdBy = uuid.Nil
			keyID = ""
		}

		visibility := database.ChatMessageVisibilityBoth
		if messageIndex%53 == 52 {
			visibility = database.ChatMessageVisibilityModel
		}

		params.CreatedBy[messageIndex] = createdBy
		params.APIKeyID[messageIndex] = keyID
		params.ModelConfigID[messageIndex] = modelConfigID
		params.Role[messageIndex] = role
		params.Content[messageIndex] = chatSearchTextContentJSON(chatSearchSeedText(faker, absoluteIndex))
		params.ContentVersion[messageIndex] = chatprompt.CurrentContentVersion
		params.Visibility[messageIndex] = visibility
	}

	return params
}

func chatSearchTextContentJSON(text string) string {
	raw, err := json.Marshal([]map[string]string{
		{"type": "text", "text": text},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
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
