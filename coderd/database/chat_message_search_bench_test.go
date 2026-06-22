//go:build bench_chat_search

/*
Quick and dirty benchmark for native PostgreSQL full-text search applied to chat_messages.
To run:

    go test -tags bench_chat_search -run=^$ -bench=BenchmarkChatMessageSearchIndex -benchtime=1x -v ./coderd/database

WARNING: VIBES ABOUND

Ranking with expression (not stored)
chat_message_search_bench_test.go:39: seed before index: users=120 total_chats=5240 seeded_messages=1045484 duration=52.576159285s
    chat_message_search_bench_test.go:45: create index duration: 18.122563581s
    chat_message_search_bench_test.go:46: after create index: index size=41 MB bytes=43147264
    chat_message_search_bench_test.go:51: seed after index: users=120 total_chats=5240 seeded_messages=1063192 duration=1m33.113173961s
    chat_message_search_bench_test.go:53: after indexed seed: index size=95 MB bytes=99704832
    chat_message_search_bench_test.go:56: query="authentication" count=50 duration=436.506558ms
    chat_message_search_bench_test.go:56: query="permission denied" count=50 duration=448.813666ms
    chat_message_search_bench_test.go:56: query="CODAGT-517" count=50 duration=316.140652ms
    chat_message_search_bench_test.go:56: query="database migration" count=50 duration=271.755078ms
    chat_message_search_bench_test.go:56: query="workspace timeout" count=50 duration=170.23749ms

Ranking with stored tsvector
    chat_message_search_bench_test.go:39: seed before index: users=120 total_chats=5240 seeded_messages=1045484 duration=1m9.879143913s
    chat_message_search_bench_test.go:45: create index duration: 35.733918554s
	chat_message_search_bench_test.go:46: after create index: index size=87 MB bytes=91373568
    chat_message_search_bench_test.go:51: seed after index: users=120 total_chats=5240 seeded_messages=1063192 duration=1m21.926884633s
    chat_message_search_bench_test.go:53: after indexed seed: index size=141 MB bytes=147734528
    chat_message_search_bench_test.go:56: query="authentication" count=50 duration=120.732181ms
    chat_message_search_bench_test.go:56: query="permission denied" count=50 duration=128.017026ms
    chat_message_search_bench_test.go:56: query="CODAGT-517" count=50 duration=83.092696ms
    chat_message_search_bench_test.go:56: query="database migration" count=50 duration=41.964317ms
    chat_message_search_bench_test.go:56: query="workspace timeout" count=50 duration=69.882221ms

Ranking with stored tsvector and NULL for non-searchable rows
    chat_message_search_bench_test.go:67: seed before index: users=120 total_chats=5240 seeded_messages=1045484 duration=1m10.601606116s
    chat_message_search_bench_test.go:73: create index duration: 35.465990465s
    chat_message_search_bench_test.go:74: after create index: index size=87 MB bytes=91258880
    chat_message_search_bench_test.go:79: seed after index: users=120 total_chats=5240 seeded_messages=1063192 duration=1m20.947277757s
    chat_message_search_bench_test.go:81: after indexed seed: index size=138 MB bytes=144801792
    chat_message_search_bench_test.go:84: query="authentication" count=50 duration=160.727851ms
    chat_message_search_bench_test.go:84: query="permission denied" count=50 duration=147.165677ms
    chat_message_search_bench_test.go:84: query="CODAGT-517" count=50 duration=107.772913ms
    chat_message_search_bench_test.go:84: query="database migration" count=50 duration=120.116589ms
    chat_message_search_bench_test.go:84: query="workspace timeout" count=50 duration=61.535032ms

With adjusted dataset to better match dev.coder.com:
    chat_message_search_bench_test.go:72: chat search bench distribution: users=120 total_chats=5240 expected_messages=1043599
    chat_message_search_bench_test.go:78: seed before index: users=120 total_chats=5240 seeded_messages=1048980 duration=1m8.07731601s
    chat_message_search_bench_test.go:84: create index duration: 35.943917994s
    chat_message_search_bench_test.go:85: after create index: index size=75 MB bytes=78225408
    chat_message_search_bench_test.go:90: seed after index: users=120 total_chats=5240 seeded_messages=1033895 duration=1m12.465452216s
    chat_message_search_bench_test.go:92: after indexed seed: index size=117 MB bytes=122839040
    chat_message_search_bench_test.go:95: query="authentication" count=50 duration=39.924913ms
    chat_message_search_bench_test.go:95: query="permission denied" count=50 duration=15.35056ms
    chat_message_search_bench_test.go:95: query="CODAGT-517" count=50 duration=26.511268ms
    chat_message_search_bench_test.go:95: query="database migration" count=50 duration=10.529819ms
    chat_message_search_bench_test.go:95: query="workspace timeout" count=50 duration=12.889135ms
*/

package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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

	createChatMessageSearchSchema(ctx, b, sqlDB)

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
// observed on the dev deployment. Averages are rounded to whole messages. The
// message generator below applies the observed root-chat role, visibility, and
// text-bearing mix separately, so these averages represent all root messages,
// not only indexed messages.
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

func createChatMessageSearchSchema(ctx context.Context, t testing.TB, sqlDB *sql.DB) {
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

ALTER TABLE chat_messages
	ADD COLUMN search_tsv tsvector;

CREATE OR REPLACE FUNCTION chat_message_search_tsv_update()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF NEW.deleted = false AND NEW.visibility IN ('user', 'both') THEN
		NEW.search_tsv := to_tsvector('simple', chat_message_search_text(NEW.content));
	ELSE
		NEW.search_tsv := NULL;
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER chat_message_search_tsv_update
BEFORE INSERT OR UPDATE OF content, deleted, visibility
ON chat_messages
FOR EACH ROW
EXECUTE FUNCTION chat_message_search_tsv_update();
`)
	require.NoError(t, err)
}

func createChatMessageSearchIndex(ctx context.Context, t testing.TB, sqlDB *sql.DB) {
	t.Helper()

	_, err := sqlDB.ExecContext(ctx, `
UPDATE chat_messages
SET search_tsv = CASE
	WHEN deleted = false AND visibility IN ('user', 'both') THEN
		to_tsvector('simple', chat_message_search_text(content))
	ELSE NULL
END;

CREATE INDEX idx_chat_messages_visible_fts
ON chat_messages
USING GIN (search_tsv)
WHERE search_tsv IS NOT NULL;
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
	for _, profile := range profiles {
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
		// t.Logf("user: %d chats:%d avg_messages:%d", userIndex, profile.Chats, profile.AvgMessages)
	}
	return seededMessages
}

// chatMessageBatchParams builds a single InsertChatMessages call for an entire
// chat. The role and content mix mirrors the dev deployment's non-archived root
// chats: about half the messages are visible tool results with no indexed text,
// about one fifth are user or assistant text-bearing messages, and a small
// slice is model-only context.
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
		message := chatSearchMessage(faker, absoluteIndex)
		createdBy := uuid.Nil
		keyID := ""
		if message.Role == database.ChatMessageRoleUser {
			createdBy = ownerID
			keyID = apiKeyID
		}

		params.CreatedBy[messageIndex] = createdBy
		params.APIKeyID[messageIndex] = keyID
		params.ModelConfigID[messageIndex] = modelConfigID
		params.Role[messageIndex] = message.Role
		params.Content[messageIndex] = message.Content
		params.ContentVersion[messageIndex] = chatprompt.CurrentContentVersion
		params.Visibility[messageIndex] = message.Visibility
	}

	return params
}

type chatSearchMessageParts struct {
	Role       database.ChatMessageRole
	Visibility database.ChatMessageVisibility
	Content    string
}

func chatSearchMessage(faker *gofakeit.Faker, index int) chatSearchMessageParts {
	// Observed on non-archived root chats in dev, not deleted:
	// tool/both 49.1%, assistant/both 45.1%, user/both 4.5%, model-only 1.3%.
	// Of those, roughly 16% are assistant text and 4% are user text. The rest
	// carry no indexed text parts.
	switch bucket := index % 1000; {
	case bucket < 491:
		return chatSearchMessageParts{
			Role:       database.ChatMessageRoleTool,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    `[]`,
		}
	case bucket < 781:
		return chatSearchMessageParts{
			Role:       database.ChatMessageRoleAssistant,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    `[]`,
		}
	case bucket < 941:
		return chatSearchMessageParts{
			Role:       database.ChatMessageRoleAssistant,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    chatSearchTextContentJSON(chatSearchSeedText(faker, index)),
		}
	case bucket < 981:
		return chatSearchMessageParts{
			Role:       database.ChatMessageRoleUser,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    chatSearchTextContentJSON(chatSearchSeedText(faker, index)),
		}
	case bucket < 987:
		return chatSearchMessageParts{
			Role:       database.ChatMessageRoleUser,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    `[]`,
		}
	default:
		return chatSearchMessageParts{
			Role:       database.ChatMessageRoleSystem,
			Visibility: database.ChatMessageVisibilityModel,
			Content:    chatSearchTextContentJSON(chatSearchSeedText(faker, index)),
		}
	}
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
	text := chatSearchTextWithApproxBytes(faker, chatSearchTextBytes(faker))
	switch {
	case index%100 == 0:
		return text + " authentication permission denied oauth callback"
	case index%137 == 0:
		return text + " CODAGT-517 database migration failed"
	case index%251 == 0:
		return text + " workspace timeout provisioner agent disconnected"
	default:
		return text
	}
}

func chatSearchTextBytes(faker *gofakeit.Faker) int {
	// Observed extracted text bytes for non-archived root text-bearing messages:
	// avg 405, p50 120, p90 979, p99 4719, max about 36k. This distribution is
	// intentionally approximate and gives the benchmark a similar long tail.
	r := faker.Number(1, 10_000)
	switch {
	case r <= 5000:
		return faker.Number(20, 120)
	case r <= 9000:
		return faker.Number(121, 979)
	case r <= 9900:
		return faker.Number(980, 4719)
	default:
		return faker.Number(4720, 36_000)
	}
}

func chatSearchTextWithApproxBytes(faker *gofakeit.Faker, target int) string {
	var builder strings.Builder
	for builder.Len() < target {
		if builder.Len() > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(faker.Word())
	}
	return builder.String()
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
	MAX(ts_rank(cm.search_tsv, search_query.query)) AS rank
FROM chat_messages cm
JOIN chats c ON c.id = cm.chat_id
CROSS JOIN search_query
WHERE c.parent_chat_id IS NULL
  AND c.archived = false
  AND cm.deleted = false
  AND cm.search_tsv IS NOT NULL
  AND cm.search_tsv @@ search_query.query
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
