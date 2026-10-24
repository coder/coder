package dbtestutil

import (
	"context"
	"fmt"
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

// guardedWriteCall invokes one guarded writer against chatID. messageID is
// a live message of that chat, for the writer that resolves the chat from a
// message id.
type guardedWriteCall struct {
	method string
	call   func(ctx context.Context, store database.Store, chatID uuid.UUID, messageID int64) error
}

// guardedWriteCalls has one entry per chatWriteGuard writer override; the
// completeness test enforces that equality so no override ships without a
// rejection case. Each call sets only the id the guard reads to find the
// chat, so the guarded query itself never runs.
var guardedWriteCalls = []guardedWriteCall{
	{method: "InsertChatMessages", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		_, err := store.InsertChatMessages(ctx, database.InsertChatMessagesParams{ChatID: chatID})
		return err
	}},
	{method: "SoftDeleteChatMessageByID", call: func(ctx context.Context, store database.Store, _ uuid.UUID, messageID int64) error {
		return store.SoftDeleteChatMessageByID(ctx, messageID)
	}},
	{method: "SoftDeleteChatMessagesAfterID", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		return store.SoftDeleteChatMessagesAfterID(ctx, database.SoftDeleteChatMessagesAfterIDParams{ChatID: chatID})
	}},
	{method: "SoftDeleteContextFileMessages", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		return store.SoftDeleteContextFileMessages(ctx, chatID)
	}},
	{method: "InsertChatQueuedMessage", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		_, err := store.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{ChatID: chatID})
		return err
	}},
	{method: "InsertChatQueuedMessageWithCreator", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		_, err := store.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{ChatID: chatID})
		return err
	}},
	{method: "DeleteChatQueuedMessage", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		return store.DeleteChatQueuedMessage(ctx, database.DeleteChatQueuedMessageParams{ChatID: chatID})
	}},
	{method: "DeleteChatQueuedMessageReturningCount", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		_, err := store.DeleteChatQueuedMessageReturningCount(ctx, database.DeleteChatQueuedMessageReturningCountParams{ChatID: chatID})
		return err
	}},
	{method: "DeleteAllChatQueuedMessages", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		return store.DeleteAllChatQueuedMessages(ctx, chatID)
	}},
	{method: "DeleteAllChatQueuedMessagesReturningCount", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		_, err := store.DeleteAllChatQueuedMessagesReturningCount(ctx, chatID)
		return err
	}},
	{method: "PopNextQueuedMessage", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		_, err := store.PopNextQueuedMessage(ctx, chatID)
		return err
	}},
	{method: "ReorderChatQueuedMessageToFront", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		_, err := store.ReorderChatQueuedMessageToFront(ctx, database.ReorderChatQueuedMessageToFrontParams{ChatID: chatID})
		return err
	}},
	{method: "ReorderChatQueuedMessageToHead", call: func(ctx context.Context, store database.Store, chatID uuid.UUID, _ int64) error {
		_, err := store.ReorderChatQueuedMessageToHead(ctx, database.ReorderChatQueuedMessageToHeadParams{ChatID: chatID})
		return err
	}},
}

// The guard installed by NewDB must reject every guarded writer on the root
// handle and inside a transaction that has not allocated a snapshot for the
// chat, and record each rejection.
func TestChatWriteGuardRejectsWritesWithoutSnapshot(t *testing.T) {
	t.Parallel()

	db, _ := NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	require.Contains(t, db.Wrappers(), "dbtestutil.chatWriteGuard")
	guard, ok := db.(*chatWriteGuard)
	require.True(t, ok, "NewDB must return the chat write guard")

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	seeded := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
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

	for _, tc := range guardedWriteCalls {
		err := tc.call(ctx, db, chat.ID, seeded.ID)
		require.ErrorContains(t, err, tc.method+" for chat "+chat.ID.String()+" outside a chat state transition")
		require.Equal(t, []chatWriteRejection{{method: tc.method, chatID: chat.ID}}, guard.rec.list())
		// The rejection is the expected outcome of this test, not a failure
		// to report at cleanup.
		guard.rec.reset()
	}

	err := db.InTx(func(tx database.Store) error {
		_, err := tx.InsertChatMessages(ctx, messageParams)
		return err
	}, nil)
	require.ErrorContains(t, err, "InsertChatMessages for chat "+chat.ID.String()+" outside a chat state transition")
	require.Equal(t, []chatWriteRejection{{method: "InsertChatMessages", chatID: chat.ID}}, guard.rec.list())
	guard.rec.reset()

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Len(t, messages, 1, "rejected writes must not reach the database")
}

// A nested InTx runs on the outer transaction handle, so a write inside it
// must see the outer transaction's allocation: the write lands and nothing
// is recorded.
func TestChatWriteGuardSharesAllocationAcrossNestedInTx(t *testing.T) {
	t.Parallel()

	db, _ := NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	guard, ok := db.(*chatWriteGuard)
	require.True(t, ok, "NewDB must return the chat write guard")

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})

	err := db.InTx(func(tx database.Store) error {
		if _, err := tx.LockChatAndBumpSnapshotVersion(ctx, chat.ID); err != nil {
			return err
		}
		return tx.InTx(func(inner database.Store) error {
			_, err := inner.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
				ChatID:    chat.ID,
				Content:   []byte(`[]`),
				CreatedBy: user.ID,
			})
			return err
		}, nil)
	}, nil)
	require.NoError(t, err)
	require.Empty(t, guard.rec.list())

	queued, err := db.CountChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, queued, "the nested allocated write must reach the database")
}

// Every generated query that inserts, updates or deletes rows of
// chat_messages or chat_queued_messages must be overridden by the guard,
// except the two search_tsv maintenance queries, which change only the
// search columns, and every override must have a rejection case in
// guardedWriteCalls. A new writer query fails this test until the guard
// learns about it and the rejection test covers it.
func TestChatWriteGuardCoversEveryChatHistoryWriter(t *testing.T) {
	t.Parallel()

	queries, err := os.ReadFile("../queries.sql.go")
	require.NoError(t, err)
	queryConst := regexp.MustCompile("(?s)\nconst \\w+ = `-- name: (\\w+) :\\w+\n([^`]*)`")
	writesChatTables := regexp.MustCompile(`(?i)\b(?:INSERT\s+INTO|MERGE\s+INTO|UPDATE|DELETE\s+FROM)\s+(?:ONLY\s+)?(?:chat_messages|chat_queued_messages)\b`)
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
	var overrides []string
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
		overrides = append(overrides, fn.Name.Name)
	}

	guardedOrExempt := append([]string{"BackfillChatMessagesSearchTsv", "ReindexStaleChatMessagesSearchTsv"}, overrides...)
	slices.Sort(writers)
	slices.Sort(guardedOrExempt)
	require.Equal(t, writers, guardedOrExempt,
		"every query writing chat_messages or chat_queued_messages must have a chatWriteGuard override "+
			"or be listed here as search_tsv maintenance, and every chatWriteGuard override must be one of "+
			"those writers or be listed in notWriters")

	tested := make([]string, 0, len(guardedWriteCalls))
	for _, tc := range guardedWriteCalls {
		tested = append(tested, tc.method)
	}
	slices.Sort(overrides)
	slices.Sort(tested)
	require.Equal(t, overrides, tested,
		"every chatWriteGuard override must have a rejection case in guardedWriteCalls, and every case must name an override")
}

// rejectionReportingTB records the Cleanup functions and Errorf calls that
// NewDB makes so a test can run the cleanup itself and inspect what it
// reported. Every other testing.TB method goes to the embedded test.
type rejectionReportingTB struct {
	*testing.T
	cleanups []func()
	errors   []string
}

func (tb *rejectionReportingTB) Cleanup(f func()) {
	tb.cleanups = append(tb.cleanups, f)
}

func (tb *rejectionReportingTB) Errorf(format string, args ...any) {
	tb.errors = append(tb.errors, fmt.Sprintf(format, args...))
}

// A rejection must fail the test at cleanup even when the caller drops the
// returned error.
func TestChatWriteGuardReportsRejectionsAtCleanup(t *testing.T) {
	t.Parallel()

	tb := &rejectionReportingTB{T: t}
	db, _ := NewDB(tb)
	ctx := testutil.Context(t, testutil.WaitShort)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})

	_, _ = db.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:    chat.ID,
		Content:   []byte(`[]`),
		CreatedBy: user.ID,
	})

	// NewDB registers its cleanups on tb, so they run here instead of at the
	// end of the test, in the order testing would use.
	for _, cleanup := range slices.Backward(tb.cleanups) {
		cleanup()
	}

	require.Len(t, tb.errors, 1)
	require.Contains(t, tb.errors[0], "chat write guard rejected InsertChatQueuedMessageWithCreator for chat "+chat.ID.String())
}
