package chatstate_test

import (
	"database/sql"
	"fmt"
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
		UPDATE chats SET retry_state_version = retry_state_version + 1 WHERE id = $1
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

// receiptContent is the stored content of a structured output receipt: a
// short text fallback plus exactly one outcome part.
func receiptContent(t *testing.T) string {
	t.Helper()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("done"),
		{Type: codersdk.ChatMessagePartTypeStructuredOutputOutcome, StructuredOutputData: []byte(`{"request_id":"r"}`)},
	})
	require.NoError(t, err)
	return string(raw.RawMessage)
}

// receiptRow is one row of a multi-row INSERT; a nil content inserts NULL.
type receiptRow struct {
	chat             uuid.UUID
	role, visibility string
	version          int
	content          *string
}

// TestMessageInsertExemptsStructuredOutputReceipts verifies that only rows
// with the exact receipt shape skip the history reset on INSERT while still
// taking their revision from the snapshot.
func TestMessageInsertExemptsStructuredOutputReceipts(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitLong)
	receipt := receiptContent(t)
	str := func(s string) *string { return &s }
	outcome := `{"type":"structured-output-outcome","structured_output_data":{"request_id":"r"}}`

	// prepare returns a chat whose snapshot is ahead of its history with a
	// generation in progress, so a reset is observable.
	prepare := func() database.Chat {
		chat := createTestChat(t, f).Chat
		_, err := f.DB.IncrementChatGenerationAttempt(ctx, chat.ID)
		require.NoError(t, err)
		chat, err = f.DB.LockChatAndBumpSnapshotVersion(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, int64(1), chat.GenerationAttempt)
		require.Less(t, chat.HistoryVersion, chat.SnapshotVersion)
		return chat
	}
	insert := func(rows ...receiptRow) {
		query, args := `INSERT INTO chat_messages (chat_id, role, visibility, content_version, content) VALUES `, []any{}
		for i, r := range rows {
			if i > 0 {
				query += ","
			}
			n := len(args)
			query += fmt.Sprintf("($%d, $%d::chat_message_role, $%d::chat_message_visibility, $%d, $%d::jsonb)", n+1, n+2, n+3, n+4, n+5)
			args = append(args, r.chat, r.role, r.visibility, r.version, r.content)
		}
		_, err := tf.sqlDB.ExecContext(ctx, query, args...)
		require.NoError(t, err)
	}
	requireFence := func(t *testing.T, before database.Chat, kept bool) {
		t.Helper()
		after, err := f.DB.GetChatByID(ctx, before.ID)
		require.NoError(t, err)
		if kept {
			require.Equal(t, before.HistoryVersion, after.HistoryVersion)
			require.Equal(t, before.GenerationAttempt, after.GenerationAttempt)
		} else {
			require.Equal(t, before.SnapshotVersion, after.HistoryVersion)
			require.Zero(t, after.GenerationAttempt)
		}
		var revision int64
		require.NoError(t, tf.sqlDB.QueryRowContext(ctx, `SELECT revision FROM chat_messages WHERE chat_id = $1 ORDER BY id DESC LIMIT 1`, before.ID).Scan(&revision))
		require.Equal(t, before.SnapshotVersion, revision)
	}
	receiptOf := func(chat uuid.UUID) receiptRow { return receiptRow{chat, "assistant", "user", 1, str(receipt)} }

	for name, tc := range map[string]struct {
		row  func(uuid.UUID) receiptRow
		kept bool
	}{
		"Receipt":         {receiptOf, true},
		"OutcomeOnly":     {func(c uuid.UUID) receiptRow { return receiptRow{c, "assistant", "user", 1, str(`[` + outcome + `]`)} }, true},
		"RoleUser":        {func(c uuid.UUID) receiptRow { return receiptRow{c, "user", "user", 1, str(receipt)} }, false},
		"RoleTool":        {func(c uuid.UUID) receiptRow { return receiptRow{c, "tool", "user", 1, str(receipt)} }, false},
		"RoleSystem":      {func(c uuid.UUID) receiptRow { return receiptRow{c, "system", "user", 1, str(receipt)} }, false},
		"VisibilityBoth":  {func(c uuid.UUID) receiptRow { return receiptRow{c, "assistant", "both", 1, str(receipt)} }, false},
		"VisibilityModel": {func(c uuid.UUID) receiptRow { return receiptRow{c, "assistant", "model", 1, str(receipt)} }, false},
		"ContentVersion0": {func(c uuid.UUID) receiptRow { return receiptRow{c, "assistant", "user", 0, str(receipt)} }, false},
		"ContentVersion2": {func(c uuid.UUID) receiptRow { return receiptRow{c, "assistant", "user", 2, str(receipt)} }, false},
		"TwoOutcomes": {func(c uuid.UUID) receiptRow {
			return receiptRow{c, "assistant", "user", 1, str(`[` + outcome + `,` + outcome + `]`)}
		}, false},
		"OutcomeAndToolCall": {func(c uuid.UUID) receiptRow {
			return receiptRow{c, "assistant", "user", 1, str(`[` + outcome + `,{"type":"tool-call","tool_call_id":"c"}]`)}
		}, false},
		"UntypedSibling": {func(c uuid.UUID) receiptRow {
			return receiptRow{c, "assistant", "user", 1, str(`[` + outcome + `,{"text":"x"}]`)}
		}, false},
		"NestedInText": {func(c uuid.UUID) receiptRow {
			return receiptRow{c, "assistant", "user", 1, str(`[{"type":"text","text":"x","nested":` + outcome + `}]`)}
		}, false},
		"NestedArray":   {func(c uuid.UUID) receiptRow { return receiptRow{c, "assistant", "user", 1, str(`[[` + outcome + `]]`)} }, false},
		"ObjectContent": {func(c uuid.UUID) receiptRow { return receiptRow{c, "assistant", "user", 1, str(outcome)} }, false},
		"StringContent": {func(c uuid.UUID) receiptRow {
			return receiptRow{c, "assistant", "user", 1, str(`"structured-output-outcome"`)}
		}, false},
		"EmptyArray":  {func(c uuid.UUID) receiptRow { return receiptRow{c, "assistant", "user", 1, str(`[]`)} }, false},
		"NullContent": {func(c uuid.UUID) receiptRow { return receiptRow{c, "assistant", "user", 1, nil} }, false},
		"ControlOnly": {func(c uuid.UUID) receiptRow {
			return receiptRow{c, "assistant", "user", 1, str(`[{"type":"structured-output-control"}]`)}
		}, false},
	} {
		chat := prepare()
		insert(tc.row(chat.ID))
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requireFence(t, chat, tc.kept)
		})
	}

	// A receipt batched with an ordinary row for the same chat keeps normal
	// fencing, and a batch spanning chats is filtered per row.
	mixed := prepare()
	insert(receiptOf(mixed.ID), receiptRow{mixed.ID, "assistant", "both", 1, str(string(userMessageContent(t, "ordinary")))})
	requireFence(t, mixed, false)
	receiptChat, ordinaryChat := prepare(), prepare()
	insert(receiptOf(receiptChat.ID), receiptRow{ordinaryChat.ID, "user", "both", 1, str(string(userMessageContent(t, "ordinary")))})
	requireFence(t, receiptChat, true)
	requireFence(t, ordinaryChat, false)

	// Editing or soft-deleting a receipt still invalidates execution.
	for _, stmt := range []string{
		`UPDATE chat_messages SET content = '[{"type":"text","text":"edited"}]'::jsonb WHERE chat_id = $1`,
		`UPDATE chat_messages SET deleted = true WHERE chat_id = $1 AND role = 'assistant'`,
	} {
		chat := prepare()
		insert(receiptOf(chat.ID))
		bumped, err := f.DB.LockChatAndBumpSnapshotVersion(ctx, chat.ID)
		require.NoError(t, err)
		_, err = tf.sqlDB.ExecContext(ctx, stmt, chat.ID)
		require.NoError(t, err)
		after, err := f.DB.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, bumped.SnapshotVersion, after.HistoryVersion, stmt)
		require.Zero(t, after.GenerationAttempt, stmt)
	}
}
