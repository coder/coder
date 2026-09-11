package dbtestutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/testutil"
)

// Recent pg_dump versions (13.22+ / 14.19+ / 15.14+ / 16.10+ / 17.6+) emit
// psql meta-commands at the head and tail of the dump that aren't valid SQL.
// normalizeDump is expected to strip them so downstream consumers (sqlc,
// schema-equality checks in scripts/migrate-test) don't have to.
//
// See https://github.com/coder/internal/issues/965.
func TestNormalizeDumpStripsRestrict(t *testing.T) {
	t.Parallel()

	// Raw string literals (backticks) make backslashes literal, so the
	// meta-command here matches what pg_dump actually emits.
	input := []byte(`-- header
\restrict XYZ

CREATE TABLE foo;

\unrestrict XYZ
`)

	out := string(normalizeDump(input))
	require.NotContains(t, out, `\restrict`, `normalizeDump must strip \restrict psql meta-command`)
	require.NotContains(t, out, `\unrestrict`, `normalizeDump must strip \unrestrict psql meta-command`)
	require.Contains(t, out, "CREATE TABLE foo;", "normalizeDump must preserve real SQL between the meta-commands")
}

// The guard installed by NewDB must reject chat history and queue writes on
// the root handle and inside a transaction that has not allocated a snapshot
// for the chat, and record each rejection.
func TestChatWriteGuardRejectsWritesWithoutSnapshot(t *testing.T) {
	t.Parallel()

	db, _ := NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	require.Contains(t, db.Wrappers(), "dbtestutil.chatWriteGuard")

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	messageParams := database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{user.ID},
		ModelConfigID:       []uuid.UUID{model.ID},
		ReasoningEffort:     []string{""},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleUser},
		Content:             []string{`[{"type":"text","text":"hello"}]`},
		ContentVersion:      []int16{1},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		RuntimeMs:           []int64{0},
	}

	_, err := db.InsertChatMessages(ctx, messageParams)
	require.ErrorContains(t, err, "InsertChatMessages for chat "+chat.ID.String()+" outside a chat state transition")

	_, err = db.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:    chat.ID,
		Content:   []byte(`[]`),
		CreatedBy: user.ID,
	})
	require.ErrorContains(t, err, "InsertChatQueuedMessageWithCreator for chat "+chat.ID.String()+" outside a chat state transition")

	guard, ok := db.(*chatWriteGuard)
	require.True(t, ok, "NewDB must return the chat write guard")
	require.Equal(t, []chatWriteRejection{
		{method: "InsertChatMessages", chatID: chat.ID},
		{method: "InsertChatQueuedMessageWithCreator", chatID: chat.ID},
	}, guard.rec.list())
	// The rejections above are the expected outcome of this test, not
	// failures to report at cleanup.
	guard.rec.reset()

	err = db.InTx(func(tx database.Store) error {
		_, err := tx.InsertChatMessages(ctx, messageParams)
		return err
	}, nil)
	require.ErrorContains(t, err, "InsertChatMessages for chat "+chat.ID.String()+" outside a chat state transition")
	require.Equal(t, []chatWriteRejection{{method: "InsertChatMessages", chatID: chat.ID}}, guard.rec.list())
	guard.rec.reset()

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Empty(t, messages, "rejected writes must not reach the database")
}

// Every generated query that inserts, updates or deletes rows of
// chat_messages or chat_queued_messages must be overridden by the guard,
// except the two search_tsv maintenance queries, which change only the
// search columns. A new writer query fails this test until the guard learns
// about it.
func TestChatWriteGuardCoversEveryChatHistoryWriter(t *testing.T) {
	t.Parallel()

	queries, err := os.ReadFile("../queries.sql.go")
	require.NoError(t, err)
	queryConst := regexp.MustCompile("(?s)\nconst \\w+ = `-- name: (\\w+) :\\w+\n([^`]*)`")
	writesChatTables := regexp.MustCompile(`(?i)\b(?:INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+(?:chat_messages|chat_queued_messages)\b`)
	var writers []string
	for _, match := range queryConst.FindAllSubmatch(queries, -1) {
		if writesChatTables.Match(match[2]) {
			writers = append(writers, string(match[1]))
		}
	}
	require.NotEmpty(t, writers, "no writer queries found; the query constant pattern no longer matches queries.sql.go")

	file, err := parser.ParseFile(token.NewFileSet(), "chatwriteguard.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	notWriters := map[string]bool{
		"InTx":                           true,
		"Wrappers":                       true,
		"InsertChat":                     true,
		"LockChatAndBumpSnapshotVersion": true,
	}
	covered := []string{"BackfillChatMessagesSearchTsv", "ReindexStaleChatMessagesSearchTsv"}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || !fn.Name.IsExported() {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if recv, ok := star.X.(*ast.Ident); !ok || recv.Name != "chatWriteGuard" || notWriters[fn.Name.Name] {
			continue
		}
		covered = append(covered, fn.Name.Name)
	}

	slices.Sort(writers)
	slices.Sort(covered)
	require.Equal(t, writers, covered,
		"queries writing chat_messages or chat_queued_messages must be guarded by chatWriteGuard or listed here as search_tsv maintenance")
}
