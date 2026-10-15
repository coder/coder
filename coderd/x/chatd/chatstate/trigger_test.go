package chatstate_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// triggerFixture is a slim variant of testFixture that also exposes a
// raw *sql.DB so the trigger tests can run UPDATE/INSERT statements
// that bypass the typed sqlc layer. Tests that only need the typed
// store should keep using newTestFixture.
type triggerFixture struct {
	f     *testFixture
	sqlDB *sql.DB
}

func newTriggerFixture(t *testing.T) *triggerFixture {
	t.Helper()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "openai",
		BaseUrl:     "http://example.invalid",
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		IsDefault: true,
	})
	f := &testFixture{
		DB:    db,
		Pub:   newRecordingPubsub(),
		User:  user,
		Org:   org,
		Model: model,
	}
	return &triggerFixture{f: f, sqlDB: sqlDB}
}

// userMessageContent returns a marshaled user message body suitable
// for raw INSERT into chat_messages.
func userMessageContent(t *testing.T, text string) []byte {
	t.Helper()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)
	return raw.RawMessage
}

// TestMessageInsertAssignsRevisionAndHistoryVersion verifies that
// inserting a chat message via the legacy InsertChatMessages query
// assigns NEW.revision from chats.snapshot_version (BEFORE trigger)
// and bumps chats.history_version + resets generation_attempt (AFTER
// STATEMENT trigger).
func TestMessageInsertAssignsRevisionAndHistoryVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)

	created := createTestChat(t, f)
	require.Equal(t, int64(1), created.Chat.SnapshotVersion)
	require.Equal(t, int64(1), created.Chat.HistoryVersion)

	// Force generation_attempt > 0 so we can prove the trigger
	// resets it on a new history change.
	_, err := f.DB.IncrementChatGenerationAttempt(ctx, created.Chat.ID)
	require.NoError(t, err)
	before, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), before.GenerationAttempt)

	// Bump snapshot_version directly to simulate a transition having
	// taken the row lock.
	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, before.SnapshotVersion+1, bumped.SnapshotVersion)

	// Insert a new assistant message via raw SQL so we know the
	// BEFORE+AFTER triggers (and only those) decide revision and
	// history_version.
	content := userMessageContent(t, "hello-after-bump")
	_, err = tf.sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility)
		VALUES ($1, 'assistant', $2::jsonb, $3, 'both')
	`, created.Chat.ID, string(content), int(chatprompt.CurrentContentVersion))
	require.NoError(t, err)

	// History version equals snapshot_version, generation_attempt resets.
	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, bumped.SnapshotVersion, after.HistoryVersion)
	require.Equal(t, int64(0), after.GenerationAttempt)

	// The inserted message picked up revision = bumped snapshot.
	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: created.Chat.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, msgs)
	last := msgs[len(msgs)-1]
	require.Equal(t, database.ChatMessageRoleAssistant, last.Role)
	require.Equal(t, bumped.SnapshotVersion, last.Revision)
}

// TestMessageUpdateAssignsNewRevisionAndHistoryVersion verifies that
// updating a chat message's content advances NEW.revision to the
// current chats.snapshot_version and that chats.history_version
// bumps to match.
func TestMessageUpdateAssignsNewRevisionAndHistoryVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: created.Chat.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, msgs)
	target := msgs[0]
	originalRevision := target.Revision

	// Bump the snapshot so the trigger sees a new revision target.
	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Greater(t, bumped.SnapshotVersion, originalRevision)

	newContent := userMessageContent(t, "edited content")
	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_messages SET content = $1::jsonb WHERE id = $2
	`, string(newContent), target.ID)
	require.NoError(t, err)

	reloaded, err := f.DB.GetChatMessageByID(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, bumped.SnapshotVersion, reloaded.Revision,
		"updated message picks up the current snapshot version")

	chatAfter, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, bumped.SnapshotVersion, chatAfter.HistoryVersion)
	require.Equal(t, int64(0), chatAfter.GenerationAttempt,
		"history change resets generation_attempt")
}

// TestMessageRevisionCannotBeSetByRuntimeCode verifies the BEFORE
// trigger rejects explicit revision values on INSERT and rejects
// revision changes on UPDATE.
func TestMessageRevisionCannotBeSetByRuntimeCode(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	content := userMessageContent(t, "explicit revision")
	_, err := tf.sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, revision)
		VALUES ($1, 'user', $2::jsonb, $3, 'both', 999)
	`, created.Chat.ID, string(content), int(chatprompt.CurrentContentVersion))
	require.Error(t, err, "INSERT with explicit revision must be rejected")
	require.Contains(t, err.Error(), "revision must be assigned by trigger")

	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: created.Chat.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, msgs)
	target := msgs[0]

	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_messages SET revision = revision + 100 WHERE id = $1
	`, target.ID)
	require.Error(t, err, "UPDATE that changes revision must be rejected")
	require.Contains(t, err.Error(), "revision must be assigned by trigger")
}

// TestMessageChatIDCannotChange verifies the BEFORE trigger rejects
// updates that change chat_messages.chat_id.
func TestMessageChatIDCannotChange(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	first := createTestChat(t, f)
	second := createTestChat(t, f)

	firstMsgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: first.Chat.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, firstMsgs)
	target := firstMsgs[0]

	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_messages SET chat_id = $1 WHERE id = $2
	`, second.Chat.ID, target.ID)
	require.Error(t, err, "UPDATE that changes chat_id must be rejected")
	require.Contains(t, err.Error(), "chat_id is immutable")
}

// TestNoopMessageUpdateDoesNotAdvanceHistoryVersion verifies that a
// no-op UPDATE on a chat_messages row (one whose OLD and NEW are
// indistinguishable) does NOT advance chats.history_version even
// when the snapshot was previously bumped. This guards against the
// AFTER UPDATE STATEMENT trigger naively reacting to every touched
// row id regardless of whether the row actually changed.
func TestNoopMessageUpdateDoesNotAdvanceHistoryVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: created.Chat.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, msgs)
	target := msgs[0]
	originalRevision := target.Revision

	// Bump snapshot so the AFTER STATEMENT guard
	// (history_version != snapshot_version) is now true.
	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.NotEqual(t, bumped.SnapshotVersion, bumped.HistoryVersion,
		"snapshot bump leaves history_version trailing")

	// No-op UPDATE: SET content = content. OLD IS NOT DISTINCT FROM NEW.
	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_messages SET content = content WHERE id = $1
	`, target.ID)
	require.NoError(t, err)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, bumped.HistoryVersion, after.HistoryVersion,
		"no-op update must NOT advance history_version")

	// And the row's revision is untouched.
	reloaded, err := f.DB.GetChatMessageByID(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, originalRevision, reloaded.Revision,
		"no-op update must NOT advance message revision")
}

// TestSearchTsvBackfillDoesNotTouchChatState verifies that the
// search_tsv backfill UPDATE leaves message revision, history_version,
// generation_attempt, and retry_state untouched. search_tsv is a
// system-maintained column; populating it is not a content change.
func TestSearchTsvBackfillDoesNotTouchChatState(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: created.Chat.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, msgs)
	target := msgs[0]
	require.Equal(t, database.ChatMessageRoleUser, target.Role)
	require.False(t, target.Deleted)
	originalRevision := target.Revision

	var tsvPending bool
	err = tf.sqlDB.QueryRowContext(ctx,
		`SELECT search_tsv IS NULL FROM chat_messages WHERE id = $1`, target.ID,
	).Scan(&tsvPending)
	require.NoError(t, err)
	require.True(t, tsvPending, "fresh message starts with search_tsv pending")

	attempt, err := f.DB.IncrementChatGenerationAttempt(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), attempt)

	withRetry, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`),
	})
	require.NoError(t, err)
	require.True(t, withRetry.RetryState.Valid)

	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.NotEqual(t, bumped.SnapshotVersion, bumped.HistoryVersion,
		"snapshot bump leaves history_version trailing")

	// Backfill-shaped UPDATE: only search_tsv and search_tsv_config change.
	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_messages
		SET search_tsv = COALESCE(to_tsvector('english', chat_message_search_text(content)), ''::tsvector),
		    search_tsv_config = 'english'
		WHERE id = $1
	`, target.ID)
	require.NoError(t, err)

	reloadedMsg, err := f.DB.GetChatMessageByID(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, originalRevision, reloadedMsg.Revision,
		"backfill must NOT advance message revision")
	err = tf.sqlDB.QueryRowContext(ctx,
		`SELECT search_tsv IS NULL FROM chat_messages WHERE id = $1`, target.ID,
	).Scan(&tsvPending)
	require.NoError(t, err)
	require.False(t, tsvPending, "backfill populates search_tsv")

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, bumped.HistoryVersion, after.HistoryVersion,
		"backfill must NOT advance history_version")
	require.Equal(t, int64(1), after.GenerationAttempt,
		"backfill must NOT reset generation_attempt")
	require.True(t, after.RetryState.Valid,
		"backfill must NOT clear retry_state")
	require.Equal(t, withRetry.RetryStateVersion, after.RetryStateVersion,
		"backfill must NOT change retry_state_version")
}

// Queue version triggers

// TestQueueInsertUpdatesQueueVersion verifies that an INSERT into
// chat_queued_messages bumps chats.queue_version to the current
// snapshot_version.
func TestQueueInsertUpdatesQueueVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	before, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), before.QueueVersion)

	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)

	content := userMessageContent(t, "queued")
	_, err = f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:    created.Chat.ID,
		Content:   content,
		CreatedBy: f.User.ID,
	})
	require.NoError(t, err)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, bumped.SnapshotVersion, after.QueueVersion,
		"INSERT into chat_queued_messages bumps queue_version")
}

// TestQueuedMessageCreatedByIsRequired verifies the database enforces
// creator metadata for every queued message row.
func TestQueuedMessageCreatedByIsRequired(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	content := userMessageContent(t, "queued-without-creator")
	_, err := tf.sqlDB.ExecContext(ctx, `
		INSERT INTO chat_queued_messages (chat_id, content, model_config_id, created_by)
		VALUES ($1, $2::jsonb, NULL, NULL)
	`, created.Chat.ID, string(content))
	require.Error(t, err)
	require.Contains(t, err.Error(), "created_by")
}

func TestLegacyQueuedMessageInsertUsesChatOwnerAsCreator(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	queued, err := f.DB.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:  created.Chat.ID,
		Content: userMessageContent(t, "legacy-queued"),
	})
	require.NoError(t, err)
	require.Equal(t, created.Chat.OwnerID, queued.CreatedBy)
}

// TestQueueUpdateContentUpdatesQueueVersion verifies that an UPDATE
// of chat_queued_messages.content bumps queue_version. The
// AFTER UPDATE trigger explicitly listens for content changes.
func TestQueueUpdateContentUpdatesQueueVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	queued, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:    created.Chat.ID,
		Content:   userMessageContent(t, "initial"),
		CreatedBy: f.User.ID,
	})
	require.NoError(t, err)

	before, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Greater(t, bumped.SnapshotVersion, before.QueueVersion)

	updated := userMessageContent(t, "updated")
	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_queued_messages SET content = $1::jsonb WHERE id = $2
	`, string(updated), queued.ID)
	require.NoError(t, err)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, bumped.SnapshotVersion, after.QueueVersion,
		"UPDATE of queued content bumps queue_version")
}

// TestQueueUpdatePositionUpdatesQueueVersion verifies that an UPDATE
// of chat_queued_messages.position (such as the reorder-to-head
// path) bumps queue_version.
func TestQueueUpdatePositionUpdatesQueueVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	q1, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:    created.Chat.ID,
		Content:   userMessageContent(t, "first"),
		CreatedBy: f.User.ID,
	})
	require.NoError(t, err)
	q2, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:    created.Chat.ID,
		Content:   userMessageContent(t, "second"),
		CreatedBy: f.User.ID,
	})
	require.NoError(t, err)
	require.NotEqual(t, q1.ID, q2.ID)

	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)

	// Move q2 to head by setting its position to q1.position - 1.
	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_queued_messages SET position = $1 WHERE id = $2
	`, q1.Position-1, q2.ID)
	require.NoError(t, err)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, bumped.SnapshotVersion, after.QueueVersion,
		"UPDATE of queued position bumps queue_version")
}

// TestQueueDeleteUpdatesQueueVersion verifies that DELETE from
// chat_queued_messages bumps queue_version.
func TestQueueDeleteUpdatesQueueVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	queued, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:    created.Chat.ID,
		Content:   userMessageContent(t, "to delete"),
		CreatedBy: f.User.ID,
	})
	require.NoError(t, err)

	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)

	rows, err := f.DB.DeleteChatQueuedMessageReturningCount(ctx, database.DeleteChatQueuedMessageReturningCountParams{
		ID:     queued.ID,
		ChatID: created.Chat.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, bumped.SnapshotVersion, after.QueueVersion,
		"DELETE from queue bumps queue_version")
}

// TestQueueVersionWrittenOncePerSnapshot verifies the queue trigger
// guard: changing N queued rows under one snapshot rewrites the
// chat_versions row once, not N times.
func TestQueueVersionWrittenOncePerSnapshot(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	for _, text := range []string{"one", "two", "three"} {
		_, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
			ChatID:    created.Chat.ID,
			Content:   userMessageContent(t, text),
			CreatedBy: f.User.ID,
		})
		require.NoError(t, err)
	}

	tx, err := tf.sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var snapshot int64
	require.NoError(t, tx.QueryRowContext(ctx, `
		UPDATE chat_versions SET snapshot_version = snapshot_version + 1
		WHERE chat_id = $1 RETURNING snapshot_version
	`, created.Chat.ID).Scan(&snapshot))
	updates := func() int64 {
		var n int64
		require.NoError(t, tx.QueryRowContext(ctx, `
			SELECT n_tup_upd FROM pg_stat_xact_user_tables WHERE relname = 'chat_versions'
		`).Scan(&n))
		return n
	}
	before := updates()
	res, err := tx.ExecContext(ctx, `DELETE FROM chat_queued_messages WHERE chat_id = $1`, created.Chat.ID)
	require.NoError(t, err)
	deleted, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(3), deleted)
	require.Equal(t, int64(1), updates()-before, "three queue changes rewrite chat_versions once")

	var queueVersion int64
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT queue_version FROM chat_versions WHERE chat_id = $1
	`, created.Chat.ID).Scan(&queueVersion))
	require.Equal(t, snapshot, queueVersion, "queue changes set queue_version")
}

// TestChatDeleteCascadesToVersions verifies that deleting a chat with
// queued messages removes its chat_versions row, even though the queue
// trigger fires during the cascade.
func TestChatDeleteCascadesToVersions(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	_, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:    created.Chat.ID,
		Content:   userMessageContent(t, "queued"),
		CreatedBy: f.User.ID,
	})
	require.NoError(t, err)

	_, err = tf.sqlDB.ExecContext(ctx, `DELETE FROM chats WHERE id = $1`, created.Chat.ID)
	require.NoError(t, err)

	var remaining int
	require.NoError(t, tf.sqlDB.QueryRowContext(ctx, `
		SELECT count(*) FROM chat_versions WHERE chat_id = $1
	`, created.Chat.ID).Scan(&remaining))
	require.Zero(t, remaining)
}

// TestLockWaitersReturnLatestVersions verifies that statements which wait
// on the chat row lock return the chat_versions values the lock holder
// committed, not the older ones from their statement snapshot.
func TestLockWaitersReturnLatestVersions(t *testing.T) {
	t.Parallel()

	for name, waiter := range map[string]func(context.Context, database.Store, uuid.UUID) (database.Chat, error){
		"GetChatByIDForUpdate": func(ctx context.Context, db database.Store, id uuid.UUID) (database.Chat, error) {
			return db.GetChatByIDForUpdate(ctx, id)
		},
		"UpdateChatTitleByID": func(ctx context.Context, db database.Store, id uuid.UUID) (database.Chat, error) {
			return db.UpdateChatTitleByID(ctx, database.UpdateChatTitleByIDParams{
				ID:          id,
				Title:       "renamed",
				TitleSource: database.ChatTitleSourceUser,
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tf := newTriggerFixture(t)
			f := tf.f
			ctx := testutil.Context(t, testutil.WaitLong)
			created := createTestChat(t, f)

			holder, err := tf.sqlDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = holder.Rollback() }()
			var bumped int64
			require.NoError(t, holder.QueryRowContext(ctx, `
				UPDATE chat_versions SET snapshot_version = snapshot_version + 1
				WHERE chat_id = (SELECT id FROM chats WHERE id = $1 FOR UPDATE)
				RETURNING snapshot_version
			`, created.Chat.ID).Scan(&bumped))

			type result struct {
				chat database.Chat
				err  error
			}
			done := make(chan result, 1)
			go func() {
				chat, err := waiter(ctx, f.DB, created.Chat.ID)
				done <- result{chat: chat, err: err}
			}()
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				var waiting int
				err := tf.sqlDB.QueryRowContext(ctx, `
					SELECT count(*) FROM pg_stat_activity
					WHERE datname = current_database() AND wait_event_type = 'Lock'
				`).Scan(&waiting)
				return err == nil && waiting > 0
			}, testutil.IntervalFast, "waiter should block on the chat row lock")
			require.NoError(t, holder.Commit())

			res := testutil.RequireReceive(ctx, t, done)
			require.NoError(t, res.err)
			require.Equal(t, bumped, res.chat.SnapshotVersion)
		})
	}
}

// TestNonQueueUpdateDoesNotUpdateQueueVersion verifies that mutations
// on other chat-related tables do NOT bump queue_version. The
// canonical case is inserting a chat message: it must update
// history_version but leave queue_version untouched.
func TestNonQueueUpdateDoesNotUpdateQueueVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	before, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)

	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)

	content := userMessageContent(t, "non-queue mutation")
	_, err = tf.sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility)
		VALUES ($1, 'assistant', $2::jsonb, $3, 'both')
	`, created.Chat.ID, string(content), int(chatprompt.CurrentContentVersion))
	require.NoError(t, err)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, before.QueueVersion, after.QueueVersion,
		"chat_messages INSERT must not bump queue_version")
	// Sanity: history_version DID move.
	require.Equal(t, bumped.SnapshotVersion, after.HistoryVersion)
}

// Retry state triggers

func TestRetryStateDefaults(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	chat, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.False(t, chat.RetryState.Valid)
	require.Equal(t, int64(0), chat.RetryStateVersion)
}

func TestRetryStateUpdateSetsRetryStateVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)

	after, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`),
	})
	require.NoError(t, err)
	require.True(t, after.RetryState.Valid)
	require.JSONEq(t,
		`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`,
		string(after.RetryState.RawMessage))
	require.Equal(t, bumped.SnapshotVersion, after.RetryStateVersion)
}

func TestRetryStateSameValueDoesNotUpdateRetryStateVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	payload := []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`)
	_, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	first, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: payload,
	})
	require.NoError(t, err)

	_, err = f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	second, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: payload,
	})
	require.NoError(t, err)
	require.Equal(t, first.RetryStateVersion, second.RetryStateVersion,
		"same retry_state payload must not update retry_state_version")
}

func TestGenerationAttemptClearsRetryState(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	_, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	withRetry, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`),
	})
	require.NoError(t, err)
	require.True(t, withRetry.RetryState.Valid)

	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	attempt, err := f.DB.IncrementChatGenerationAttempt(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), attempt)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.False(t, after.RetryState.Valid)
	require.Equal(t, bumped.SnapshotVersion, after.RetryStateVersion,
		"clearing retry_state on generation attempt bumps retry_state_version")
}

func TestGenerationAttemptWithNullRetryStateDoesNotUpdateRetryStateVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	before, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.False(t, before.RetryState.Valid)

	_, err = f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	_, err = f.DB.IncrementChatGenerationAttempt(ctx, created.Chat.ID)
	require.NoError(t, err)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.False(t, after.RetryState.Valid)
	require.Equal(t, before.RetryStateVersion, after.RetryStateVersion,
		"generation attempt with null retry_state leaves retry_state_version unchanged")
}

func TestRetryStateVersionCannotBeSetByRuntimeCode(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	_, err := tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_versions SET retry_state_version = retry_state_version + 1 WHERE chat_id = $1
	`, created.Chat.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "retry_state_version must be assigned by trigger")
}

func TestHistoryChangeClearsRetryState(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	_, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	_, err = f.DB.IncrementChatGenerationAttempt(ctx, created.Chat.ID)
	require.NoError(t, err)
	_, err = f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`),
	})
	require.NoError(t, err)

	bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, created.Chat.ID)
	require.NoError(t, err)
	content := userMessageContent(t, "history clears retry state")
	_, err = tf.sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility)
		VALUES ($1, 'assistant', $2::jsonb, $3, 'both')
	`, created.Chat.ID, string(content), int(chatprompt.CurrentContentVersion))
	require.NoError(t, err)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), after.GenerationAttempt)
	require.False(t, after.RetryState.Valid)
	require.Equal(t, bumped.SnapshotVersion, after.RetryStateVersion,
		"history reset of generation_attempt clears retry_state")
}
