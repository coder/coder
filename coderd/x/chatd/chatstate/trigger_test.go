package chatstate_test

import (
	"database/sql"
	"testing"

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
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
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
		DB:     db,
		PubSub: ps,
		Pub:    newRecordingPubsub(),
		User:   user,
		Org:    org,
		Model:  model,
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
