package chatstate_test

import (
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

// insertRawAssistantMessage inserts an assistant message with raw SQL so
// only the BEFORE trigger decides its revision.
func insertRawAssistantMessage(t *testing.T, tf *triggerFixture, chatID uuid.UUID, text string) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	content := userMessageContent(t, text)
	_, err := tf.sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility)
		VALUES ($1, 'assistant', $2::jsonb, $3, 'both')
	`, chatID, string(content), int(chatprompt.CurrentContentVersion))
	require.NoError(t, err)
}

// TestMessageInsertStampsCommittedRevision verifies the transition
// protocol for message inserts: the BEFORE trigger stamps NEW.revision
// with the version the enclosing transaction will commit
// (snapshot_version + 1), the insert itself leaves the chats row
// untouched, and the commit write moves history_version to that version
// and resets generation_attempt.
func TestMessageInsertStampsCommittedRevision(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)

	created := createTestChat(t, f)
	require.Equal(t, int64(1), created.Chat.SnapshotVersion)
	require.Equal(t, int64(1), created.Chat.HistoryVersion)

	// Force generation_attempt > 0 so we can prove the commit write
	// resets it on a history change.
	_, err := f.DB.IncrementChatGenerationAttempt(ctx, database.IncrementChatGenerationAttemptParams{ID: created.Chat.ID})
	require.NoError(t, err)
	locked, err := f.DB.LockChatForTransition(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), locked.Chat.GenerationAttempt)
	require.Equal(t, int64(2), locked.Chat.SnapshotVersion, "the attempt increment is a commit write")

	insertRawAssistantMessage(t, tf, created.Chat.ID, "hello-before-commit")

	untouched, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, locked.Chat, untouched, "a message insert must not write the chats row")

	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: created.Chat.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, msgs)
	last := msgs[len(msgs)-1]
	require.Equal(t, database.ChatMessageRoleAssistant, last.Role)
	require.Equal(t, locked.Chat.SnapshotVersion+1, last.Revision,
		"revision is the version the transaction commits")

	after, err := f.DB.BumpChatSnapshotVersion(ctx, database.BumpChatSnapshotVersionParams{
		ID:             created.Chat.ID,
		HistoryChanged: true,
	})
	require.NoError(t, err)
	require.Equal(t, last.Revision, after.Chat.SnapshotVersion)
	require.Equal(t, last.Revision, after.Chat.HistoryVersion)
	require.Equal(t, int64(0), after.Chat.GenerationAttempt)
}

// TestMessageUpdateStampsCommittedRevision verifies that a material
// UPDATE of a chat message advances NEW.revision to the version the
// transaction will commit and that the commit write moves
// history_version to match.
func TestMessageUpdateStampsCommittedRevision(t *testing.T) {
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

	locked, err := f.DB.LockChatForTransition(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, target.Revision, locked.Chat.SnapshotVersion)

	newContent := userMessageContent(t, "edited content")
	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_messages SET content = $1::jsonb WHERE id = $2
	`, string(newContent), target.ID)
	require.NoError(t, err)

	reloaded, err := f.DB.GetChatMessageByID(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, locked.Chat.SnapshotVersion+1, reloaded.Revision,
		"updated message picks up the committed version")

	untouched, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, locked.Chat, untouched, "a message update must not write the chats row")

	after, err := f.DB.BumpChatSnapshotVersion(ctx, database.BumpChatSnapshotVersionParams{
		ID:             created.Chat.ID,
		HistoryChanged: true,
	})
	require.NoError(t, err)
	require.Equal(t, reloaded.Revision, after.Chat.HistoryVersion)
	require.Equal(t, int64(0), after.Chat.GenerationAttempt)
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

// TestNoopMessageUpdateDoesNotAdvanceRevision verifies that a no-op
// UPDATE on a chat_messages row (one whose OLD and NEW are
// indistinguishable) does NOT advance the row's revision.
func TestNoopMessageUpdateDoesNotAdvanceRevision(t *testing.T) {
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

	// No-op UPDATE: SET content = content. OLD IS NOT DISTINCT FROM NEW.
	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_messages SET content = content WHERE id = $1
	`, target.ID)
	require.NoError(t, err)

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

	attempt, err := f.DB.IncrementChatGenerationAttempt(ctx, database.IncrementChatGenerationAttemptParams{ID: created.Chat.ID})
	require.NoError(t, err)
	require.Equal(t, int64(1), attempt.Chat.GenerationAttempt)

	withRetry, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`),
	})
	require.NoError(t, err)
	require.True(t, withRetry.Chat.RetryState.Valid)

	before, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)

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
	require.Equal(t, before, after, "backfill must NOT touch the chats row")
	require.Equal(t, int64(1), after.GenerationAttempt)
	require.True(t, after.RetryState.Valid)
}

// Queue version

// TestQueueWritesLeaveChatUntouchedUntilCommit verifies that queue
// writes no longer touch the chats row: queue_version moves only when a
// commit write records the change.
func TestQueueWritesLeaveChatUntouchedUntilCommit(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	before, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), before.QueueVersion)

	queued, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:    created.Chat.ID,
		Content:   userMessageContent(t, "queued"),
		CreatedBy: f.User.ID,
	})
	require.NoError(t, err)
	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_queued_messages SET content = $1::jsonb WHERE id = $2
	`, string(userMessageContent(t, "updated")), queued.ID)
	require.NoError(t, err)
	rows, err := f.DB.DeleteChatQueuedMessageReturningCount(ctx, database.DeleteChatQueuedMessageReturningCountParams{
		ID:     queued.ID,
		ChatID: created.Chat.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	untouched, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, before, untouched, "queue writes must not write the chats row")

	after, err := f.DB.BumpChatSnapshotVersion(ctx, database.BumpChatSnapshotVersionParams{
		ID:           created.Chat.ID,
		QueueChanged: true,
	})
	require.NoError(t, err)
	require.Equal(t, before.SnapshotVersion+1, after.Chat.SnapshotVersion)
	require.Equal(t, after.Chat.SnapshotVersion, after.Chat.QueueVersion,
		"the commit write records the queue change at the committed version")
	require.Equal(t, before.HistoryVersion, after.Chat.HistoryVersion,
		"a queue change must not move history_version")
}

// TestExecutionStateCommitRecordsQueueChange verifies the exec-state
// commit write records queue changes the same way as the bump-only one.
func TestExecutionStateCommitRecordsQueueChange(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	after, err := f.DB.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{
		ID:           created.Chat.ID,
		Status:       database.ChatStatusWaiting,
		QueueChanged: true,
	})
	require.NoError(t, err)
	require.Equal(t, created.Chat.SnapshotVersion+1, after.Chat.SnapshotVersion)
	require.Equal(t, after.Chat.SnapshotVersion, after.Chat.QueueVersion)
	require.Equal(t, created.Chat.HistoryVersion, after.Chat.HistoryVersion)
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

// TestHistoryCommitDoesNotUpdateQueueVersion verifies that committing a
// history change leaves queue_version untouched.
func TestHistoryCommitDoesNotUpdateQueueVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	before, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)

	insertRawAssistantMessage(t, tf, created.Chat.ID, "non-queue mutation")
	after, err := f.DB.BumpChatSnapshotVersion(ctx, database.BumpChatSnapshotVersionParams{
		ID:             created.Chat.ID,
		HistoryChanged: true,
	})
	require.NoError(t, err)
	require.Equal(t, before.QueueVersion, after.Chat.QueueVersion,
		"a history change must not bump queue_version")
	require.Equal(t, after.Chat.SnapshotVersion, after.Chat.HistoryVersion)
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

	after, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`),
	})
	require.NoError(t, err)
	require.True(t, after.Chat.RetryState.Valid)
	require.JSONEq(t,
		`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`,
		string(after.Chat.RetryState.RawMessage))
	require.Equal(t, created.Chat.SnapshotVersion+1, after.Chat.SnapshotVersion,
		"the retry state write is a commit write")
	require.Equal(t, after.Chat.SnapshotVersion, after.Chat.RetryStateVersion)
}

func TestRetryStateSameValueDoesNotUpdateRetryStateVersion(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	payload := []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`)
	first, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: payload,
	})
	require.NoError(t, err)

	second, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: payload,
	})
	require.NoError(t, err)
	require.Equal(t, first.Chat.SnapshotVersion+1, second.Chat.SnapshotVersion)
	require.Equal(t, first.Chat.RetryStateVersion, second.Chat.RetryStateVersion,
		"same retry_state payload must not update retry_state_version")
}

func TestGenerationAttemptClearsRetryState(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	withRetry, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`),
	})
	require.NoError(t, err)
	require.True(t, withRetry.Chat.RetryState.Valid)

	attempt, err := f.DB.IncrementChatGenerationAttempt(ctx, database.IncrementChatGenerationAttemptParams{ID: created.Chat.ID})
	require.NoError(t, err)
	require.Equal(t, int64(1), attempt.Chat.GenerationAttempt)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.False(t, after.RetryState.Valid)
	require.Equal(t, withRetry.Chat.SnapshotVersion+1, after.SnapshotVersion,
		"the attempt increment is a commit write")
	require.Equal(t, after.SnapshotVersion, after.RetryStateVersion,
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

	_, err = f.DB.IncrementChatGenerationAttempt(ctx, database.IncrementChatGenerationAttemptParams{ID: created.Chat.ID})
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

	_, err := f.DB.IncrementChatGenerationAttempt(ctx, database.IncrementChatGenerationAttemptParams{ID: created.Chat.ID})
	require.NoError(t, err)
	withRetry, err := f.DB.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{
		ID:         created.Chat.ID,
		RetryState: []byte(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`),
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), withRetry.Chat.GenerationAttempt)

	insertRawAssistantMessage(t, tf, created.Chat.ID, "history clears retry state")
	after, err := f.DB.BumpChatSnapshotVersion(ctx, database.BumpChatSnapshotVersionParams{
		ID:             created.Chat.ID,
		HistoryChanged: true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), after.Chat.GenerationAttempt)
	require.False(t, after.Chat.RetryState.Valid)
	require.Equal(t, after.Chat.SnapshotVersion, after.Chat.RetryStateVersion,
		"history reset of generation_attempt clears retry_state")
}
