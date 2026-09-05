package agenttime_test

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/agenttime"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

type agentTimeFixture struct {
	user database.User
	org  database.Organization
	chat database.Chat
}

func TestLiveAgentTimeAccounting(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)

	day1 := time.Date(2025, 1, 1, 23, 30, 0, 0, time.UTC)
	day2 := time.Date(2025, 1, 2, 0, 30, 0, 0, time.UTC)
	ids := []int64{
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleSystem, database.ChatMessageVisibilityUser, day1, int64Ptr(100), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, day2, int64Ptr(200), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, day2, int64Ptr(300), true),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, day2, int64Ptr(400), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, day2, int64Ptr(0), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, day2, nil, false),
	}

	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 1), 100)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 2), 900)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, ids, 5)

	err := db.SoftDeleteChatMessageByID(ctx, ids[2])
	require.NoError(t, err)
	_, err = db.ArchiveChatByID(ctx, fixture.chat.ID)
	require.NoError(t, err)

	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 1), 100)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 2), 900)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, ids, 5)
}

func TestAgentTimeMultiRowInsertTrigger(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	rows, err := sqlDB.QueryContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, runtime_ms, created_at, compressed)
		VALUES
			($1, 'assistant'::chat_message_role, '[]'::jsonb, 1, 'both'::chat_message_visibility, 100, $2, false),
			($1, 'tool'::chat_message_role, '[]'::jsonb, 1, 'model'::chat_message_visibility, 200, $3, true),
			($1, 'user'::chat_message_role, '[]'::jsonb, 1, 'user'::chat_message_visibility, NULL, $3, false)
		RETURNING id
	`, fixture.chat.ID, time.Date(2025, 1, 5, 23, 59, 0, 0, time.UTC), time.Date(2025, 1, 6, 0, 1, 0, 0, time.UTC))
	require.NoError(t, err)
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.Len(t, ids, 3)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 5), 100)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 6), 200)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, ids, 2)
}

func TestAgentTimeAccountingRollsBackWithMessageInsert(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, runtime_ms, created_at)
		VALUES ($1, 'assistant'::chat_message_role, '[]'::jsonb, 1, 'both'::chat_message_visibility, 100, $2)
	`, fixture.chat.ID, time.Date(2025, 1, 7, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	require.EqualValues(t, 0, countChatMessages(ctx, t, sqlDB, fixture.chat.ID))
	requireNoDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 7))
}

func TestAgentTimeConcurrentAccountingClaimsOnce(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	ids := []int64{
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC), int64Ptr(100), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, time.Date(2025, 1, 3, 13, 0, 0, 0, time.UTC), int64Ptr(200), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityModel, time.Date(2025, 1, 3, 14, 0, 0, 0, time.UTC), int64Ptr(300), false),
	}
	clearAgentTimeAccounting(ctx, t, sqlDB, ids)

	var claimedTotal atomic.Int64
	var eg errgroup.Group
	for range 8 {
		eg.Go(func() error {
			claimed, err := db.AccountAgentTimeMessages(ctx, ids)
			claimedTotal.Add(claimed)
			return err
		})
	}
	require.NoError(t, eg.Wait())
	require.EqualValues(t, len(ids), claimedTotal.Load())
	requireAgentTimeMarkerCount(ctx, t, sqlDB, ids, int64(len(ids)))
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 3), 600)
}

func TestAgentTimeAggregateFailureRollsBackMessageInsert(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	_, err := sqlDB.ExecContext(ctx, `
		CREATE FUNCTION fail_agent_time_daily_write()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION 'agent time daily write failed';
		END;
		$$;
		CREATE TRIGGER trigger_fail_agent_time_daily_write
		BEFORE INSERT OR UPDATE ON agent_time_daily
		FOR EACH ROW
		EXECUTE FUNCTION fail_agent_time_daily_write();
	`)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, runtime_ms, created_at)
		VALUES ($1, 'assistant'::chat_message_role, '[]'::jsonb, 1, 'both'::chat_message_visibility, 100, $2)
	`, fixture.chat.ID, time.Date(2025, 1, 4, 12, 0, 0, 0, time.UTC))
	require.ErrorContains(t, err, "agent time daily write failed")
	require.EqualValues(t, 0, countChatMessages(ctx, t, sqlDB, fixture.chat.ID))
}

func TestAgentTimeAccountingDoesNotBumpChatState(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	messageID := insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Now().UTC(), int64Ptr(100), false)
	clearAgentTimeAccounting(ctx, t, sqlDB, []int64{messageID})
	var revisionBefore, revisionAfter int64
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT revision FROM chat_messages WHERE id=$1", messageID).Scan(&revisionBefore))

	_, err := sqlDB.ExecContext(ctx, `UPDATE chats SET generation_attempt = 7 WHERE id = $1`, fixture.chat.ID)
	require.NoError(t, err)
	before := readChatState(ctx, t, sqlDB, fixture.chat.ID)

	accounted, err := db.AccountAgentTimeMessages(ctx, []int64{messageID})
	require.NoError(t, err)
	require.EqualValues(t, 1, accounted)
	require.Equal(t, before, readChatState(ctx, t, sqlDB, fixture.chat.ID))
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT revision FROM chat_messages WHERE id=$1", messageID).Scan(&revisionAfter))
	require.Equal(t, revisionBefore, revisionAfter)
}

func TestAgentTimeStatusCoverageDates(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)

	_, err := sqlDB.ExecContext(ctx, `UPDATE agent_time_capture SET capture_started_at = $1 WHERE id = 1`, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	status, err := db.GetAgentTimeStatus(ctx, fixture.org.ID)
	require.NoError(t, err)
	require.False(t, status.BackfillComplete)
	require.Empty(t, status.BackfillError)
	require.Zero(t, status.ProcessedMessages)
	require.Equal(t, "2025-01-01", status.EarliestDate.Format("2006-01-02"))

	_, err = sqlDB.ExecContext(ctx, `UPDATE agent_time_capture SET capture_started_at = $1 WHERE id = 1`, time.Date(2025, 1, 1, 0, 0, 0, int(time.Microsecond), time.UTC))
	require.NoError(t, err)
	status, err = db.GetAgentTimeStatus(ctx, fixture.org.ID)
	require.NoError(t, err)
	require.Equal(t, "2025-01-02", status.EarliestDate.Format("2006-01-02"))
}

func TestAgentTimeBackfillIsResumableAndMarkerBased(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	ids := []int64{
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC), int64Ptr(100), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Date(2025, 2, 1, 13, 0, 0, 0, time.UTC), int64Ptr(200), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Date(2025, 2, 2, 12, 0, 0, 0, time.UTC), int64Ptr(300), false),
	}
	clearAgentTimeAccounting(ctx, t, sqlDB, ids)
	isolateBackfillOrg(ctx, t, sqlDB, fixture.org.ID, ids[len(ids)-1])

	event, err := agenttime.RunOnce(ctx, db, 2)
	require.NoError(t, err)
	require.True(t, event.Locked)
	require.True(t, event.ResetCursor)
	require.False(t, event.Completed)
	status := readBackfillStatus(ctx, t, sqlDB, fixture.org.ID)
	require.Zero(t, status.CursorMessageID)

	event, err = agenttime.RunOnce(ctx, db, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, event.SelectedMessages)
	require.EqualValues(t, 2, event.ProcessedMessages)

	event, err = agenttime.RunOnce(ctx, db, 2)
	require.NoError(t, err)
	require.EqualValues(t, 1, event.SelectedMessages)
	require.EqualValues(t, 1, event.ProcessedMessages)

	event, err = agenttime.RunOnce(ctx, db, 2)
	require.NoError(t, err)
	require.True(t, event.Completed)
	status = readBackfillStatus(ctx, t, sqlDB, fixture.org.ID)
	require.True(t, status.CompletedAt.Valid)
	require.EqualValues(t, 3, status.ProcessedMessages)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 2, 1), 300)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 2, 2), 300)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, ids, 3)
}

func TestAgentTimeBackfillSkipsBusyChats(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	busyChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    fixture.org.ID,
		OwnerID:           fixture.user.ID,
		LastModelConfigID: fixture.chat.LastModelConfigID,
	})
	busyID := insertAgentTimeMessage(ctx, t, sqlDB, busyChat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC), int64Ptr(100), false)
	freeID := insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Date(2025, 3, 1, 13, 0, 0, 0, time.UTC), int64Ptr(200), false)
	clearAgentTimeAccounting(ctx, t, sqlDB, []int64{busyID, freeID})
	isolateBackfillOrg(ctx, t, sqlDB, fixture.org.ID, 0)

	lockTx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = lockTx.Rollback() }()
	_, err = lockTx.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, busyChat.ID)
	require.NoError(t, err)

	event, err := agenttime.RunOnce(ctx, db, 10)
	require.NoError(t, err)
	require.True(t, event.Locked)
	require.EqualValues(t, 1, event.SelectedMessages)
	require.EqualValues(t, 1, event.ProcessedMessages)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, []int64{freeID}, 1)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, []int64{busyID}, 0)

	require.NoError(t, lockTx.Rollback())
	event, err = agenttime.RunOnce(ctx, db, 10)
	require.NoError(t, err)
	require.True(t, event.ResetCursor)
	event, err = agenttime.RunOnce(ctx, db, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, event.SelectedMessages)
	require.EqualValues(t, 1, event.ProcessedMessages)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 3, 1), 300)
}

func TestAgentTimeBackfillPersistsFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	id := insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Date(2025, 3, 2, 12, 0, 0, 0, time.UTC), int64Ptr(100), false)
	clearAgentTimeAccounting(ctx, t, sqlDB, []int64{id})
	isolateBackfillOrg(ctx, t, sqlDB, fixture.org.ID, 0)
	_, err := sqlDB.ExecContext(ctx, `
		CREATE FUNCTION fail_agent_time_daily_write_for_backfill()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION 'agent time backfill daily write failed';
		END;
		$$;
		CREATE TRIGGER trigger_fail_agent_time_daily_write_for_backfill
		BEFORE INSERT OR UPDATE ON agent_time_daily
		FOR EACH ROW
		EXECUTE FUNCTION fail_agent_time_daily_write_for_backfill();
	`)
	require.NoError(t, err)

	_, err = agenttime.RunOnce(ctx, db, 10)
	require.ErrorContains(t, err, "agent time backfill daily write failed")
	status := readBackfillStatus(ctx, t, sqlDB, fixture.org.ID)
	require.Contains(t, status.LastError, "agent time backfill daily write failed")
	require.True(t, status.LastErrorAt.Valid)
	require.False(t, status.CompletedAt.Valid)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, []int64{id}, 0)
}

func TestAgentTimeBackfillUsesAdvisoryLock(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	lockTx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = lockTx.Rollback() }()
	_, err = lockTx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, database.LockIDAgentTimeBackfill)
	require.NoError(t, err)

	event, err := agenttime.RunOnce(ctx, db, 1)
	require.NoError(t, err)
	require.False(t, event.Locked)
}

type chatState struct {
	SnapshotVersion   int64
	HistoryVersion    int64
	GenerationAttempt int64
}

func setupAgentTimeFixture(t testing.TB, db database.Store) agentTimeFixture {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
	})
	return agentTimeFixture{user: user, org: org, chat: chat}
}

func insertAgentTimeMessage(ctx context.Context, t testing.TB, db *sql.DB, chatID uuid.UUID, role database.ChatMessageRole, visibility database.ChatMessageVisibility, createdAt time.Time, runtimeMS *int64, compressed bool) int64 {
	t.Helper()

	var runtime any
	if runtimeMS != nil {
		runtime = *runtimeMS
	}
	var id int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, runtime_ms, created_at, compressed)
		VALUES ($1, $2::chat_message_role, '[]'::jsonb, 1, $3::chat_message_visibility, $4, $5, $6)
		RETURNING id
	`, chatID, string(role), string(visibility), runtime, createdAt, compressed).Scan(&id)
	require.NoError(t, err)
	return id
}

func clearAgentTimeAccounting(ctx context.Context, t testing.TB, db *sql.DB, messageIDs []int64) {
	t.Helper()

	_, err := db.ExecContext(ctx, `DELETE FROM chat_message_agent_time_accounted WHERE message_id = ANY($1)`, pq.Array(messageIDs))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM agent_time_daily; DELETE FROM agent_time_organization_daily`)
	require.NoError(t, err)
}

func readChatState(ctx context.Context, t testing.TB, db *sql.DB, chatID uuid.UUID) chatState {
	t.Helper()

	var state chatState
	err := db.QueryRowContext(ctx, `SELECT snapshot_version, history_version, generation_attempt FROM chats WHERE id = $1`, chatID).Scan(&state.SnapshotVersion, &state.HistoryVersion, &state.GenerationAttempt)
	require.NoError(t, err)
	return state
}

func requireDailyAgentTime(ctx context.Context, t testing.TB, db *sql.DB, orgID uuid.UUID, userID uuid.UUID, day time.Time, expected int64) {
	t.Helper()

	var actual int64
	err := db.QueryRowContext(ctx, `
		SELECT agent_time_ms
		FROM agent_time_daily
		WHERE organization_id = $1 AND user_id = $2 AND day = $3
	`, orgID, userID, day).Scan(&actual)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	requireAgentTimeSummaryInvariant(ctx, t, db)
}

func requireNoDailyAgentTime(ctx context.Context, t testing.TB, db *sql.DB, orgID uuid.UUID, userID uuid.UUID, day time.Time) {
	t.Helper()

	var actual int64
	err := db.QueryRowContext(ctx, `
		SELECT agent_time_ms
		FROM agent_time_daily
		WHERE organization_id = $1 AND user_id = $2 AND day = $3
	`, orgID, userID, day).Scan(&actual)
	require.ErrorIs(t, err, sql.ErrNoRows)
	requireAgentTimeSummaryInvariant(ctx, t, db)
}

func requireAgentTimeMarkerCount(ctx context.Context, t testing.TB, db *sql.DB, messageIDs []int64, expected int64) {
	t.Helper()

	var actual int64
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_message_agent_time_accounted
		WHERE message_id = ANY($1)
	`, pq.Array(messageIDs)).Scan(&actual)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func countChatMessages(ctx context.Context, t testing.TB, db *sql.DB, chatID uuid.UUID) int64 {
	t.Helper()

	var count int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_messages WHERE chat_id = $1`, chatID).Scan(&count)
	require.NoError(t, err)
	return count
}

func int64Ptr(v int64) *int64 {
	return &v
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func requireAgentTimeSummaryInvariant(ctx context.Context, t testing.TB, db *sql.DB) {
	t.Helper()
	var mismatch bool
	err := db.QueryRowContext(ctx, `
 SELECT EXISTS (
  SELECT 1 FROM (
   SELECT organization_id,day,SUM(agent_time_ms) AS agent_time_ms
   FROM agent_time_daily GROUP BY 1,2
  ) canonical FULL JOIN agent_time_organization_daily overview USING (organization_id,day)
  WHERE canonical.agent_time_ms IS DISTINCT FROM overview.agent_time_ms
 )`).Scan(&mismatch)
	require.NoError(t, err)
	require.False(t, mismatch, "organization summaries must equal canonical user/day sums")
}

func isolateBackfillOrg(ctx context.Context, t testing.TB, db *sql.DB, orgID uuid.UUID, cursorMessageID int64) {
	t.Helper()

	_, err := db.ExecContext(ctx, `
		INSERT INTO agent_time_backfill_status (organization_id, cursor_message_id, completed_at)
		SELECT id, CASE WHEN id = $1 THEN $2::bigint ELSE 0 END, CASE WHEN id = $1 THEN NULL ELSE now() END
		FROM organizations
		ON CONFLICT (organization_id) DO UPDATE SET
			cursor_message_id = EXCLUDED.cursor_message_id,
			completed_at = EXCLUDED.completed_at,
			processed_messages = 0,
			last_error = '',
			last_error_at = NULL,
			updated_at = now()
	`, orgID, cursorMessageID)
	require.NoError(t, err)
}

func readBackfillStatus(ctx context.Context, t testing.TB, db *sql.DB, orgID uuid.UUID) database.AgentTimeBackfillStatus {
	t.Helper()

	var status database.AgentTimeBackfillStatus
	err := db.QueryRowContext(ctx, `
		SELECT organization_id, cursor_message_id, processed_messages, completed_at, last_error, last_error_at, updated_at
		FROM agent_time_backfill_status
		WHERE organization_id = $1
	`, orgID).Scan(&status.OrganizationID, &status.CursorMessageID, &status.ProcessedMessages, &status.CompletedAt, &status.LastError, &status.LastErrorAt, &status.UpdatedAt)
	require.NoError(t, err)
	return status
}

func TestAgentTimeCaptureFollowsHistoryLocks(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	fixture := setupAgentTimeFixture(t, db)
	childChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID: fixture.org.ID, OwnerID: fixture.user.ID,
		LastModelConfigID: fixture.chat.LastModelConfigID,
		ParentChatID:      uuid.NullUUID{UUID: fixture.chat.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: fixture.chat.ID, Valid: true},
	})
	parent, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer parent.Rollback()
	child, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer child.Rollback()
	// Match chatstate's lock and snapshot bump before message insertion.
	for _, row := range []struct {
		tx *sql.Tx
		id uuid.UUID
	}{{parent, fixture.chat.ID}, {child, childChat.ID}} {
		_, err = row.tx.ExecContext(ctx, "SELECT id FROM chats WHERE id=$1 FOR UPDATE", row.id)
		require.NoError(t, err)
		_, err = row.tx.ExecContext(ctx, "UPDATE chats SET snapshot_version=snapshot_version+1 WHERE id=$1", row.id)
		require.NoError(t, err)
	}
	var childPID int
	require.NoError(t, child.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&childPID))
	insert := `INSERT INTO chat_messages (chat_id,role,content,content_version,visibility,runtime_ms,created_at)
 VALUES ($1,'assistant','[]',1,'both',$2,'2025-01-01')`
	childDone := make(chan error, 1)
	go func() {
		_, err := child.ExecContext(ctx, insert, childChat.ID, 100)
		if err == nil {
			err = child.Commit()
		}
		childDone <- err
	}()
	// The history update rechecks the parent/root FK. It must wait before
	// acquiring aggregates shared with the parent, not after acquiring them.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var blocked bool
		err := sqlDB.QueryRowContext(ctx, "SELECT cardinality(pg_blocking_pids($1)) > 0", childPID).Scan(&blocked)
		return err == nil && blocked
	}, testutil.IntervalFast)
	_, err = parent.ExecContext(ctx, insert, fixture.chat.ID, 200)
	require.NoError(t, err)
	require.NoError(t, parent.Commit())
	select {
	case err := <-childDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 1), 300)
}
